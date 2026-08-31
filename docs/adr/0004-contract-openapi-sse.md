# ADR-0004: Spec-first OpenAPI, with SSE for live state

- **Status:** Accepted
- **Date:** 2026-08-08
- **Amended:** 2026-08-08 — specification version fixed at 3.0.3; SSE failure mode corrected
  after A7 (see [Amendments](#amendments))

> The title deliberately carries no version number. The decision is *spec-first OpenAPI*;
> which minor version is a constraint recorded below, and it is expected to change.

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

`api/openapi.yaml` is **hand-authored** and is the single source of truth, written in
**OpenAPI 3.0.3** (see Amendment 1). Go server interfaces and all client SDKs are
generated from it; CI regenerates and fails on drift (ADR-0009).

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

## Amendments

### Amendment 1 — 2026-08-08: specification version is 3.0.3, not 3.1

**What changed.** This ADR originally specified OpenAPI 3.1. The contract is written in
**3.0.3**.

**Why.** The generator, `oapi-codegen` v2.4.1, does not support 3.1
([oapi-codegen#373](https://github.com/oapi-codegen/oapi-codegen/issues/373)). This was
tested rather than assumed during M1.1: pointed at a 3.1 document it emits

```
WARNING: You are using an OpenAPI 3.1.x specification, which is not yet supported by
oapi-codegen … it is recommended to downgrade your spec to 3.0.x
```

and then **continues with reduced functionality instead of failing.** That failure mode is
the dangerous part. A future version bump would degrade generation silently, and the first
symptom would be missing client types rather than a build error.

**Guard.** `TestSpecIsOpenAPI30` in `agent/internal/api/gen/spec_test.go` fails the build
if the declared version stops beginning with `3.0`, converting a silent degradation into a
loud one and pointing the reader back here.

**What this costs.** 3.0.3 lacks features 3.1 has and that this contract would use:
`type: [string, "null"]` for genuine nullability (3.0 forces optional-or-present),
`const`, full JSON Schema 2020-12 alignment, and `examples` on schemas. None block the v0
surface; all would improve it. The practical effect today is that optional fields are
modelled by omission from `required` rather than by explicit nullability, which is a
weaker statement of intent.

**Future task, explicitly owned.** Upgrade to OpenAPI 3.1 once generator support is
production-ready. This is not "someday" — it has a trigger and a checklist:

1. `oapi-codegen` closes issue #373 with non-experimental 3.1 support, **or** the project
   moves to a generator that has it.
2. Verify the Kotlin/Swift/TypeScript generators chosen for the clients also handle 3.1,
   since the contract serves all of them and the weakest generator sets the ceiling.
3. Bump `openapi:` to `3.1.0`, delete `TestSpecIsOpenAPI30`, regenerate, and confirm the
   drift gate stays green.
4. Supersede this amendment with another recording the outcome.

**What did not change.** The decision itself. Spec-first, hand-authored, generated
clients, CI drift enforcement, reads-as-stream and writes-as-async-RPC all stand exactly
as originally recorded. This amendment records a tooling constraint on how that decision
is expressed, not a reversal of it — which is why it is an amendment rather than a
superseding record. See ADR-0000.

### Amendment 2 — 2026-08-08: Doze freezes the stream, it does not kill it

**What changed.** This record originally stated that "Android Doze kills held streams", and
concluded from that the stream is foreground-only. Assumption A7 was measured on the target
phone and tailnet (`docs/m0-findings.md`), and the conclusion is right for the wrong reason.

**What was measured.** Foreground behaviour is excellent — on mobile data, screen on, the
stream ran with zero missed events and zero reconnects. With the screen off, on both Wi-Fi
and cellular, it ran normally for roughly a minute and then went silent for **108 and 168
seconds** respectively, before erroring and recovering in 3–4 seconds.

**The correction.** Doze does not close the connection. It freezes the radio, TCP
backpressures, and the stream **continues to report itself as connected** for the whole
stall. The error surfaces only when the queue flushes on wake. No events are lost; they
arrive late, in a burst.

The distinction matters because the two failures need different handling. A stream that
dies loudly is something a client reconnects from. A stream that lies leaves a console
showing a green dot for a service that crashed three minutes earlier — which is exactly
what ADR-0008 calls "confidently wrong", and the state that ADR's `unknown` exists to
prevent.

**Requirements this adds.**

1. **Clients must not trust connection state.** A client treats data as stale when no event
   has arrived within roughly twice the heartbeat interval, regardless of whether the
   transport claims to be connected, and renders `unknown` (ADR-0008) rather than the last
   known values. The mechanism already exists; A7 established that stream silence must feed
   it, not only poll failure.
2. **The stream must carry a periodic heartbeat**, so silence is unambiguous. Without one, a
   quiet system is indistinguishable from a frozen one.
3. **The server needs write deadlines.** A7 showed the agent's own sequence counter stalling
   in step with the client — the blocked write backpressured the sending goroutine. Without
   a deadline, one frozen phone parks a goroutine until TCP gives up on its own schedule.

**What did not change.** Everything else. SSE remains the transport: the foreground case,
which is the case CueSeek is for, was flawless over a real cellular tailnet. Reconnect
delivering a full snapshot with no replay buffer was exercised twice and worked, so that
choice is now measured rather than assumed. "Foreground-only, nothing background-critical
depends on the stream" stands — the reasoning behind it is simply now correct.

**What this does not settle.** Deep Doze was not measured; the 15-minute stationary phase
was skipped once the pattern proved identical across two networks. It would refine how long
a stall can last. It would not change any requirement above, because all three hold at three
minutes and at thirty.

### Amendment 3 — 2026-08-09: on clients, types are generated and transport is not

**What changed.** This record says "Go server interfaces and all client SDKs are generated
from it". For the Android client that is narrowed: **Kotlin wire types are generated from
`api/openapi.yaml` and committed, with a Gradle drift check mirroring the Go one. The eight
REST calls and the stream reader are hand-written.**

**Why.** The property this decision exists to protect is that a client cannot silently
disagree with the contract about the shape of what crosses the wire. Generated DTOs plus a
drift gate preserve that entirely. Generating the transport as well would add an
openapi-generator toolchain to CI and a wrapper over exceptions-as-errors output, in
exchange for eight function signatures over a surface small enough to read in one sitting.

It also could not cover the endpoint that matters most. **No OpenAPI generator models
`text/event-stream`**, so `GET /v1/stream` — where the freshness rules from Amendment 2 live
— would be hand-written regardless. The Go server hand-wrote its side of the stream for
precisely this reason, which makes this the second time the same limitation has shaped an
implementation rather than a new argument.

**Where the line is.** Generated types stay internal to `:core:api`; the module exposes
domain types and a `Result`-style error model, per ADR-0009. The threshold at which this
trade flips is roughly thirty endpoints, or a second hand-written client — whichever comes
first.

**What did not change.** Spec-first, hand-authored, single source of truth, CI drift
enforcement, reads-as-stream and writes-as-async-RPC. The contract still cannot disagree
with any client about shapes; a generator simply stopped being the way that guarantee is
obtained for the parts where it was buying nothing.

### Amendment 4 — 2026-08-31: the stream carries the host, not only its services

**What changed.** The stream's event vocabulary gains a fourth type, `host_updated`, and
`Snapshot` gains an optional `host_metrics`. A new `HostMetrics` schema carries CPU,
memory, storage and thermal readings, and `GET /v1/host/metrics` serves the same payload for
clients that are not holding a stream.

**Why not on `System`.** M3's plan placed host metrics on the system surface, reasoning that
`SystemInfo` already carries host identity. That is right about ownership and silent about
delivery, and delivery is where it breaks. `System` is sent exactly twice — once from
`GET /v1/system`, once as `snapshot.system` at the moment a stream opens — because nothing
in it changes for the life of the process. Metrics change every few seconds. Inside `System`
a CPU figure would have been correct for one second and then frozen until the client
reconnected, sitting under a live heartbeat indicator claiming to be current. On a console
whose entire premise is that staleness must be visible (Amendment 2), that is the worst
available outcome, and it would have been invisible in every test that did not wait.

**Why not a pseudo-service.** Modelling the host as a service would have reused the poller,
the cache, the capability registry and the row renderer, for a fraction of the code. It was
rejected because it lies about the domain in a way the UI then has to work around: the host
would appear in the roster and the tally, so "two of three healthy" would count the computer
as one of its own services, against ADR-0005's scope rule. It also gets worse rather than
better with time — once M3.7 adds power actions, "restart the host" and "restart Jellyfin"
would be the same affordance in the same list, which is precisely the confusion M3.7 was
separated from M3.1 to avoid.

**Cadence.** Metrics are collected on their own ticker, defaulting to 10s against the
service poll's 30s, and the collector is a separate goroutine rather than part of the
adapter poller. Nothing in that loop applies here: there is no adapter, no upstream to time
out against, no health to report and no nudge semantics. The faster interval is not
gratuitous — utilisation averaged over thirty seconds hides the five-second spike that
explains a hot machine.

**Absent is not zero, again.** Every field but `collected_at` is optional, and `null` and
`[]` are distinguished on the wire. Hardware differs more than services do: a virtual
machine exposes no temperature sensors, a container may see no usable `/proc/stat`, and the
first collection after a restart cannot compute utilisation at all, because the kernel
reports cumulative counters and one sample is not a measurement. `204 No Content` on the
read endpoint says the same thing at the request level. This is the M3.5 rule applied where
it is harder to get right and easier to get away with.

**What did not change.** Spec-first with a CI drift gate, snapshot-on-connect with no replay
buffer, reads-as-stream and writes-as-async-RPC, and the freshness rules from Amendment 2 —
which metrics obey more strictly than services do. A stale service degrades to `unknown` and
keeps its timestamps, because "healthy three minutes ago" is still information; stale
metrics are dropped entirely, because a three-minute-old CPU percentage is not.
