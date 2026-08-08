// Package store is the agent's persistence layer: the device registry, token hashes
// and the audit log.
//
// SQLite under /var/lib/cueseek, not a JSON file rewritten in place. The data is
// small, but it is written concurrently and a crash mid-write must not lose the
// registry that every client depends on to authenticate. This is the right amount of
// database (ADR-0006).
//
// # What is stored
//
//	devices    id, display name, platform, scopes, created, last seen
//	tokens     device id, token HASH, issued, revoked
//	audit      device id, action, target, outcome, timestamp
//
// # Rules
//
//   - Store token hashes, never tokens. A token is shown to a client exactly once, at
//     pairing time, and is unrecoverable afterwards. If this file leaks, it must not
//     be a set of working credentials.
//
//   - Revocation is immediate and per-device. Losing a phone means deleting one row,
//     not rotating a secret shared by every client.
//
//   - Pairing codes are single-use and short-lived, and redemption is rate-limited
//     with backoff. A short code is guessable given unlimited attempts.
//
//   - The audit log is append-only and records which device did what. An operations
//     console that can reboot a machine should be able to answer "who rebooted it".
package store
