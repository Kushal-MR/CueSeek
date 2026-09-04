# qBittorrent

Full support. CueSeek asks qBittorrent about itself, so the dashboard shows whether it is
running, whether it can reach peers, what it is moving and how fast.

It is **not** a torrent manager. Adding, pausing, prioritising and deleting torrents belong
to qBittorrent's own Web UI, which is exactly what tapping the row opens. The transfer list
CueSeek shows is a read-only activity view.

## 1. Check whether you need credentials

Most installs do not. qBittorrent has **Options → Web UI → "Bypass authentication for
clients on localhost"**, it is on by default, and the agent is on localhost — so there is
nothing to log in with and nothing to log in to.

Leave `username` and `password` out unless you turned that off. If you did:

```bash
printf '%s' 'YOUR_PASSWORD' | sudo tee /etc/cueseek/qbittorrent.pass
sudo chown root:cueseek /etc/cueseek/qbittorrent.pass
sudo chmod 0640 /etc/cueseek/qbittorrent.pass
```

Set **both** or neither. A username with no password is rejected at startup rather than
failing later as a login error from qBittorrent, which is the wrong place to learn about a
half-finished edit.

## 2. Find the unit name

**This one is the trap.** On most distributions the unit is `qbittorrent.service` — *not*
`qbittorrent-nox.service`, whatever the process calls itself and whatever the unit's own
description says. It was the first misconfiguration found on the first machine CueSeek was
ever installed on.

```bash
systemctl list-units --type=service --all | grep -i qbit
```

Whatever that prints goes in **both** the config and the polkit rule.

## 3. Configure it

```yaml
services:
  - id: qbittorrent
    name: qBittorrent
    type: qbittorrent
    unit: qbittorrent.service
    base_url: http://127.0.0.1:8080
    web_ui:
      scheme: http
      port: 8080
      path: /
    poll_interval: 30s
    timeout: 5s
```

`8080` is qBittorrent's default Web UI port and is where both the adapter and your phone go
— `base_url` over loopback for the agent, `web_ui` for the client, which composes the origin
from the address it paired with.

`web_ui` matters more here than for any other service: managing torrents happens in
qBittorrent's interface, and getting you there in one tap is most of what this row is for.

With credentials:

```yaml
    username: admin
    password_file: /etc/cueseek/qbittorrent.pass
```

## 4. Allow the unit

```bash
sudo nano /etc/polkit-1/rules.d/10-cueseek.rules
```

Add `qbittorrent.service` to `allowedUnits`. Then:

```bash
sudo systemctl restart polkit
sudo systemctl restart cueseekd.service
sudo cueseekd check
```

## What you get

**Health**, from one authenticated call to `/api/v2/transfer/info`. That endpoint was chosen
because it answers both questions at once — whether the credentials work and whether
qBittorrent can reach its peers. `/api/v2/app/version` would prove only the first, and a
torrent client that is up but firewalled is precisely the state an operations console exists
to surface.

**Its own word for its state**, carried verbatim into the row: `connected`, `firewalled`,
`disconnected`. Not a paraphrase — when the dashboard says "firewalled", that is
qBittorrent's word.

**Transfers**: global download and upload rates from qBittorrent's own totals, the number of
torrents, how many are actively moving data, and a ranked sample with name, state, progress,
speed and ETA.

Seeding does not count as active. It is upload, it is usually permanent, and counting it
would mean a client that finished everything last week still reports "12 active" forever —
which trains you to ignore the number.

The sample is ordered actively-downloading-fastest-first, then everything else newest-first,
and is stable between polls. Ranking purely by speed was tried and was wrong: everything
seeding, paused or finished ties at zero, so past the first row the order was luck, and a
torrent that finished a minute ago dropped to zero and vanished.

**Start, stop and restart**, through systemd and the polkit allowlist rather than through
qBittorrent's API. The confirmation says active torrents will be paused and will resume when
it starts again.

## What the states mean

| Dashboard | What happened | What to do |
| --- | --- | --- |
| **healthy** — "connected" | working | — |
| **healthy** — "firewalled" | running, authenticated, transferring; peers cannot connect inbound | forward its listening port for better speeds. Often deliberate, so it does not colour the row |
| **degraded** — "disconnected" | running, connected to no peers | the host's network, and qBittorrent's connection settings |
| **degraded** — "refused … and no credentials are configured" | the localhost bypass is off | turn it back on, or set username and password |
| **degraded** — "rejected the request … check username and password" | the credentials are wrong | fix them |
| **degraded** — "not with qBittorrent transfer information" | `base_url` points at something else | check the port |
| **degraded** — "returned HTTP 5xx" | up and broken | its own logs |
| **unknown** — "did not report a connection status" | answered, decoded, said nothing about its connection | a missing field is not a green light |
| **unknown** — "a connection status this version of CueSeek does not recognise" | a newer qBittorrent | harmless; the value is shown verbatim regardless |
| **unreachable** — "connection refused" | nothing listening on `base_url` | is the unit running? |
| **unreachable** — "did not respond before the request deadline" | up but not answering in `timeout` | usually a rechecking client under heavy I/O |

An unrecognised status becomes **unknown, and is still displayed**. Guessing whether a new
value means healthy or degraded is the one thing that would be worse than saying so — the
same stance the Android client takes toward capabilities that postdate it
([ADR-0007](../adr/0007-client-capability-registry.md)).

## About sessions

qBittorrent hands out a session cookie, and cookies expire. When one does, the agent logs in
again once and retries, silently. An expired session is not reported as an authentication
failure, because that would send you to check a password that was never the problem.

A failed login is different: qBittorrent answers `200` with the body `Fails.` for bad
credentials rather than a 4xx, so the body is checked and not only the status. A login that
reported success while having failed would turn every later refusal into an infinite
re-login loop.

## Torrent counts

Up to 200 torrents are fetched per poll and then ranked, so the sample you see is the part
worth seeing. Past 200, the total under-reports and the oldest are dropped — which at two
hundred is a library, not a home server.

## What CueSeek deliberately does not do

Add, pause, resume, delete, reprioritise, retag, or change any setting. A console that grew
half a torrent manager would be worse at it than the real one and would still not replace
it. That is what the web-interface link is for.
