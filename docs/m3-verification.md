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

## M3.4 — qBittorrent adapter ✅

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

### Real-device acceptance ✅ — 2026-08-25

Against the real qBittorrent 
(`qbittorrent.service`, Web UI on 8080, localhost auth bypass **off**, so the agent
authenticates with `username` + `password_file`) on `kushal-HP-paviliong6`, from the OnePlus
CPH2707 over Tailscale.

**The APK was not rebuilt.** The build on the phone was compiled on 2026-08-13, twelve days
before the qBittorrent adapter existed, and the last commit touching client code is
`7ceb24f` from that same session. Everything below is that binary. Reinstalling would have
destroyed the only interesting thing about this test.

| # | Behaviour | Result |
| --- | --- | --- |
| 1 | qBittorrent appears in the roster | "qBittorrent / Healthy"; the tally reads 2 and both rows sit in the one existing surface |
| 2 | The row has the same two targets as Jellyfin | Body `[28,868]–[1062,1106]`, actions `[1062,903]–[1230,1071]` — 48dp, adjacent, non-overlapping |
| 3 | Semantics are generated from agent data | "qBittorrent, Healthy" and "Actions for qBittorrent" |
| 4 | ⋮ lists the discovered lifecycle actions | **Restart qBittorrent** neutral, **Stop qBittorrent** in the error role |
| 5 | The row body opens the service's own interface | Chrome opened `http://100.92.18.125:8080/` and qBittorrent's login page rendered |
| 6 | The URL is composed, not received | The tailnet address the phone paired with, never the agent's `127.0.0.1` |
| 7 | **The service is actually usable from the phone** | Logged in and added torrents, which downloaded |
| 8 | Restart, Stop and Start behave as on Jellyfin | Stop required the hold; Start replaced it once the unit was inactive |
| 9 | On-demand refresh covers a two-service fleet | One pull updated both rows |

**Item 7 is the product claim.** CueSeek gets you to the service; qBittorrent manages the
torrents. The `web_ui` capability is what makes that division work rather than being an
excuse for a missing feature.

**Item 1 is the architectural claim.** A build that predates the adapter rendered the new
service, its health, its capabilities and its actions with no release, no contract version
bump and no screen edited.

`connection_status` was `connected` throughout. Read from the row rather than from the API:
the supporting line showed the status label, and the roster shows a reason there whenever
one exists — so the reason list was empty, which under this adapter's mapping happens only
for `connected`. `firewalled` and `disconnected` were therefore **not** exercised on real
hardware; both are covered by unit tests only.

### Not claimed

- **`firewalled` and `disconnected` have not been seen on real hardware.** The host stayed
  connected throughout, so the reason-without-a-downgrade behaviour that makes `firewalled`
  interesting is asserted by test only.
- **The expired-session re-login has not been observed against the real service.** It needs
  a session to age out, which takes longer than an acceptance run. Covered by
  `TestExpiredSessionIsRetriedNotReported`.
- The **`transfers` capability** is M3.5. This adapter reports service health only; the
  per-torrent list is deliberately absent.
- `-race` did not run locally (no cgo on the development machine); CI runs it.
- **`agent_version` still reports `0.0.0-dev`** on the deployed binary. `main.go` declares it
  for ldflags injection and the documented build command does not set it, so no deployment
  can be told apart from another through the API. Noticed during this run; a packaging
  concern rather than an M3.4 one, and deliberately left alone.

---

## M3.5 — Activity: `transfers` and `now_playing` ✅

Verified on the OnePlus CPH2707 over Tailscale against `kushal-HP-paviliong6`, agent from
commit `9d66267`, with real playback and a real torrent library.

