// Package health derives overall status from raw signals: systemd unit state,
// adapter reachability, and host metrics.
//
// The agent computes this, not the clients (ADR-0008). If each client decided for
// itself what counts as degraded, the phone and the watch would eventually disagree
// about the same server — and the one you happened to be looking at would be the one
// you believed.
//
// A status is always accompanied by its reasons. "Degraded" alone is not actionable;
// "degraded: qBittorrent unreachable for 4m, disk 94% full" is. The reasons are what
// the UI actually shows, and what makes a notification worth reading.
//
// # unknown is a real state
//
// Before the first poll completes, and whenever cached state has aged past its
// tolerance, the correct answer is unknown — not healthy, and not an error. An
// operations console that displays a stale green light while it cannot reach the
// machine is worse than one that displays nothing, because it is confidently wrong.
//
// The four states — healthy, degraded, unreachable, unknown — are a closed set shared
// by the API, both clients and the design system's status language. Adding a fifth is
// a contract change, not an implementation detail.
package health
