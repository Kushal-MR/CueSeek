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

From a release, which is the supported path and needs no clone and no Go toolchain:

```bash
# from https://github.com/Kushal-MR/CueSeek/releases
sha256sum -c SHA256SUMS
tar xzf cueseek-agent_*_linux_amd64.tar.gz
cd cueseek-agent_*_linux_amd64
sudo ./install.sh
```

The tarball is self-contained — the agent, the unit, the polkit rule, the annotated example
configuration and the installer, side by side. `install.sh` finds the binary next to itself
and verifies it against the `cueseekd.sha256` shipped beside it.

From source, for development:

```bash
./scripts/release-agent.sh          # builds dist/, the same way CI does
sudo ./dist/cueseek-agent_*/install.sh
```

`linux/amd64` only. There is no `arm64` build: it cannot be tested here, and this project
does not ship an architecture it has not run.

The installer refuses rather than half-installing: it checks for systemd, for polkit
≥ 0.106, and that the binary is a Linux ELF. It prints the ordered next steps instead of
starting the service, because the operator still has to choose a bind address.

**It never overwrites what you have edited.** `/etc/cueseek/config.yaml` is left alone if
it exists, the database is never touched, and a `10-cueseek.rules` that differs from the
shipped one is preserved with the new version written beside it as `.rules.new`. That last
protection arrived in M4.3; before it, re-running the installer to upgrade the binary
silently reverted the allowlist to the shipped one, and the symptom — a restart that
worked yesterday being refused today — pointed nowhere near the cause.

Re-run it to upgrade the binary. `--uninstall` removes the unit, rule and binary while
keeping your configuration and paired devices; `--uninstall --purge` removes those too.

**A fresh install manages no services.** The shipped `config.example.yaml` activates
nothing, so the agent starts, reports the machine's own vitals — which need no
configuration and no privilege — and shows an empty service list. Worked examples for
every supported type sit commented at the bottom of that file.

## The polkit rule is the security boundary

`10-cueseek.rules` is the most important file in this directory and one of the most
important in the repository. It is the complete statement of what CueSeek is permitted to
do to the machine:

- `org.freedesktop.systemd1.manage-units`, restricted to an allowlist of unit names **and**
  to the verbs `restart`, `try-restart`, `reload-or-restart`, `start` and `stop`. Notably
  absent: `enable`, `disable`, `mask` — so a stopped unit stays enabled and returns on the
  next boot. A stop costs one reboot rather than persisting.

  `start` and `stop` were deliberately excluded until M3.1, on the grounds that a restart
  is self-healing and a stop is not. Adding them widened the ceiling and is recorded, with
  what bounds it, in [ADR-0002 Amendment 1](../docs/adr/0002-host-privilege-dbus-polkit.md).

Granted to the `cueseek` user and nobody else. Everything else returns `NOT_HANDLED`, so
the rule only ever adds permissions and never revokes ones another rule granted.

**Host power actions are enabled as of M3.7.** All four together — `reboot`, `power-off`
and the `*-multiple-sessions` variant of each. logind consults the second form when another
user is logged in, so granting only the plain ones works perfectly for whoever tests it
alone and fails the first time somebody is sitting at the console.

They were shipped commented out until there was a code path calling logind, because
granting a service permission to reboot a machine it cannot ask to reboot is a standing
risk with no benefit. That code path is now the `host.power` scope, which `cueseekd pair`
never grants by default — **a device paired before M3.7 cannot power the machine off and
must be paired again to gain it**, which is the scope model working rather than a bug.

This is the one grant in the rule with no allowlist behind it: a unit grant is bounded to
named units, and there is no target narrower than the machine. Note what stays absent —
`suspend` and `hibernate`. A suspended host is unreachable over the tailnet and cannot be
woken by the thing that suspended it, which turns a remote console into a way to lock
yourself out ([ADR-0002 Amendment 2](../docs/adr/0002-host-privilege-dbus-polkit.md)).

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

### A service may have no unit at all

`unit` is optional. A service configured without one is **watched but not controlled**: it
reports health and offers its web interface, and advertises no lifecycle actions, because
there is nothing to act on. It needs no entry in the polkit rule for the same reason —
there is nothing to authorise.

This is the honest shape for something running from a container image rather than a
package. It is not container support: nothing here starts, stops or inspects a container.
What it avoids is refusing a configuration the agent would have served correctly, which is
what requiring a unit did until M4.3 — both adapters had always handled its absence.

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

