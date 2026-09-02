# CueSeek

**An operations console for self-hosted home servers.**

CueSeek gives you one consistent way to reach every service running on your home server.

It talks to Jellyfin, qBittorrent and others through their APIs, and adds the host-level
control they cannot provide themselves.

> **Status: M3 complete.** The agent and the Android client both work, and the whole path —
> phone → Tailscale → `cueseekd` → polkit → systemd — has been verified end to end on real
> hardware. Jellyfin and qBittorrent are supported in full; any other systemd unit is
> supported for health and control. The phone can read the machine's own vitals and reboot
> or shut it down.
> See [What works today](#what-works-today).

---

## What CueSeek is

A single console for **health, activity and control** across many self-hosted services and
the machine they run on:

- **Health** — is each service up, reachable and behaving? Is the host healthy?
- **Activity** — what is happening right now: playback sessions, transfers, jobs.
- **Control** — restart a service, reboot the host, shut it down.

## What CueSeek is not

- **Not a replacement** for Jellyfin, qBittorrent, Sonarr, Immich or Home Assistant.
- **Not a media client.** CueSeek shows you *that* something is playing, not the library
  you browse to play it.
- **Not a general dashboard.** Adding a service means adding its health, activity and
  control surface — never its full domain UI. Immich support will mean Immich *jobs and
  storage*, not photo browsing.

---

## What works today

Three milestones are complete. Everything below was verified on real hardware against the
real agent, not against mocks — the records are in
[`docs/m2-p6-verification.md`](docs/m2-p6-verification.md) and
[`docs/m3-verification.md`](docs/m3-verification.md).

**The agent — `cueseekd`.** Runs as an unprivileged `cueseek` user on a systemd host.
Serves the REST contract and an SSE event stream, stores paired devices and hashed tokens
in SQLite, polls Jellyfin and qBittorrent on its own schedule for health *and for what they
are doing*, watches any other systemd unit for whether it is running, reads the machine's
own CPU, memory, storage and temperatures from `/proc` and `/sys`, and starts, stops and
restarts units — or reboots and shuts down the host itself — through polkit, without ever
holding root.

**The Android client.** Pair by entering the host's address and a single-use code printed
by `cueseekd pair`. A live dashboard shows every service's health and activity, updated over
SSE, with the machine's own vitals underneath. Tapping a service row opens its detail; the
trailing menu carries the web interface and its lifecycle actions, gated by the risk the
agent assigns. The host menu carries reboot and shut down, which name what they will
interrupt before you hold the button. Outcomes arrive as stream events rather than being
assumed from the acknowledgement.

**What it refuses to do** is as deliberate as what it does. It never shows stale data as
current: if the agent goes quiet the client degrades to `unknown` from a clock, never from
what the connection claims about itself. A value the agent could not read is absent rather
than zero, so an unreadable sensor never renders as a cold machine. And the UI never branches
on which service it is — a test enforces it, which is why qBittorrent reached the phone with
no client change at all.

Verified end to end over Tailscale, on a phone and a real Linux host:

| | |
| --- | --- |
| **Pair a device** | Real single-use code redeemed over the tailnet |
| **See health** | Matches `systemctl` — checked against two independent measurements, Jellyfin's own API and systemd watching the process |
| **Restart a service** | Through polkit; `ActiveEnterTimestamp` moved, and the outcome arrived over the stream *after* systemd recorded it |
| **Recover** | From a VPN outage, a Wi-Fi↔cellular switch, a locked phone, a starved stream and a host reboot |

---

## Architecture

CueSeek is a **host agent that also aggregates service APIs** — not an API aggregator that
happens to reboot things. That ordering matters: restarting a service, rebooting the
machine and reading CPU/disk/thermals are all host-level operations that no service API
can perform. The agent is therefore mandatory, not an optimisation.

```
Phone / Wear ──Tailscale──▶ cueseekd  (user: cueseek, no sudo)
                              │
                              ├─ api/      REST + SSE · token auth · scopes
                              ├─ health/   computed overall status
                              ├─ store/    SQLite: devices, token hashes, audit log
                              ├─ host/     HostController ──D-Bus──▶ systemd / logind
                              │                                    ▲ polkit rule
                              └─ adapters/ registry, one goroutine per adapter
                                    ├─ jellyfin    ──HTTP───▶ Jellyfin
                                    ├─ qbittorrent ──HTTP───▶ qBittorrent
                                    └─ systemd     ──D-Bus──▶ any unit
```

### Two tiers of service support

| Tier | Types | What health means |
| --- | --- | --- |
| **Full** | `jellyfin`, `qbittorrent` | The service answered its own API — plus what it is doing: playback sessions, transfers |
| **Basic** | `systemd` — any unit | The process is running |

The basic tier says the process is up, **not** that the service is answering: a wedged
service that has not exited reads as healthy. That limit is real and is stated rather than
hidden. Both tiers get lifecycle control and a link to the service's own interface.

Moving up a tier is one line of configuration — `type: systemd` becomes `type: plex` when
that adapter exists — and needs no app update, because an adapter reaches the phone through
capabilities the client already renders.

Properties:

**The agent never runs as root.** It holds no special privileges of its own. A shipped
polkit rule grants the `cueseek` user exactly `start`, `stop` and `restart` on an allowlist
of named units, plus `Reboot`/`PowerOff` via logind — and nothing else. That rule is the
complete statement of the ceiling, and it is short enough to read before trusting it.
See [ADR-0002](docs/adr/0002-host-privilege-dbus-polkit.md).

**The spec is the source of truth.** `api/openapi.yaml` is hand-authored. The Go server
interfaces and every client SDK are generated from it, and CI fails if they drift. 
See [ADR-0004](docs/adr/0004-contract-openapi-sse.md).

**Services declare capabilities; clients render capabilities.** There is no fat `Adapter`
interface, and no `switch (serviceId)` in any client. A service advertises
`health`, `control`, `now_playing`, `transfers`… and each client decides what those look
like on its form factor. A phone card and a watch tile are the same capability rendered
differently. See [ADR-0005](docs/adr/0005-capability-based-adapters.md).

**Every device is paired separately and scoped separately.** Tokens carry
`read` / `service.control` / `devices.manage` / `host.power` as independent grants, so a
watch can restart Jellyfin while being structurally incapable of powering off the server
or of revoking your phone.
See [ADR-0006](docs/adr/0006-device-pairing-scoped-tokens.md).

### Access model

CueSeek assumes your client can already reach the host — over Tailscale, WireGuard or your
LAN and it should **never be exposed directly to the internet.**

This is the same guidance Home Assistant and Immich give for their own remote access. It
also means the largest attack surface — a public endpoint that can power off a machine —
simply does not exist. See [ADR-0001](docs/adr/0001-vpn-only-remote-access.md).

---

## Repository layout

| Path | Contents |
| --- | --- |
| `api/openapi.yaml` | The contract. Single source of truth for every client and the agent. |
| `agent/` | `cueseekd`, the Go host agent. |
| `agent/internal/domain/` | Shared vocabulary: scopes, devices, audit entries. Depends on nothing. |
| `agent/internal/config/` | Configuration loading and validation, incl. the managed-unit allowlist. |
| `agent/internal/api/` | HTTP + SSE transport, auth middleware, scope enforcement. |
| `agent/internal/adapters/` | Capability interfaces, adapter registry, per-service adapters, and the generic `systemd` one. |
| `agent/internal/health/` | Aggregates per-service health into one overall status, with reasons. |
| `agent/internal/host/` | `HostController`; systemd/logind over D-Bus. |
| `agent/internal/store/` | SQLite: device registry, token hashes, audit log. |
| `clients/android/` | Android phone client (Compose). |
| `clients/wear/` | Wear OS client. Placeholder until M5. |
| `deploy/` | systemd unit, polkit rule, install script, packaging. |
| `docs/adr/` | Architecture Decision Records. Start here. |
| `docs/DESIGN.md` | The design system: palette, type, shape, motion, and the rules behind them. |

`api/` sits outside `agent/` on purpose. The spec is not the agent's — it is the contract
both sides bind to, and nesting it under the server would quietly make the server its
owner.

---

## Design decisions

Every significant decision is recorded as an ADR with its rationale **and its accepted
cost**. If you read one thing in this repository, read [`docs/adr/`](docs/adr/).

The visual side has its own record. [`docs/DESIGN.md`](docs/DESIGN.md) states the palette,
type scale, shape grammar, motion rules and accessibility floor as values rather than as
prose — the file a designer, a contributor or a design tool should be handed first.

## Roadmap

| Milestone | Scope | Status |
| --- | --- | --- |
| **Setup** | Repository skeleton, ADRs, contract placeholder | ✅ Done |
| **M0** | Architecture validation spike: polkit + D-Bus, and SSE over a tailnet | ✅ Done — [findings](docs/m0-findings.md). A7 closed: SSE viable, but Doze freezes it silently rather than killing it (ADR-0004 Amendment 2) |
| **M1** | Agent: pairing, scoped tokens, Jellyfin health + restart | ✅ Done — contract, store, API, host control, adapters, SSE stream and [deployment](deploy/) |
| **M2** | Android client: pair by entering host address + code, capability-driven dashboard, one action | ✅ Done — verified end to end over Tailscale against the real agent ([record](docs/m2-p6-verification.md)) |
| **M3** | qBittorrent, `web_ui`, activity, host metrics, power actions | ✅ Done — nine phases, each verified on hardware as it landed ([plan](docs/m3-plan.md) · [record](docs/m3-verification.md)). A second adapter reached the phone with **zero client changes**, and the reboot was confirmed by a changed kernel boot id |
| **M4** | Productization: licence, neutral defaults, the `systemd` adapter, `cueseekd check`, released artefacts, documentation | 🔨 In progress — [plan](docs/m4-plan.md). Proven by installing on a machine that has never seen CueSeek |
| **M5** | Wear OS standalone client, tiles and complications | ⬜ |
| **M6** | A third adapter, used to measure whether the abstraction held | ⬜ |

M4 was previously the Wear milestone; the renumber and its reasoning are
[ADR-0011 Amendment 2](docs/adr/0011-sequencing-spike-then-slice.md).


## Installing

You need a Linux host with systemd and polkit 0.106 or later, and Tailscale, WireGuard or a
LAN between it and your phone. No clone, and no Go toolchain.

```bash
# from the releases page
sha256sum -c SHA256SUMS
tar xzf cueseek-agent_*_linux_amd64.tar.gz
cd cueseek-agent_*_linux_amd64
sudo ./install.sh
```

The tarball is self-contained — agent, systemd unit, polkit rule, annotated example
configuration and installer. It manages no services as shipped, which is a working install:
the machine's own vitals need no configuration and no privilege. Add your services when you
want them.

Then, at any point:

```bash
sudo cueseekd check
```

which reports whether the configuration, the polkit allowlist and the units actually agree,
and changes nothing.

**`linux/amd64` only.** Developed and tested on Linux Mint 22.3 with systemd 255 and polkit
124, over Tailscale. Other distributions should work and are untested. There is no `arm64`
build — it cannot be tested here. NAS appliances are not supported.

## Development

**Requirements**

- Go 1.25+ — the agent
- JDK 21+ — the clients. The screenshot-test plugin declares a JVM 21 floor, so 17 cannot
  resolve it. The modules still *target* JVM 17 bytecode
- Android Studio recent enough for AGP 9.3.1, if you want the IDE. The command line does
  not need it
- A Linux host running systemd — the agent's only supported target today
- Tailscale, WireGuard or LAN connectivity between client and host

```bash
git clone https://github.com/Kushal-MR/CueSeek.git
```

```bash
cd agent && go build ./...
```

To produce the release tarball the same way CI does — static, reproducible, with the
version stamped from `git describe`:

```bash
./scripts/release-agent.sh
```

**Platform support.** The agent targets systemd-based Linux. This is a real limitation, not
an oversight: `RestartUnit`, `Reboot` and `PowerOff` are systemd/logind operations. The
`HostController` interface exists so that an OpenRC, BSD or macOS backend is a swap rather
than a rewrite, but **none is implemented, and none is planned.** On any other platform the
host layer returns `ErrUnsupportedPlatform` and the agent advertises no power actions rather
than offering buttons that could only fail.

## Security

CueSeek can restart services, reboot the host and power it off. What it is permitted to do
is stated in full by one short polkit rule you install by hand and can read in two minutes:
[`deploy/10-cueseek.rules`](deploy/10-cueseek.rules). The agent itself runs as an
unprivileged user and holds no capabilities.

[`SECURITY.md`](SECURITY.md) covers the model, the risks that are knowingly accepted, what
CueSeek deliberately cannot do, and how to report a vulnerability — privately, through
GitHub's advisory flow rather than a public issue.

## Licence

[Apache License 2.0](LICENSE).

Third-party components are listed in [`NOTICE`](NOTICE). The bundled IBM Plex fonts are
under the SIL Open Font License 1.1, which is a separate licence from the one above.