| # | Behaviour | Result |
| --- | --- | --- |
| 1 | `now_playing` renders live | "1 session · direct play" · 3 Idiots · "2009 · kushal · Chrome" · "1:37:33 / 2:51:07 · paused" |
| 2 | **Transcode detection, both branches** | An incompatible format played to the phone reported **1 transcoding** alongside a direct-play session on the laptop — 2 sessions, 1 transcoding |
| 3 | Subtitle picks the right source | "2009" for a film, from `ProductionYear`; the series branch is covered by test |
| 4 | `transfers` renders live | Real names, progress, rates, ETAs and sizes across 11 torrents |
| 5 | **State crosses verbatim** | `downloading`, `uploading`, `stalledUP` and **`missingFiles`** — the last is a state the agent never enumerated and it passed through untouched |
| 6 | Aggregate rates, not summed | "↓ 11.5 MB/s ↑ 234 kB/s" from `/transfer/info`, matching qBittorrent's own display |
| 7 | Idle says nothing | With every torrent finished, the row reads "Healthy" rather than "0 active" |
| 8 | Sample cap and remainder | "and 1 more transfer" at 11 torrents against a cap of 10 |
| 9 | **Ordering** | After the ranking fix, the most recently added torrent leads; before it, it was invisible |
| 10 | Stale | Headline collapses to "—", progress rules go achromatic, `client_stale` reason shown |
| 11 | Dark and light | Both correct; sage rules legible on each surface |
| 12 | Row semantics match the display | "Jellyfin, 1 playing" |

### Six defects, all found by using it

None of these were caught by the 31 tests written for the phase, because each one is about
how the pieces meet rather than about what any of them computes.

1. **The renderers had no route.** `CapabilitySection` draws only in the detail sheet, and
   the sheet opened only for a service *without* a `web_ui`. On a host where every service
   has one — which is this host — `now_playing` and `transfers` could not be reached at all.
2. **`web_ui` had no renderer**, so the sheet said "Update CueSeek to view this" for a
   capability supported since M3.2. Invisible until the sheet became reachable.
3. **The sheet could not scroll.** Content past the fold was clipped rather than below the
   fold, so the tail of any transfers list was unreachable.
4. **The facts line clipped to one line**, silently dropping the size: "11m …".
5. **Ordering ranked only downloads.** `sort=dlspeed` ties everything seeding, paused or
   finished at zero, so past the first item the order was arbitrary — a torrent that
   finished a minute ago dropped to zero and vanished from the sample. Which one fell off
   was luck.
6. **Row semantics diverged from the row.** The activity line changed the visible text but
   not the `contentDescription`, so a screen reader said "Healthy" while the screen said
   "1 playing".

Defect 1 is the one worth keeping. The unit tests all passed throughout, because they test
the mapping and not the way in — a renderer can be correct, registered, and unreachable.

### Changed as a result

- **Activity moved onto the service row**, which is where a glanceable fact belongs. A
  health reason still outranks it, and an idle service says nothing at all.
- **The row and the ⋮ menu swapped jobs.** The body opens the detail; the menu opens the web
  interface. Recorded in [`m3-plan.md`](m3-plan.md) and [`DESIGN.md`](DESIGN.md), both of
  which described the old model.
- **The sample cap went from 10 to 20**, since the sheet now scrolls.

### Not claimed

- **"and N more" has not been re-verified at the new cap of 20.** It was confirmed at 10;
  exercising it now needs 21 or more torrents.
- **A library larger than `torrentsFetchLimit` (200)** was not tested. Beyond that `total`
  understates, and the oldest torrents are dropped before ranking.
- `-race` did not run locally (no cgo on the development machine); CI runs it.

---

## M3.6 — Host metrics ✅

Verified on `kushal-HP-paviliong6` (Ubuntu, kernel 7.0.0-28, 4 cores, 3.8 GiB, one ext4
root) and the OnePlus CPH2707 over Tailscale, against an agent cross-compiled from the
uncommitted M3.6 branch and reporting `agent_version=m3.6-uncommitted`.

Driven over SSH with key auth rather than by hand; only the four privileged steps —
installing the binary, the unit, the config and minting a pairing code — were run by the
operator.

