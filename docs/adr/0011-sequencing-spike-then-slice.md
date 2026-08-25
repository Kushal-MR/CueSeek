# ADR-0011: De-risking spike, then a thin end-to-end slice

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

The architecture rests on assumptions that have not been tested on a real machine. The
load-bearing one is ADR-0002: that a polkit rule cleanly grants an unprivileged daemon
`RestartUnit` and `PowerOff` on this specific Linux Mint host. If that is wrong, ADR-0002
changes, and ADR-0003's central justification changes with it.

Two plausible orderings pull the wrong way. Building the agent to completion first gives
no visual feedback for weeks and designs the API without a real consumer, which is how
awkward contracts happen. Building the whole Android app against a mock generated from
the spec — which spec-first uniquely enables — is attractive and defers every host-level
assumption to the very end, while producing a spec written without an implementation to
keep it honest.

## Decision

1. **Spike first, and throw it away.** Prove on the real host: the polkit rule grants
   `RestartUnit` on an allowlisted unit and `Reboot`/`PowerOff` via logind; unit state
   reads return usable values; an SSE stream survives the tailnet on mobile data with the
   phone's screen off.
2. **Then one thin slice, end to end** — pair a device, see one service's health, restart
   it — deployed and used daily on the real machine.
3. **Then widen**, capability by capability, along a proven path.
4. **A third adapter as a measurement.** When adding it, count the files changed outside
   its own package. More than a registry entry and the spec means ADR-0005 is wrong and
   must be fixed before a fourth.

## Consequences

- The riskiest assumption is tested in week one, when it is cheapest to be wrong.
- The API is designed against a real consumer, so its awkward parts surface immediately.
- Daily real use is the acceptance test that matters. Bugs found that way are the ones
  worth fixing.
- Step 4 turns "is the abstraction good?" from an opinion into a number.
- Cost: roughly three weeks with nothing that looks impressive. The design work that
  makes this a portfolio piece is deliberately deferred behind plumbing.
- Cost: **the spike must actually be deleted.** It will work, and keeping it will be
  tempting. Code written to answer a question is rarely code worth maintaining, and a
  spike promoted to production quietly reintroduces every shortcut taken to answer the
  question faster.

## Amendments

### Amendment 1 — 2026-08-27: the measurement, taken one adapter early

Step 4 says to count the files changed outside a new adapter's package when adding **a
third**. The count was taken at the *second* — qBittorrent, in M3.4 — because waiting would
have meant carrying an unmeasured assumption through two more phases that build on it
(`transfers` in M3.5 and host metrics in M3.6 both extend the capability model).

**The result.** New package `agent/internal/adapters/qbittorrent/`, and outside it:

| File | Change |
| --- | --- |
| `adapters/builtin/builtin.go` | one line in the factory map |
| `config/config.go` | `username`, `password`, `password_file` |
| `domain/health.go` | one reason code, `peer_connectivity` |

Plus `deploy/config.example.yaml`, which is documentation. **No contract change** — the
generator produced no diff — and **no client change at all**: qBittorrent reached the phone
through the `control` and `web_ui` capabilities Jellyfin already used, with no Android
release and no screen edited.

**Read against this ADR's own bar, that is three files where it hoped for two, so it is not
a clean pass.** Taking each in turn:

- `builtin.go` is the anticipated registry entry.
- `domain/health.go` is a shared vocabulary that grows with the *kinds of thing that can be
  wrong*, not with the number of services — the same O(capabilities) property that makes
  `capabilityProbes` acceptable. A fourth HTTP-shaped adapter adds nothing here.
- `config.Service` is the one that matters. It grew because qBittorrent authenticates with a
  login rather than an API key, which is a real difference in shape rather than a leak. But
  it is exactly the direction a leak arrives from, and the growth is per-*service-type*,
  which is the property step 4 was written to catch.

**Decision.** ADR-0005 stands; nothing here suggests the capability model is wrong. The
bound moves to configuration instead:

> A third credential shape must become a per-adapter options map, not a fourth pair of
> fields on `config.Service`.

Recorded now, while the reasoning is cheap, rather than discovered at the fourth adapter
when three call sites already depend on the flat shape.

**What the count did not measure.** Neither adapter has yet implemented a capability the
other lacks — `transfers` and `now_playing` are both still unimplemented. The genuinely
hard question ADR-0005 poses is whether *those* fit without reshaping the interface, and
M3.5 is what answers it. This measurement covers breadth of services, not depth of
capabilities.
