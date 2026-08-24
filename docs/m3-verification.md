# M3 verification record

What each M3 phase proved, on real hardware, and what it did not. Kept phase by phase as
they land, the same way [`m2-p6-verification.md`](m2-p6-verification.md) was kept for M2.

Environment throughout: `cueseekd` on `kushal-HP-paviliong6`, bound to its tailnet address,
reached from a OnePlus CPH2707 over Tailscale.

---

## M3.1 — Service lifecycle: Start and Stop ✅

Deployed to the real host via `install.sh --binary`, which replaced the binary and the
polkit rule and left the config and the paired-devices database untouched.

| # | Behaviour | Result |
| --- | --- | --- |
| 1 | Stopping the unit from the host changes what the agent advertises | `systemctl stop jellyfin` → the app offered **Start** |
| 2 | **The existing Android client discovers Start with no rebuild** | The M2 build, compiled before `start` existed, rendered and invoked it |
| 3 | Start from the app reaches systemd through polkit | Jellyfin started successfully |
| 4 | The host agrees it happened | `systemctl is-active` reported active, with a **new `ActiveEnterTimestamp`** |
| 5 | Health recovers on its own | The app returned to Healthy on the next poll |
| 6 | Stop is available on a running service | Offered alongside Restart, gated as destructive |

Item 2 is the one worth keeping. `Action` is `{id, label, risk, description}` — data, not
schema — so adding two verbs needed no contract change, no regenerated types and no client
release. An Android build produced before the feature existed rendered it, gated it on the
risk level the agent assigned, and invoked it correctly.

That is ADR-0005's claim tested rather than asserted, and it is the same property that will
make qBittorrent's lifecycle free in M3.4.

### Verified before deployment

- Full agent suite green; `gofmt` and `go vet` clean on the host platform **and** under
  `GOOS=linux`.
- The systemd backend cross-compiles for the deployment target — worth stating because
  `systemd_linux.go` is behind a build tag and is never compiled on a Windows developer
  machine unless forced.
- No contract drift, confirming actions are data.
- New host tests assert that `start` and `stop` refuse an unlisted unit **before** the
  backend is called, not merely that they return an error. The widening was to the verbs;
  the unit allowlist is unchanged.

### Not claimed

- **Recovery of a stopped service across a reboot** was reasoned about — `enable` and
  `disable` remain outside the agent's ceiling, so a stopped unit stays enabled and returns
  on the next boot — but was not exercised on the host.
- **polkit refusing an unlisted unit at the D-Bus layer** was not re-tested here. The
  agent's own allowlist refuses first, which is what the tests cover; the second layer was
  last exercised in M0.

### One unexplained observation

During the acceptance run the phone entered the Unverified state repeatedly, roughly every
30 seconds, recovering whenever an action or poll delivered an event. It then stopped
happening with no change to the agent, the host or any code, and has not returned.

Recorded rather than dropped, because a freshness bug is the one class of defect this
client exists to prevent, and a symptom that vanishes on its own is not the same as a
symptom explained.

What was established while it was happening:

- The agent's event emission is not at fault. An identical build, run locally against a
  stub, emitted heartbeats every 15s and `service_updated` every poll for 70 seconds
  without a gap.
- The heartbeat interval is a fixed 15s (`server.go`), so a 30s silence means heartbeats
  were not reaching the client rather than being emitted late.
- No code was changed in response. Inventing a fix for a symptom whose cause was never
  found would risk making the display *look* fresh without it being fresh, which is worse
  than the bug.

If it recurs, the three diagnostics that would settle it are: whether the phone reads
"Stream open" or "Reconnecting", the raw event stream observed from the host with `curl -N`,
and whether `journalctl -u cueseekd` shows `stream subscriber fell behind`.

---

## M3.3 — Android: the row interaction model ✅

The service row became two targets. Its body *uses* the service; the trailing `⋮` *operates
on* it. The dashboard was rebuilt onto the phone and exercised against the live agent on
`kushal-HP-paviliong6` over Tailscale.

