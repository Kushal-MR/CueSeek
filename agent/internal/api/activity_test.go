package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// decodeServices reads the list endpoint into loosely-typed maps.
//
// Deliberately not into the generated structs: the point of these tests is what actually
// appears on the wire, and decoding into a type with omitempty would hide precisely the
// absent-versus-zero distinction being asserted.
func decodeServices(t *testing.T, env *testEnv, token string) []map[string]any {
	t.Helper()
	resp := env.do(t, http.MethodGet, "/v1/services", token, nil)
	defer resp.Body.Close()

	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func healthyNow() domain.Health {
	return domain.Health{
		Status: domain.StatusHealthy, Reachable: true, ObservedAt: time.Now().UTC(),
	}
}

// TestActivityIsOmittedWhenNotObserved is the null-versus-empty rule at the wire boundary.
//
// A service that implements neither capability, or whose last read failed, must omit the
// field entirely. Sending `{"sessions":0}` would tell every client the server is idle,
// which is an observation the agent never made.
func TestActivityIsOmittedWhenNotObserved(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	env.cache.Put("jellyfin", adapters.Observation{Health: healthyNow()})

	svc := decodeServices(t, env, token)[0]
	if _, present := svc["now_playing"]; present {
		t.Error("now_playing was sent for a service with no observation")
	}
	if _, present := svc["transfers"]; present {
		t.Error("transfers was sent for a service with no observation")
	}
}

// TestIdleActivityIsSentAsZeroNotOmitted is the other half of the same rule. The agent
// asked, and the answer was "nothing" — which is information a client should show.
func TestIdleActivityIsSentAsZeroNotOmitted(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	env.cache.Put("jellyfin", adapters.Observation{
		Health:     healthyNow(),
		NowPlaying: &domain.NowPlaying{Items: []domain.PlaybackSession{}},
		Transfers:  &domain.Transfers{Items: []domain.TransferItem{}},
	})

	svc := decodeServices(t, env, token)[0]

	playing, ok := svc["now_playing"].(map[string]any)
	if !ok {
		t.Fatalf("now_playing missing or wrong shape: %#v", svc["now_playing"])
	}
	if playing["sessions"].(float64) != 0 {
		t.Errorf("sessions = %v, want 0", playing["sessions"])
	}
	// An empty array, not null: the contract declares items required.
	if items, ok := playing["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("items = %#v, want an empty array", playing["items"])
	}

	moving, ok := svc["transfers"].(map[string]any)
	if !ok {
		t.Fatalf("transfers missing: %#v", svc["transfers"])
	}
	if moving["active"].(float64) != 0 || moving["total"].(float64) != 0 {
		t.Errorf("counts = %v/%v, want zeroes", moving["active"], moving["total"])
	}
}

// TestOptionalSessionFieldsAreOmittedNotZeroed — sending 0 for an unknown position would
// park every progress bar at the start of the file, which is a claim rather than a gap.
func TestOptionalSessionFieldsAreOmittedNotZeroed(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	env.cache.Put("jellyfin", adapters.Observation{
		Health: healthyNow(),
		NowPlaying: &domain.NowPlaying{
			Sessions: 1,
			Items: []domain.PlaybackSession{{
				ID: "s1", Title: "The Bear",
				// Everything else deliberately absent: a live stream with no duration,
				// no reported user, no device name.
			}},
		},
	})

	svc := decodeServices(t, env, token)[0]
	item := svc["now_playing"].(map[string]any)["items"].([]any)[0].(map[string]any)

	for _, field := range []string{"subtitle", "user", "client", "position_seconds", "duration_seconds"} {
		if _, present := item[field]; present {
			t.Errorf("%q was sent despite being unknown: %v", field, item[field])
		}
	}
	// The required ones are always there, including the false booleans.
	if item["title"] != "The Bear" {
		t.Errorf("title = %v", item["title"])
	}
	if item["paused"] != false || item["transcoding"] != false {
		t.Errorf("required booleans missing: %#v", item)
	}
}

// TestTransferStateCrossesTheWireVerbatim — a client must receive the service's own word,
// including one this agent has never seen.
func TestTransferStateCrossesTheWireVerbatim(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	env.cache.Put("jellyfin", adapters.Observation{
		Health: healthyNow(),
		Transfers: &domain.Transfers{
			Active: 1, Total: 1,
			DownloadRateBytes: 12_000_000,
			Items: []domain.TransferItem{{
				ID: "abc", Name: "ubuntu.iso", State: "stalledDL", Progress: 0.5,
			}},
		},
	})

	svc := decodeServices(t, env, token)[0]
	moving := svc["transfers"].(map[string]any)
	item := moving["items"].([]any)[0].(map[string]any)

	if item["state"] != "stalledDL" {
		t.Errorf("state = %v, want it unmapped", item["state"])
	}
	if moving["download_rate_bytes"].(float64) != 12_000_000 {
		t.Errorf("download_rate_bytes = %v", moving["download_rate_bytes"])
	}
	// eta_seconds was zero, meaning unknown, so it must not appear as a real estimate.
	if _, present := item["eta_seconds"]; present {
		t.Errorf("eta_seconds was sent for an unknown estimate: %v", item["eta_seconds"])
	}
}

// TestActivityComesFromTheSameSnapshotAsHealth — pairing a fresh activity reading with a
// stale status would produce a service that is "unreachable" while three things play.
func TestActivityComesFromTheSameSnapshotAsHealth(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	// Recent, deliberately. Backdating past the cache's tolerance would make it degrade
	// the health to `unknown` — correct behaviour, but it would test the watchdog rather
	// than the pairing of health and activity.
	observed := time.Now().UTC().Truncate(time.Second)
	env.cache.Put("jellyfin", adapters.Observation{
		Health: domain.Health{
			Status: domain.StatusDegraded, Reachable: true, ObservedAt: observed,
		},
		NowPlaying: &domain.NowPlaying{Sessions: 3, Transcoding: 2},
	})

	svc := decodeServices(t, env, token)[0]

	if svc["health"].(map[string]any)["status"] != "degraded" {
		t.Errorf("health = %v", svc["health"])
	}
	playing := svc["now_playing"].(map[string]any)
	if playing["sessions"].(float64) != 3 || playing["transcoding"].(float64) != 2 {
		t.Errorf("activity did not come from the same observation: %#v", playing)
	}
}
