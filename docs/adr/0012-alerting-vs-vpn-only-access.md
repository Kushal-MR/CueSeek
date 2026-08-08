# ADR-0012: Alerting reopens the access model

- **Status:** Proposed — deferred, recorded now to prevent surprise
- **Date:** 2026-08-08

## Context

The most obvious feature request after the MVP is alerting: *tell me when Jellyfin dies*.
It looks like a small addition to a system that already computes health (ADR-0008).

It is not. Push notifications on Android require a cloud intermediary — Firebase Cloud
Messaging — because the platform will not let an app hold a socket open indefinitely to
receive them. ADR-0001 explicitly declined to build any cloud component, and the agent is
reachable only over a private network. There is no path from `cueseekd` to a push
notification that does not cross that boundary.

This is recorded now, while ADR-0001 is fresh, precisely because it will not feel like an
architectural decision when it arrives. It will feel like a feature.

## Decision

**Deferred.** No alerting in the MVP. When it is taken up, it is an architectural
decision that supersedes or amends ADR-0001, not a feature ticket.

The options, none yet chosen:

1. **Periodic client polling** (`WorkManager` over the tailnet). No new infrastructure,
   no third party. Coarse — 15-minute minimum intervals — and only fires while the client
   has network access to the host, which is often exactly when it does not.
2. **A persistent foreground service.** Near-real-time and fully self-contained, at the
   cost of a permanent notification and battery drain that users will reasonably object
   to. Increasingly restricted by the platform.
3. **CueSeek Cloud relay.** The only approach that delivers a genuine push when the
   server is unreachable or off. Requires infrastructure, uptime, multi-tenancy and
   end-to-end encryption so the relay cannot read state — and it reverses ADR-0001's
   central premise.
4. **Delegate.** Emit to an existing channel the user already runs — ntfy, Gotify,
   Telegram, a webhook — and let it handle delivery. Small, honest, self-hostable, in
   keeping with the product's positioning, and arguably the right first answer.

## Consequences

- The MVP ships with no alerting, and the README should not imply otherwise.
- Option 4 is currently the most likely first step: it is additive, requires no
  infrastructure, and does not touch ADR-0001.
- **The hardest case is unsolvable without a relay.** If the server is down or off the
  network, an agent-originated notification cannot be sent — by definition. Any design
  that only alerts while the server is healthy misses the failure people actually care
  about, and this should be stated plainly rather than discovered by a user.
- Whichever option is chosen, this ADR is superseded by one that records it, together
  with its effect on ADR-0001.