| # | Behaviour | Result |
| --- | --- | --- |
| 1 | Collector starts | `host metrics started interval=10s mounts=[/]` |
| 2 | `GET /v1/host/metrics` | 200 with cpu, memory, storage, thermal and uptime |
| 3 | **Memory matches the kernel** | `total_bytes` = 3894936 kB exactly; `available_bytes` tracks `MemAvailable`, not `MemFree` |
| 4 | **Storage matches `df`** | 490577010688 / 174561456128 bytes, byte-identical to `df -B1 --output=size,avail`, confirming `Bavail` excludes the root reserve |
| 5 | **CPU tracks real load** | 2–3% idle → **100.00%** with four cores pinned → 1.6% after; `load1` still 1.41 while usage had fallen to 1.58%, which is the pair measuring different things, observed |
| 6 | Utilisation on the first payload | Present, not absent — the priming sample closes the window to one second |
| 7 | Thermals, labelled | `coretemp Package id 0` / `Core 0` / `Core 1` at their own 87°C `_max`, plus unlabelled `acpitz` falling back to the chip name |
| 8 | `_max` preferred over `_crit` | 87 reported, not 105 |
| 9 | `host_updated` on the stream | Every 10s, interleaved with `service_updated` at 30s and heartbeats at 15s, one monotonic `seq` |
| 10 | Snapshot carries metrics | `snapshot.host_metrics` present at `seq` 0 |
| 11 | **Pull-to-refresh does not blank the strip** | Values updated in place; the read endpoint is what makes this true |
| 12 | **Stale drops the strip entirely** | Wi-Fi off 40s → "Unverified", dashed hairline, "Stream open · no data 40s", services degraded to `unknown` keeping "Last verified 12:05:53" — and no vitals at all |
| 13 | Dark and light | Both correct; meter tracks and fills legible on each surface |
| 14 | M3.5 unaffected | Detail sheet still opens from the row body with health, controls, web interface and now-playing |

### Forward compatibility, observed a third time

Before upgrading the phone, the **M3.5 build** was left running against the M3.6 agent for
several minutes. It stayed live and correct while receiving a `host_updated` event every ten
seconds that it had never heard of, treating each as `Unrecognised` — traffic that resets
the freshness clock and renders nothing (ADR-0007). After M3.1's Start action and M3.4's
whole new service, this is the third time the skew contract has held without a client
rebuild, and the first time in the direction of a new *event type*.

### Two defects, both found by using it

1. **`ProcSubset=pid` hid the metrics from the agent.** The first live payload carried
   storage and all four sensors and omitted CPU, memory and uptime. The unit's own
   hardening mounts `/proc` with `subset=pid`, which hides `/proc/stat`, `/proc/meminfo`,
   `/proc/loadavg` and `/proc/uptime` — and is exactly why `/proc/self/mounts` still
   resolved the device name, since that one is per-process. The files are `-r--r--r--`, so
   permissions were ruled out first.

   The code was not wrong: an unreadable source produced an absent field rather than a
   fabricated zero, which is the rule this payload is built on. It was the deployment that
   made the feature useless, and no unit test could have found it because the sandbox does
   not exist in a test.

   Fixed by `ProcSubset=all` in `deploy/cueseekd.service`, with the cost written into the
   file. `ProtectProc=invisible` is kept, so other users' processes stay hidden;
   `/proc/sys` stays read-only and `/proc/kmsg` stays gone. systemd offers no setting
   between `pid` and `all`.

2. **The footnote wrapped, stranding a word.** "45°C coretemp Core 0" pushed the line to
   two, leaving "Core 0" alone on the second — while "45°C acpitz" on the same machine
   fitted. Which sensor is hottest changes minute to minute, so the layout broke and healed
   on its own, which is the worst kind of defect to meet later. The footnote now shows the
   chip (`coretemp`) and screen readers still get the full label.

### Follow-up round

Four of the five items below were open after the first pass. Three were closed by testing
the machine as it is, rather than by inventing hardware it does not have.

| # | Behaviour | Result |
| --- | --- | --- |
| 15 | **204 when there is nothing to report** | `enabled: false`, restart → `HTTP 204`, body **0 bytes**, journal says "host metrics disabled by configuration" |
| 16 | 204 end to end | The strip **left the phone** while collection was off and returned when it was restored — absent rendering as nothing, not as zeroes |
| 17 | **Two watched filesystems** | `storage_mounts: ["/", "/srv/cueseek-vitals"]`, both reported |
| 18 | **Fullest wins** | At 0% the strip showed `/` (64%); at 88% it switched to the tmpfs; emptying it switched back |
| 19 | **Pressure at ≥85%** | 87.5% → amber rule |
| 20 | **Pressure at ≥95%** | 96.9% → red rule |
| 21 | Long mount labels | `/srv/cueseek-vitals` ellipsises to `/srv/cues…` and keeps its gap from the value |

The 204 was produced deliberately rather than raced. The natural window on a running agent
is the one second between the priming sample and the first publish, which is not a thing to
sit and wait for; switching collection off reaches the same state and is the case an
operator can actually cause.

