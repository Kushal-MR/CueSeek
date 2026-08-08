# ADR-0007: Client-side capability registry, not server-driven UI

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

ADR-0005 produced a server that describes what each service can do. The apparent logical
next step is to let it describe how to *draw* that too — ship layout descriptors, make
each client a generic renderer, and support a new service with no client release at all.

That conclusion is wrong here, for two reasons.

Server-driven UI means inventing a layout language. Every hour spent on that language is
an hour not spent on typography, motion and the details that make an app feel designed —
and a polished UI is an explicit goal of this project. It also makes clients untestable
in isolation, removes compile-time safety and Compose previews, and turns every design
change into a coordinated server release.

Second, it fails precisely where CueSeek needs to succeed. A phone card and a 1.4" watch
tile are not the same layout at different sizes; they are different designs answering
different questions. A layout descriptor that serves both well would have to encode both,
at which point the server is maintaining two designs it cannot preview.

## Decision

**The server owns semantics; each client owns presentation.**

Clients hold a map of capability id to renderer, over typed models generated from the
spec. The agent sends `now_playing` and conforming data; each client decides what that
looks like on its form factor.

Branching on service id in any client is a defect.

## Consequences

- Phone and Wear render the same capability completely differently, from one contract.
- UI is fully previewable, screenshot-testable and designable without a running agent.
- Adding a service ships new UI to existing installations only once clients support the
  capability — the trade accepted in exchange for the above.
- **Version skew is permanent, not transitional.** The Wear app will routinely be older
  than the agent. An unknown capability must render as its display label with an "update
  to view this" affordance — never an empty box, never a crash. This requires the agent
  to send a human-readable label alongside every capability id.
- The agent reports its own version and API version in the stream snapshot, so a client
  can say honestly when the *agent* is the outdated side.
- Cost: every new capability requires a release on every client that should display it.
  With four eventual client platforms, that is real recurring work.
- Cost: the registry is a runtime lookup, so a missing renderer is caught by tests and
  degradation rather than by the compiler.
