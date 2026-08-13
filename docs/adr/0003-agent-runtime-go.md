# ADR-0003: Go for the agent runtime

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

The agent is a long-running daemon on a Linux host that must speak D-Bus (ADR-0002),
poll several HTTP services concurrently, serve an HTTP and SSE API, and install cleanly
on a machine whose owner did not sign up to manage a language runtime.

The obvious candidate was Kotlin with Ktor: the author is an Android developer, and one
language across agent and client would allow sharing models via KMP.

It does not survive ADR-0002. D-Bus support on the JVM is thin — `dbus-java` is weakly
maintained and awkward on modern systems — so the realistic outcome of choosing Kotlin is
falling back to shelling out to `systemctl`, silently reversing the privilege decision.
A JVM's memory footprint on a box that is also transcoding video is a further, smaller
argument.

The premise behind "one language" also does not hold. Consistency across five client
platforms has to come from a **schema**, not a shared language — Swift, TypeScript and Go
were never going to share Kotlin models. Once the contract carries that weight
(ADR-0004), the agent's language is a free choice.

## Decision

Write the agent in Go. `godbus/dbus` and `coreos/go-systemd` for host control,
`gopsutil` for metrics, a cgo-free SQLite driver to preserve static linking.

## Consequences

- The strongest systemd/D-Bus library ecosystem available, in the language the
  surrounding tooling is written in.
- A single static binary with no runtime dependencies. Packaging is a file copy;
  installation does not involve a JVM, a venv or a dependency tree.
- Goroutines and `context` map directly onto per-adapter polling with independent
  timeouts, which is the agent's core runtime shape.
- Cost: a second language to learn and maintain alongside Kotlin. Real, but polyglot
  boundaries are honest here — they follow the deployment target, not preference.
- Cost: **no code sharing with the clients at all.** Every domain type exists twice, in
  Go and in Kotlin. This is only acceptable because both are generated from one spec and
  CI fails on drift. Without that gate this decision would be a mistake.

## Amendments

### Amendment 1 — 2026-08-13: a client may ask the agent to look again

**On the citation.** This ADR is nominally about choosing Go, and its consequences mention
that goroutines and `context` "map directly onto per-adapter polling with independent
timeouts". That sentence is the closest thing the record has to the polling isolation rule,
and the codebase has cited **ADR-0003** for that rule from the first commit — in
`poller.go`, `cache.go`, `handlers.go`, `server.go`, `config.go` and a test named for it.
The rule was therefore never written down as a decision of its own. Amending it here keeps
the amendment where every reader is already being sent, rather than minting a new number
that a dozen comments do not point at.

**The rule as it stood.** A client request never causes an upstream call. The poller
observes each service on its own timer and writes to a cache; the API serves that cache.
The guarantee this buys is that a wedged Jellyfin cannot hang the dashboard — no request
handler ever waits on a service that may never answer.

**Why it needed to widen.** The rule made the agent's cache the *only* clock, and that
produced a dashboard that could be knowingly wrong. Two cases:

1. **After an action.** The agent stops Jellyfin, and then serves "healthy" from a cache it
   has every reason to know is stale — it performed the change itself. Until the next tick,
   up to `poll_interval` later, the console contradicts something the operator did through
   that same console thirty seconds earlier.
2. **Manual verification.** An operator who changes something and wants to confirm it has
   no way to ask. Pull-to-refresh was added to the Android client believing it addressed
   this; it does not, because re-reading the cache returns the same answer. That is the
   defect that prompted this amendment.

**Decision.** Polls may now be triggered by something other than the ticker:

- The agent re-polls a service immediately after an action on it reaches a terminal state.
- `POST /v1/refresh` asks the agent to re-poll every service.

**What is unchanged, and is the whole point.** *No request handler waits on an upstream
service.* `POST /v1/refresh` returns `202` having done nothing but nudge; the poll runs on
the service's own goroutine, under its own timeout, exactly as a ticked poll does, and the
result reaches clients over the stream through the ordinary `service_updated` path. A
Jellyfin that never answers delays nothing but its own next observation. The isolation
property survives intact — what changes is only that the poll *schedule* is no longer
solely a timer.

**Bounds.**

- A nudge that arrives within `minRefreshInterval` of the last poll starting is dropped, so
  a client cannot turn a gesture into a request amplifier against the upstream service.
- Nudges are conflated per service: several arriving during one poll cause one more, not a
  queue.
- The endpoint requires `read` and mutates nothing. Refreshing is looking, not acting, and a
  device paired without `service.control` should still be able to check.

**Rejected: making `POST /v1/refresh` block until the polls finish.** It would let the
client clear its own spinner on a definite answer instead of waiting for a stream event.
It is also precisely the thing the original rule forbids: one wedged service would hold an
HTTP handler open for its full timeout, and a client that retried would stack them. The
spinner is worth less than the guarantee.