The thresholds were crossed on a **64 MB tmpfs** added as a second watched filesystem, not
by filling the root disk. Reaching 85% of 457 GB means writing about 215 GB to the disk the
media library lives on, which is slow and genuinely risky for no extra coverage — a tmpfs is
memory, vanishes on unmount, and crosses the identical code path. It also closed the
multiple-mounts and fullest-wins items, which one filesystem could never have exercised.

### Two more defects from the follow-up

7. **The vitals footnote belonged to nothing.** Uptime, load and temperature sat in one
   full-width dot-separated line under a three-column grid, aligned to none of it. Each
   column now carries its own second line — `load 0.1 of 4`, `3.1 GB free`, `175 GB free` —
   and only the two host-level facts remain, anchored left and right the way the provenance
   line above them is.

   The storage value became a percentage in the same change. It read "175 GB free" above a
   two-thirds-full bar, which made the one column most likely to matter the only one whose
   number and rule disagreed. Free bytes moved to the line beneath.

8. **A long mount label ran into its value**, rendering `/srv/cuese…88%` with no gap. Found
   the moment a mount point longer than `/` existed.

9. **The strip kept changing which sensor it was talking about.** `acpitz` (a chassis sensor
   with no stated limit) and `coretemp` (the CPU, limit 87°C) sit within a few degrees on
   this machine and traded places minute to minute, so picking the hottest made the label
   flicker between unrelated hardware while nothing was happening. It now ranks by how close
   each sensor is to **its own** limit and prefers sensors that state one — which is both
   the more useful question and, on ordinary hardware, a stable answer.

10. **"load 0.1 of 4" read as cores in use.** It is not: load average counts processes
    waiting to run or blocked on disk, can exceed the core count, and is a different
    measurement from the percentage above it — which is why both are shown. Now "0 queued",
    with the core count moved under the CPU meter where it reads as capacity.

11. **The host line was not on the grid.** Uptime and temperature spanned the full width
    anchored to opposite edges, so uptime sat under one column's left edge and the
    temperature against the screen edge — nothing lined up with anything and the line read
    as overflow. Both now sit in grid cells, left-aligned like the detail lines above them.

### Not claimed

- **No machine without sensors was tested.** The empty-versus-absent thermal distinction is
  asserted by tests only. This hardware has four working sensors and there is no honest way
  to remove them — pointing the collector at a fake `/sys` would be a worse unit test, not a
  real-device one.
- **Memory pressure was never crossed on real readings.** Deliberately: `pressureTint` is
  one function shared by memory and storage, so item 19 and item 20 exercise the same code.
  Filling 3.8 GB of RAM on a box running Jellyfin and qBittorrent risks the OOM killer
  taking a real service for no additional coverage.
- **A single sensor reading above its own `high_celsius`** has not been seen, so "Running
  hot" and the hot colour on the temperature are unit-tested only. This machine idles around
  45°C against an 87°C limit, and heating a laptop past 87°C to watch a label change is not
  a test worth running.
- **"Memory almost full" and "Memory under pressure"** were not seen on real readings, for
  the reason given above: they are the same `hostConcern` branch as the disk, and filling
  3.8 GB of RAM risks the OOM killer taking a real service.
- The `m36-probe` device (scope `read`) is still paired and its token is in `/tmp` on the
  host, which clears at reboot.

### Decided, not deferred

A filesystem at 97% turned its rule red while the headline above it still read "All good" —
the console being cheerful about the one thing on screen that was not. **Host pressure now
reaches the headline and stops there**: the verdict says "Disk almost full", while the tally
rule and the roster stay about services, because a machine is not one of its own services.

Ranked above `unknown` services on purpose. A filesystem at 97% is a fact somebody has to
act on; `unknown` is the absence of a fact. Services genuinely needing attention still
outrank both. CPU is deliberately never a complaint — a processor at 100% is a transcode
doing its job, and announcing it would cry wolf every time somebody watched a film.

The thresholds are now one pair of constants shared by the headline and the rule, rather
than two copies of 0.85, so the two can never disagree about whether something is wrong —
which is the same defect in a different form.

Verified on the phone against the same tmpfs:

| # | Behaviour | Result |
| --- | --- | --- |
| 22 | Headline at ≥85% | "Disk filling up", beside an amber rule |
| 23 | Headline at ≥95% | "Disk almost full", beside a red rule |
| 24 | Headline and rule agree | Both crossed on the same reading, which is what sharing the constants buys |
| 25 | It recovers | Emptying the filesystem returned the headline to "Operational" and the strip to `/` — the state is derived, not latched |
| 26 | Services are untouched | Tally stayed fully green and "✓ 2" throughout, and both rows stayed "Running" |

