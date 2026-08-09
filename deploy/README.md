# Deployment

Production artifacts for the CueSeek agent, validated against the M0 spike results
([`docs/m0-findings.md`](../docs/m0-findings.md)).

```
deploy/
├─ cueseekd.service       systemd unit, hardened, runs as an unprivileged user
├─ 10-cueseek.rules       polkit rule — the authorisation boundary
├─ config.example.yaml    annotated reference configuration
└─ install.sh             user, directories, binary, unit, polkit rule
```

## Installing

```bash
GOOS=linux GOARCH=amd64 go build -o cueseekd ./cmd/cueseekd   # from agent/
sudo ./install.sh --binary /path/to/cueseekd
```

The installer refuses rather than half-installing: it checks for systemd, for polkit
≥ 0.106, and that the binary is a Linux ELF. It never overwrites an existing
`/etc/cueseek/config.yaml`, never touches the database, and does not start the service —
a fresh install has no API key, so starting it could only fail. It prints the ordered
next steps instead.

Re-run it to upgrade the binary. `--uninstall` removes the unit, rule and binary while
keeping your configuration and paired devices; `--uninstall --purge` removes those too.

## The polkit rule is the security boundary

`10-cueseek.rules` is the most important file in this directory and one of the most
important in the repository. It is the complete statement of what CueSeek is permitted to
do to the machine:

- `org.freedesktop.systemd1.manage-units`, restricted to an allowlist of unit names **and**
  to the verbs `restart`, `try-restart` and `reload-or-restart`. Notably absent: `start`,
  `stop`, `enable`, `disable`, `mask` — the agent cannot leave a service switched off.

Granted to the `cueseek` user and nobody else. Everything else returns `NOT_HANDLED`, so
the rule only ever adds permissions and never revokes ones another rule granted.

**Host power actions are shipped commented out.** M0 proved `login1.reboot` and
`login1.power-off` work for a session-less `cueseek` — it genuinely rebooted the target
host. They are disabled anyway because the agent has no code path that calls logind: the
`host.power` scope is wired to nothing until M3. Granting a service permission to reboot a
machine it cannot ask to reboot is a standing risk with no benefit. Uncomment all four
actions together when M3 lands, including the `*-multiple-sessions` variants — omitting
those is a classic cause of "works alone, fails in practice"
([ADR-0002](../docs/adr/0002-host-privilege-dbus-polkit.md)).

### polkit 0.106 is a hard requirement

Below that version, rules are `.pkla` files that cannot inspect action details. The
per-unit allowlist would silently degrade to *"cueseek may restart any unit"* — far more
than this file claims to grant. `install.sh` refuses on such a host rather than installing
a rule that lies. M0 identified this as architecture-breaking; the target host runs
polkit 124.

### The allowlist is deliberately duplicated

The same unit names appear in `10-cueseek.rules` and in `config.yaml`, and both are
enforced. The agent refuses an unlisted unit before D-Bus is touched; polkit refuses it
again behind that. **Do not generate one from the other** — that would collapse two
independent checks into one and remove the defence in depth ADR-0002 asks for. If they
disagree, the narrower wins, which is the safe direction.

## Service hardening

The unit runs as a dedicated unprivileged `cueseek` user — never root — with an empty
capability bounding set. The agent needs exactly three things: the D-Bus system socket,
outbound TCP to services on this host, and its state directory. Everything else is
removed: `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `PrivateDevices`,
`RestrictAddressFamilies` to `AF_UNIX AF_INET AF_INET6`, `SystemCallFilter=@system-service`,
`MemoryDenyWriteExecute`, and the `Protect*`/`Restrict*` family.

`StateDirectory=cueseek` gives `/var/lib/cueseek`, created and owned by systemd. It is the
only path the agent can write to.

If the service fails to start after a distribution upgrade, relax `SystemCallFilter` first,
then `MemoryDenyWriteExecute` — and report it rather than deleting the whole block.

## Binding

The agent binds to a specific address, never `0.0.0.0`. Configuration validation refuses a
wildcard bind unless `allow_unrestricted: true` is set deliberately, so widening is visible
in a config diff rather than something that happens by accident.

CueSeek assumes reachability is already handled by Tailscale, WireGuard or the LAN, and it
is not built to be exposed to the internet
([ADR-0001](../docs/adr/0001-vpn-only-remote-access.md)). Binding broadly and relying on
"it's only on my LAN" is how self-hosted tools end up indexed by scanners.

If you bind to a tailnet address, that interface does not exist at early boot. Uncomment
the `After=tailscaled.service` lines in the unit, or rely on `Restart=on-failure` to
recover.

## Diagnosing a refused restart

Three different problems look identical through the API. This tells them apart:

```bash
sudo -u cueseek /usr/local/bin/cueseekd host restart -config /etc/cueseek/config.yaml jellyfin.service
```

- *"unit is not managed by this agent"* — the unit is missing from `config.yaml`
- *"not authorized … Interactive authentication required"* — polkit refused; the rule is
  missing, or its allowlist disagrees with the config
- *"unit not found"* — the name is wrong; check `systemctl list-units --type=service`

## Packaging

A `.deb` is not built yet. `install.sh` is the supported path.
