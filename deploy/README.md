# Deployment

> **Placeholder.** Real artifacts land alongside the agent in M2, validated by the M0
> spike.

## Planned contents

```
deploy/
├─ cueseekd.service              systemd unit, hardened
├─ 10-cueseek.rules              polkit rule — the authorisation boundary
├─ config.example.yaml           annotated reference configuration
├─ install.sh                    user, directories, unit, polkit rule
└─ packaging/                    .deb, later others
```

## The polkit rule is the security boundary

`10-cueseek.rules` is the most important file in this directory and one of the most
important in the repository. It is the complete statement of what CueSeek is permitted
to do to the machine:

- `org.freedesktop.systemd1.manage-units` — restricted to an allowlist of unit names
- `org.freedesktop.login1.power-off`
- `org.freedesktop.login1.reboot`

Granted to the `cueseek` user, and nothing else. An operator should be able to read this
file and know the ceiling without reading any Go. Keep it short, keep it commented, and
never widen it for convenience — if the agent needs a new privilege, that is a decision
worth making explicitly ([ADR-0002](../docs/adr/0002-host-privilege-dbus-polkit.md)).

## Service hardening

The unit runs as a dedicated unprivileged `cueseek` user — never root — with systemd's
sandboxing applied: `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`,
`PrivateTmp`, and a `StateDirectory` for the SQLite database. The agent needs D-Bus and
outbound HTTP to localhost services; it should be able to do very little else.

## Binding

The agent binds to the private-network interface, never `0.0.0.0`. CueSeek assumes
network reachability is already handled by Tailscale, WireGuard or the LAN, and it is not
built to be exposed to the internet ([ADR-0001](../docs/adr/0001-vpn-only-remote-access.md)).

Binding broadly and relying on "it's only on my LAN" is how self-hosted tools end up
indexed by scanners. The default must be safe, and the config should make widening it a
conscious act.
