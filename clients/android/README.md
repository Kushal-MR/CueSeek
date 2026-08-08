# CueSeek for Android

> **Placeholder.** The Gradle project lands in M2. This file records what will be built
> here and the constraints it must respect, so the structure is reviewable first.

## Planned modules

```
clients/android/
├─ app/                 assembly, navigation, DI wiring
├─ core/
│  ├─ api/              GENERATED from api/openapi.yaml, plus a hand-written wrapper
│  ├─ design/           tokens + component catalogue (shared with Wear)
│  ├─ data/             repositories, token storage, SSE client, offline cache
│  └─ model/            domain types the UI actually speaks
└─ feature/
   ├─ pairing/          QR scan → redeem code → store token
   ├─ dashboard/        capability-driven service list
   └─ settings/         paired devices, server management
```

## Constraints

**`core:api` is generated but wrapped.** Generated clients leak their generator's
idioms — nullable everything, `Any?` for `oneOf`, exceptions for error states. A thin
hand-written layer maps those to sealed domain types and a `Result`-style error model,
so no ViewModel ever sees a generated type. Without that layer, changing generators
later becomes a rewrite instead of a swap.

**No `when (serviceId)` anywhere in the UI.** The app holds a map of capability id to
composable renderer and looks it up. Branching on service identity discards the entire
point of capability discovery, and it is the single easiest way to quietly undo
ADR-0005. Treat it as a review-blocking defect.

**Unknown capabilities degrade visibly.** An app built before Immich support will meet
an `immich_jobs` capability it cannot render. It shows the capability's display label
with an "update CueSeek to view this" affordance — never an empty box, and never a
crash. This is a permanent condition, not a transitional one: the Wear app in
particular will routinely be older than the agent.

**The stream is a foreground affordance.** A held SSE connection does not survive Doze.
Anything that must be correct in the background is a poll or a notification, never the
stream.

**Multiple servers exist in the data layer from day one.** The MVP UI shows one, but
every repository is keyed by host id. Retrofitting that later is a navigation rewrite;
carrying it now is one field.

See [ADR-0007](../../docs/adr/0007-client-capability-registry.md) and
[ADR-0010](../../docs/adr/0010-design-system-m3-expressive.md).
