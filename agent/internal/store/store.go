package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // cgo-free driver; keeps the single-static-binary property

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

var (
	// ErrNotFound is returned when no row matches.
	ErrNotFound = errors.New("not found")
	// ErrInvalidToken is returned for any token that does not authenticate — unknown,
	// malformed or revoked alike. The caller must not distinguish between them in a
	// response: telling a client which of the three it was helps enumerate valid tokens.
	ErrInvalidToken = errors.New("invalid token")
)

// tokenPrefix marks CueSeek tokens in logs, screenshots and secret scanners.
//
// Costs nothing and makes an accidentally-pasted token recognisable as a credential
// rather than as noise. The same reasoning gives GitHub its `ghp_` prefix.
const tokenPrefix = "csk_"

// Store is the agent's persistence layer: device registry, token hashes, audit log.
type Store struct {
	db *sql.DB
}

// Open opens (creating if absent) the SQLite database at path and migrates it.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}

	// SQLite permits one writer at a time. Serialising every connection removes an
	// entire category of SQLITE_BUSY races in exchange for throughput that is
	// irrelevant here — this database holds a handful of devices and an audit log, and
	// is read a few times per minute.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		// WAL lets readers proceed during a write and survives crashes better than the
		// default rollback journal.
		"PRAGMA journal_mode = WAL",
		// Without this, SQLite trusts the OS to flush and a power cut can corrupt.
		"PRAGMA synchronous = NORMAL",
		// Off by default in SQLite, which surprises everyone.
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close releases the database.
func (s *Store) Close() error { return s.db.Close() }

