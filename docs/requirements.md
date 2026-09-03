# Requirements

Read this before downloading. CueSeek is deliberately narrow about what it runs on, and it
is better to find out here than after `install.sh` has refused.

## The short version

A Linux machine with **systemd** and **polkit 0.106 or later**, on **x86-64**, that your
phone can already reach over a VPN or your LAN. An Android phone on **8.0 or later**.

If that describes your server, CueSeek will very probably work. If any part of it does not,
it will not, and no amount of configuration will change that.

## The host

| | |
| --- | --- |
| Init system | **systemd**. Not optional — see below. |
| Authorisation | **polkit ≥ 0.106**, with JS rules in `/etc/polkit-1/rules.d/` |
| Architecture | **`linux/amd64`** (x86-64) |
| Privileges to install | root, once |
| Privileges to run | none — the agent runs as its own unprivileged user |
| Network | an address your phone can reach: Tailscale, WireGuard, or the same LAN |

**Why systemd is not negotiable.** Restarting a service, rebooting the machine and reading
its CPU and disk are host-level operations no service API can perform. CueSeek asks systemd
and logind for them over D-Bus, authorised by a polkit rule
([ADR-0002](adr/0002-host-privilege-dbus-polkit.md)). There is a `HostController` interface
so that an OpenRC or BSD backend would be a swap rather than a rewrite, but none is
implemented and none is planned.

**Why polkit 0.106 specifically.** Older polkit uses `.pkla` files, which cannot inspect
action details. The per-unit allowlist would silently degrade to *"CueSeek may restart any
unit"* — far more than the shipped rule claims to grant. `install.sh` refuses on such a host
rather than installing a rule that lies about its own ceiling.

### Check before you download

```bash
systemctl --version | head -1
pkaction --version
uname -m
```

Anything other than `x86_64`, a missing `pkaction`, or polkit below 0.106 means CueSeek is
not for this machine.

## The phone

| | |
| --- | --- |
| Android | **8.0 (API 26)** or later |
| Install | sideloading an APK — CueSeek is not on Google Play |
| Network | on the same VPN or LAN as the host |

There is no iOS client, and no web interface. A Wear OS client is planned for M5.

## Services

Two tiers, and the difference is what CueSeek observes.

| Tier | `type:` | Health means | Also get |
| --- | --- | --- | --- |
| **Full** | `jellyfin`, `qbittorrent` | the service answered its own API | what it is doing — playback sessions, transfers |
| **Basic** | `systemd` | the process is running | start/stop/restart, a link to its own web interface |

**The basic tier is honest about its limit**: it reports that the *process* is up, not that
the service is *answering*. A wedged service that has not exited reads as healthy. Closing
that gap needs an HTTP probe, which is a different adapter and is not built yet.

Anything systemd manages works at the basic tier — Plex, Sonarr, Immich, Syncthing,
Vaultwarden, Samba, Postgres, a `docker compose` stack behind a unit. A service with no unit
at all can still be watched if it has an HTTP API CueSeek understands; it simply offers no
lifecycle actions, because there is nothing to act on.

## What was actually tested

Not a claim about what works everywhere — a record of what this was run against.

| | |
| --- | --- |
| Distribution | Linux Mint 22.3 (Ubuntu 24.04 "noble" base) |
| Kernel | 7.0.0-28-generic, x86_64 |
| systemd | 255 |
| polkit | 124 |
| Jellyfin | 10.11.11 |
| qBittorrent | qbittorrent-nox 4.6.3 |
| VPN | Tailscale 1.102.2 |
| Phone | Android 16 (API 36), OnePlus CPH2707 |
| Agent | built with Go 1.25 |

Other distributions, other versions of those services, and other phones **should** work.
None of them has been tried. The verification records are in
[`m4-verification.md`](m4-verification.md) and [`m3-verification.md`](m3-verification.md).

## Not supported

Stated plainly so nobody spends an evening finding out.

- **NAS appliances** — Synology DSM, QNAP QTS and similar. They are Linux, but their
  services are not standard systemd units and `/etc/polkit-1/rules.d/` generally is not
  there. `install.sh` refuses rather than half-installing. TrueNAS SCALE has systemd but
  runs its apps in k3s, which CueSeek does not see.
- **`arm64`, including Raspberry Pi.** No build is published. Not because it could not
  work — because it cannot be tested here, and this project does not ship an architecture
  it has not run.
- **Non-systemd Linux** — OpenRC, runit, s6.
- **Windows, macOS, BSD.** The agent compiles there so it can be developed on them; every
  host operation returns `ErrUnsupportedPlatform` and no power actions are advertised.
- **Containers as a managed thing.** CueSeek cannot start, stop or inspect a container. A
  compose stack behind a systemd unit works, because what it manages is the unit.
- **Exposure to the internet.** CueSeek terminates no TLS, has no certificate, and can power
  off the machine. It refuses at startup to bind a wildcard address without an explicit
  opt-out. Reachability is the VPN's job
  ([ADR-0001](adr/0001-vpn-only-remote-access.md)).
- **Multiple hosts from one phone.** The data layer has carried a host id since M2, but the
  interface shows one.
- **Alerting.** CueSeek tells you when you look. It does not tell you when you are not
  looking ([ADR-0012](adr/0012-alerting-vs-vpn-only-access.md)).

## Next

[Install](install.md) · [Pairing](pairing.md) · [Security model](../SECURITY.md)