| # | Behaviour | Result |
| --- | --- | --- |
| 1 | The row exposes exactly two independent targets | Accessibility dump: body `[28,626]–[1062,864]`, button `[1062,661]–[1230,829]` — adjacent, non-overlapping |
| 2 | The trailing target meets Material's minimum | 168px at 3.5× = **48dp square**, with the glyph at 18dp inside it |
| 3 | The two are announced differently | "Jellyfin, Healthy" and "Actions for Jellyfin" |
| 4 | The menu is built from the agent's actions, not from the service's identity | Offered **Restart Jellyfin** and **Stop Jellyfin**, the two verbs the agent advertised |
| 5 | Risk gating survives the move off the sheet | Stop rendered in the error role, and opening it produced the hold-to-confirm dialog carrying the agent's own consequence text |
| 6 | A service with no `web_ui` falls back to detail rather than to nothing | Body tap opened the sheet, showing health, its reason, and the `control` capability |

Item 6 was observed *before* the host was given a `web_ui` block, which is why it is the
fallback that is recorded here and the browser path that is recorded under M3.2 below.

### Found on the device, not in a test

- **"Stop Jellyfin Jellyfin?"** — the confirmation title appended the service name to a label
  that already contained it. The agent owns that copy; the dialog now prints it verbatim.
- **The sheet duplicated the menu.** With actions in both, one screen offered the same two
  verbs three times over: the `control` capability's summary, a pair of buttons, and the new
  menu. The sheet is now detail-only, so a destructive verb has exactly one entry point to
  keep gated correctly.

Neither was visible from the source or from a unit test, which is the same lesson as M2's
UTC timestamp bug: some defects only exist once the thing is in your hand.

### Fixed in passing

The row previously used `clearAndSetSemantics`, which collapses the row to one node **and
erases the clickable's action** along with it. It read correctly to TalkBack while having
nothing left to activate. It now merges instead, so the node keeps its action and announces
the `onClickLabel` — which names the actual destination, browser or sheet, since the two
differ enough that guessing would mislead exactly the users who depend on it.

---

## M3.2 — Contract and agent: `web_ui` ✅

Out of numerical order because that is the order it happened in. M3.2 shipped the contract
and the agent side; nothing consumed it until M3.3 existed, so the two were accepted
together on the real host. M3.2 is what the agent says, M3.3 is what the phone does with it,
and neither is worth much demonstrated alone.

The `web_ui` block was added to Jellyfin's entry in the host's existing
`/etc/cueseek/config.yaml` — `scheme: http`, `port: 8096`, `path: /` — alongside the
M3.2 binary. The config, the pairing database and the Jellyfin credential were left in
place. Reading `/v1/services` from the host confirmed Jellyfin advertising the `web_ui`
capability and carrying the block through to the wire.

### Real-device acceptance ✅

Run on the OnePlus CPH2707 over Tailscale against `kushal-HP-paviliong6`.

| # | Behaviour | Result |
| --- | --- | --- |
| 1 | The agent advertises the capability | Jellyfin carried `web_ui` |
| 2 | The row's body opens the service itself | Tapping it launched the phone's browser |
| 3 | **The URL is composed, not received** | It resolved through the paired Tailscale address, not a hardcoded LAN address the agent could have supplied |
| 4 | The destination is real | Jellyfin's web interface loaded and was usable from the phone |
| 5 | The two targets stay separate | Tapping `⋮` opened no browser |
| 6 | Lifecycle actions remain behind the menu | Restart Jellyfin and Stop Jellyfin, listed separately |
| 7 | Risk gating survived the move off the sheet | Stop still required a sustained hold |

Item 3 is the one worth keeping. The agent polls Jellyfin at `127.0.0.1:8096`, an address
that means nothing to a phone, and it never sends an origin at all. What reached the browser
was the tailnet host this device paired with, joined to the port and path the agent
configured — so the same single configuration would work unchanged on the LAN, and a wrong
or compromised agent has no field in which to put a host of its choosing.

