# ADR-0010: Material 3 Expressive with an owned token layer

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

A polished interface is an explicit goal, not a nice-to-have. But the fastest route to a
*good* UI and the route to a *distinctive* one are different, and only one of them reads
as a design contribution.

Stock Material 3 with a seed colour ships quickly and looks like every other Android app.
A fully bespoke design system maximises differentiation and means rebuilding
accessibility, focus handling, state layers, motion curves and Wear-specific behaviours —
almost certainly worse than the platform does, and in direct competition with the
architecture work for the only available time.

There is also a product constraint. CueSeek is an instrument panel, not a content app.
Its primary interaction is a two-second glance answering "is everything fine?", often on
a watch. That places unusual weight on status legibility and almost none on rich media
presentation.

## Decision

Build on Material 3 Expressive, and own a `core:design` module containing CueSeek's own
colour, type, shape and motion tokens plus a component catalogue with Compose previews
and screenshot tests.

Inherit the fundamentals. Own the identity.

## Consequences

- Accessibility, touch targets, state layers, motion physics and Wear parity come from
  the platform and stay correct without maintenance.
- Distinctive without rebuilding primitives; the catalogue is itself a reviewable
  artifact and the basis for screenshot regression tests.
- **The highest-value output is a single status language.** What `healthy`, `degraded`,
  `unreachable` and `unknown` look like — colour, shape, motion, and a redundant
  non-colour encoding — is defined once in tokens and never re-decided. It must hold up
  in dark mode, under dynamic colour, on an always-on 1.4" display at low brightness, and
  for colour-blind users. Most dashboards encode status in green/amber/red alone and fail
  three of those four cases.
- Tokens are shared with Wear; components are not. The same capability is deliberately
  rendered differently per form factor (ADR-0007).
- Cost: dynamic colour and a fixed brand identity pull against each other. Where they
  conflict, status colours win — they carry meaning, and meaning must not be themeable.
- Cost: a component catalogue is real ongoing maintenance, and screenshot tests are
  famously noisy. Worth it here only because status rendering is correctness-critical
  rather than decorative.
