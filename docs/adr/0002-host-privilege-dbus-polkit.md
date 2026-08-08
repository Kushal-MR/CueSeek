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
