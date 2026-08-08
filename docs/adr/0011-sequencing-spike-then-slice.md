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
