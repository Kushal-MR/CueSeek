# ADR-0002: Unprivileged agent, host control via D-Bus and polkit

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

Two of the three MVP capabilities are host-level. No Jellyfin or qBittorrent API can
restart its own process, reboot the machine, or report CPU, RAM, disk or thermals. The
agent must therefore be able to perform privileged operations, and the question is how it
obtains that privilege.

Options considered: run the agent as root; run it unprivileged with a narrow `sudoers`
allowlist; split it into an unprivileged process and a small root helper over a unix
socket; or run it unprivileged and ask systemd and logind over D-Bus, authorised by
polkit.

A detail settled it. The dashboard needs unit *state*, not just unit control. Over D-Bus
that is `ActiveState`, `SubState` and `ActiveEnterTimestamp` as typed values from the
same connection used to restart things. The sudoers route needs two mechanisms — shelling
out to control, and parsing `systemctl show` text to observe — and the parsing has to keep
working across systemd versions.

## Decision

`cueseekd` runs as its own unprivileged system user and never elevates. It calls
`org.freedesktop.systemd1` (`RestartUnit`, unit property reads) and
`org.freedesktop.login1` (`Reboot`, `PowerOff`) over D-Bus. A polkit rule shipped in
`deploy/` grants exactly those actions to that user, scoped to an allowlist of units.

No sudo. No shell. No `systemctl` invocation, ever.

## Consequences

- Less code, not more: one mechanism for both observation and control.
- No command-injection class. Service names never become part of a command string —
  which matters once managed units are user-configurable.
- Authorisation is legible to the operator. One short rule file states the complete
  ceiling of what CueSeek can do, auditable without reading any Go.
- The allowlist is enforced twice, in the agent and in the polkit rule. Cheap defence in
  depth when one half is a config file.
- Cost: **couples the host layer to systemd.** Mitigated by putting everything behind a
  `HostController` interface so an OpenRC, BSD or macOS backend is a swap rather than a
  rewrite — but that mitigation is untested until a second backend exists.
- Cost: installation is more involved than dropping in a binary. A polkit rule must be
  placed correctly, and getting it wrong fails in ways that are unintuitive to debug.
- **This is the riskiest assumption in the architecture and is validated first**, by a
  throwaway spike, before anything depends on it. See ADR-0011.

## Amendments

### Amendment 1 — 2026-08-11: the ceiling widens to `start` and `stop`

**What changed.** This ADR's decision named `RestartUnit` and unit property reads. The
agent may now also call `StartUnit` and `StopUnit`, and the shipped polkit rule grants the
`start` and `stop` verbs alongside `restart` on the same unit allowlist.

**Why.** M3 turns CueSeek from a monitoring app into a service control panel. Restart alone
cannot express the two things an operator actually wants when something is misbehaving:
take it down, and bring it back. Without `stop`, the only way to stop a service from a
phone is to not have that capability at all.

**What this costs, stated plainly.** The original rule carried this comment:

> Note what is absent: start, stop, enable, disable, mask. The agent cannot leave a
> service switched off.

That sentence is no longer true, and the reason it was written still holds: a `restart`
grant is self-healing, because whatever it does, the service is running afterwards. A
`stop` grant is not. A stolen `service.control` token, or a bug in the agent, can now leave
Jellyfin down until somebody notices.

Three things bound that risk, and none of them is new machinery:

1. **The unit allowlist is unchanged.** `start` and `stop` are grantable only for units
   already named in both the config and the rule. Widening the *verbs* does not widen the
   *targets*.
2. **`enable`, `disable` and `mask` remain absent.** A stopped unit is still enabled, so it
   returns on the next boot. The damage is bounded by one reboot rather than being
   persistent, and the client's confirmation says so.
3. **`stop` is classified `destructive`, not `disruptive`** (ADR-0005's risk vocabulary),
   which routes it to the client's press-and-hold confirmation rather than a single tap.
   The risk level is carried on the wire, so this holds for every client including ones
   written later.

**What was considered and rejected.** A separate `service.lifecycle` scope, distinct from
`service.control`, would let a device restart without being able to stop. It was rejected
for now because it doubles the scope vocabulary to express a distinction no user has asked
for, and scopes are hard to change once devices are paired with them. If a real need
appears — a shared device, a kiosk — this is the shape it should take, and the tokens are
per-device precisely so that becomes possible without a migration.

**Consequence for `Actions()`.** `adapters.Controllable.Actions()` was documented as
"Static: this is the menu, not the kitchen." It is no longer static: Start is offered only
when the unit is inactive, and Stop only when it is active. The list is derived from
`host.UnitState` on each poll and shipped in `Service.actions`, so clients continue to
render what they are given and gain no new branching. This costs nothing on the wire —
actions were always data.
