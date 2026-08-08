# CueSeek

**An operations console for self-hosted home servers.**

CueSeek gives you one consistent way to answer *"is everything fine?"* — and to do
something about it when the answer is no — across every service running on your box.

It talks to Jellyfin, qBittorrent and others through their APIs, and adds the host-level
control they cannot provide themselves.

> **Status: pre-alpha.** This repository currently contains the architecture, the decision
> record, and the project skeleton. There is no working software yet. See
> [Roadmap](#roadmap) for what is actually built.

---

## What CueSeek is

A single console for **health, activity and control** across many self-hosted services and
the machine they run on:

- **Health** — is each service up, reachable and behaving? Is the host healthy?
- **Activity** — what is happening right now: playback sessions, transfers, jobs.
- **Control** — restart a service, reboot the host, shut it down.

## What CueSeek is not

This distinction is the product, so it is stated first and enforced everywhere:

- **Not a replacement** for Jellyfin, qBittorrent, Sonarr, Immich or Home Assistant.
- **Not a media client.** CueSeek shows you *that* something is playing, not the library
  you browse to play it.
- **Not a general dashboard.** Adding a service means adding its health, activity and
  control surface — never its full domain UI. Immich support will mean Immich *jobs and
  storage*, not photo browsing.

That constraint is what keeps the adapter interface small enough to remain an adapter
interface, and what keeps CueSeek from collapsing into a lowest-common-denominator
link-grid.

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
                                    ├─ jellyfin    ──HTTP──▶ Jellyfin
                                    └─ qbittorrent ──HTTP──▶ qBittorrent
```

Four properties worth calling out:

**The agent never runs as root.** It holds no special privileges of its own. A shipped
polkit rule grants the `cueseek` user exactly `RestartUnit` on an allowlist of units plus
`Reboot`/`PowerOff` via logind — nothing else. One rule file states the complete ceiling of
what CueSeek can do to your machine, and you can read it in under a minute.
See [ADR-0002](docs/adr/0002-host-privilege-dbus-polkit.md).

**The spec is the source of truth.** `api/openapi.yaml` is hand-authored. The Go server
interfaces and every client SDK are generated from it, and CI fails if they drift. The
spec is not a description of what the agent happens to do.
See [ADR-0004](docs/adr/0004-contract-openapi-sse.md).

**Services declare capabilities; clients render capabilities.** There is no fat `Adapter`
interface, and no `switch (serviceId)` in any client. A service advertises
`health`, `control`, `now_playing`, `transfers`… and each client decides what those look
like on its form factor. A phone card and a watch tile are the same capability rendered
differently. See [ADR-0005](docs/adr/0005-capability-based-adapters.md).

**Every device is paired separately and scoped separately.** Tokens carry
`read` / `service.control` / `host.power` as independent grants, so a watch can restart
Jellyfin while being structurally incapable of powering off the server.
See [ADR-0006](docs/adr/0006-device-pairing-scoped-tokens.md).

### Access model

CueSeek assumes your client can already reach the host — over Tailscale, WireGuard or your
LAN. It deliberately does **not** implement NAT traversal, a cloud relay, TLS termination
or certificate management, and it should never be exposed directly to the internet.

This is the same guidance Home Assistant and Immich give for their own remote access. It
also means the largest attack surface — a public endpoint that can power off a machine —
simply does not exist. See [ADR-0001](docs/adr/0001-vpn-only-remote-access.md).

---

## Repository layout

| Path | Contents |
| --- | --- |
| `api/openapi.yaml` | The contract. Single source of truth for every client and the agent. |
| `agent/` | `cueseekd`, the Go host agent. |
| `agent/internal/api/` | HTTP + SSE transport, auth middleware, scope enforcement. |
| `agent/internal/adapters/` | Capability interfaces, adapter registry, per-service adapters. |
| `agent/internal/health/` | Derives overall status from unit state, reachability, metrics. |
| `agent/internal/host/` | `HostController`; systemd/logind over D-Bus. |
| `agent/internal/store/` | SQLite: device registry, token hashes, audit log. |
| `clients/android/` | Android phone client (Compose). |
| `clients/wear/` | Wear OS client. |
| `deploy/` | systemd unit, polkit rule, install script, packaging. |
| `docs/adr/` | Architecture Decision Records. Start here. |

`api/` sits outside `agent/` on purpose. The spec is not the agent's — it is the contract
both sides bind to, and nesting it under the server would quietly make the server its
owner.

---

## Design decisions

Every significant decision is recorded as an ADR with its rationale **and its accepted
cost**. If you read one thing in this repository, read [`docs/adr/`](docs/adr/).

| ADR | Decision |
| --- | --- |
| [0001](docs/adr/0001-vpn-only-remote-access.md) | VPN-only remote access; no relay, no public exposure |
| [0002](docs/adr/0002-host-privilege-dbus-polkit.md) | Unprivileged agent; systemd/logind via D-Bus + polkit |
| [0003](docs/adr/0003-agent-runtime-go.md) | Go for the agent |
| [0004](docs/adr/0004-contract-openapi-sse.md) | Spec-first OpenAPI (3.0.3) + SSE |
| [0005](docs/adr/0005-capability-based-adapters.md) | Capability interfaces with runtime discovery |
| [0006](docs/adr/0006-device-pairing-scoped-tokens.md) | Device pairing with per-device scoped tokens |
| [0007](docs/adr/0007-client-capability-registry.md) | Client-side capability registry, not server-driven UI |
| [0008](docs/adr/0008-multi-host-and-computed-health.md) | Multi-host data model; agent computes health |
| [0009](docs/adr/0009-monorepo-contract-drift-gate.md) | Monorepo with a contract-drift CI gate |
| [0010](docs/adr/0010-design-system-m3-expressive.md) | Material 3 Expressive + owned token layer |
| [0011](docs/adr/0011-sequencing-spike-then-slice.md) | De-risking spike, then thin end-to-end slice |
| [0012](docs/adr/0012-alerting-vs-vpn-only-access.md) | *(Proposed)* Alerting reopens the access model |

---

## Roadmap

| Milestone | Scope | Status |
| --- | --- | --- |
| **Setup** | Repository skeleton, ADRs, contract placeholder | ✅ Done |
| **M0** | Architecture validation spike: polkit + D-Bus, and SSE over a tailnet | 🟡 polkit/D-Bus proven — [findings](docs/m0-findings.md); SSE test (A7) outstanding |
| **M1** | Agent: pairing, scoped tokens, Jellyfin health + restart | 🟡 in progress — contract and CI drift gate landed |
| **M2** | Android client: pair by QR, one capability-driven card, one action | ⬜ |
| **M3** | qBittorrent, `now_playing`, host metrics, power actions, design system | ⬜ |
| **M4** | Wear OS standalone client, tiles and complications | ⬜ |
| **M5** | A third adapter, used to measure whether the abstraction held | ⬜ |

**Setup** is unnumbered on purpose: it produced structure and decisions, but no behaviour.
Numbering starts where the software does.

**M0 runs before the agent, deliberately.** It is throwaway code whose only job is to prove
that a polkit rule really does grant an unprivileged daemon `RestartUnit` and `PowerOff` on
a real machine. If that assumption were wrong, several decisions would change — and the
cheapest moment to discover it is before anything depends on it.

---

## Development

**Requirements**

- Go 1.24+ — the agent
- JDK 17+ and Android Studio — the clients
- A Linux host running systemd — the agent's only supported target today
- Tailscale, WireGuard or LAN connectivity between client and host

```bash
git clone https://github.com/Kushal-MR/CueSeek.git
```

```bash
cd agent && go build ./...
```

**Platform support.** The agent targets systemd-based Linux. This is a real limitation, not
an oversight: `RestartUnit`, `Reboot` and `PowerOff` are systemd/logind operations. The
`HostController` interface exists so that an OpenRC, BSD or macOS backend is a swap rather
than a rewrite, but none is implemented.

## License

Not yet chosen.
