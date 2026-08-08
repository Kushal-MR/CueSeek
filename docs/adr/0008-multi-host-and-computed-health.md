# ADR-0008: Multi-host data model, and health computed by the agent

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

Two decisions taken together because both concern where truth lives.

**Multiple hosts.** CueSeek's premise is plural — "home servers". The MVP manages one
machine, and it is tempting to model exactly that. But an app built around a single
implicit server threads that assumption through navigation, repositories and view models,
and unpicking it later is a rewrite of exactly the layer that is hardest to change.
Pairing is already per-host, so the concept exists whether or not it is modelled.

The alternative topology — a designated "hub" agent aggregating other agents — would
require an inter-agent protocol, service discovery and a failure story for the hub. It
buys nothing a client-side list does not.

**Health.** Overall status could be reported per service and combined by each client. But
"what colour is the dot" is a policy decision, and if every client makes it independently
they eventually disagree — leaving the operator to believe whichever screen they happen
to be holding.

## Decision

One `cueseekd` per host; the **client** aggregates across hosts. No hub agent, no
inter-agent protocol. Multi-host is modelled in the client data layer from day one, with
single-host UI in the MVP.

The **agent** computes overall status — `healthy`, `degraded`, `unreachable`, `unknown` —
from unit state, adapter reachability and host metrics, and returns the reasons alongside
it.

## Consequences

- Adding multi-host UI later is a navigation change, not a data-layer rewrite.
- Every host stays independent: one unreachable machine does not affect the others.
- Status is consistent across phone, watch and any future client, by construction.
- Reasons travel with the status. "Degraded" is not actionable; "degraded: qBittorrent
  unreachable for 4m, disk 94% full" is, and it is also what a notification would say.
- **`unknown` is a first-class state.** Before the first poll, and whenever cached state
  has gone stale, the honest answer is "I don't know". Displaying stale green while the
  agent is unreachable is worse than displaying nothing, because it is confidently wrong.
- The four states are a closed set shared by the API, both clients and the design
  system's status language. Adding a fifth is a contract change.
- Cost: carrying a host id through the MVP data layer that nothing yet varies by.
- Cost: health policy is baked into the agent, so tuning thresholds means an agent
  release rather than a client setting. Acceptable while thresholds are few; if they
  become user-configurable, this ADR needs revisiting.
