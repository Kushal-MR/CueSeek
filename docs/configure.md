# Configuration

```
/etc/cueseek/config.yaml     root:cueseek, 0640
```

**The file the installer puts there is the reference.** It is annotated line by line, with a
worked example of every supported service type commented at the bottom. This page is the map
of it — what the four blocks are for, which decisions are actually yours, and how the agent
behaves when you get one wrong. For the meaning of an individual field, read the comment
above that field in the file itself; keeping the explanation next to the value is the only
way it stays true.

```bash
sudo nano /etc/cueseek/config.yaml
```

If yours has drifted from the shipped one, the current version is always in the release
tarball, and in [`deploy/config.example.yaml`](../deploy/config.example.yaml).

## The four blocks

| Block | What it decides | How often you touch it |
| --- | --- | --- |
| `bind` | which address the phone connects to | once |
| `storage` | where paired devices and the audit log live | never |
| `host` | which filesystems to report | rarely |
| `services` | everything you actually see on the dashboard | whenever you add software |

Only two of them need a decision. `storage` has one correct value and the unit creates the
directory for it; `host` works with no configuration at all, because it reads `/proc` and
`/sys`, which need no privilege and exist on every machine.

### bind

The one setting that decides whether the install works. Loopback as shipped, which is
unreachable from your phone **on purpose** — the safe default is the one that protects
somebody who installs this and never reads a word of documentation.

```yaml
bind:
  address: "100.64.0.5:7777"    # tailscale ip -4
```

A wildcard bind (`0.0.0.0`, `::`, or an empty host) is refused unless you also set
`allow_unrestricted: true`. Widening the exposure of something that can power off the
machine should be a line somebody can find in a config diff, not a default.

### host

Every value under it is already its default; deleting the whole block changes nothing. The
one thing worth setting is which filesystems appear:

```yaml
host:
  metrics:
    storage_mounts: ["/", "/mnt/media"]
```

Listed rather than discovered, because there is no honest guess. Your media might be on
`/srv`, a ZFS dataset, or nothing at all, and enumerating every mount would bury the two you
care about under loop devices, tmpfs and container overlays.

### services

Empty as shipped, and **that is a working install** — the agent reports the machine's own
vitals and an empty service list. Every entry you add is one row on the dashboard.

```yaml
services:
  - id: jellyfin          # stable, internal; never change it
    name: Jellyfin        # what the phone shows; change it freely
    type: jellyfin        # which adapter runs
    unit: jellyfin.service
```

`id` and `type` are the only required fields. Two Jellyfin servers on one host are two
entries with different ids — not a special case anywhere in the code.

## The three decisions per service

### 1. Which `type`

| `type` | Health comes from | You also get |
| --- | --- | --- |
| `jellyfin` | Jellyfin's own API | what is playing, and who is watching |
| `qbittorrent` | qBittorrent's own API | transfer rates and a torrent list |
| `systemd` | the unit, via systemd | nothing beyond "the process is running" |

Guides: [Jellyfin](services/jellyfin.md), [qBittorrent](services/qbittorrent.md). Anything
else on the machine is `type: systemd` — Plex, Sonarr, Immich, Syncthing, Vaultwarden,
Samba, Postgres, a compose stack behind a unit.

**Understand what the basic tier does not tell you.** `active` means the process has not
exited. A service that is wedged, out of disk, or answering every request with a 500 is
`active`, and CueSeek will show it green. There is no generic HTTP probe yet, and pretending
otherwise would be worse than the gap.

Moving up a tier later is a one-line edit. Keep the same `id`, change `type`, add
`base_url` and a credential — the phone gains the new capability with no app update, because
capabilities are discovered at runtime
([ADR-0007](adr/0007-client-capability-registry.md)).

### 2. Whether to give it a `unit`

`unit` is optional, and it is the only thing that decides whether the row has buttons.

- **With a unit** — start, stop and restart, provided the same name is also in
  `allowedUnits` in `/etc/polkit-1/rules.d/10-cueseek.rules`. Both are enforced, and are
  deliberately not generated from each other ([ADR-0002](adr/0002-host-privilege-dbus-polkit.md)).