// migrations is an append-only list. Each entry moves the schema forward by one step,
// and its index+1 becomes the database's user_version.
//
// This is a migration system in about forty lines rather than a dependency. It is enough
// because the rules are simple and absolute: never edit an existing entry, never reorder,
// only append. Editing entry 0 would leave every already-migrated database untouched
// while new ones got the new schema — two divergent shapes with the same version number,
// which is the failure mode migration tools exist to prevent.
var migrations = []string{
	// 1: devices and audit log.
	`
	CREATE TABLE devices (
		id           TEXT PRIMARY KEY,
		name         TEXT NOT NULL,
		platform     TEXT NOT NULL,
		scopes       TEXT NOT NULL,
		token_hash   TEXT NOT NULL UNIQUE,
		created_at   TEXT NOT NULL,
		last_seen_at TEXT
	);

	CREATE TABLE audit (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		at          TEXT NOT NULL,
		device_id   TEXT NOT NULL DEFAULT '',
		device_name TEXT NOT NULL DEFAULT '',
		action      TEXT NOT NULL,
		target      TEXT NOT NULL DEFAULT '',
		outcome     TEXT NOT NULL,
		detail      TEXT NOT NULL DEFAULT ''
	);

	CREATE INDEX idx_audit_at ON audit(at DESC);
	`,

	// 2: pairing codes.
	//
	// These live in the database rather than in the daemon's memory because
	// `cueseekd pair` is a separate process from the running agent. An in-memory code
	// minted by the CLI would be invisible to the server that has to redeem it.
	//
	// Storing the hash, not the code, keeps the rule from ADR-0006 uniform: nothing in
	// this file is ever a working credential.
	`
	CREATE TABLE pairing_codes (
		code_hash  TEXT PRIMARY KEY,
		scopes     TEXT NOT NULL,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
	);

	CREATE INDEX idx_pairing_codes_expires_at ON pairing_codes(expires_at);
	`,

	// 3: agent metadata.
	//
	// Currently just the host id, which clients use to key their multi-host data model
	// (ADR-0008) and which must therefore survive restarts, hostname changes and IP
	// changes.
	//
	// Generated and stored rather than derived from /etc/machine-id: machine-id is
	// absent on non-Linux systems, which would make the agent untestable off its
	// deployment target, and exposing it directly over an API is discouraged because it
	// is a stable cross-application identifier for the machine.
	`
	CREATE TABLE meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`,
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version > len(migrations) {
		// The binary is older than the database. Continuing would operate on a schema
		// this code has never seen.
		return fmt.Errorf(
			"database schema version %d is newer than this agent understands (%d); "+
				"downgrade is not supported", version, len(migrations))
	}

	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		// PRAGMA does not accept bound parameters. i is a loop index over a package-level
		// slice, so there is no user input anywhere near this string.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("set user_version %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- tokens

// newToken returns a fresh bearer token: 256 bits from crypto/rand, URL-safe, prefixed.
func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// hashToken returns the hex-encoded SHA-256 of a token.
//
// SHA-256 rather than bcrypt or argon2, deliberately, for two reasons.
//
// First, password hashers are slow on purpose so that guessing a low-entropy human
// secret is expensive. A token here is 256 bits of cryptographic randomness — there is
// nothing to guess, and the slowness would tax every request while buying nothing.
//
// Second, bcrypt salts each row, so identifying which device presented a token would
// mean hashing the candidate against every row in the table. A deterministic hash turns
// authentication into a single indexed lookup.
//
// What is stored is never a working credential: if this file leaks, the hashes cannot be
// replayed.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newDeviceID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate device id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// ---------------------------------------------------------------- devices

// CreateDevice registers a device and returns it together with its plaintext token.
//
// The token is returned exactly once and cannot be recovered afterwards — only its hash
// is persisted. Callers must hand it straight to the client and never log it.
func (s *Store) CreateDevice(
	ctx context.Context, name string, platform domain.Platform, scopes []domain.Scope,
) (domain.Device, string, error) {
	if strings.TrimSpace(name) == "" {
		return domain.Device{}, "", errors.New("device name must not be empty")
	}
	for _, sc := range scopes {
		if !sc.Valid() {
			return domain.Device{}, "", fmt.Errorf("invalid scope %q", sc)
		}
	}

	id, err := newDeviceID()
	if err != nil {
		return domain.Device{}, "", err
	}
	token, err := newToken()
	if err != nil {
		return domain.Device{}, "", err
	}

	dev := domain.Device{
		ID:        id,
		Name:      name,
		Platform:  platform,
		Scopes:    scopes,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO devices (id, name, platform, scopes, token_hash, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		dev.ID, dev.Name, string(dev.Platform), encodeScopes(dev.Scopes),
		hashToken(token), formatTime(dev.CreatedAt),
	)
	if err != nil {
		return domain.Device{}, "", fmt.Errorf("insert device: %w", err)
	}
	return dev, token, nil
}

// AuthenticateToken resolves a bearer token to its device and records the sighting.
//
// Returns ErrInvalidToken for anything that does not match, without distinguishing why.
func (s *Store) AuthenticateToken(ctx context.Context, token string) (domain.Device, error) {
	if token == "" {
		return domain.Device{}, ErrInvalidToken
	}

	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, platform, scopes, created_at, last_seen_at
		   FROM devices WHERE token_hash = ?`, hashToken(token))

	dev, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Device{}, ErrInvalidToken
	}
	if err != nil {
		return domain.Device{}, err
	}

	// Best-effort: a device that authenticated should not be rejected because we could
	// not record that it did.
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE devices SET last_seen_at = ? WHERE id = ?`, formatTime(now), dev.ID,
	); err == nil {
		dev.LastSeenAt = &now
	}
	return dev, nil
}

// ListDevices returns all paired devices, newest first.
func (s *Store) ListDevices(ctx context.Context) ([]domain.Device, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, platform, scopes, created_at, last_seen_at
		   FROM devices ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices := []domain.Device{}
	for rows.Next() {
		dev, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, dev)
	}
	return devices, rows.Err()
}

