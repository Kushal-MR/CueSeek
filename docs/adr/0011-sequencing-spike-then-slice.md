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

### Amendment 2 — 2026-09-01: productization becomes M4; Wear moves to M5

**What changed.** A milestone is inserted. M4 is now productization — making CueSeek
installable by someone who has never seen the development host. The Wear OS client moves
from M4 to **M5**, and step 4's third-adapter measurement from M5 to **M6**.

| Was | Is | Milestone |
| --- | --- | --- |
| M4 | **M4** | Productization (new) |
| M4 | **M5** | Wear OS standalone client, tiles and complications |
| M5 | **M6** | A third adapter, as the measurement step 4 requires |

**Why the order changes.** This ADR's step 3 says "then widen, capability by capability,
along a proven path", and step 4 measures the abstraction at a third adapter. Neither step
says anything about who can run the result, because when this was written the answer was
obviously "the author". That assumption is no longer wanted, and it is cheapest to correct
before the client count grows.

Two concrete reasons for this position rather than a later one:

1. **ADR-0007 states that every new capability requires a release on every client that
   should display it, and calls it real recurring work.** Wear starts that meter. Work that
   touches defaults, packaging and documentation is strictly cheaper with one client than
   with two, and none of it becomes easier by waiting.
2. **The repository currently cannot be used by anyone else at all** — there is no `LICENSE`
   file, so default copyright applies. Every milestone that widens what CueSeek does is
   built on top of something nobody may legally run. That is a one-file fix, and it belongs
   in front of the queue rather than behind a client.

**What did not change.** Everything this record decided. Spike first and throw it away;
then one thin slice end to end; then widen capability by capability; then measure the
abstraction at a third adapter by counting the files changed outside its package, with
Amendment 1's narrower bound on configuration still binding. M4 adds no capability and no
contract change by design, so it does not consume step 3 or pre-empt step 4 — it is
orthogonal work that was never sequenced because it was never anticipated.

**Reading older records.** Documents written before this date use M4 to mean the Wear
milestone: `docs/adr/0013-android-client-architecture.md`, `docs/m2-p6-verification.md` and
`docs/m3-verification.md`. They are **left as written**. Rule 3 of the ADR format is that an
accepted decision is never rewritten, and a verification record is dated evidence rather
than a live plan — the same treatment M2's `P0`–`P6` phase naming received when M3 adopted
the numeric form. This table is how those references decode. Forward-looking documents and
code comments were updated, because a comment describing what a future milestone will do is
wrong rather than historical once the milestone moves.

**Cost.** Wear is the strongest available demonstration that ADR-0005 and ADR-0007 hold —
one contract, two form factors, no server change — and it is now a milestone further away.
Accepted because that demonstration is worth no less in three months, and because a
demonstration nobody can install is worth less than it looks.

### Amendment 3 — 2026-09-01: the measurement at the third adapter, and what it missed

Step 4 asks for a count when a third adapter is added. M4.4 added one — `systemd`, which
observes a unit rather than a service's own API — so the count is taken here.

**The result.** New package `agent/internal/adapters/systemd/`, and outside it:

| File | Change |
| --- | --- |
| `adapters/builtin/builtin.go` | one line in the factory map |
| `domain/health.go` | five reason codes |

**Two files, which is what step 4 named as the passing mark**, and better than Amendment 1's
count in the place that mattered. Specifically:

- **`config.Service` did not grow.** That is the bound Amendment 1 set — "a third
  credential shape must become a per-adapter options map, not a fourth pair of fields" —
  and it held, though it was not really tested: this adapter needs no credentials at all.
  The bound stands, untried, for whichever adapter next needs one.
- **No contract change.** The generator produced no diff. `health`, `control` and `web_ui`
  already existed, and reason codes are strings by design rather than an enum.
- **No client change.** A `systemd` service reaches the phone through capabilities Jellyfin
  and qBittorrent already used, with no Android release and no screen edited — the same
  property M3.4 observed, now with an adapter of a structurally different kind.
- `domain/health.go` grew as Amendment 1 predicted it would: with the *kinds of thing that
  can be wrong*, not with the number of services. A fourth unit-backed adapter adds none of
  these five, because a unit can be missing, stopped, failed, transitioning or unreadable
  regardless of what it runs.

**What this measurement does not settle, stated plainly.** Amendment 1 closed by naming the
genuinely hard question: whether a capability *one adapter has and the others lack* fits
without reshaping the interface. This does not answer it either, and in a way it is a
weaker test than Amendment 1's — `systemd` implements a strict *subset* of what the
existing adapters do. An adapter can always fit by doing less.

**So step 4 is not discharged.** The measurement that matters is an adapter that observes
something new — Plex's sessions, Immich's jobs, a queue that is neither `now_playing` nor
`transfers` — and it stays scheduled for **M6**. This amendment records a real but partial
result rather than allowing a favourable count on an easy case to close the question.

**What did not change.** The decision, and every step in it. Spike, thin slice, widen,
measure at a third adapter — with the measurement now taken once and explicitly held open.
