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