- **Without one** — watched, not controlled. Health and a web-interface link, no buttons.
  This is the right shape for something running from a container image, and it needs no
  polkit entry at all, because there is nothing to authorise.

`type: systemd` is the exception: there the unit *is* the only source of health, so it is
required.

**Use the exact unit name.** It is not reliably what the software calls itself:

```bash
systemctl list-units --type=service --all | grep -i <name>
```

### 3. Whether it has a web interface

```yaml
web_ui:
  scheme: http    # defaults to http
  port: 8096
  path: /web      # defaults to /
```

Note the absent field: **there is no host.** The agent does not know which address a given
client reached it on — a phone on the tailnet and a laptop on the LAN use different ones —
so it sends only the parts that do not vary, and the client composes
`scheme://{the address it paired with}:{port}{path}`.

That is why one configuration works from everywhere, and why a compromised agent cannot
send your phone to an origin you never configured. Omit the block and the service simply
does not advertise the capability.

## Secrets

Two ways to supply one. Prefer the file:

```yaml
api_key_file: /etc/cueseek/jellyfin.key
password_file: /etc/cueseek/qbittorrent.pass
```

```bash
printf '%s' 'YOUR_SECRET' | sudo tee /etc/cueseek/jellyfin.key
sudo chown root:cueseek /etc/cueseek/jellyfin.key
sudo chmod 0640 /etc/cueseek/jellyfin.key
```

A configuration that *names* a secret can be pasted into a bug report; one that contains it
cannot. Contents are trimmed, so a trailing newline is harmless.

Setting both the inline and the file form is rejected at startup rather than resolved by
precedence — a silent winner between two values an operator wrote on purpose is the kind of
thing nobody debugs successfully. So is a missing or **empty** key file: an empty credential
would otherwise surface much later, as an unexplained 401 from the service.

## Timing

Both are optional and both have defaults that are right for a home server.

| Field | Default | |
| --- | --- | --- |
| `poll_interval` | `30s` | how often this service is asked about itself. Minimum `1s` |
| `timeout` | `5s` | budget for one upstream request. Must be shorter than `poll_interval` |

A client request never triggers an upstream call. The agent polls on its own schedule and
serves what it last saw, so a wedged Jellyfin cannot hang your dashboard
([ADR-0003](adr/0003-agent-runtime-go.md)). The cost of that is a value up to one interval
old, which is why anything the agent has not refreshed in three intervals is marked stale
rather than shown as fact.

Host metrics poll faster — `10s` — because they are local file reads costing microseconds
rather than network calls, and CPU averaged over thirty seconds hides the spike worth
seeing.

## What happens when it is wrong

**Unknown keys are rejected.** A typo like `adress:` fails at startup instead of being
ignored and silently leaving you on a default you did not choose.

**Every problem is reported, not just the first.** One restart, one full list.

Startup refuses rather than degrades for: a unit name with no type suffix
(`jellyfin` → `jellyfin.service`), a wildcard bind without `allow_unrestricted`, a relative
`storage.path`, duplicate service ids, both forms of a credential, a username with no
password, a `timeout` at or beyond its `poll_interval`, a `web_ui.scheme` that is not
`http` or `https`, and — for `type: systemd` — any of `base_url`, `api_key`, `api_key_file`,
`username`, `password` or `password_file`, since nothing would ever read them.

That last one is not pedantry. Configuring a credential for an adapter that never contacts
the service almost always means the operator expected CueSeek to understand it and reached
for the generic type by mistake; the message says so and names the fields.

## After every edit

```bash
sudo systemctl restart cueseekd.service
sudo cueseekd check
```

`check` resolves every unit against systemd, compares the config's allowlist against the
polkit rule's **in both directions**, confirms the bind address exists on an interface, tests
that the state directory is writable, and asks each adapter for its health. It changes
nothing, and it is the fastest way to find out whether the file you just edited says what
you meant.

Run it **as root** — `/etc/polkit-1/rules.d` is `0750 root:polkitd`, so any other user can
read the configuration and not the rule, which is half of what `check` exists to compare.

When it reports something you do not recognise, [troubleshooting](troubleshooting.md) lists
every failure that has actually happened on a real machine.
