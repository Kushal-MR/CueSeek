# Architecture Decision Records

An ADR captures **one decision**, the context that forced it, and the consequences —
including the ones we did not want. Together they explain why CueSeek is shaped the way
it is, which is the thing a directory listing can never tell you.

## Why these exist

Code shows what was decided. It never shows what was rejected, or what price was
knowingly paid. Six months from now the reasoning behind ADR-0002 will not be
reconstructable from `internal/host/`, and without it someone — probably the author —
will "simplify" the polkit rule into a sudoers entry and reintroduce a problem that was
already solved.

They are also the honest answer to "is this over-engineered?". Every record below states
its cost. A decision with no listed downside has not been thought about hard enough.

## Format

[Michael Nygard's format](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions):
Context, Decision, Consequences. Short and readable in a couple of minutes.

**Status** is one of `Proposed`, `Accepted`, `Superseded by ADR-XXXX`, `Deprecated`.

## Rules

1. **One decision per record.** If it needs "and", it is two ADRs.
2. **Write it when the decision is made**, not afterwards. A retroactive ADR documents;
   a contemporaneous one reasons. The difference is visible.
3. **An accepted decision is never rewritten.** Changing your mind means writing a new
   record that supersedes the old one. The record of a decision that turned out badly is
   more valuable than its absence — deleting it hides the very thing worth learning from.
4. **Amend when the decision stands but a constraint changed.** A tooling limitation or a
   pinned external version gets a dated entry in a `## Amendments` section at the end of
   the record, stating what did *not* change. Test: would you decide differently today?
   If yes, supersede. If not, amend — see ADR-0004's Amendment 1 for the shape.
5. **State the cost.** Every decision closed a door. Say which one.
6. **Number sequentially, never reuse a number.**

## Index

| # | Title | Status |
| --- | --- | --- |
| [0000](0000-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0001](0001-vpn-only-remote-access.md) | VPN-only remote access | Accepted |
| [0002](0002-host-privilege-dbus-polkit.md) | Unprivileged agent via D-Bus and polkit | Accepted · amended ×2 (latest 2026-08-31) |
| [0003](0003-agent-runtime-go.md) | Go for the agent runtime | Accepted · amended ×1 (latest 2026-08-13) |
| [0004](0004-contract-openapi-sse.md) | Spec-first OpenAPI with SSE | Accepted · amended ×4 (latest 2026-08-31) |
| [0005](0005-capability-based-adapters.md) | Capability-based adapters | Accepted |
| [0006](0006-device-pairing-scoped-tokens.md) | Device pairing with scoped tokens | Accepted · amended ×3 (latest 2026-08-09) |
| [0007](0007-client-capability-registry.md) | Client-side capability registry | Accepted |
| [0008](0008-multi-host-and-computed-health.md) | Multi-host model and agent-computed health | Accepted · amended ×1 (latest 2026-09-01) |
| [0009](0009-monorepo-contract-drift-gate.md) | Monorepo with a contract-drift gate | Accepted |
| [0010](0010-design-system-m3-expressive.md) | M3 Expressive with an owned token layer | Accepted |
| [0011](0011-sequencing-spike-then-slice.md) | De-risking spike, then thin slice | Accepted · amended ×4 (latest 2026-09-05) |
| [0012](0012-alerting-vs-vpn-only-access.md) | Alerting reopens the access model | Proposed |
| [0013](0013-android-client-architecture.md) | Four shared `core` modules; features are packages | Accepted |
