package store

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

func TestCreateAndRedeemPairingCode(t *testing.T) {
	s := newTestStore(t)
	want := []domain.Scope{domain.ScopeRead, domain.ScopeServiceControl}

	code, err := s.CreatePairingCode(t.Context(), want, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	if len(code) != pairingCodeLength+1 || !strings.Contains(code, "-") {
		t.Errorf("code %q is not in XXXX-XXXX form", code)
	}

	got, err := s.RedeemPairingCode(t.Context(), code)
	if err != nil {
		t.Fatalf("RedeemPairingCode: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("scopes = %v, want %v", got, want)
	}
}

// TestPairingCodeIsSingleUse is the property the whole design rests on: a code seen once
// on a screen must not remain usable afterwards.
func TestPairingCodeIsSingleUse(t *testing.T) {
	s := newTestStore(t)
	code, err := s.CreatePairingCode(t.Context(), []domain.Scope{domain.ScopeRead}, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}

	if _, err := s.RedeemPairingCode(t.Context(), code); err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if _, err := s.RedeemPairingCode(t.Context(), code); err != ErrInvalidPairingCode {
		t.Errorf("second redeem: err = %v, want ErrInvalidPairingCode", err)
	}
}

// TestConcurrentRedeemYieldsOneWinner exercises the atomicity claim directly. A separate
// SELECT-then-DELETE would let two callers both observe the row before either deleted it.
func TestConcurrentRedeemYieldsOneWinner(t *testing.T) {
	s := newTestStore(t)
	code, err := s.CreatePairingCode(t.Context(), []domain.Scope{domain.ScopeRead}, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}

	const racers = 8
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
	)
	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.RedeemPairingCode(t.Context(), code); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("%d concurrent redemptions succeeded, want exactly 1", successes)
	}
}

// expireAllPairingCodes ages every outstanding code into the past.
//
// White-box, and deliberately so: CreatePairingCode refuses a negative TTL, which is the
// right production behaviour but leaves no public way to construct an expired code. The
// behaviour under test is redemption, not creation.
func expireAllPairingCodes(t *testing.T, s *Store) {
	t.Helper()
	past := formatTime(time.Now().UTC().Add(-time.Hour))
	if _, err := s.db.Exec(`UPDATE pairing_codes SET expires_at = ?`, past); err != nil {
		t.Fatalf("age pairing codes: %v", err)
	}
}

func TestCreatePairingCodeRejectsNegativeTTL(t *testing.T) {
	s := newTestStore(t)
	_, err := s.CreatePairingCode(t.Context(), []domain.Scope{domain.ScopeRead}, -time.Second)
	if err == nil {
		t.Error("negative ttl was accepted; it would silently become the default")
	}
}

func TestExpiredPairingCodeRejected(t *testing.T) {
	s := newTestStore(t)
	code, err := s.CreatePairingCode(t.Context(), []domain.Scope{domain.ScopeRead}, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	expireAllPairingCodes(t, s)

	if _, err := s.RedeemPairingCode(t.Context(), code); err != ErrInvalidPairingCode {
		t.Errorf("err = %v, want ErrInvalidPairingCode", err)
	}
	// The expired row is consumed even though redemption failed.
	n, err := s.CountPairingCodes(t.Context())
	if err != nil {
		t.Fatalf("CountPairingCodes: %v", err)
	}
	if n != 0 {
		t.Errorf("%d codes remain; the expired row was not cleaned up", n)
	}
}

// TestRedeemIsForgivingOfTypography: the operator reads the code off a terminal and types
// it into a phone. Case and dashes are noise; the underlying secret is the same.
func TestRedeemIsForgivingOfTypography(t *testing.T) {
	variants := map[string]func(string) string{
		"as issued":  func(c string) string { return c },
		"lowercase":  strings.ToLower,
		"no dashes":  func(c string) string { return strings.ReplaceAll(c, "-", "") },
		"spaced out": func(c string) string { return strings.ReplaceAll(c, "-", " ") },
		"padded":     func(c string) string { return "  " + c + "\n" },
	}
	for name, transform := range variants {
		t.Run(name, func(t *testing.T) {
			s := newTestStore(t)
			code, err := s.CreatePairingCode(t.Context(),
				[]domain.Scope{domain.ScopeRead}, time.Minute)
			if err != nil {
				t.Fatalf("CreatePairingCode: %v", err)
			}
			if _, err := s.RedeemPairingCode(t.Context(), transform(code)); err != nil {
				t.Errorf("redeem(%q) failed: %v", transform(code), err)
			}
		})
	}
}

func TestRedeemRejectsGarbage(t *testing.T) {
	s := newTestStore(t)
	for name, code := range map[string]string{
		"empty":            "",
		"punctuation only": "----",
		"wrong code":       "AAAA-AAAA",
		"excluded chars":   "IIII-OOOO", // normalise strips these entirely
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := s.RedeemPairingCode(t.Context(), code); err != ErrInvalidPairingCode {
				t.Errorf("err = %v, want ErrInvalidPairingCode", err)
			}
		})
	}
}

