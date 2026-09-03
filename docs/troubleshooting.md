# Troubleshooting

Every failure listed here has actually happened, on a real machine, to somebody who then had
to work out why. That is the only reason any of them is written down.

## Start here

```bash
sudo cueseekd check
```

It resolves every configured unit against systemd, compares the configuration's allowlist
against the polkit rule's **in both directions**, confirms the bind address exists on an
interface, tests that the state directory is writable, and asks every adapter for its
health. It changes nothing, and every finding it reports names the next command to run.

If that is clean and something is still wrong, the journal is next:

```bash
journalctl -u cueseekd -n 100 --no-pager
```

---

## The agent will not start

### "read config … no such file or directory"

An `api_key_file` or `password_file` names a path that is not there. Startup resolves
file-backed secrets before anything else, so a missing key file stops the agent rather than
producing a service that is permanently unhealthy for an unexplained reason.

```bash
sudo ls -l /etc/cueseek/
```

Create the file, or comment out the service. An **empty** file fails the same way, on
purpose — an empty credential would fail later, as a mystifying 401 from the service.

### "unit … has no type suffix"

`unit: jellyfin` rather than `unit: jellyfin.service`. Caught at startup because the
alternative is an opaque D-Bus error the first time somebody taps Restart.

### "listens on all interfaces … refused by default"

`bind.address` is `0.0.0.0`, `::` or has an empty host. CueSeek terminates no TLS and can
power off the machine, so a wildcard bind has to be a decision somebody made on purpose and
can be found in a config diff. Bind to one address, or set `allow_unrestricted: true` if you
genuinely meant it.

### "bind: address already in use"

Something is already on that port — usually CueSeek itself.

```bash
systemctl status cueseekd
```

**If you were running a subcommand when this happened, check your version.** Before M4.5b an
unrecognised subcommand fell through and started the daemon, silently discarding every flag
after it. So on an older binary, `cueseekd check` loaded `/etc/cueseek/config.yaml`, ignored
`-config`, and tried to bind an address the running agent already held.

```bash
cueseekd -version
```

Anything reporting `0.0.0-dev`, or older than `v0.1.0`, has this behaviour. Newer binaries
refuse an unknown subcommand and print usage.

---

## The phone cannot reach the agent

### It never connects at all

Almost always the bind address.

```bash
sudo cueseekd check    # "bind address" line
```

`127.0.0.1` is the shipped default and is deliberately unreachable from anywhere else. Set
`bind.address` to your VPN address:

```bash
tailscale ip -4
```

Then confirm the phone is on the same tailnet, and that the address in the app matches the
one in `bind.address` exactly — including the port.

### It worked yesterday and not after a reboot

Look for this in the journal:

```
bind address is not present yet, waiting for it  address=100.x.y.z:7777 giving_up_after=1m30s
```

That is **normal**, not a fault. On a cold boot the VPN interface exists before it has an
address, so the agent retries for up to 90 seconds. It should be followed within seconds by:

```
bind address became available  address=100.x.y.z:7777 waited=8s attempts=5
```

If it never becomes available, the VPN did not come up — check `tailscale status`, not
CueSeek. Adding `After=tailscaled.service` does **not** help and was measured making it
slightly worse: it orders against tailscaled having *started*, and a started tailscaled has
not authenticated yet.

---

## A restart or stop is refused

Three different problems look identical through the app. This tells them apart:

```bash
sudo -u cueseek cueseekd host restart -config /etc/cueseek/config.yaml jellyfin.service
```

| Message | Meaning | Fix |
| --- | --- | --- |
| `unit is not managed by this agent` | not in `services:` | add it to the config |
| `not authorized … Interactive authentication required` | **polkit refused** | the unit is missing from `allowedUnits` |
| `unit not found` | systemd has no such unit | the name is wrong — see below |

The middle one is the common case, and it does not read like what it is. polkit is asking
for a password that a daemon can never supply, so the message says "interactive
authentication required" rather than "denied".

```bash
sudo cueseekd check    # "polkit allowlist" line names exactly which units disagree
```

Both lists are enforced and are deliberately not generated from each other
([ADR-0002](adr/0002-host-privilege-dbus-polkit.md)). After editing the rule:

```bash
sudo systemctl restart polkit
```

### The unit name is not what the software calls itself

This is the single most likely misconfiguration on a new install, and it was found on the
very first machine: **qBittorrent's unit is `qbittorrent.service`**, even though the unit
describes itself as "qBittorrent-nox" and the binary is `qbittorrent-nox`.

Never guess:

```bash
systemctl list-units --type=service --all | grep -i <name>
```

Whatever that prints goes in **both** the config and the polkit rule.

---

