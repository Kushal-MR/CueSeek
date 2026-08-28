package qbittorrent

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

func writeTorrents(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// ---------------------------------------------------------------- mapping

// TestActiveCountsOnlyWhatMovesData is the decision that keeps the number meaningful.
//
// Seeding is excluded on purpose: it is usually permanent, so counting it would mean a
// client that finished everything last week reports "12 active" forever, and a number that
// is always the same is a number nobody reads.
func TestActiveCountsOnlyWhatMovesData(t *testing.T) {
	torrents := []torrent{
		{Hash: "a", Name: "one", State: "downloading"},
		{Hash: "b", Name: "two", State: "stalledDL"},
		{Hash: "c", Name: "three", State: "seeding"},
		{Hash: "d", Name: "four", State: "pausedUP"},
		{Hash: "e", Name: "five", State: "forcedDL"},
		{Hash: "f", Name: "six", State: "queuedDL"},
	}

	moving := transfersFrom(torrents, transferInfo{})

	if moving.Total != 6 {
		t.Errorf("Total = %d, want 6 — everything tracked", moving.Total)
	}
	// downloading and forcedDL only. stalledDL is waiting on peers, queuedDL has not
	// started, seeding and pausedUP are not downloads.
	if moving.Active != 2 {
		t.Errorf("Active = %d, want 2, from %+v", moving.Active, moving.Items)
	}
}

// TestStateCrossesVerbatim — the difference between "stalled" and "queued" is exactly what
// tells an operator whether to care, and a shared vocabulary would discard it.
func TestStateCrossesVerbatim(t *testing.T) {
	moving := transfersFrom([]torrent{
		{Hash: "a", Name: "one", State: "stalledDL"},
		{Hash: "b", Name: "two", State: "somethingNewInQbit5"},
	}, transferInfo{})

	if moving.Items[0].State != "stalledDL" {
		t.Errorf("State = %q, want it unmapped", moving.Items[0].State)
	}
	if moving.Items[1].State != "somethingNewInQbit5" {
		t.Errorf("an unrecognised state must pass through, got %q", moving.Items[1].State)
	}
}

// TestETAPlaceholderBecomesUnknown — qBittorrent reports 8640000 for "not in any useful
// timeframe". Passed through it renders as a hundred days and looks like a CueSeek defect.
func TestETAPlaceholderBecomesUnknown(t *testing.T) {
	cases := []struct {
		eta  int
		want int
	}{
		{eta: 8640000, want: 0},
		{eta: 8640001, want: 0},
		{eta: 0, want: 0},
		{eta: -1, want: 0},
		{eta: 120, want: 120},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.eta), func(t *testing.T) {
			moving := transfersFrom([]torrent{{Hash: "a", ETA: tc.eta}}, transferInfo{})
			if got := moving.Items[0].ETASeconds; got != tc.want {
				t.Errorf("ETASeconds = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestProgressIsClampedToTheContract — the value crosses into a progress bar, where an
// out-of-range number renders as an overflowing element rather than as an obviously wrong
// figure somebody would notice.
func TestProgressIsClampedToTheContract(t *testing.T) {
	moving := transfersFrom([]torrent{
		{Hash: "a", Progress: -0.5},
		{Hash: "b", Progress: 1.5},
		{Hash: "c", Progress: 0.42},
	}, transferInfo{})

	if moving.Items[0].Progress != 0 {
		t.Errorf("negative progress = %v, want 0", moving.Items[0].Progress)
	}
	if moving.Items[1].Progress != 1 {
		t.Errorf("overshooting progress = %v, want 1", moving.Items[1].Progress)
	}
	if moving.Items[2].Progress != 0.42 {
		t.Errorf("valid progress was altered: %v", moving.Items[2].Progress)
	}
}

// TestRatesComeFromTheAggregateNotTheSample is why the health call's response is kept.
//
// Summing a capped list would understate a busy client, and understating exactly when
// things are busy is the worst possible time to be wrong.
func TestRatesComeFromTheAggregateNotTheSample(t *testing.T) {
	rates := transferInfo{DownloadSpeed: 12_000_000, UploadSpeed: 800_000}
	moving := transfersFrom([]torrent{{Hash: "a", DlSpeed: 5}}, rates)

	if moving.DownloadRateBytes != 12_000_000 {
		t.Errorf("DownloadRateBytes = %d, want the aggregate", moving.DownloadRateBytes)
	}
	if moving.UploadRateBytes != 800_000 {
		t.Errorf("UploadRateBytes = %d, want the aggregate", moving.UploadRateBytes)
	}
	if moving.Items[0].DownloadRateBytes != 5 {
		t.Errorf("the per-item rate should still be its own: %d", moving.Items[0].DownloadRateBytes)
	}
}

// TestNothingQueuedIsEmptyNotNil — a nil slice encodes as JSON null, and "nothing queued"
// must not arrive looking like "we could not ask".
func TestNothingQueuedIsEmptyNotNil(t *testing.T) {
	moving := transfersFrom(nil, transferInfo{})
	if moving.Items == nil {
		t.Error("Items is nil; it must be an empty slice")
	}
	if moving.Active != 0 || moving.Total != 0 {
		t.Errorf("counts = %d/%d, want zeroes", moving.Active, moving.Total)
	}
}

// ---------------------------------------------------------------- transport

// TestTransfersRetriesAnExpiredSession mirrors the health path: a cookie has a lifetime,
// and an idle agent will outlive one.
func TestTransfersRetriesAnExpiredSession(t *testing.T) {
	var logins, listed int
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == loginPath:
			logins++
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: fmt.Sprintf("sid-%d", logins)})
			_, _ = w.Write([]byte("Ok."))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents"):
			listed++
			if listed == 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			writeTorrents(w, `[{"hash":"a","name":"one","state":"downloading","progress":0.5}]`)
		default:
			writeTransferInfo(w, "connected")
		}
	})

	svc := newAuthenticatedAdapter(t, server.URL)
	moving, err := svc.(adapters.TransferProvider).Transfers(t.Context())
	if err != nil {
		t.Fatalf("Transfers: %v", err)
	}
	if len(moving.Items) != 1 {
		t.Fatalf("Items = %d, want 1 after the silent re-login", len(moving.Items))
	}
	if logins != 2 {
		t.Errorf("logins = %d, want 2 (initial, then after the 403)", logins)
	}
}

