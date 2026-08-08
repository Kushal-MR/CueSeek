# ADR-0005: Capability-based adapters with runtime discovery

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

CueSeek must eventually support Jellyfin, qBittorrent, Sonarr, Immich and Home Assistant.
These have genuinely different domain models: playback sessions, torrent transfers,
download queues, background jobs, entity states. There is no honest set of fields that
describes all of them.

Two failure modes are well documented in this space. A single rich interface with
optional fields becomes a union of everything, and clients end up branching on service
name to decide what to render — reintroducing the coupling adapters exist to remove. A
fully generic entity model, in the style of Home Assistant, achieves uniformity by
discarding the domain detail that makes each view useful, and produces the
lowest-common-denominator dashboard.

A separate question is how adapters are *loaded*. Out-of-process plugins over gRPC would
let third parties write adapters in any language — attractive, and premature: with two
adapters there is no evidence about what that interface should be, and a plugin API is
expensive to change once published.

## Decision

A minimal `Service` interface (id, display name, health), plus narrow optional
capabilities that adapters opt into and the registry discovers by type assertion:
`Controllable`, `NowPlayingProvider`, `TransferProvider`, and more as needed.

Services advertise their capabilities through the API. Clients render per capability.

Adapters are **compiled in** and registered explicitly. No plugin loader. Revisit at
roughly five adapters.

## Consequences

- Each service exposes exactly what it has, with no empty fields and no pretending.
- Adding a service means adding a package and a registry line. ADR-0011's final
  milestone measures this directly: if a third adapter touches anything else, the
  abstraction is wrong and must be fixed before a fourth.
- New capabilities can reach existing installations as an agent update, with clients
  degrading gracefully until they catch up (ADR-0007).
- **Capability names are public API.** `now_playing` will be shared by Jellyfin, Plex and
  Emby, so it must be specified semantically in the spec rather than shaped around
  whichever service was implemented first. Getting this wrong produces `now_playing_v2`.
- **Actions must be data**, carrying an id, display label and risk level, so clients can
  gate destructive actions without knowing what they do.
- **Health separates reachability from reported status.** "Unreachable" and "degraded"
  are different facts; a console that conflates them lies to its operator.
- Cost: type assertions are a weaker contract than a compiler-checked interface. A
  capability implemented with a subtly wrong signature silently fails to register, and
  the registry needs tests that assert expected capabilities are actually discovered.
- Cost: no third-party adapters without a fork, until and unless a plugin API is added.