// TestPlaintextPairingCodeIsNotStored mirrors the equivalent token test: the database
// must never hold something that can be replayed.
func TestPlaintextPairingCodeIsNotStored(t *testing.T) {
	s := newTestStore(t)
	code, err := s.CreatePairingCode(t.Context(), []domain.Scope{domain.ScopeRead}, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}

	var stored string
	if err := s.db.QueryRow(`SELECT code_hash FROM pairing_codes`).Scan(&stored); err != nil {
		t.Fatalf("read code_hash: %v", err)
	}
	if strings.Contains(stored, normalisePairingCode(code)) {
		t.Fatal("plaintext pairing code is recoverable from the database")
	}
	if len(stored) != 64 {
		t.Errorf("code_hash length = %d, want 64 (hex SHA-256)", len(stored))
	}
}

func TestCreatePairingCodeValidatesScopes(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreatePairingCode(t.Context(), nil, time.Minute); err == nil {
		t.Error("code with no scopes was accepted")
	}
	if _, err := s.CreatePairingCode(t.Context(), []domain.Scope{"root"}, time.Minute); err == nil {
		t.Error("code with an unknown scope was accepted")
	}
}

func TestPurgeExpiredPairingCodes(t *testing.T) {
	s := newTestStore(t)
	scopes := []domain.Scope{domain.ScopeRead}

	if _, err := s.CreatePairingCode(t.Context(), scopes, time.Minute); err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	expireAllPairingCodes(t, s) // only the first code is expired

	if _, err := s.CreatePairingCode(t.Context(), scopes, time.Hour); err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}

	n, err := s.PurgeExpiredPairingCodes(t.Context())
	if err != nil {
		t.Fatalf("PurgeExpiredPairingCodes: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d, want 1", n)
	}
	remaining, err := s.CountPairingCodes(t.Context())
	if err != nil {
		t.Fatalf("CountPairingCodes: %v", err)
	}
	if remaining != 1 {
		t.Errorf("%d codes remain, want 1", remaining)
	}
}

// TestPairingCodesAreDistinct guards the character-selection arithmetic. The alphabet is
// 32 symbols and each character takes five bits, so no symbol should be favoured; a
// biased generator would show up as collisions far sooner than chance allows.
func TestPairingCodesAreDistinct(t *testing.T) {
	s := newTestStore(t)
	seen := make(map[string]bool)
	for i := range 200 {
		code, err := s.CreatePairingCode(t.Context(), []domain.Scope{domain.ScopeRead}, time.Hour)
		if err != nil {
			t.Fatalf("CreatePairingCode: %v", err)
		}
		if seen[code] {
			t.Fatalf("duplicate code %q at iteration %d", code, i)
		}
		seen[code] = true
	}
}