// TestTransfersFailureDoesNotAffectHealth is the capability boundary. qBittorrent
// answering /transfer/info but refusing /torrents/info is up.
func TestTransfersFailureDoesNotAffectHealth(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v2/torrents") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeTransferInfo(w, "connected")
	})
	svc := newAdapter(t, server.URL, adapters.Deps{})

	if _, err := svc.(adapters.TransferProvider).Transfers(t.Context()); err == nil {
		t.Fatal("expected an error from a failing torrents endpoint")
	}
	if h := health(t, svc); h.Status != domain.StatusHealthy {
		t.Errorf("health = %v, want healthy", h.Status)
	}
}

// TestTransfersReusesTheRatesFromHealth — /transfer/info is already fetched every poll, so
// asking for it twice would double this adapter's request count for no new information.
func TestTransfersReusesTheRatesFromHealth(t *testing.T) {
	var infoCalls int
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v2/torrents") {
			writeTorrents(w, `[]`)
			return
		}
		infoCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"connection_status":"connected","dl_info_speed":999,"up_info_speed":111}`))
	})

	svc := newAdapter(t, server.URL, adapters.Deps{})
	health(t, svc) // one poll's health call populates the rates

	moving, err := svc.(adapters.TransferProvider).Transfers(t.Context())
	if err != nil {
		t.Fatalf("Transfers: %v", err)
	}
	if moving.DownloadRateBytes != 999 || moving.UploadRateBytes != 111 {
		t.Errorf("rates = %d/%d, want those from the health call",
			moving.DownloadRateBytes, moving.UploadRateBytes)
	}
	if infoCalls != 1 {
		t.Errorf("/transfer/info called %d times, want 1 — Transfers must reuse it", infoCalls)
	}
}

func TestTransfersRejectsAMalformedBody(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v2/torrents") {
			writeTorrents(w, `{"not":"an array"}`)
			return
		}
		writeTransferInfo(w, "connected")
	})
	svc := newAdapter(t, server.URL, adapters.Deps{})

	if _, err := svc.(adapters.TransferProvider).Transfers(t.Context()); err == nil {
		t.Fatal("expected a decode error")
	}
}

// ---------------------------------------------------------------- ranking

func torrentAt(hash, state string, speed, addedOn int64) torrent {
	return torrent{Hash: hash, Name: hash, State: state, DlSpeed: speed, AddedOn: addedOn}
}

// TestFinishedTorrentsDoNotFallOffTheEnd is the defect a real library exposed.
//
// The old query sorted by `dlspeed`, which ranks only the torrents that are downloading.
// Everything seeding, paused or finished ties at zero, so past the first item the order was
// whatever qBittorrent happened to return — and a torrent that finished a minute ago
// dropped to zero speed and vanished from the sample. Which one fell off was luck.
func TestFinishedTorrentsDoNotFallOffTheEnd(t *testing.T) {
	torrents := []torrent{
		torrentAt("old-seed", "stalledUP", 0, 1_000),
		torrentAt("older-seed", "stalledUP", 0, 500),
		torrentAt("just-finished", "uploading", 0, 9_000),
		torrentAt("downloading", "downloading", 11_000_000, 2_000),
	}

	moving := transfersFrom(torrents, transferInfo{})
	order := make([]string, 0, len(moving.Items))
	for _, item := range moving.Items {
		order = append(order, item.ID)
	}

	want := []string{"downloading", "just-finished", "old-seed", "older-seed"}
	if !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

// TestActiveDownloadsLeadRankedBySpeed — the first tier, where speed is the meaningful
// ranking because these are the only torrents actually moving.
func TestActiveDownloadsLeadRankedBySpeed(t *testing.T) {
	torrents := []torrent{
		torrentAt("newest-seed", "stalledUP", 0, 99_000),
		torrentAt("slow-dl", "downloading", 1_000, 10),
		torrentAt("fast-dl", "downloading", 9_000_000, 20),
	}

	moving := transfersFrom(torrents, transferInfo{})

	if moving.Items[0].ID != "fast-dl" || moving.Items[1].ID != "slow-dl" {
		t.Errorf("downloads should lead fastest-first, got %v", moving.Items)
	}
	// Even the newest torrent in the library sits behind anything actually downloading.
	if moving.Items[2].ID != "newest-seed" {
		t.Errorf("an idle torrent outranked a download: %v", moving.Items)
	}
}

// TestRankingIsStableAcrossPolls — the list redraws every thirty seconds, and rows that
// reorder under a thumb are worse than rows in a duller order. `added_on` never changes,
// unlike speed or last-activity.
func TestRankingIsStableAcrossPolls(t *testing.T) {
	base := []torrent{
		torrentAt("a", "stalledUP", 0, 300),
		torrentAt("b", "uploading", 0, 200),
		torrentAt("c", "pausedUP", 0, 100),
	}

	first := transfersFrom(base, transferInfo{})

	// A later poll: the same torrents, upload rates churning, order from qBittorrent
	// arbitrarily different. Nothing that affects the ranking has changed.
	shuffled := []torrent{base[2], base[0], base[1]}
	second := transfersFrom(shuffled, transferInfo{})

	for i := range first.Items {
		if first.Items[i].ID != second.Items[i].ID {
			t.Fatalf("order changed between polls: %v then %v", first.Items, second.Items)
		}
	}
}

// TestActiveCountCoversEverythingFetchedNotJustTheSample — the count is the truth and the
// sample is a view of it, so trimming must not change the number.
func TestActiveCountCoversEverythingFetchedNotJustTheSample(t *testing.T) {
	torrents := make([]torrent, 0, domain.MaxActivityItems+5)
	// More downloads than fit in the sample.
	for i := range domain.MaxActivityItems + 5 {
		torrents = append(torrents,
			torrentAt(fmt.Sprintf("dl-%d", i), "downloading", int64(i), int64(i)))
	}

	moving := transfersFrom(torrents, transferInfo{})

	if moving.Active != domain.MaxActivityItems+5 {
		t.Errorf("Active = %d, want every fetched download counted", moving.Active)
	}
	if moving.Total != domain.MaxActivityItems+5 {
		t.Errorf("Total = %d", moving.Total)
	}
	// transfersFrom does not trim; the poller does. Everything fetched is ranked and
	// returned, so the cap applies to one place rather than two.
	if len(moving.Items) != domain.MaxActivityItems+5 {
		t.Errorf("items = %d; trimming belongs to the poller", len(moving.Items))
	}
}

// TestFetchesGenerouslyRatherThanExactly — ranking needs to see the items it is ranking,
// and this is a loopback call, so the fetch is cheap and the tailnet frame is what is not.
func TestFetchesGenerouslyRatherThanExactly(t *testing.T) {
	var gotQuery string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v2/torrents") {
			gotQuery = r.URL.RawQuery
			writeTorrents(w, `[]`)
			return
		}
		writeTransferInfo(w, "connected")
	})

	svc := newAdapter(t, server.URL, adapters.Deps{})
	if _, err := svc.(adapters.TransferProvider).Transfers(t.Context()); err != nil {
		t.Fatalf("Transfers: %v", err)
	}

	if !strings.Contains(gotQuery, fmt.Sprintf("limit=%d", torrentsFetchLimit)) {
		t.Errorf("query = %q, want limit=%d", gotQuery, torrentsFetchLimit)
	}
	// Newest-first from the server, so the generous fetch drops the oldest rather than an
	// arbitrary slice, and `rank` reorders what it gets.
	if !strings.Contains(gotQuery, "sort=added_on") {
		t.Errorf("query = %q, want newest-first ordering", gotQuery)
	}
	if strings.Contains(gotQuery, "dlspeed") {
		t.Errorf("query = %q still sorts by speed, which ranks only downloads", gotQuery)
	}
}