### Binding to a VPN address at boot

The agent handles this itself. On a cold boot, `tailscaled` creates the interface
immediately but assigns the address only after authenticating, tens of seconds later — so
`bind()` fails with `EADDRNOTAVAIL`. Rather than treating a normal condition as a crash,
`Listen` waits up to 90 seconds for the address to appear, retrying every 2 seconds, and
logs `bind address is not present yet, waiting for it`. A port conflict or permission
error still fails immediately, because those never resolve on their own.

No unit ordering is required. `After=tailscaled.service` is available commented out and
only makes the boot sequence tidier.

**Do not add `tailscale0.device` to `After=` or `Wants=`.** systemd resolves any `*.device`
unit name back to a device path, reads it as a device node at `/tailscale0`, and blocks
for the full `DefaultTimeoutStartSec` waiting for something that never appears — measured
at 89 seconds on the target host. It appears to fix the race because a 90-second delay
guarantees the VPN is up by then. Ordering on the real unit
(`sys-subsystem-net-devices-tailscale0.device`) is also insufficient: it signals that the
interface exists, not that the address has been assigned.

## Checking an install before anything goes wrong

```bash
sudo /usr/local/bin/cueseekd check -config /etc/cueseek/config.yaml
```

**`check` needs a binary that has it** — M4.5 or later. Before M4.5b an unrecognised
subcommand was silently ignored and the agent started instead, so running `check` against
an older binary loads `/etc/cueseek/config.yaml`, ignores every flag after the word, and
tries to bind an address the running agent already holds. Confirm what is installed with
`cueseekd -version` before concluding anything from the output.

**As root, not as `cueseek`.** `/etc/polkit-1/rules.d` is `0750 root:polkitd` on Debian and
Ubuntu, and the `cueseek` user belongs to no group but its own — so it can read the config
and not the rule, and the allowlist comparison is the whole point. Run as any other user
the check says so and carries on, rather than reporting a rule it cannot see as broken.

Resolves every configured unit against systemd, reads the installed polkit rule and
compares the two allowlists **in both directions**, confirms the bind address exists on an
interface, tests that the state directory is writable, and asks each adapter for its
health. It changes nothing, and exits non-zero only when something the configuration asks
for cannot happen — a deliberately stopped service is a warning, not a failure.

The allowlist comparison is the reason it exists. The two copies are deliberately not
generated from each other, and until now nothing checked that they agree; when they do not,
the agent starts perfectly, the dashboard looks perfectly normal, and the first restart
fails with an authorisation error that reads like a broken install.

It also catches the partial power grant that ADR-0002 Amendment 2 warns about: a rule
granting `reboot` and `power-off` without their `-multiple-sessions` variants works
perfectly for whoever tests it alone and fails the first time somebody is at the console.

**It reads the rule textually rather than evaluating it.** If it cannot find the allowlist
it says so instead of guessing, because reporting a unit as granted when it is not would be
worse than saying nothing.

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

A tar.gz built by [`scripts/release-agent.sh`](../scripts/release-agent.sh) and published
by [`.github/workflows/release.yml`](../.github/workflows/release.yml) on a `v*` tag.
`install.sh` is the supported install path; no `.deb` or `.rpm` is built, because two
packaging formats for one installer is maintenance with no reader.

Three properties are worth knowing about the artefact:

**Static.** `CGO_ENABLED=0`, which the pure-Go `modernc.org/sqlite` exists to make
possible. One binary runs on any x86-64 Linux with systemd, rather than against whichever
glibc built it.

**Reproducible.** `-trimpath`, an mtime taken from the commit rather than the clock, fixed
ownership, sorted entries, and `gzip -n`. Building the same tag twice gives byte-identical
output — verified, not assumed.

**Modes are stated, not inherited.** The archive's permissions come from the script rather
than from the filesystem it was built on. The first version did inherit them and shipped a
non-executable binary, because this project is developed on Windows where the executable
bit is inferred from a shebang and a Linux ELF has none.

Every release also carries a **build provenance attestation**, so an operator can confirm
the tarball came from this repository's workflow rather than from the release page alone:

```bash
gh attestation verify cueseek-agent_*_linux_amd64.tar.gz --repo Kushal-MR/CueSeek
```
