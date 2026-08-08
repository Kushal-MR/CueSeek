# ADR-0004: Spec-first OpenAPI, with SSE for live state

- **Status:** Accepted
- **Date:** 2026-08-08
- **Amended:** 2026-08-08 — specification version fixed at 3.0.3 (see [Amendments](#amendments))

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
