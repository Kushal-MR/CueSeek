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

## Amendments

### Amendment 1 — 2026-09-01: health is derived from reachability alone, not from three inputs

**What changed.** Nothing in the code. This record described inputs to overall health that
were never built, and the description is corrected here rather than the code being bent to
match it.

The Decision above says the agent computes overall status "from unit state, adapter
reachability and host metrics". Two of those three are not true and never have been:

| Claimed input | Reality |
| --- | --- |
| Adapter reachability | **True.** Each adapter's `Health` probes the service's own API, and `health.Overall` aggregates the results. |
| Unit state | **Not used for health.** `UnitState` is read only by `adapters/lifecycle.go`, to decide whether Start or Stop currently applies. No health path reads it. |
| Host metrics | **Not used.** `health.Overall(services []ServiceHealth, at time.Time)` takes services and a clock. Nothing else is passed to it, and `internal/health` imports neither the metrics package nor anything that would let it. |

The Consequences section carries the same error by example — "degraded: qBittorrent
unreachable for 4m, disk 94% full" — where the first clause is real and the second has
never been produced by anything.

**Why it was wrong.** This record was written on 2026-08-08, before M1 and three weeks
before host metrics existed at all (M3.6). It described the intended shape of a subsystem
that had not been built, which is exactly what a contemporaneous ADR is supposed to do. What
did not happen is anyone returning to it when the built shape turned out narrower. M3.6
delivered metrics as their own stream event with their own cadence (ADR-0004 Amendment 4)
and never wired them into health, for a reason that was good and simply went unrecorded: a
disk threshold is a policy number, and this record's own final consequence already names
threshold configurability as the thing that would force it to be revisited.

**Why it is amended rather than implemented.** Feeding metrics into health means choosing
what "too full" means on somebody else's disk. That is a decision with a cost — it either
ships a number invented here, or it makes health policy configurable and pays the price
this record already flagged. Making it inside a phase whose purpose is documentation
accuracy would smuggle a policy choice in under a typo fix. The claim is corrected; the
decision stays open and stays here.

**Read the Decision as:** the agent computes overall status from each service's observed
health, which today comes from adapter reachability and what the service reports about
itself. Unit state governs which actions are offered. Host metrics are reported separately
and do not affect status.

**What this does not change.** Everything the record actually decided. One agent per host
with client-side aggregation and no hub; the agent rather than each client deciding what
colour the dot is; the four-state closed set with `unknown` as a first-class value; reasons
travelling with the status. All of those are built and all of them hold.

**A note for whoever revisits this.** M4.4 adds a `systemd` adapter whose health comes from
unit state, since it has no other source. That makes unit state a health input **for that
adapter**, through the ordinary `Service.Health` path — not a second input to `Overall`, and
not a change to this decision. If a later phase does wire metrics into overall health, it
supersedes this amendment rather than extending it.
