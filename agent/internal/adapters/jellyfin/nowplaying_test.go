package jellyfin

import (
	"context"
	"net/http"
	"testing"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// ptr is a local helper: Jellyfin uses null to mean "not applicable" on several numeric
// fields, and the difference between absent and zero is load-bearing in subtitleFor.
func ptr[T any](v T) *T { return &v }

func episode(series string, season, number int) session {
	s := session{ID: "sess-1", UserName: "kushal", DeviceName: "Living Room TV"}
	s.NowPlayingItem = &struct {
		Name              string `json:"Name"`
		SeriesName        string `json:"SeriesName"`
		ParentIndexNumber *int   `json:"ParentIndexNumber"`
		IndexNumber       *int   `json:"IndexNumber"`
		ProductionYear    *int   `json:"ProductionYear"`
		RunTimeTicks      int64  `json:"RunTimeTicks"`
		Type              string `json:"Type"`
	}{
		Name:              "Forks",
		SeriesName:        series,
		ParentIndexNumber: ptr(season),
		IndexNumber:       ptr(number),
		RunTimeTicks:      int64(1800) * ticksPerSecond,
		Type:              "Episode",
	}
	return s
}

// ---------------------------------------------------------------- mapping

// TestIdleSessionsAreNotPlayback is the distinction that decides whether an idle house
// looks busy. Jellyfin keeps a session object for every connected client, including ones
// sitting on a menu, and counting those would make the capability useless.
func TestIdleSessionsAreNotPlayback(t *testing.T) {
	sessions := []session{
		{ID: "idle-1", UserName: "kushal"},
		episode("The Bear", 2, 7),
		{ID: "idle-2", UserName: "guest"},
	}

	playing := nowPlayingFrom(sessions)

	if playing.Sessions != 1 {
		t.Errorf("Sessions = %d, want 1 — only one client is actually playing", playing.Sessions)
	}
	if len(playing.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(playing.Items))
	}
}

func TestSubtitleIsBuiltOnlyFromWhatJellyfinSent(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*session)
		want   string
	}{
		{
			name:   "series with season and episode",
			mutate: func(s *session) {},
			want:   "The Bear · S2E7",
		},
		{
			name: "series without numbering falls back to the series alone",
			mutate: func(s *session) {
				s.NowPlayingItem.ParentIndexNumber = nil
				s.NowPlayingItem.IndexNumber = nil
			},
			want: "The Bear",
		},
		{
			name: "a film uses its year",
			mutate: func(s *session) {
				s.NowPlayingItem.SeriesName = ""
				s.NowPlayingItem.ProductionYear = ptr(1997)
			},
			want: "1997",
		},
		{
			// The contract says this field is never synthesised. Nothing to say is
			// better than a label the agent invented from a file path.
			name: "nothing to say produces nothing",
			mutate: func(s *session) {
				s.NowPlayingItem.SeriesName = ""
				s.NowPlayingItem.ProductionYear = nil
			},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := episode("The Bear", 2, 7)
			tc.mutate(&s)
			got := nowPlayingFrom([]session{s})
			if got.Items[0].Subtitle != tc.want {
				t.Errorf("Subtitle = %q, want %q", got.Items[0].Subtitle, tc.want)
			}
		})
	}
}

// TestTranscodingIsCountedAndFlagged covers the number that explains a hot machine.
//
// Jellyfin reports TranscodingInfo whenever it is doing *any* work, including remuxing, so
// "direct" means both streams direct. Treating the mere presence of the object as
// transcoding would over-report; ignoring it would under-report.
func TestTranscodingIsCountedAndFlagged(t *testing.T) {
	direct := episode("The Bear", 2, 7)
	direct.ID = "direct"
	direct.TranscodingInfo = &struct {
		IsVideoDirect bool `json:"IsVideoDirect"`
		IsAudioDirect bool `json:"IsAudioDirect"`
	}{IsVideoDirect: true, IsAudioDirect: true}

	audioOnly := episode("The Bear", 2, 7)
	audioOnly.ID = "audio-transcode"
	audioOnly.TranscodingInfo = &struct {
		IsVideoDirect bool `json:"IsVideoDirect"`
		IsAudioDirect bool `json:"IsAudioDirect"`
	}{IsVideoDirect: true, IsAudioDirect: false}

	plain := episode("The Bear", 2, 7)
	plain.ID = "no-info"

	playing := nowPlayingFrom([]session{direct, audioOnly, plain})

	if playing.Sessions != 3 {
		t.Fatalf("Sessions = %d, want 3", playing.Sessions)
	}
	if playing.Transcoding != 1 {
		t.Errorf("Transcoding = %d, want 1 — only the audio transcode counts", playing.Transcoding)
	}

	byID := map[string]bool{}
	for _, item := range playing.Items {
		byID[item.ID] = item.Transcoding
	}
	if byID["direct"] {
		t.Error("a fully direct session was flagged as transcoding")
	}
	if !byID["audio-transcode"] {
		t.Error("an audio transcode was not flagged")
	}
	if byID["no-info"] {
		t.Error("a session with no TranscodingInfo was flagged")
	}
}

