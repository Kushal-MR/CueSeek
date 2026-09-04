# Install

Check [requirements](requirements.md) first. Everything here assumes a Linux host with
systemd and polkit ≥ 0.106 on x86-64.

Fifteen minutes, most of it deciding which services you want.

## 1. Download and verify

From the [releases page](https://github.com/Kushal-MR/CueSeek/releases):

```bash
sha256sum -c SHA256SUMS
```

Optionally, confirm the archive came from this repository's workflow rather than merely
from a page that says so:

```bash
gh attestation verify cueseek-agent_*_linux_amd64.tar.gz --repo Kushal-MR/CueSeek
```

That needs GitHub CLI **2.49 or later**. Ubuntu 24.04 ships 2.45, which has no `attestation`
command at all — if you see "unknown command", run it from a machine with a newer `gh`
rather than concluding the artefact is unverifiable.

## 2. Install

```bash
tar xzf cueseek-agent_*_linux_amd64.tar.gz
cd cueseek-agent_*_linux_amd64
sudo ./install.sh
```

The tarball is self-contained — the agent, the systemd unit, the polkit rule, the annotated
example configuration and the installer, side by side. Nothing to clone.

What it does: creates an unprivileged `cueseek` system user with no home and no shell,
installs the binary to `/usr/local/bin/cueseekd`, the unit, the polkit rule, and
`/etc/cueseek/config.yaml`, and creates `/var/lib/cueseek`.

What it refuses to do: install on a host without systemd, on polkit below 0.106, or with a
binary that is not a Linux executable. It **does not start the service** — you still have to
choose a bind address.

**It never overwrites what you have edited.** An existing `config.yaml` is left alone. The
database is never touched. A `10-cueseek.rules` that differs from the shipped one is
preserved, and the new version is written beside it as `.rules.new` for you to diff.

## 3. Point it at your phone

```bash
sudo nano /etc/cueseek/config.yaml
```

Set `bind.address` to an address your phone can reach. On Tailscale:

```bash
tailscale ip -4
```

```yaml
bind:
  address: "100.64.0.5:7777"    # yours will differ
```

The default is loopback, which is safe and unreachable from anywhere else — a deliberate
choice for somebody who installs this and skips the documentation.

No unit ordering is needed for a VPN address. On a cold boot the interface exists before it
has an address, so the agent retries `bind()` for up to 90 seconds and says so in the
journal. `After=tailscaled.service` was tried, measured, and made it worse.

## 4. Start it

```bash
sudo systemctl enable --now cueseekd.service
systemctl status cueseekd.service
```

**As shipped it manages no services, and that is a working install.** The agent reports the
machine's own CPU, memory, storage and temperatures with no further configuration and no
privilege at all, and shows an empty service list until you add some.

## 5. Pair your phone

See [pairing](pairing.md). Briefly:

```bash
sudo -u cueseek cueseekd pair
```

Install `cueseek_*.apk` from the same release page, then type the address, port and code.

At this point you have a working dashboard of the machine itself.

## 6. Add your services

```bash
sudo nano /etc/cueseek/config.yaml
```

Worked examples for every supported type sit commented at the bottom of that file.
Uncomment one and edit it. [Configuration](configure.md) is the map of what those options
mean; [Jellyfin](services/jellyfin.md) and [qBittorrent](services/qbittorrent.md) have their
own walkthroughs, and everything else on the machine is `type: systemd`.

**Use the exact unit name.** It often differs from what the software calls itself — on the
development host, qBittorrent's unit is `qbittorrent.service` even though the unit
describes itself as "qBittorrent-nox":

```bash
systemctl list-units --type=service | grep -i <name>
```

## 7. Allow those units in the polkit rule

```bash
sudo nano /etc/polkit-1/rules.d/10-cueseek.rules
```

Add each unit to `allowedUnits`. **The names there and in the config must agree**, and both
are enforced — the agent refuses an unlisted unit before D-Bus is touched, and polkit
refuses it again behind that. The duplication is deliberate defence in depth
([ADR-0002](adr/0002-host-privilege-dbus-polkit.md)), which is why the rule is not generated
from the config.

A service you do not list here is still watched. It simply offers no start, stop or restart,
because nothing authorised it.

```bash
sudo systemctl restart cueseekd.service
```

## 8. Check your work

```bash
sudo cueseekd check
```

Resolves every configured unit against systemd, compares the two allowlists **in both
directions**, confirms the bind address exists on an interface, tests that the state
directory is writable, and asks each adapter for its health. It changes nothing.

**As root, not as `cueseek`.** `/etc/polkit-1/rules.d` is `0750 root:polkitd` on Debian and
Ubuntu, and the `cueseek` user belongs to no group but its own — so it can read the config
and not the rule, and the allowlist comparison is the whole point. Run as anyone else and
`check` says so and carries on rather than reporting a rule it cannot see as broken.

A healthy install looks like this:

```
CueSeek check

  ok    configuration       /etc/cueseek/config.yaml parsed; 2 services
  ok    bind address        100.64.0.5:7777 is assigned to tailscale0
  ok    state directory     /var/lib/cueseek is writable by root — this does not
                            prove the cueseek user can
  ok    polkit allowlist    agrees with the configuration on 2 units
  ok    power actions       all four logind actions granted
  ok    unit jellyfin.service      active (running)
  ok    unit qbittorrent.service   active (running)
  ok    jellyfin            healthy
  ok    qbittorrent         healthy — it reports "connected"

9 ok, 0 warnings, 0 failures
```

It exits non-zero only on failures. A deliberately stopped service is a warning, not a
failure — otherwise nobody would put this in a script.

## Upgrading

Download the new release and re-run the installer. It replaces the binary and the unit,
leaves your configuration and database alone, and preserves a polkit rule you have edited.

```bash
sudo ./install.sh
sudo systemctl restart cueseekd.service
cueseekd -version
```

**Your paired devices survive.** The token lives in `/var/lib/cueseek/cueseek.db`, which the
installer does not touch — verified across an M3.7 → v0.1.0 upgrade with no re-pairing.

The Android client is versioned separately and does not have to match. A client four
milestones behind a v0.1.0 agent was observed working unchanged; capabilities it does not
recognise render as "Update CueSeek to view this" rather than breaking
([ADR-0007](adr/0007-client-capability-registry.md)).

## Uninstalling

```bash
sudo ./install.sh --uninstall           # keeps config and paired devices
sudo ./install.sh --uninstall --purge   # removes those too, and the user
```

`--purge` deletes `/var/lib/cueseek`, so every device must pair again. That is also the
recovery route if you ever lose every token.

## When something is wrong

[**Troubleshooting**](troubleshooting.md) lists every failure that has actually happened on
a real machine, and what each one means.

`cueseekd check` first. Then:

```bash
journalctl -u cueseekd -f
```

If a restart is refused, this reports which of the three layers said no — the agent's
allowlist, polkit, or systemd itself:

```bash
sudo -u cueseek cueseekd host restart -config /etc/cueseek/config.yaml <unit>
```

## Building from source instead

You need Go 1.25+ and a Linux target.

```bash
git clone https://github.com/Kushal-MR/CueSeek.git
cd CueSeek
./scripts/release-agent.sh
sudo ./dist/cueseek-agent_*/install.sh
```

That produces the same artefact CI does, the same way — static, `-trimpath`, version stamped
from `git describe`.
