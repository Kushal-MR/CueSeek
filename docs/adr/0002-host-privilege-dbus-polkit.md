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

### Amendment 2 — 2026-08-31: the ceiling widens to reboot and power off

**What changed.** The agent may now call `Reboot` and `PowerOff` on
`org.freedesktop.login1`, and the shipped polkit rule grants the four corresponding actions
that were written and commented out when this ADR was first accepted. The dormant
`host.power` scope, which has existed since M1 and guarded nothing, is now the thing that
authorises them.

**Why now, and not earlier.** The rule file's own comment states the reason it was disabled:

> granting a service permission to reboot a machine it cannot ask to reboot is a standing
> risk with no benefit

That sentence stops being true the moment there is a code path, and not before. M0 proved
the mechanism works — an unprivileged, session-less `cueseek` genuinely rebooted the target
host — so nothing here is being discovered, only connected.

**All four actions or none.** `reboot`, `power-off`, and the `-multiple-sessions` variant of
each. logind consults the second form when another user is logged in, so a rule granting
only the first works perfectly for the operator testing it alone and fails the day somebody
is at the console. That is a failure mode which appears late, on someone else's machine, and
looks like a permissions bug rather than a missing line.

**What this costs, stated plainly.** Every previous grant was bounded by "the damage is one
reboot". This one is the reboot. Two properties that held for service control do not hold
here:

1. **There is no allowlist to bound it.** A unit grant is scoped to named units; a power
   grant has no target narrower than the machine.
2. **Power off is not self-healing, and not reversible from the phone.** A reboot returns on
   its own. A power-off requires physical access to undo, so a stolen `host.power` token can
   take the machine off the network until somebody walks to it.

Four things bound that, and only the last is new:

1. **`host.power` is a separate scope**, granted per device and never by default.
   `cueseekd pair` defaults to `read,service.control`, and printed a warning about this
   grant before anything could use it.
2. **Scope is enforced in the agent**, not in the client. A token without `host.power` is
   refused by the API regardless of what UI produced the request.
3. **`destructive` risk**, routing both actions to the client's press-and-hold confirmation
   rather than a tap. Carried on the wire, so it holds for clients written later.
4. **The confirmation names what is currently happening on the machine.** The agent already
   knows there is a transcode running or a torrent mid-download (M3.5), and a console that
   holds that information and does not mention it while asking about a power-off is wasting
   the one thing it is uniquely placed to say.

**What was considered and rejected.**

*A fourth risk level above `destructive`.* Powering off is genuinely more consequential than
stopping a service — a stop is undone from the same screen, a power-off is undone by walking
across the room. But the risk vocabulary is public API shared with clients that already
exist, and adding a level to express one distinction would force every client to handle a
value it has no interaction for. The consequence is stated in the confirmation copy instead,
which is where a user actually reads it.

*Refusing the action while something is in flight.* Rejected. The operator owns the machine
and may have very good reasons to power it off mid-transcode. Blocking would make the tool
argue with the person it exists to serve; naming the consequence respects them and still
prevents the accident.

*`suspend` and `hibernate`.* Absent, and left absent. A suspended host is unreachable over
the tailnet and cannot be woken by the thing that suspended it, which turns a remote console
into a way to lock yourself out.