Items 5 and 6 together are the interaction model tested rather than asserted: one row, two
meanings, and no path by which *using* a service can reach *operating on* it by accident.

### Not claimed

- **A service configured with `https`** was not exercised; the reference deployment is
  plain HTTP behind the VPN, per ADR-0001.
- **`ACTION_VIEW` falling through to a native app** was not exercised, and by design cannot
  be arranged from here: the intent carries no package, so the resolution is the phone's
  default-app setting rather than anything CueSeek decides.

---

## M3.3a — On-demand refresh ✅ *(see the caveat below)*

Deployed to the real host and exercised on the OnePlus CPH2707 over Tailscale on
2026-08-13, during the session that built it.

| # | Behaviour | Result |
| --- | --- | --- |
| 1 | The gesture reaches the agent | A manual pull returned Jellyfin at **`0s`** — the agent had observed on the spot, not replayed a cache |
| 2 | The pull has three legible states | Dim and partial below the threshold; full primary in one step at it; a band sweeping while the request is outstanding |
| 3 | Releasing below the threshold does nothing | Correct — no request, no indicator |
| 4 | The indicator does not collide with the header | Fixed after the first build parked it exactly on the tally rule; the page now travels with the pull |
| 5 | **A failed refresh moves no clock** | Taken offline, the screen stayed Unverified throughout — "no data 125s", Jellyfin still Unknown, last-verified timestamp unmoved |
| 6 | A hung refresh clears itself | The in-flight request ended on the 15s read timeout and the indicator cleared; it did not hang |
| 7 | Existing behaviour survives | Watchdog tripped correctly, dashed rule and "Stream open / no data" intact, stream reconnected on its own when the network returned |
| 8 | M3.3 did not regress | ⋮ still opened Restart and Stop; the row body still launched Jellyfin's web UI |

Item 5 is the one that matters. It was observed on hardware rather than only asserted in a
test, and it is the guarantee the whole feature is balanced on: **asking is not being
answered.** A pull that could clear the stale state on its own would make a dead agent look
alive, which is the single failure this client exists to prevent.

### Found on the device, not in a test

- **The indicator parked on top of the tally rule.** Material moves the indicator alone and
  leaves the content still, which suits a spinner on a card. It does not suit an indicator
  deliberately shaped like the rule already under the headline — the two overlapped and read
  as one broken bar. The page now travels with the pull.
- **The band looked stuck.** It was animating. A 40% block in a 96dp track travels barely
  57dp and spent half of each cycle going backwards, and reversal reads as waiting rather
  than progress. It now runs one direction only.

### Not claimed

- **The success path was never photographed.** On a healthy connection the round trip
  resolves in under 300ms — faster than `screencap` latency. The evidence that the request
  path runs is the offline test, where a refresh was visibly in flight for the full timeout.
- **A long, slow refresh** was not watched to see whether the band becomes tiring over the
  full 15s read timeout.
- **`-race` has never run locally** — the development machine has no cgo. CI runs it.

### ⚠️ Caveat — this record is second-hand relative to the repository

Everything above was observed during development and written down two weeks later, from a
session transcript rather than from a fresh run. The code, the tests and the ADR amendment
are all present and green in the repository; the *device observations* are not independently
reproducible from it.

Recorded this way rather than dropped, and rather than dressed up as a fresh acceptance run.
If M3.3a needs to be trusted for something load-bearing, re-run items 1, 5 and 7 — they take
about five minutes and are the three that carry the guarantees.

---

## M3.4 — qBittorrent adapter ✅ *(automated; not yet on the real host)*

The phase whose output is a **number** rather than a feature: ADR-0011 step 4 asks how many
files change outside a new adapter's own package, and turns "is the abstraction good?" into
something answerable.

### The measurement

