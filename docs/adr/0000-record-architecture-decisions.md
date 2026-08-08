# ADR-0000: Record architecture decisions

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

CueSeek is a long-lived, single-maintainer project spanning a Go agent, an OpenAPI
contract, and several client platforms that will be built months apart. Decisions made
during the initial design phase — the access model, the privilege model, the adapter
abstraction — constrain everything built afterwards, but leave no trace in the code
explaining *why* they were made or what was rejected.

The specific failure mode this guards against is not forgetting a decision. It is
remembering the decision but not its reason, and then "simplifying" it away. A future
maintainer looking at a polkit rule and a D-Bus client sees complexity, and the obvious
cleanup is a sudoers entry and a shell command — reintroducing a problem that was
already solved and paid for.

## Decision

Record every significant architectural decision as an ADR in `docs/adr/`, using
[Michael Nygard's format](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions):
Context, Decision, Consequences.

Records are written **when the decision is made**, are immutable once accepted, and are
superseded by new records rather than edited. Every record states its cost.

## Consequences

- Rationale survives in the repository, next to the code it constrains.
- Reversing a decision becomes deliberate: it requires writing a record explaining what
  changed, rather than a quiet refactor.
- Records of decisions that turned out badly are kept. They are the most instructive
  ones, and deleting them hides exactly what should be learned.
- Cost: discipline. An ADR written six months late documents rather than reasons, and
  the difference is obvious to anyone reading it. If the habit lapses, the directory
  becomes a museum rather than a working record.
