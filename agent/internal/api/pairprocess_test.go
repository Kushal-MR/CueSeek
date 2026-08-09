package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// Reproduces the deployment topology exactly, which no other test does.
//
// On the real host there are TWO processes holding the same SQLite file:
//
//	cueseekd pair -config ...   mints a pairing code, then exits
//	cueseekd -config ...        the running agent, which redeems it
//
// Every other test in this package uses a single *sql.DB. That difference is the only
// untested variable between a passing suite and a failing box, so it is the first thing
// to eliminate.
func TestPairAcrossTwoDatabaseHandles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	// Handle A — the `cueseekd pair` process.
	minter, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open minter store: %v", err)
	}
	code, err := minter.CreatePairingCode(t.Context(),
		[]domain.Scope{domain.ScopeRead, domain.ScopeServiceControl}, 5*time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	// The CLI exits after minting.
	if err := minter.Close(); err != nil {
		t.Fatalf("close minter store: %v", err)
	}

	// Handle B — the long-running agent.
	agentStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open agent store: %v", err)
	}
	t.Cleanup(func() { agentStore.Close() })

	srv, err := New(Options{
		Store:        agentStore,
		Registry:     adapters.NewRegistry(),
		Cache:        adapters.NewCache(),
		AgentVersion: "test",
		HostID:       "test-host",
	})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Redeem through the running agent, exactly as curl does.
	pairResp, err := ts.Client().Post(ts.URL+"/v1/pair", "application/json",
		jsonBody(t, map[string]string{
			"code": code, "device_name": "Phone", "platform": "cli",
		}))
	if err != nil {
		t.Fatalf("POST /v1/pair: %v", err)
	}
	defer pairResp.Body.Close()
	if pairResp.StatusCode != http.StatusCreated {
		t.Fatalf("pair status = %d, want 201", pairResp.StatusCode)
	}

	issued := decode[struct {
		Token  string `json:"token"`
		Device struct {
			Id string `json:"id"`
		} `json:"device"`
	}](t, pairResp)

	if len(issued.Token) != 47 {
		t.Fatalf("token length = %d, want 47", len(issued.Token))
	}
	t.Logf("issued token for device %s", issued.Device.Id)

	// The failing step on the real host: authenticate immediately, no restart between.
	for _, path := range []string{"/v1/system", "/v1/devices", "/v1/services"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+issued.Token)

		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		// Read as raw bytes: /v1/devices and /v1/services return arrays, /v1/system an
		// object, and a failure returns a problem document. Decoding into a fixed shape
		// would fail on the shape rather than on the status.
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s with a token minted across two handles: status = %d, want 200; body=%s",
				path, resp.StatusCode, raw)
		}
	}

	// And the store agrees the device is there.
	device, err := agentStore.AuthenticateToken(t.Context(), issued.Token)
	if err != nil {
		t.Fatalf("AuthenticateToken directly against the store: %v", err)
	}
	if device.ID != issued.Device.Id {
		t.Errorf("device id = %q, want %q", device.ID, issued.Device.Id)
	}
}

// TestTokenSurvivesAgentRestart covers the other deployment reality: the token is stored,
// not held in memory, so a restarted agent must still accept it.
func TestTokenSurvivesAgentRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	first, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	device, token, err := first.CreateDevice(t.Context(), "Phone", domain.PlatformCLI,
		[]domain.Scope{domain.ScopeRead})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// A brand-new handle, as after `systemctl restart cueseekd`.
	second, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { second.Close() })

	got, err := second.AuthenticateToken(t.Context(), token)
	if err != nil {
		t.Fatalf("token rejected after reopen: %v", err)
	}
	if got.ID != device.ID {
		t.Errorf("device id = %q, want %q", got.ID, device.ID)
	}
}

// jsonBody marshals a value for a request body.
func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(raw)
}