// GetDevice returns one device, or ErrNotFound.
func (s *Store) GetDevice(ctx context.Context, id string) (domain.Device, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, platform, scopes, created_at, last_seen_at
		   FROM devices WHERE id = ?`, id)
	dev, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Device{}, ErrNotFound
	}
	return dev, err
}

// RevokeDevice deletes a device, immediately invalidating its token.
//
// A hard delete rather than a revoked flag: the row's only purpose is to authenticate a
// token, and keeping dead credentials around invites a future query that forgets to
// filter them. What the device did survives in the audit log, which stores its name
// rather than referencing this row.
func (s *Store) RevokeDevice(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM devices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------- audit

// AppendAudit records one action. The log is append-only; nothing updates or deletes it.
func (s *Store) AppendAudit(ctx context.Context, e domain.AuditEntry) error {
	if e.Action == "" {
		return errors.New("audit entry requires an action")
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit (at, device_id, device_name, action, target, outcome, detail)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		formatTime(e.At.UTC()), e.DeviceID, e.DeviceName,
		e.Action, e.Target, string(e.Outcome), e.Detail)
	if err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}

// ListAudit returns the most recent entries, newest first.
func (s *Store) ListAudit(ctx context.Context, limit int) ([]domain.AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, at, device_id, device_name, action, target, outcome, detail
		   FROM audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []domain.AuditEntry{}
	for rows.Next() {
		var (
			e     domain.AuditEntry
			at    string
			outcm string
		)
		if err := rows.Scan(&e.ID, &at, &e.DeviceID, &e.DeviceName,
			&e.Action, &e.Target, &outcm, &e.Detail); err != nil {
			return nil, err
		}
		if e.At, err = parseTime(at); err != nil {
			return nil, err
		}
		e.Outcome = domain.Outcome(outcm)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ---------------------------------------------------------------- helpers

// scanner is satisfied by both *sql.Row and *sql.Rows, so single-row and multi-row reads
// share one decoder and cannot drift apart.
type scanner interface{ Scan(dest ...any) error }

func scanDevice(sc scanner) (domain.Device, error) {
	var (
		dev      domain.Device
		platform string
		scopes   string
		created  string
		lastSeen sql.NullString
	)
	if err := sc.Scan(&dev.ID, &dev.Name, &platform, &scopes, &created, &lastSeen); err != nil {
		return domain.Device{}, err
	}

	dev.Platform = domain.Platform(platform)
	dev.Scopes = decodeScopes(scopes)

	var err error
	if dev.CreatedAt, err = parseTime(created); err != nil {
		return domain.Device{}, err
	}
	if lastSeen.Valid {
		t, err := parseTime(lastSeen.String)
		if err != nil {
			return domain.Device{}, err
		}
		dev.LastSeenAt = &t
	}
	return dev, nil
}

// Scopes are stored as a comma-separated string rather than a join table.
//
// They are a closed set of at most three values, always read with the device and never
// queried independently. A join table would add a migration, a join and a second insert
// to model a field that is conceptually a small enum set. If scopes ever grow or need
// querying on their own, this is the migration to write.
func encodeScopes(scopes []domain.Scope) string {
	parts := make([]string, len(scopes))
	for i, s := range scopes {
		parts[i] = string(s)
	}
	return strings.Join(parts, ",")
}

func decodeScopes(s string) []domain.Scope {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	scopes := make([]domain.Scope, 0, len(parts))
	for _, p := range parts {
		// Drop anything unrecognised. A scope removed from a future version of the agent
		// must not resurrect itself as a permission this binary cannot reason about.
		if sc := domain.Scope(p); sc.Valid() {
			scopes = append(scopes, sc)
		}
	}
	return scopes
}

// SQLite has no date type. RFC3339 in UTC sorts lexicographically in the same order it
// sorts chronologically, and stays readable when someone opens the file with the sqlite3
// CLI to work out what happened — which for an operations tool, someone will.
func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", s, err)
	}
	return t.UTC(), nil
}
