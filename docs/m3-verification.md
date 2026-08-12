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
