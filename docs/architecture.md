# Architecture

How CueSeek fits together, what it does today, and how to build it. The short version is the
[README](../README.md); the reasoning behind each choice is in the [decision records](adr/).

## What works today

Everything below was verified on real hardware against the real agent, not against mocks —
the records are [M2](m2-p6-verification.md), [M3](m3-verification.md),
[M4](m4-verification.md) and [M5](m5-verification.md).

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

**The Wear OS client — shipped, and the point of the architecture.** It pairs
against the agent *directly* and mints its own token: the phone hands over only the host's
address over the Wearable Data Layer, because typing `192.168.1.10` on a round screen is
where people give up. No credential ever passes through the phone, so the watch has its own
row, its own scopes and its own revocation
([ADR-0014](adr/0014-watch-pairing-address-handoff.md)). It polls rather than holding
the stream, because nothing background-critical may depend on SSE and a watch radio is the
case that rule was written for.

It shares the tokens and the verdict with the phone and **none of the components** — Wear
Material 3 is a different library, and a shrunk phone layout is the most common way a Wear
app looks wrong. The dashboard leads with the verdict because that is the whole question at
arm's length; the detail screen shows *one* session or transfer rather than a list, since
twelve rows is a flick on a thumb and twelve crown rotations on a wrist. Actions carry the
same risk ceremony the phone uses, and the wrist gets three haptics and no more: a hold
crossing its threshold, the agent accepting, the agent refusing.

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

## How the pieces connect

CueSeek is a **host agent that also aggregates service APIs** — not an API aggregator that
happens to reboot things. That ordering matters: restarting a service, rebooting the
machine and reading CPU/disk/thermals are all host-level operations that no service API
can perform. The agent is therefore mandatory, not an optimisation.

```
Phone ──────Tailscale────┐
                         ├──▶ cueseekd  (user: cueseek, no sudo)
Watch ──────LAN / VPN────┘      │
                                ├─ api/      REST + SSE · token auth · scopes
                                ├─ health/   computed overall status
                                ├─ store/    SQLite: devices, token hashes, audit log
                                ├─ host/     HostController ──D-Bus──▶ systemd / logind
                                │                                    ▲ polkit rule
                                └─ adapters/ registry, one goroutine per adapter
                                      ├─ jellyfin    ──HTTP───▶ Jellyfin
                                      ├─ qbittorrent ──HTTP───▶ qBittorrent
                                      └─ systemd     ──D-Bus──▶ any units
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
See [ADR-0002](adr/0002-host-privilege-dbus-polkit.md).

**The spec is the source of truth.** `api/openapi.yaml` is hand-authored. The Go server
interfaces and every client SDK are generated from it, and CI fails if they drift. 
See [ADR-0004](adr/0004-contract-openapi-sse.md).

**Services declare capabilities; clients render capabilities.** There is no fat `Adapter`
interface, and no `switch (serviceId)` in any client. A service advertises
`health`, `control`, `now_playing`, `transfers`… and each client decides what those look
like on its form factor. A phone card and a watch tile are the same capability rendered
differently. See [ADR-0005](adr/0005-capability-based-adapters.md).

**Every device is paired separately and scoped separately.** Tokens carry
`read` / `service.control` / `devices.manage` / `host.power` as independent grants, so a
watch can restart Jellyfin while being structurally incapable of powering off the server
or of revoking your phone.
See [ADR-0006](adr/0006-device-pairing-scoped-tokens.md).

### Access model

CueSeek assumes your client can already reach the host — over Tailscale, WireGuard or your
LAN and it should **never be exposed directly to the internet.**

This is the same guidance Home Assistant and Immich give for their own remote access. It
also means the largest attack surface — a public endpoint that can power off a machine —
simply does not exist. See [ADR-0001](adr/0001-vpn-only-remote-access.md).

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
| `clients/android/` | Android phone client (Compose), and the four shared `core` modules both clients build on. |
| `clients/wear/` | Wear OS client (Wear Compose Material 3). A project in the `clients/android` build, not its own — see `settings.gradle.kts`. |
| `deploy/` | systemd unit, polkit rule, install script, packaging. |
| `docs/adr/` | Architecture Decision Records. Start here. |
| `docs/DESIGN.md` | The design system: palette, type, shape, motion, and the rules behind them. |

`api/` sits outside `agent/` on purpose. The spec is not the agent's — it is the contract
both sides bind to, and nesting it under the server would quietly make the server its
owner.

---

## Design decisions

Every significant decision is recorded as an ADR with its rationale **and its accepted
cost**. If you read one thing in this repository, read [`docs/adr/`](adr/).

The visual side has its own record. [`docs/DESIGN.md`](DESIGN.md) states the palette,
type scale, shape grammar, motion rules and accessibility floor as values rather than as
prose — the file a designer, a contributor or a design tool should be handed first.

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
