package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// Tests run against a real SQLite file in t.TempDir(), not a mock.
//
// The interesting behaviour here *is* the SQL: the UNIQUE constraint, the migration
// sequence, what DELETE does to a token. A mocked database would assert that we called
// the functions we called, which proves nothing.

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func createTestDevice(t *testing.T, s *Store, name string) (domain.Device, string) {
	t.Helper()
	dev, token, err := s.CreateDevice(t.Context(), name, domain.PlatformAndroid,
		[]domain.Scope{domain.ScopeRead, domain.ScopeServiceControl})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	return dev, token
}

func TestOpenCreatesSchema(t *testing.T) {
	s := newTestStore(t)

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != len(migrations) {
		t.Errorf("user_version = %d, want %d", version, len(migrations))
	}
}

// TestMigrateIsIdempotent covers the restart path: every agent start calls migrate on an
// already-migrated database, and it must be a no-op rather than an error or a re-run.
func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	dev, _ := createTestDevice(t, s1, "Pixel 8")
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	if _, err := s2.GetDevice(t.Context(), dev.ID); err != nil {
		t.Errorf("device did not survive reopen: %v", err)
	}
}

func TestCreateDeviceReturnsUsableToken(t *testing.T) {
	s := newTestStore(t)
	dev, token := createTestDevice(t, s, "Pixel 8")

	if !strings.HasPrefix(token, tokenPrefix) {
		t.Errorf("token %q lacks prefix %q", token, tokenPrefix)
	}
	// 32 random bytes, base64url without padding, plus the prefix.
	if want := len(tokenPrefix) + 43; len(token) != want {
		t.Errorf("token length = %d, want %d", len(token), want)
	}
	if dev.ID == "" {
		t.Error("device id is empty")
	}
	if dev.LastSeenAt != nil {
		t.Error("LastSeenAt should be nil before the device is ever seen")
	}
}

// TestTokensAreUnique guards against a broken random source. Two devices created back to
// back must not collide — and if they did, the UNIQUE constraint on token_hash would
// surface it here rather than in production.
func TestTokensAreUnique(t *testing.T) {
	s := newTestStore(t)
	seen := make(map[string]bool)
	for i := range 50 {
		_, token := createTestDevice(t, s, "device")
		if seen[token] {
			t.Fatalf("duplicate token generated on iteration %d", i)
		}
		seen[token] = true
	}
}

// TestPlaintextTokenIsNotStored is the test that matters most in this file.
//
// ADR-0006 promises the database never holds a working credential. This asserts it
// against the actual bytes on disk rather than trusting that hashToken is called.
func TestPlaintextTokenIsNotStored(t *testing.T) {
	s := newTestStore(t)
	_, token := createTestDevice(t, s, "Pixel 8")

	var stored string
	if err := s.db.QueryRow(`SELECT token_hash FROM devices`).Scan(&stored); err != nil {
		t.Fatalf("read token_hash: %v", err)
	}
	if stored == token {
		t.Fatal("plaintext token stored in the database")
	}
	if strings.Contains(stored, strings.TrimPrefix(token, tokenPrefix)) {
		t.Fatal("stored value contains the plaintext token")
	}
	if len(stored) != 64 { // hex-encoded SHA-256
		t.Errorf("token_hash length = %d, want 64", len(stored))
	}
}

func TestAuthenticateToken(t *testing.T) {
	s := newTestStore(t)
	dev, token := createTestDevice(t, s, "Pixel 8")

	got, err := s.AuthenticateToken(t.Context(), token)
	if err != nil {
		t.Fatalf("AuthenticateToken: %v", err)
	}
	if got.ID != dev.ID {
		t.Errorf("id = %q, want %q", got.ID, dev.ID)
	}
	if !got.HasScope(domain.ScopeRead) || !got.HasScope(domain.ScopeServiceControl) {
		t.Errorf("scopes not round-tripped: %v", got.Scopes)
	}
	if got.HasScope(domain.ScopeHostPower) {
		t.Error("device gained host.power, which it was never granted")
	}
	if got.LastSeenAt == nil {
		t.Error("LastSeenAt not recorded on authentication")
	}
}

func TestAuthenticateTokenRejectsBadInput(t *testing.T) {
	s := newTestStore(t)
	_, valid := createTestDevice(t, s, "Pixel 8")

	cases := map[string]string{
		"empty":            "",
		"garbage":          "not-a-token",
		"right prefix":     tokenPrefix + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"truncated":        valid[:len(valid)-1],
		"extra character":  valid + "x",
		"prefix stripped":  strings.TrimPrefix(valid, tokenPrefix),
		"case altered":     strings.ToUpper(valid),
		"whitespace added": " " + valid,
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.AuthenticateToken(t.Context(), token); err != ErrInvalidToken {
				t.Errorf("err = %v, want ErrInvalidToken", err)
			}
		})
	}
}

