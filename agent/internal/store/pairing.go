package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// ErrInvalidPairingCode is returned for a code that is unknown, expired or already
// redeemed.
//
// One error for all three cases, deliberately. A caller told that a code was "expired"
// rather than "unknown" has learned that the code was once real, which is exactly the
// signal an attacker wants while guessing.
var ErrInvalidPairingCode = errors.New("invalid pairing code")

// pairingAlphabet excludes I, O, 0 and 1 — the characters people misread when copying a
// code off a terminal onto a phone. Exactly 32 symbols, so each character carries 5 bits.
const pairingAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// pairingCodeLength of 8 gives 40 bits of entropy. That is far short of a token's 256,
// and is only safe because a code is single-use, expires in minutes, and redemption is
// rate limited. Those three properties are load-bearing, not belt-and-braces.
const pairingCodeLength = 8

// DefaultPairingTTL is short on purpose: the code exists for the seconds between an
// operator reading it off a screen and a phone redeeming it.
const DefaultPairingTTL = 5 * time.Minute

// CreatePairingCode mints a code that can be redeemed once, for the given scopes.
//
// Returns the plaintext code, which is displayed to the operator and never stored.
func (s *Store) CreatePairingCode(
	ctx context.Context, scopes []domain.Scope, ttl time.Duration,
) (string, error) {
	if len(scopes) == 0 {
		return "", errors.New("pairing code must grant at least one scope")
	}
	for _, sc := range scopes {
		if !sc.Valid() {
			return "", fmt.Errorf("invalid scope %q", sc)
		}
	}
	// Zero means "unspecified, use the default". Negative is a caller error, and
	// silently turning it into a five-minute code would hide the mistake behind a code
	// that works — the worst way to find out.
	switch {
	case ttl < 0:
		return "", fmt.Errorf("pairing code ttl must not be negative, got %s", ttl)
	case ttl == 0:
		ttl = DefaultPairingTTL
	}

	code, err := newPairingCode()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO pairing_codes (code_hash, scopes, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		hashToken(normalisePairingCode(code)), encodeScopes(scopes),
		formatTime(now), formatTime(now.Add(ttl)))
	if err != nil {
		return "", fmt.Errorf("insert pairing code: %w", err)
	}
	return code, nil
}

// RedeemPairingCode consumes a code and returns the scopes it grants.
//
// Redemption is atomic: the row is deleted in the same transaction that reads it, so two
// concurrent requests presenting the same code cannot both succeed. Doing this as a
// separate SELECT then DELETE would leave a window in which both callers see the row.
func (s *Store) RedeemPairingCode(ctx context.Context, code string) ([]domain.Scope, error) {
	normalised := normalisePairingCode(code)
	if normalised == "" {
		return nil, ErrInvalidPairingCode
	}
	hash := hashToken(normalised)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	var (
		scopes    string
		expiresAt string
	)
	err = tx.QueryRowContext(ctx,
		`SELECT scopes, expires_at FROM pairing_codes WHERE code_hash = ?`, hash,
	).Scan(&scopes, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidPairingCode
	}
	if err != nil {
		return nil, err
	}

	// Delete regardless of expiry: a presented code is spent either way, and leaving an
	// expired row behind serves no purpose.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM pairing_codes WHERE code_hash = ?`, hash); err != nil {
		return nil, err
	}

	exp, err := parseTime(expiresAt)
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(exp) {
		// Commit so the expired row is actually removed, then report failure.
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrInvalidPairingCode
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	granted := decodeScopes(scopes)
	if len(granted) == 0 {
		// The stored scopes decoded to nothing, meaning this binary no longer recognises
		// any of them. Granting a device zero permissions would look like success and
		// behave like failure.
		return nil, ErrInvalidPairingCode
	}
	return granted, nil
}

// PurgeExpiredPairingCodes removes codes that can no longer be redeemed, and reports how
// many it deleted. Housekeeping only — expiry is enforced at redemption regardless.
func (s *Store) PurgeExpiredPairingCodes(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM pairing_codes WHERE expires_at <= ?`, formatTime(time.Now().UTC()))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CountPairingCodes reports how many codes are outstanding. Used by tests and by the
// pair command to warn an operator who has minted codes and never used them.
func (s *Store) CountPairingCodes(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pairing_codes`).Scan(&n)
	return n, err
}

// newPairingCode returns a code formatted as XXXX-XXXX.
//
// Characters are drawn with rejection-free indexing: the alphabet is exactly 32 symbols,
// so five bits map onto it without bias. An alphabet whose length did not divide 256
// evenly would make naive modulo arithmetic favour early characters.
func newPairingCode() (string, error) {
	buf := make([]byte, pairingCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate pairing code: %w", err)
	}

	out := make([]byte, 0, pairingCodeLength+1)
	for i, b := range buf {
		if i == pairingCodeLength/2 {
			out = append(out, '-')
		}
		out = append(out, pairingAlphabet[b&0x1f]) // low 5 bits: uniform over 32 symbols
	}
	return string(out), nil
}

// normalisePairingCode makes redemption forgiving of how a human retyped the code, while
// keeping the stored hash canonical. Case, dashes and stray whitespace are all noise.
func normalisePairingCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if strings.ContainsRune(pairingAlphabet, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