## Reboot and shut down are missing or greyed out

### Greyed out, with a line saying why

The device's token lacks `host.power`. It is never granted by default, and **a device paired
before M3.7 cannot have it** — tokens carry what they were granted and nothing widens
retroactively. Pair again:

```bash
sudo -u cueseek cueseekd pair -scopes read,service.control,host.power
```

### Absent entirely

The agent is not offering them, which means either the platform has no logind or the polkit
rule grants nothing. `sudo cueseekd check` reports the power actions line.

Greyed-out and absent are deliberately different: one is your device's permission, the other
is the agent's capability, and they have completely different fixes.

### Reboot works for you and fails for somebody else

The classic partial grant. `reboot` and `power-off` were granted without their
`-multiple-sessions` variants, which logind consults whenever another user is logged in. It
works perfectly when you test it alone and fails the first time somebody is at the console.

```bash
sudo cueseekd check    # "power actions" reports this as a failure
```

All four or none ([ADR-0002 Amendment 2](adr/0002-host-privilege-dbus-polkit.md)).

---

## The dashboard looks wrong

### Everything says "unknown", and the header says stale

The agent stopped sending. The client degrades to `unknown` from its own clock rather than
trusting the connection to notice its own death — showing stale green while the agent is
unreachable would be confidently wrong, which is worse than showing nothing.

Check the agent is up and reachable. If it is, the stream went quiet: Android's Doze freezes
the radio, and a connection can report itself as connected while delivering nothing for two
or three minutes. Pull to refresh, or bring the app back to the foreground.

### A service says healthy but does not actually work

Expected, if it is configured as `type: systemd`. That tier reports **the process is
running**, from systemd — not that the service is answering. A wedged service that has not
exited reads as healthy.

Use `type: jellyfin` or `type: qbittorrent` where they apply; those ask the service itself.
There is no generic HTTP probe yet.

### "Update CueSeek to view this"

The agent advertised a capability this build of the app does not know how to draw. Not an
error — a client meets capabilities that postdate it for the whole life of the project
([ADR-0007](adr/0007-client-capability-registry.md)). Update the app.

### A value is missing rather than zero

Deliberate. An absent field means "could not read", never zero. A virtual machine exposes no
temperature sensors; the first collection after a restart cannot compute CPU utilisation
because the kernel reports cumulative counters and one sample is not a measurement.
Rendering zero would claim an idle, cold machine that never answered.

---

## Pairing

### The code is rejected

Codes are **single use** and expire after **five minutes**. Unknown, expired and
already-redeemed codes are deliberately indistinguishable, so the message will not tell you
which — mint a new one.

Spacing and case do not matter; the agent strips and uppercases before matching.

### The app says it is paired but every request fails

The token could not be decrypted — usually because the device lock changed, which
invalidates the Keystore key that sealed it. The app drops the record and asks you to pair
again rather than failing every request with a 401 it cannot explain. Pair again.

### Everything unpaired after restoring a backup

By design. The credential store is excluded from cloud backup and device-to-device transfer:
a restored copy is sealed by a key that no longer exists, so it would produce a phone that
believes it is paired and cannot prove it. Honestly unpaired is the better state.

---

## Install and upgrade

### `install.sh` refuses

| Message | Meaning |
| --- | --- |
| `systemd not found` | CueSeek requires it — see [requirements](requirements.md) |
| `polkit x.y cannot enforce a per-unit allowlist` | polkit below 0.106. Rules would degrade to granting restart on **any** unit, which is far more than the shipped rule claims — so this is a refusal, not a warning |
| `is not a Linux executable` | a Windows or macOS cross-build. Build with `GOOS=linux` |
| `does not match cueseekd.sha256` | the download is incomplete or altered. Fetch it again |

### My polkit edits disappeared after an upgrade

They did, if you upgraded from a build older than M4.3. `install.sh` used to overwrite the
rule unconditionally, so a binary upgrade silently reverted the allowlist to the shipped one
— and the symptom arrived later, as a restart that worked yesterday being refused today.

Current versions preserve a modified rule and write the new one beside it as `.rules.new`.

### `gh attestation verify` says "unknown command"

Your `gh` is older than 2.49. Ubuntu 24.04 ships 2.45. Run it from a machine with a newer
`gh`; the artefact is fine.

---

## Getting help

Include the output of these, which between them describe the whole installation:

```bash
cueseekd -version
sudo cueseekd check
journalctl -u cueseekd -n 50 --no-pager
```

**`check` prints no secrets** — it names configuration paths and unit names, never a key or a
token. The journal does not either. Read them before pasting anywhere public regardless; your
unit names and bind address are in there, and the bind address is a machine on your network.
