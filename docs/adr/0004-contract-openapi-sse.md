# ADR-0004: Spec-first OpenAPI 3.1, with SSE for live state

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

Five client platforms will eventually bind to this API, written in four languages, and
released on different schedules. The agent is written in a language none of them share
(ADR-0003), so the contract is the only thing holding them together.

The read and write paths have genuinely different shapes. "What is currently playing" is
live state: fetching it means either polling aggressively — expensive on a watch — or
showing stale data. Actions, by contrast, are plain request/response and want nothing
clever. Picking one paradigm for both would compromise one of them.

gRPC was considered and rejected: browsers need a proxy, it adds weight on Wear OS, and
losing the ability to hand someone a `curl` command costs more in a self-hosted tool than
the stronger typing gains. WebSockets were rejected as bidirectional machinery — with a
connection state machine, ping/pong and resume logic — for a capability never needed,
since clients have nothing to stream to the server.

## Decision

`api/openapi.yaml` is **hand-authored** and is the single source of truth. Go server
interfaces and all client SDKs are generated from it; CI regenerates and fails on drift
(ADR-0009).

- Reads: `GET /v1/stream` (SSE) — full snapshot first, then deltas. `GET /v1/services`
  for cold start and clients that cannot hold a stream.
- Writes: `POST .../actions/{id}`, returning **`202 Accepted` + an action id**, with
  progress and terminal state delivered over the stream.

## Consequences

- Reads are push and writes are RPC, each in the shape that suits it.
- The spec is a contract, not a description. Generating it *from* handlers would make it
  a report of whatever was written, which is the failure this decision exists to prevent.
- `curl`-debuggable end to end; `EventSource` is native in browsers for a future web
  client.
- **Destructive actions require the async model.** A synchronous reboot handler never
  gets to write its response — the machine is gone — so the client sees a network error
  on a successful reboot. Acknowledge first, execute after a short delay.
- **SSE does not survive Android Doze.** The stream is a foreground affordance only.
  Nothing background-critical may depend on it; Wear OS polls instead.
- Reconnect yields a new full snapshot. No `Last-Event-ID` replay buffer — the state is
  small and correctness beats bandwidth.
- The stream event schema is versioned independently of the URL path, because old Wear
  builds will outlive refactors.
- Cost: hand-authoring a spec is slower than annotating handlers, and it must be written
  before the code it describes — which is uncomfortable while the domain is still moving.
