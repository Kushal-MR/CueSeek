package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

const metaKeyHostID = "host_id"

// HostID returns this host's stable identifier, generating it on first call.
//
// Clients key their multi-host data model on this (ADR-0008), so it must not change when
// the hostname, the IP address or the tailnet address does. Tying it to the database
// means it survives exactly as long as the agent's state does, which is the correct
// lifetime: a fresh database is a fresh installation.
//
// Idempotent and safe to call concurrently — the INSERT is conditional and the read
// afterwards is authoritative, so two racing callers converge on the same value rather
// than one overwriting the other.
func (s *Store) HostID(ctx context.Context) (string, error) {
	id, err := s.getMeta(ctx, metaKeyHostID)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return "", err
	}

	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate host id: %w", err)
	}
	generated := hex.EncodeToString(buf[:])

	// OR IGNORE, then re-read: if another process inserted first, its value wins and
	// both callers return the same id.
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO meta (key, value) VALUES (?, ?)`,
		metaKeyHostID, generated); err != nil {
		return "", fmt.Errorf("store host id: %w", err)
	}
	return s.getMeta(ctx, metaKeyHostID)
}

func (s *Store) getMeta(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return value, err
}
