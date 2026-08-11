# CueSeek

**An operations console for self-hosted home servers.**

CueSeek gives you one consistent way to answer *"is everything fine?"* — and to do
something about it when the answer is no — across every service running on your box.

It talks to Jellyfin, qBittorrent and others through their APIs, and adds the host-level
control they cannot provide themselves.

> **Status: M2 complete.** The agent and the Android client both work, and the whole path —
> phone → Tailscale → `cueseekd` → polkit → systemd — has been verified end to end on real
> hardware. See [What works today](#what-works-today).

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

## What works today

Two milestones are complete. Both were verified on real hardware against the real agent,
not against mocks — the record is in [`docs/m2-p6-verification.md`](docs/m2-p6-verification.md).

**The agent — `cueseekd`.** Runs as an unprivileged `cueseek` user on a systemd host.
Serves the REST contract and an SSE event stream, stores paired devices and hashed tokens
in SQLite, polls Jellyfin for health on its own schedule, and restarts units through polkit
without ever holding root.

**The Android client.** Pair by entering the host's address and a single-use code printed
by `cueseekd pair`. A live dashboard shows every service's health, updated over SSE. Tap a
service to see its detail and restart it; the outcome arrives as a stream event rather than
being assumed from the acknowledgement.

Verified end to end over Tailscale, on a phone and a real Linux host:

| | |
| --- | --- |
| **Pair a device** | Real single-use code redeemed over the tailnet; replaying it is refused |
| **See health** | Matches `systemctl` — two independent measurements, the adapter polling Jellyfin's API and systemd watching the process |
| **Restart a service** | Through polkit; `ActiveEnterTimestamp` moved, and the outcome arrived over the stream *after* systemd recorded it |
| **Never be confidently wrong** | Agent suspended with its sockets intact: the app reported `Unverified` and "Stream open" side by side while the data aged |
| **Recover** | From a VPN outage, a Wi-Fi↔cellular switch, a locked phone, a starved stream and a host reboot |

The fourth row is the one the architecture exists for. A client that trusted its transport
would have shown a healthy green service two minutes after the agent stopped speaking.

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
`read` / `service.control` / `devices.manage` / `host.power` as independent grants, so a
watch can restart Jellyfin while being structurally incapable of powering off the server
or of revoking your phone.
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
| `agent/internal/domain/` | Shared vocabulary: scopes, devices, audit entries. Depends on nothing. |
| `agent/internal/config/` | Configuration loading and validation, incl. the managed-unit allowlist. |
| `agent/internal/api/` | HTTP + SSE transport, auth middleware, scope enforcement. |
| `agent/internal/adapters/` | Capability interfaces, adapter registry, per-service adapters. |
| `agent/internal/health/` | Derives overall status from unit state, reachability, metrics. |
| `agent/internal/host/` | `HostController`; systemd/logind over D-Bus. |
| `agent/internal/store/` | SQLite: device registry, token hashes, audit log. |
| `clients/android/` | Android phone client (Compose). |
| `clients/wear/` | Wear OS client. Placeholder until M4. |
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
| [0013](docs/adr/0013-android-client-architecture.md) | Four shared `core` modules; features are packages |

---

## Roadmap

| Milestone | Scope | Status |
| --- | --- | --- |
| **Setup** | Repository skeleton, ADRs, contract placeholder | ✅ Done |
| **M0** | Architecture validation spike: polkit + D-Bus, and SSE over a tailnet | ✅ Done — [findings](docs/m0-findings.md). A7 closed: SSE viable, but Doze freezes it silently rather than killing it (ADR-0004 Amendment 2) |
| **M1** | Agent: pairing, scoped tokens, Jellyfin health + restart | ✅ Done — contract, store, API, host control, adapters, SSE stream and [deployment](deploy/) |
| **M2** | Android client: pair by entering host address + code, capability-driven dashboard, one action | ✅ Done — verified end to end over Tailscale against the real agent ([record](docs/m2-p6-verification.md)) |
| **M3** | qBittorrent, `now_playing`, host metrics, power actions | ⬜ |
| **M4** | Wear OS standalone client, tiles and complications | ⬜ |
| **M5** | A third adapter, used to measure whether the abstraction held | ⬜ |

**Setup** is unnumbered on purpose: it produced structure and decisions, but no behaviour.
Numbering starts where the software does.

**Phases within a milestone are numbered `M<n>.<k>`** — `M1.1` … `M1.8`, and `M3.1` onward.
M2's commits use `M2 P0` … `M2 P6`, an earlier variant of the same idea, kept as-is because
that history is merged and public.

**M0 runs before the agent, deliberately.** It is throwaway code whose only job is to prove
that a polkit rule really does grant an unprivileged daemon `RestartUnit` and `PowerOff` on
a real machine. If that assumption were wrong, several decisions would change — and the
cheapest moment to discover it is before anything depends on it.

**M2 pairs by typed code, not by QR.** This milestone previously said "pair by QR". The
agent emits no QR and no payload format exists on either side, so shipping a scanner would
have meant adding server work in service of a client convenience, inside the milestone
whose entire purpose is to prove the path end to end. The payload format is recorded as
future work in [ADR-0006, Amendment 3](docs/adr/0006-device-pairing-scoped-tokens.md).

---

## Development

**Requirements**

- Go 1.24+ — the agent
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

**Platform support.** The agent targets systemd-based Linux. This is a real limitation, not
an oversight: `RestartUnit`, `Reboot` and `PowerOff` are systemd/logind operations. The
`HostController` interface exists so that an OpenRC, BSD or macOS backend is a swap rather
than a rewrite, but none is implemented.

## License

Not yet chosen.
