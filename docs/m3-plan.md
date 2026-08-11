# M3 plan — service control, a second adapter, and the host itself

M2 proved the path works. M3 makes CueSeek a control panel rather than a monitor: a second
service, real lifecycle control, the host's own vitals, and the ability to power it down.

Written down because M2's phase plan lived only in a conversation, which made it impossible
to review and easy to drift from.

## Naming

Phases are `M3.1` … `M3.8`, matching `M1.1`–`M1.8`. (M2 used `M2 P0`–`M2 P6`, an earlier
variant of the same idea; that history is merged and public and is left alone.)

## Interaction model, decided before any code

Two rules, applied to every service rather than to Jellyfin specially:

- **The main row body means "use this thing."** If the service advertises `web_ui`, tapping
  it opens that service's own interface in a browser. Nothing is prioritised or hardcoded —
  no native-app preference, no per-service knowledge in the client.
- **The trailing ⋮ menu means "do something to this thing."** It lists whichever actions
  the agent advertises for that service right now, gated by the risk level the agent
  assigns.

A service with no `web_ui` falls back to the detail sheet on body tap. That is a useful
destination, not a disabled state.

The URL is **composed client-side** from the paired host address plus a scheme, port and
path supplied by the agent. Never a whole URL from the server: composing it locally means a
compromised agent cannot hand back `javascript:` or point the browser at a host the user
never paired with, and it means the same configuration works over Tailscale and on the LAN
without the user maintaining two addresses.

## Phases

| Phase | Scope | Depends on |
| --- | --- | --- |
| M3.1 | Service lifecycle: Start, Stop, Restart | — |
| M3.2 | Contract + agent: `web_ui` | — |
| M3.3 | Android: row interaction model and ⋮ menu | M3.1, M3.2 |
| M3.4 | qBittorrent adapter | M3.1, M3.2 |
| M3.5 | Activity capabilities: `transfers`, `now_playing` | M3.4 |
| M3.6 | Host metrics: CPU, memory, storage, thermals | M3.2 |
| M3.7 | Host power actions | M3.1, M3.3 |
| M3.8 | Verification, documentation, ADR closure | all |

Each phase is independently verifiable and separately committed.

---

### M3.1 — Service lifecycle: Start, Stop, Restart

**No contract change.** `Action` is `{id, label, risk, description}`, and actions are data,
so `stop` and `start` are new values rather than new schema. That is the point of ADR-0005,
and it makes the acceptance test unusually strong.

- ADR-0002 **Amendment 1**: the privilege ceiling widens to `start`/`stop`, with the cost
  written down. Recorded *before* the code.
- `deploy/10-cueseek.rules`: add `start` and `stop` to the verb allowlist. The **unit**
  allowlist is unchanged — wider verbs, same targets.
- `host.Backend` / `host.Controller`: `StartUnit`, `StopUnit` beside `RestartUnit`.
- `adapters.UnitControl`: the same three.
- `Controllable.Actions()` becomes state-derived: Start only when the unit is inactive,
  Stop only when active. Derived from `host.UnitState` per poll.
- `stop` is classified **destructive**, routing it to press-and-hold in the client.

**Acceptance**

- Stopping the unit from the host changes the action list the API serves, and the change
  arrives over SSE.
- **The unmodified M2 client renders Start when the service is stopped.** No client rebuild.
  If this holds, capability-driven action discovery is proven empirically rather than
  asserted.
- The agent still refuses a unit outside the allowlist, and polkit refuses it again behind
  that.

**Verify on the host:** `systemctl stop jellyfin` → app offers Start → Start from the app →
`ActiveEnterTimestamp` moves and health returns to healthy.

---

### M3.2 — Contract and agent: `web_ui`

Spec-first, per ADR-0009. Follows the precedent already in the contract, where the `control`
capability declares support and the sibling `actions` array carries the payload.

- `api/openapi.yaml`: a `web_ui` capability id, and `web_ui: {scheme, port, path}` on
  `Service`.
- `config`: a per-service `web_ui` block.
- Adapters advertise the capability when it is configured.
- Regenerate Go and Kotlin; the drift gate proves both sides agree.

**Acceptance:** `GET /v1/services` carries `web_ui` for Jellyfin, and the **unmodified M2
client** renders "Update CueSeek to view this" for it — forward compatibility observed
rather than assumed.

---

### M3.3 — Android: the row interaction model

- Row body → `ACTION_VIEW` with the composed URL; falls back to the detail sheet when the
  service has no `web_ui`.
- Trailing ⋮ → the agent's advertised actions, risk-gated: `safe` fires, `disruptive`
  confirms, `destructive` holds.
- Stop's confirmation states the consequence plainly: the service stays down until it is
  started again or the host reboots.
- Two touch targets in one row, each with its own semantics and no double ripple.

**Tests:** URL composition across tailnet, LAN, custom scheme/port/path; refusal of
malformed values; the existing no-service-id-branching guard still passing.

---

### M3.4 — qBittorrent adapter

New adapter package plus one line in `builtin.go`. This is **ADR-0011's stated
measurement**: how many files change outside the new adapter's own package? The intended
answer is two — that file and the config.

**Acceptance:** qBittorrent appears in the app with **zero client changes**, including its
web UI and its lifecycle menu. Managing torrents happens in qBittorrent's own Web UI, which
is what the `web_ui` capability is for; CueSeek handles the service, not the torrents.

---

### M3.5 — Activity: `transfers` and `now_playing`

Deliberately after M3.4. `adapters/adapter.go` warns that shaping `now_playing` before a
second media server exists bakes Jellyfin's DTOs into a contract Plex and Emby must also
satisfy. Two adapters first, then the shape.

Contract, agent and new client renderers, with golden images.

---

### M3.6 — Host metrics

The HP server's own vitals: **CPU usage, memory usage, storage usage, and thermals where
the hardware exposes them.**

These are not a service capability — they belong to the host. Placement follows the
existing shape rather than inventing one: `SystemInfo` already carries host identity, so
metrics extend the system surface rather than pretending to be a service.

Read from `/proc` and `/sys` directly. No new privilege: this is reading, and the agent
already runs on the machine.

---

### M3.7 — Host power actions

- Enable the polkit power block, **all four actions together** — the rule warns that
  omitting the `*-multiple-sessions` variants is a classic "works alone, fails in practice".
- Wire the dormant `host.power` scope, which has existed since M1 and is connected to
  nothing.
- `destructive` risk, press-and-hold.
- Kept separate from M3.1 on purpose: stopping a service and powering off a machine are
  different orders of consequence and deserve separate review.

---

### M3.8 — Verification, documentation, ADR closure

Real-device end-to-end over Tailscale, as M2 P6 did. README and roadmap updated. ADR
amendments closed. The `After=tailscaled.service` deployment item left open by M2 is
finished here.

## Motion and polish

The design system's rule is **motion means data arrived**, which sits awkwardly with "make
it as expressive as possible" until the distinction is named: *interaction feedback is not
ambient motion.* A menu opening because a finger touched it is a response to input, not a
claim about liveness. What stays banned is decorative loops and shimmer, because on this
screen anything that moves on a timer is a thing that lies about freshness.

So M3's motion budget goes to: the ⋮ menu's enter and exit, correct state layers on two
touch targets in one row, sheet transitions, spring-driven changes when Start replaces Stop,
and — the one place motion genuinely carries data — M3 Expressive's wavy linear progress
indicator for qBittorrent transfers.