// TestRevokeDeviceInvalidatesToken is the property ADR-0006 sells per-device tokens on:
// losing a phone must invalidate exactly that phone.
func TestRevokeDeviceInvalidatesToken(t *testing.T) {
	s := newTestStore(t)
	lost, lostToken := createTestDevice(t, s, "Lost Phone")
	kept, keptToken := createTestDevice(t, s, "Watch")

	if err := s.RevokeDevice(t.Context(), lost.ID); err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}

	if _, err := s.AuthenticateToken(t.Context(), lostToken); err != ErrInvalidToken {
		t.Errorf("revoked token still authenticates: %v", err)
	}
	if _, err := s.AuthenticateToken(t.Context(), keptToken); err != nil {
		t.Errorf("unrelated device %s was affected by revocation: %v", kept.ID, err)
	}
	if err := s.RevokeDevice(t.Context(), lost.ID); err != ErrNotFound {
		t.Errorf("second revoke: err = %v, want ErrNotFound", err)
	}
}

func TestListDevices(t *testing.T) {
	s := newTestStore(t)

	got, err := s.ListDevices(t.Context())
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	// Empty slice, not nil: this is serialised straight to JSON, where nil becomes
	// `null` and an empty slice becomes `[]`. Clients should never have to handle both.
	if got == nil {
		t.Error("ListDevices returned nil; want an empty slice")
	}

	createTestDevice(t, s, "Pixel 8")
	createTestDevice(t, s, "Watch")

	if got, err = s.ListDevices(t.Context()); err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestGetDeviceNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetDevice(t.Context(), "nope"); err != ErrNotFound {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateDeviceRejectsInvalidInput(t *testing.T) {
	s := newTestStore(t)

	if _, _, err := s.CreateDevice(t.Context(), "  ", domain.PlatformAndroid, nil); err == nil {
		t.Error("blank device name was accepted")
	}
	_, _, err := s.CreateDevice(t.Context(), "Phone", domain.PlatformAndroid,
		[]domain.Scope{"root"})
	if err == nil {
		t.Error("unknown scope was accepted")
	}
}

// TestAuditSurvivesDeviceRevocation covers the reason DeviceName is denormalised: the
// audit trail must outlive the device, because a revoked device is often exactly the one
// being investigated.
func TestAuditSurvivesDeviceRevocation(t *testing.T) {
	s := newTestStore(t)
	dev, _ := createTestDevice(t, s, "Lost Phone")

	err := s.AppendAudit(t.Context(), domain.AuditEntry{
		DeviceID:   dev.ID,
		DeviceName: dev.Name,
		Action:     "service.restart",
		Target:     "jellyfin",
		Outcome:    domain.OutcomeAccepted,
	})
	if err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}

	if err := s.RevokeDevice(t.Context(), dev.ID); err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}

	entries, err := s.ListAudit(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	if entries[0].DeviceName != "Lost Phone" {
		t.Errorf("DeviceName = %q; the audit trail lost who did it", entries[0].DeviceName)
	}
	if entries[0].At.IsZero() {
		t.Error("At was not defaulted")
	}
}

func TestListAuditOrderAndLimit(t *testing.T) {
	s := newTestStore(t)
	for _, target := range []string{"first", "second", "third"} {
		if err := s.AppendAudit(t.Context(), domain.AuditEntry{
			Action: "service.restart", Target: target, Outcome: domain.OutcomeSucceeded,
		}); err != nil {
			t.Fatalf("AppendAudit: %v", err)
		}
	}

	entries, err := s.ListAudit(t.Context(), 2)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	// Newest first. Ordered by id rather than timestamp: three rows written in the same
	// millisecond would tie on time, and a flaky ordering assertion is worse than none.
	if entries[0].Target != "third" || entries[1].Target != "second" {
		t.Errorf("order = %q, %q; want third, second", entries[0].Target, entries[1].Target)
	}
}

func TestAppendAuditRequiresAction(t *testing.T) {
	s := newTestStore(t)
	if err := s.AppendAudit(t.Context(), domain.AuditEntry{Target: "jellyfin"}); err == nil {
		t.Error("audit entry without an action was accepted")
	}
}

func TestTimeRoundTrip(t *testing.T) {
	s := newTestStore(t)
	dev, _ := createTestDevice(t, s, "Pixel 8")

	got, err := s.GetDevice(t.Context(), dev.ID)
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if !got.CreatedAt.Equal(dev.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, dev.CreatedAt)
	}
	if got.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt location = %v, want UTC", got.CreatedAt.Location())
	}
}

// TestTokenFingerprintMatchesStoredHash: the fingerprint logged at pairing must be a
// prefix of the value actually written to token_hash, or comparing it against a rejected
// token's fingerprint proves nothing.
func TestTokenFingerprintMatchesStoredHash(t *testing.T) {
	s := newTestStore(t)
	_, token := createTestDevice(t, s, "Phone")

	var stored string
	if err := s.db.QueryRow(`SELECT token_hash FROM devices`).Scan(&stored); err != nil {
		t.Fatalf("read token_hash: %v", err)
	}

	fingerprint := TokenFingerprint(token)
	if len(fingerprint) != 16 {
		t.Errorf("fingerprint %q is not 16 characters", fingerprint)
	}
	if !strings.HasPrefix(stored, fingerprint) {
		t.Errorf("fingerprint %q is not a prefix of the stored hash %q", fingerprint, stored)
	}
	// Never the token itself, and never enough to reconstruct it.
	if strings.Contains(token, fingerprint) {
		t.Error("the fingerprint leaks part of the token")
	}
	if TokenFingerprint("") != "none" {
		t.Error("an empty token should fingerprint as \"none\", not as a hash of nothing")
	}
}
