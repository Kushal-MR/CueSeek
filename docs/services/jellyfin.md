# Jellyfin

Full support. CueSeek asks Jellyfin about itself, so the dashboard shows whether the server
is answering, whether the API key still works, and what is playing right now.

It is **not** a Jellyfin client. Browsing libraries, editing metadata and controlling
playback belong to Jellyfin's own app; tapping the row takes you there. CueSeek looks after
the service.

## 1. Create an API key

In Jellyfin: **Dashboard → API Keys → +**. Name it something you will recognise in six
months — `cueseek` does nicely. Copy the key; Jellyfin will not show it again.

The key is read-only in effect: CueSeek only ever issues `GET` to `/System/Info` and
`/Sessions`, and restarts go through systemd rather than through Jellyfin's own
`/System/Restart`.

## 2. Put it in a file

```bash
printf '%s' 'YOUR_KEY_HERE' | sudo tee /etc/cueseek/jellyfin.key
sudo chown root:cueseek /etc/cueseek/jellyfin.key
sudo chmod 0640 /etc/cueseek/jellyfin.key
```

`api_key:` inline works too, if you would rather manage one file. Set one or the other —
both is rejected at startup.

## 3. Find the unit name

```bash
systemctl list-units --type=service --all | grep -i jellyfin
```

Usually `jellyfin.service`. Check anyway; this is the single most common misconfiguration
on a new install, and it is not always what you would guess.

## 4. Configure it

```yaml
services:
  - id: jellyfin
    name: Jellyfin
    type: jellyfin
    unit: jellyfin.service
    base_url: http://127.0.0.1:8096
    api_key_file: /etc/cueseek/jellyfin.key
    web_ui:
      scheme: http
      port: 8096
      path: /web/index.html
    poll_interval: 30s
    timeout: 5s
```

`base_url` is where **the agent** reaches Jellyfin — loopback, because they share a machine.
`web_ui` is where **your phone** reaches it, and carries no host: the client composes the
origin from the address it paired with. The two ports are usually the same and are stated
separately because they do not have to be.

Running Jellyfin in a container? Omit `unit`. The row is then watched but not controlled —
health and the web link, no buttons — and needs no polkit entry, because there is nothing to
authorise.

## 5. Allow the unit, if you gave it one

```bash
sudo nano /etc/polkit-1/rules.d/10-cueseek.rules
```

Add `jellyfin.service` to `allowedUnits`. Then:

```bash
sudo systemctl restart polkit
sudo systemctl restart cueseekd.service
sudo cueseekd check
```

## What you get

**Health**, from an authenticated call to `/System/Info` — deliberately the authenticated
endpoint rather than the public one, so a wrong or expired key shows up as a wrong key
rather than staying invisible until something else quietly returns nothing.

**Now playing**, from `/Sessions`: how many streams, how many are transcoding, and per
stream the title, an episode or year line, the user, the device, whether it is paused, and
position against duration.

Sessions with nothing playing are skipped. Jellyfin keeps a session object for a connected
client sitting on a menu, and counting those would make an idle house look busy — the
opposite of what the capability is for.

**Restart**, through systemd rather than Jellyfin's own restart endpoint. A Jellyfin wedged
enough to need restarting is often too wedged to honour its own API, and going via the host
layer means the request passes the unit allowlist and then polkit
([ADR-0002](../adr/0002-host-privilege-dbus-polkit.md)). The confirmation warns that anyone
watching will be interrupted.

The status line stays empty on purpose. Jellyfin publishes no self-assessment — no
"ok"/"degraded" string — and inventing one from the version or server name would put a
guess in a field the contract defines as the service's own words.

## What the states mean

| Dashboard | What happened | What to do |
| --- | --- | --- |
| **healthy** | `/System/Info` answered | — |
| **healthy**, with a note about a pending restart | Jellyfin applied changes needing a restart | restart it when convenient. Not amber: worth telling you, not worth an alarm |
| **degraded** — "rejected the API key (HTTP 401)" or 403 | the key is wrong, revoked, or from another server | reissue under Dashboard → API Keys |
| **degraded** — "not with Jellyfin system information" | `base_url` points at something that is not Jellyfin | check the port |
| **degraded** — "returned HTTP 5xx" | Jellyfin is up and broken | its own logs |
| **degraded** — "reports that it is shutting down" | mid-shutdown | wait |
| **unreachable** — "connection refused" | nothing is listening on `base_url` | is the unit running? |
| **unreachable** — "did not respond before the request deadline" | up but not answering in `timeout` | a wedged or very busy server; raise `timeout` only if this is normal for yours |

A bad key is **degraded, not unreachable**, and the distinction is the point: the fix is a
new API key, not a look at the network, and conflating the two sends you to the wrong place
([ADR-0005](../adr/0005-capability-based-adapters.md)).

## Two servers on one host

Two entries of `type: jellyfin` with different ids. Not a special case anywhere in the code.

```yaml
  - id: jellyfin-main
    name: Jellyfin
    ...
  - id: jellyfin-4k
    name: Jellyfin (4K)
    ...
```

## What CueSeek deliberately does not do

Library browsing, metadata, playback control, user management, transcode settings. Every one
of them would grow the adapter interface until it stopped being an adapter interface, and
Jellyfin's own app is better at all of them.

Even `now_playing` is read-only and translated into a vocabulary shared with Plex and Emby —
no play-method strings, no transcode reasons, no codec detail. Those are Jellyfin's words,
and the capability is not Jellyfin's.