| | |
| --- | --- |
| New package | `agent/internal/adapters/qbittorrent/` — adapter and tests |
| Production files changed outside it | **3** (`builtin.go`, `config.go`, `domain/health.go`) |
| Deployment/docs files changed | 1 (`deploy/config.example.yaml`) |
| **Contract changes** | **0** — `go generate ./...` produced no diff |
| **Android changes** | **0** — no file under `clients/` was touched |

Two of the three were anticipated by the plan. `domain/health.go` was not, and gained one
reason code for a service that is running and answering but cannot reach its peers.

### What that proves, and what it does not

**Proved.** A second service type reaches the phone with no client release, no contract
version bump, and no screen edit — through the same `control` and `web_ui` capabilities
Jellyfin already used. The registry, the poller, capability discovery, the on-demand
refresh, the ⋮ menu, the risk ladder and the freshness watchdog all required nothing.

The lifecycle implementation is **two sentences of copy**. Everything else — which actions
apply in which unit state, their risk levels, the hold-to-confirm classification, the
confirmation wording — comes from `adapters.AvailableLifecycleActions`.

**Not proved, and worth watching.** `config.Service` grew three fields because qBittorrent
authenticates with a login rather than an API key. That is a real difference in shape rather
than a leak, but it is the direction a leak would come from: **a third credential shape
should become a per-adapter options map, not a fourth pair of fields.**

### Verified by test

| # | Behaviour | Where |
| --- | --- | --- |
| 1 | Connection status maps to health, all five cases including an unrecognised one | `TestConnectionStatusMapping` |
| 2 | `firewalled` is a reason, **not** a status downgrade | `TestFirewalledIsAReasonNotAStatus` |
| 3 | `reported_status` crosses verbatim, in the service's own casing | `TestReportedStatusCrossesVerbatim` |
| 4 | No credentials configured → no login attempted (the localhost-bypass deployment) | `TestNoCredentialsMeansNoLogin` |
| 5 | An expired session is silently re-established, not reported as an auth failure | `TestExpiredSessionIsRetriedNotReported` |
| 6 | A rejected password is **degraded and reachable**, never unreachable | `TestBadCredentialsAreDegradedNotUnreachable` |
| 7 | A 403 with no credentials names qBittorrent's own bypass setting | `TestForbiddenWithoutCredentialsNamesTheSetting` |
| 8 | Errors never carry the password | `TestErrorsDoNotLeakCredentials` |
| 9 | Control advertised only when a unit **and** a host layer exist | `TestControlIsAdvertisedOnlyWhenPerformable` |
| 10 | Lifecycle actions and risk come from the shared descriptors | `TestLifecycleActionsComeFromTheSharedDescriptors` |
| 11 | Invoke goes through the host layer, never systemd directly | `TestInvokeDelegatesToHostLayer` |
| 12 | A mixed Jellyfin + qBittorrent fleet advertises per-configuration, not per-type | `TestMixedFleetBuildsAndAdvertisesPerAdapter` |

Item 12 is the one that carries the claim. The two services are deliberately asymmetric —
qBittorrent with a `web_ui` and no unit, Jellyfin with a unit and no `web_ui` — so if
capabilities were a property of the type rather than of the configuration, both would come
back identical.

### Found by the tests, before it shipped

A rejected password initially surfaced as **`unreachable`**. qBittorrent answers `200` with
the body `Fails.` for a bad credential, so the failure arrived as a transport-shaped error
and was classified as one. That is precisely the conflation ADR-0005 calls out — "check the
network" and "check the password" send an operator to different places. Fixed with a typed
`authError` and asserted by items 6 and 7.

### Not claimed

- **Nothing has run against a real qBittorrent.** Every observation above is against an
  `httptest` server shaped like qBittorrent's API. The real-host acceptance — install
  qBittorrent, configure the block, confirm the row appears with its ⋮ menu and its web UI —
  has **not** been performed.
- The **`transfers` capability** is M3.5. This adapter reports service health only; the
  per-torrent list is deliberately absent.
- `-race` did not run locally (no cgo on the development machine); CI runs it.
