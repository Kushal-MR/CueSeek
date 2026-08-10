# ADR-0013: Four shared `core` modules; feature areas are packages

- **Status:** Accepted
- **Date:** 2026-08-09

## Context

`clients/android/README.md` planned seven Gradle modules — four under `core/` and three
under `feature/` — before any of them existed. M2 is the first milestone that has to build
them, so the plan now has to be either executed or corrected.

Module boundaries are unusually expensive to get wrong in this project. Two clients will
eventually assemble the same data layer (ADR-0007), and whatever `:core:model` and
`:core:design` end up containing is what Wear inherits in M4. But module count is not free
either: every module beyond the first adds a build file, and the fourth or fifth is
normally where a project acquires an included build of convention plugins to stop the
duplication — a second build system to understand before reading any application code.

The question is therefore not "how many modules is idiomatic" but **which boundaries
something will actually cross**.

## Decision

Four modules under `core/`, and feature areas as packages inside `:app`.

```
:core:model    Kotlin/JVM   domain vocabulary. Depends on nothing.
:core:api      Kotlin/JVM   generated wire types, hand-written transport, error mapping.
:core:data     Android      repositories, credential storage, live-state machine.
:core:design   Android      tokens and component catalogue.
:app           Android      assembly, navigation, and the pairing/dashboard packages.
```

**`:core:model` and `:core:api` are plain Kotlin/JVM modules, not Android libraries.** The
domain vocabulary the UI speaks *cannot* reference the Android framework — the compiler
enforces what would otherwise be a review comment — and it mirrors `agent/internal/domain`,
which depends on nothing for the same reason. The practical dividend is in `:core:api`:
its MockWebServer tests run on the JVM in milliseconds, with no device and no Robolectric,
which is what makes it reasonable to test every documented status code rather than the
happy path.

**Feature areas stay packages.** `pairing` and `dashboard` are directories in `:app` until
something crosses their boundary. `core/` is different in kind: M4 shares `:core:model` and
`:core:design` with Wear, so those boundaries are real today, before the second consumer
exists, and retrofitting them would mean moving every file that references them.

Four build files are duplicated rather than factored into a convention plugin. At five
modules that duplication is smaller than the machinery that would remove it; the trigger to
revisit is feature-module promotion in M3, not a line count.

## Consequences

- The rule "no ViewModel sees a generated type" (ADR-0009) is enforced by module
  visibility rather than by discipline: generated types are internal to `:core:api`.
- `:core:model` cannot accumulate Android dependencies, so Wear inherits it in M4 without
  an audit — the compiler already did the audit, continuously.
- Fast JVM tests for the transport and error mapping, which is where M2's correctness
  actually lives (`docs/m2-android-api.md` §4 and §8).
- The dependency graph is legible from `settings.gradle.kts` alone, without opening a
  `build-logic` project first.
- Cost: **package boundaries inside `:app` are not enforced.** Nothing stops `dashboard`
  from reaching into `pairing`'s internals, and nothing will notice if it does. This is a
  real hole and the honest answer to it is promotion in M3, not vigilance.
- Cost: four near-identical `build.gradle.kts` files. Each new core module copies one, and
  a toolchain bump edits five files instead of one. Acceptable at this size; the moment it
  is not, the fix is well-known and mechanical.
- Cost: the mixed JVM/Android module split means two slightly different build file shapes
  in one tree, which reads as inconsistency until you know why. Hence this record.
