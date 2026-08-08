# ADR-0009: Monorepo with a contract-drift CI gate

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

ADR-0004 made a hand-authored spec the single source of truth for an agent and several
clients in different languages. That claim is worth nothing unless something enforces it.
"Spec-first" maintained by discipline degrades the first time a handler is faster to edit
than a YAML file.

Repository structure decides how expensive it is to keep them aligned. A polyrepo split —
a versioned contract package consumed by an agent repo and a client repo — has clean
boundaries and matches how a multi-team organisation ships. For a single maintainer in
an exploratory phase, it makes every cross-cutting change a three-repository dance with a
publish step in the middle, and most changes for the next six months will touch the spec,
a handler and a screen together.

## Decision

A single repository. `api/`, `agent/`, `clients/android/`, `clients/wear/`, `deploy/` and
`docs/` side by side, with generated code committed.

CI regenerates from `api/openapi.yaml` and **fails if the result differs from the
committed tree**.

## Consequences

- A spec change, its handler and its screen land in one atomic, reviewable commit.
- The drift gate makes spec-first an enforced property rather than an intention. Without
  it, ADR-0003's acceptance of zero code sharing between agent and clients would not have
  been safe.
- Committing generated code keeps the tree buildable without a codegen toolchain and
  makes contract changes visible in review — the diff shows what the API change actually
  did to every client.
- **Generated code must be wrapped, not consumed directly.** Generators leak their
  idioms: nullable everything, `Any?` for `oneOf`, exceptions for error states. A thin
  hand-written layer maps to sealed domain types and a `Result`-style error model, so
  swapping generators later is a swap rather than a rewrite.
- One clone, one README, one CI badge — the architecture is legible in thirty seconds to
  anyone opening the repository.
- Cost: mixed toolchains in one CI pipeline (Go, Gradle, spec linting), and a full
  checkout larger than any one contributor needs.
- Cost: no independent versioning. Agent and clients release from one history, which will
  become awkward if their cadences diverge sharply. Extracting a versioned contract
  package remains possible later, and this ADR should be superseded if that happens.
