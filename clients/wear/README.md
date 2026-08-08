# CueSeek for Wear OS

> **Placeholder.** Lands in M4, after the phone client and the design system.

## Position

The watch is not a smaller phone app. It answers one question — *is everything fine?* —
in about two seconds, and offers a small number of actions. Everything else belongs on
the phone.

**Standalone, not tethered.** The watch pairs with the agent directly and holds its own
scoped token. It does not proxy through the phone app over the Data Layer. That means it
works when the phone is elsewhere, and it means a lost watch is revoked independently of
the phone (ADR-0006).

**Polls, does not stream.** `GET /v1/services` on wake, not a held SSE connection. A
persistent stream on a watch is a battery problem in exchange for latency nobody
perceives on a glanceable surface (ADR-0004).

**Typically issued `read` and `service.control`, but not `host.power`.** A device worn in
public and unlocked by proximity should not be able to shut down a server. The
restriction is enforced by the agent, so it holds regardless of what the watch UI offers.

## Planned surface

- **Tile** — overall status plus per-service dots. The primary interaction.
- **Complication** — a single status glyph on the watch face.
- **App** — service detail and confirmed actions.

## Reuse

Consumes `core:api` and the design system's status language directly. It does **not**
reuse the phone's composables: the same capability is deliberately rendered differently
on each form factor, which is precisely what a client-side capability registry buys and
what a server-driven layout could never do well (ADR-0007).

The status language — what `healthy` / `degraded` / `unreachable` / `unknown` look like —
must survive an always-on display at low brightness on a 1.4" screen, in addition to
dark mode, dynamic colour, and colour-blind users. That constraint shapes the tokens for
every client, and the watch is where it gets tested honestly
([ADR-0010](../../docs/adr/0010-design-system-m3-expressive.md)).