Two words changed with it: the healthy verdict is **"Operational"** rather than "All good",
and service rows read **"Running"** rather than "Healthy". Display labels only — the
contract's `healthy` / `degraded` / `unreachable` / `unknown` vocabulary is public API and
did not move.

---

## M3.7 — Host power actions ⏳ reboot verified, power-off outstanding

Verified on `kushal-HP-paviliong6` and the OnePlus CPH2707 over Tailscale, agent
`m3.7-35a6998`, with the polkit power block enabled for the first time.

| # | Behaviour | Result |
| --- | --- | --- |
| 1 | Endpoints absent before the upgrade | `GET` and `POST /v1/host/actions*` → 404 on the M3.6 agent |
| 2 | Polkit rule installed | All four actions, diff printed before applying; polkit reloaded cleanly |
| 3 | Actions listed at `read` scope | Both, `destructive`, with their consequences |
| 4 | **Invoking without `host.power` → 403** | *"this operation requires the `host.power` scope; this device holds read"* — and the machine was untouched |
| 5 | Menu before re-pairing | Both items greyed, with "This device was not paired with permission to power the machine" |
| 6 | **Re-pairing is required and works** | Forget → pair with a code granting `host.power`; the agent logged the new scopes |
| 7 | Menu after re-pairing | Both items enabled, explanatory line gone |
| 8 | Confirmation with nothing active | **No "Right now:" line** — 12 torrents present, none active, so none counted as work |
| 9 | **Confirmation with work in flight** | "Right now: 1 stream playing." in amber, above the hold bar |
| 10 | Cancel does nothing | Verified against uptime and the journal: no power line, no boot change |
| 11 | A tap is not a hold | The press-and-hold bar ignored taps throughout; only a sustained hold fired |
| 12 | **Reboot** | Held → machine went fully unreachable → back in under a minute |
| 13 | **It was a real reboot** | Kernel `boot_id` changed `a1ea5cb5…` → `f77a0e18…`, boot time 11:43 → 18:02, uptime 6h18m → 0m. A service restart cannot change a boot id |
| 14 | Services returned unaided | `cueseekd`, `jellyfin`, `qbittorrent` all active after boot, no intervention |
| 15 | The phone recovered on its own | Went stale, reconnected, and showed "up 6m" — the machine's new uptime, from the client that caused it |

### Acknowledge-before-acting, observed

```
18:01:43  host power action accepted         action=reboot device=CPH2707
18:01:44  host power action handed to logind action=reboot
18:02     system boot
```

One second between accepting and calling logind, which is the 750 ms send window plus
rounding. The 202 was written and delivered before the machine was touched — the property
this endpoint exists to get right, and the one that cannot be tested by unit tests alone
because the failure mode is a response that never arrives.

The device is **named** in the accept line. Once the machine goes down that journal entry is
the only surviving record of who asked, which is why it is logged at warn rather than info.

### No defects

Nothing was found that needed fixing. Unusual for this project, and worth stating plainly
rather than implying the testing was thin: every mechanism M3.7 uses — the risk ladder, the
press-and-hold, the scope check, the 202-plus-id shape — had already been built and exercised
by earlier phases. M3.7 connected them. The parts that were genuinely new are the logind call
and the polkit grant, and both are small enough to be right or obviously wrong.

One process note: an SSH outage midway through was initially read as a reboot in progress. It
was not — `who -b` showed continuous uptime, and the cause was a connection made by raw
address rather than through the alias carrying the key. Recorded because the wrong reading
was briefly stated as fact.

### Not verified

- **Power off has not been run.** It needs physical access to undo, so it is deliberately
  the last thing tested and is not claimed. Everything up to and including the confirmation
  dialog is verified; only the `PowerOff` D-Bus call itself is untested, and it differs from
  the verified `Reboot` path by one method name.
- **The `-multiple-sessions` variants** were not exercised: nobody was logged in at the
  console. They are granted, and the failure they prevent only appears when somebody is.
- **`host_action_progress`** has never been seen on the wire, because no power action has
  failed. It is unit-tested on both sides. Producing one live would mean deliberately
  breaking the polkit rule.
