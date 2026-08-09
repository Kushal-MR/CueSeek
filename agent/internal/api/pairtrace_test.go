package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// TestPairTokenMatchesStoredHash instruments one real pairing end to end and prints:
//
//	returned token   — the exact string in the HTTP response body
//	sha256(returned) — computed here, independently of the agent's own hashing
//	stored hash      — read straight out of the SQLite file on a separate connection
//
// The stored hash is deliberately NOT read through the store package. If a defect existed
// in hashing or storage, reading it back through the same code could hide the mismatch by
// making the same mistake twice.
func TestPairTokenMatchesStoredHash(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace.db")

	// Mint a code from a separate handle, as `cueseekd pair` does.
	minter, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open minter: %v", err)
	}
	code, err := minter.CreatePairingCode(t.Context(),
		[]domain.Scope{domain.ScopeRead, domain.ScopeServiceControl}, 5*time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	minter.Close()

	agentStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open agent store: %v", err)
	}
	t.Cleanup(func() { agentStore.Close() })

	srv, err := New(Options{
		Store:    agentStore,
		Registry: adapters.NewRegistry(),
		Cache:    adapters.NewCache(),
		HostID:   "test-host",
	})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	resp, err := ts.Client().Post(ts.URL+"/v1/pair", "application/json",
		jsonBody(t, map[string]string{
			"code": code, "device_name": "Phone", "platform": "cli",
		}))
	if err != nil {
		t.Fatalf("POST /v1/pair: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("pair status = %d", resp.StatusCode)
	}

	issued := decode[struct {
		Token  string `json:"token"`
		Device struct {
			Id string `json:"id"`
		} `json:"device"`
	}](t, resp)

	// Computed here, with the standard library, not with the agent's helper.
	sum := sha256.Sum256([]byte(issued.Token))
	returnedHash := hex.EncodeToString(sum[:])

	// Read the row with a fresh connection straight to the file.
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite directly: %v", err)
	}
	defer raw.Close()

	var storedID, storedHash string
	var rowCount int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM devices`).Scan(&rowCount); err != nil {
		t.Fatalf("count devices: %v", err)
	}
	if err := raw.QueryRow(`SELECT id, token_hash FROM devices`).Scan(&storedID, &storedHash); err != nil {
		t.Fatalf("read device row: %v", err)
	}

	t.Logf("devices in table : %d", rowCount)
	t.Logf("device id        : returned=%s  stored=%s", issued.Device.Id, storedID)
	t.Logf("returned token   : %s (%d chars)", issued.Token, len(issued.Token))
	t.Logf("sha256(returned) : %s", returnedHash)
	t.Logf("stored hash      : %s", storedHash)

	if rowCount != 1 {
		t.Fatalf("expected exactly one device, found %d", rowCount)
	}
	if storedID != issued.Device.Id {
		t.Errorf("device id mismatch: response %q, database %q", issued.Device.Id, storedID)
	}
	if storedHash != returnedHash {
		t.Fatalf("STORED HASH DOES NOT MATCH THE RETURNED TOKEN\n"+
			"  sha256(returned) = %s\n  stored           = %s\n"+
			"  the token in the response is not the string that was hashed",
			returnedHash, storedHash)
	}

	// And the round trip closes: that token authenticates.
	if _, err := agentStore.AuthenticateToken(t.Context(), issued.Token); err != nil {
		t.Errorf("token that matches the stored hash was still rejected: %v", err)
	}
}

// TestCreateDeviceGeneratesExactlyOneToken pins the invariant the trace depends on:
// repeated calls must each store the hash of the token they returned, with no second
// generation and no reuse.
func TestCreateDeviceGeneratesExactlyOneToken(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "many.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	tokensByID := map[string]string{}
	for i := range 20 {
		device, token, err := st.CreateDevice(t.Context(), "d", domain.PlatformCLI,
			[]domain.Scope{domain.ScopeRead})
		if err != nil {
			t.Fatalf("CreateDevice %d: %v", i, err)
		}
		tokensByID[device.ID] = token
	}

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer raw.Close()

	rows, err := raw.Query(`SELECT id, token_hash FROM devices`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var id, storedHash string
		if err := rows.Scan(&id, &storedHash); err != nil {
			t.Fatalf("scan: %v", err)
		}
		token, ok := tokensByID[id]
		if !ok {
			t.Errorf("device %s exists in the table but was never returned", id)
			continue
		}
		sum := sha256.Sum256([]byte(token))
		if want := hex.EncodeToString(sum[:]); storedHash != want {
			t.Errorf("device %s: stored %s, sha256(returned token) %s", id, storedHash, want)
		}
		seen++
	}
	if seen != len(tokensByID) {
		t.Errorf("matched %d rows, created %d devices", seen, len(tokensByID))
	}
}