func TestTicksBecomeSeconds(t *testing.T) {
	s := episode("The Bear", 2, 7)
	s.PlayState = &struct {
		PositionTicks int64 `json:"PositionTicks"`
		IsPaused      bool  `json:"IsPaused"`
	}{PositionTicks: int64(742) * ticksPerSecond, IsPaused: true}

	item := nowPlayingFrom([]session{s}).Items[0]

	if item.PositionSeconds != 742 {
		t.Errorf("PositionSeconds = %d, want 742", item.PositionSeconds)
	}
	if item.DurationSeconds != 1800 {
		t.Errorf("DurationSeconds = %d, want 1800", item.DurationSeconds)
	}
	if !item.Paused {
		t.Error("Paused was not carried through")
	}
}

// TestClientPrefersTheDeviceName — "Living Room TV" locates a stream in the house;
// "Jellyfin Android" only says it is not the web player.
func TestClientPrefersTheDeviceName(t *testing.T) {
	withDevice := episode("The Bear", 2, 7)
	withDevice.Client = "Jellyfin Android"
	if got := nowPlayingFrom([]session{withDevice}).Items[0].Client; got != "Living Room TV" {
		t.Errorf("Client = %q, want the device name", got)
	}

	withoutDevice := episode("The Bear", 2, 7)
	withoutDevice.DeviceName = ""
	withoutDevice.Client = "Jellyfin Android"
	if got := nowPlayingFrom([]session{withoutDevice}).Items[0].Client; got != "Jellyfin Android" {
		t.Errorf("Client = %q, want the app name as fallback", got)
	}
}

// TestNothingPlayingIsEmptyNotNil — the field crosses the wire as a JSON array, and a nil
// slice would encode as null. "Nothing is playing" is an observation and must not arrive
// looking like "we could not ask".
func TestNothingPlayingIsEmptyNotNil(t *testing.T) {
	playing := nowPlayingFrom(nil)
	if playing.Items == nil {
		t.Error("Items is nil; it must be an empty slice")
	}
	if playing.Sessions != 0 || playing.Transcoding != 0 {
		t.Errorf("counts = %d/%d, want zeroes", playing.Sessions, playing.Transcoding)
	}
}

// ---------------------------------------------------------------- transport

func TestNowPlayingSendsTheAPIKey(t *testing.T) {
	var gotKey, gotPath string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Emby-Token")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`[]`))
	})

	svc := newAdapter(t, server.URL, adapters.Deps{})
	if _, err := svc.(adapters.NowPlayingProvider).NowPlaying(t.Context()); err != nil {
		t.Fatalf("NowPlaying: %v", err)
	}
	if gotKey != testAPIKey {
		t.Errorf("X-Emby-Token = %q", gotKey)
	}
	if gotPath != sessionsPath {
		t.Errorf("path = %q, want %q", gotPath, sessionsPath)
	}
}

// TestNowPlayingFailureIsAnErrorNotAHealthOpinion is the boundary that keeps the two
// capabilities independent. A server that answers /System/Info and refuses /Sessions is
// up, and the poller turns this error into an absent payload rather than a sick service.
func TestNowPlayingFailureIsAnErrorNotAHealthOpinion(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == sessionsPath {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		writeSystemInfo(w, `{"ServerName":"box","Version":"10.9"}`)
	})
	svc := newAdapter(t, server.URL, adapters.Deps{})

	if _, err := svc.(adapters.NowPlayingProvider).NowPlaying(t.Context()); err == nil {
		t.Fatal("expected an error from a refused /Sessions")
	}
	// And health is untouched by it.
	if h := health(t, svc); h.Status != domain.StatusHealthy {
		t.Errorf("health = %v, want healthy — /Sessions is not a health endpoint", h.Status)
	}
}

func TestNowPlayingRejectsAMalformedBody(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"not":"an array"}`))
	})
	svc := newAdapter(t, server.URL, adapters.Deps{})

	if _, err := svc.(adapters.NowPlayingProvider).NowPlaying(t.Context()); err == nil {
		t.Fatal("expected a decode error")
	}
}

// TestNowPlayingIsAdvertisedIndependentlyOfControl — being unable to restart something
// does not stop you watching it, so the capabilities must not be coupled.
func TestNowPlayingIsAdvertisedIndependentlyOfControl(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeSystemInfo(w, `{"ServerName":"box"}`)
	})

	// No unit, so no control.
	svc := newAdapter(t, server.URL, adapters.Deps{})
	if _, ok := svc.(adapters.Controllable); ok {
		t.Fatal("precondition: this adapter should not be controllable")
	}
	if _, ok := svc.(adapters.NowPlayingProvider); !ok {
		t.Error("now_playing must not depend on control")
	}
}

var _ = context.Background
