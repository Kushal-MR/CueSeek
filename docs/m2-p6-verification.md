# M2 P6 — real-agent verification

What has been proven against the real `cueseekd`, what has not, and the exact procedure for
the part that needs a phone with Tailscale.

The distinction this document exists to keep honest: **everything below was run against the
real agent binary, its real adapter, its real SQLite store and its real SSE stream.** The
only stand-in was Jellyfin itself, because there is no Jellyfin on the development machine.
Earlier phases used a hand-written agent double; that double is not used here at all.

## Environment

| Piece | What ran |
| --- | --- |
| Agent | `cueseekd` built from this tree, `agent_version=0.0.0-dev`, `api_version=0.1.0` |
| Host control | `internal/host/unsupported.go` — the non-Linux stub, which fails loudly |
| Adapter | the real `jellyfin` adapter, polling every 10s with a 5s timeout |
| Jellyfin | a stub answering `/System/Info`, so the adapter's own mapping is exercised |
| Client | the P5 Android build, on a Vivo V2214 (Android 12) |
| Transport | first `adb reverse` over USB, then **the real network with no tunnel** |

## Verified against the real agent

| # | Behaviour | Evidence |
| --- | --- | --- |
| 1 | Unauthenticated request is refused with a conformant problem document | `GET /v1/system` returned `type: .../problems/unauthorized`, `status: 401`, with the detail naming the missing bearer token |
| 2 | A real single-use code pairs the device | `cueseekd pair` printed `YG4B-W4FQ`; the phone paired and the agent logged `device paired device_id=c447f439c923ff8c name=V2214 scopes="read, service.control"` |
| 3 | The code is genuinely single-use | Replaying the same code returned `403 invalid-pairing-code`, "The pairing code is not valid." |
| 4 | Health propagates over the real stream | Jellyfin healthy → dashboard showed `All good`, one service, `live` |
| 5 | Real degraded health, with the agent's own reason | Jellyfin set to reject the key → the adapter produced *"Jellyfin rejected the API key (HTTP 401)"*, which reached the phone within one poll and moved the verdict to `1 needs attention` |
| 6 | `action-unavailable` shows the agent's words verbatim | Restart invoked → the host stub refused → the phone showed our framing plus **"host control is not supported on this platform"**, exactly as `docs/m2-android-api.md` §4 requires |
| 7 | The agent binds a specific non-loopback interface with `allow_unrestricted: false` | Rebound to `192.168.1.6:7777`; startup accepted it and logged `api listening address=192.168.1.6:7777` |
| 8 | Android reaches it over a real network hop, in cleartext | `adb reverse --remove-all`, then paired with a second real code (`KYET-LR68`) using `192.168.1.6`. The stream ran with no USB tunnel, which is what `network_security_config.xml` exists to permit |

Item 8 matters most: a tailnet address is a specific non-loopback interface reached in
cleartext, and that is now demonstrated end to end. What Tailscale adds on top is the
interface itself.

## Checked and already correct

- **Boot ordering.** `deploy/cueseekd.service` ships `After=tailscaled.service` and
  `Wants=tailscaled.service` commented out, ready to enable when binding to a tailnet
  address.
- **Bind before the interface exists.** The agent retries `EADDRNOTAVAIL` rather than
  dying, which is what `m1.8-listenretry` is. The unit's `Restart=on-failure` is a
  backstop, not the mechanism.

## Verified over Tailscale, on the main phone

Run on a OnePlus CPH2707 (Android 16) with Tailscale connected — `tun0`, `100.121.26.17/32`,
MTU 1280 — against `cueseekd` on `kushal-hp-paviliong6` at `100.92.18.125:7777`, reached by
MagicDNS at ~24 ms.

| # | Behaviour | Evidence |
| --- | --- | --- |
| 9 | The real Android → Tailscale → `cueseekd` path | Real code `8F7Q-SP96` from `sudo cueseekd pair` redeemed from the phone; the dashboard came up naming the host |
| 10 | SSE genuinely delivers across the tailnet | Service age read 11s, 26s, then **12s** — the reset is a `service_updated` arriving, not a connection merely held open |
| 11 | A 1280-byte MTU does not break the stream | No stall or truncation across a 40s observation |
| 12 | Health matches the host | App reported Healthy; `systemctl status jellyfin` reported `active (running) since 09:36:27 IST`. Two independent measurements — the adapter polls Jellyfin's HTTP API, systemd watches the process — agreeing |
| 13 | Backgrounding and resume | Screen off 70s, then unlock: reconnected immediately to live with a fresh snapshot |
| 14 | Network transition | Wi-Fi disabled so Tailscale moved to cellular, then restored. Live on both sides |
| 15 | **A real restart, through polkit** | Confirmed on the phone at 11:19:18. `ActiveEnterTimestamp` moved from `09:36:27` to **`11:19:24 IST`**, and "Restart Jellyfin finished" appeared by 11:19:28 |

Item 15 closes the chain. The confirmation carried the agent's own action description
rather than our fallback copy; the `202` produced "accepted — waiting for the agent" rather
than a success message; and "finished" appeared only when an `action_progress` event with
`status: succeeded` and the matching `action_id` arrived over the stream.

The three timestamps corroborate each other. The tap preceded systemd's restart by six
seconds, and the client reported success *after* systemd recorded it — which is what the
202-plus-stream design exists to guarantee. A client that assumed the `202` meant "done"
would have claimed success four seconds before the service actually came back.

Proven end to end: **phone → Tailscale → cueseekd → polkit → systemd → back over SSE.**

| # | Behaviour | Evidence |
| --- | --- | --- |
| 16 | Recovery from a VPN outage, unattended | Operator toggled Tailscale off, switched to mobile data, locked the phone for several minutes, then re-enabled Tailscale. The app recovered and a subsequent Jellyfin restart succeeded on the same stored token |
| 17 | **The freshness watchdog, against the real agent** | `systemctl kill --signal=STOP cueseekd`. At 35s and 70s the phone read `Unverified`, with **"Stream open"** beside `no data 75s` then `no data 110s`, a dashed tally, a dashed clock mark and `Last verified 11:52:59` |
| 18 | Recovery from starvation | `--signal=CONT` → back to `All good`, live, within 8s, with no re-pairing and no visible reconnect state |

Item 17 is the one this client exists for, and it took a real agent to prove properly.
`SIGSTOP` suspends the process without touching its sockets, so the TCP session genuinely
stays `ESTABLISHED` — the stream really was open while the agent said nothing. A client that
trusted its transport would have shown a green "All good" and a healthy Jellyfin 110 seconds
after the agent stopped speaking. That is precisely the "confidently wrong" state ADR-0008
exists to prevent.

`Last verified 11:52:59` also confirms the local-time fix against real `observed_at` data off
the wire, rather than against a fixture.

Item 18 says the watchdog is a rendering decision and nothing more: it never interferes with
the transport, and feeding the stream again restores the display without re-pairing. The 8s
sample cannot distinguish "same connection resumed" from "reconnected quickly"; the
user-visible outcome is what is claimed.

Item 13 deserves a note, because it is easy to mistake for a Doze test. It is not one. The
stream is torn down when the app is backgrounded (ADR-0004 Amendment 2), so Doze never gets
the chance to freeze it, and on resume the client takes a fresh snapshot rather than showing
aged data. The freshness watchdog defends a different case — the app visible while the
stream goes quiet — which cannot be forced against a live agent without making it stop
emitting, and so remains verified only against a controllable double.

## Not verified, and not to be claimed

One item remains:

1. **Boot ordering in practice.** The host was already bound to its tailnet address, so the
   commented `After=tailscaled.service` lines never had to be enabled. Untested across a
   reboot. The listen retry that makes it survivable *is* unit-tested.

This does not block M2, and should be revisited if the agent is ever redeployed.

## The final acceptance test — passed, 2026-08-11

Kept for the record and for re-running after a redeployment. Note that steps 1–4 on the
host proved unnecessary: the agent was already bound to its tailnet address.

### On the Linux host

```bash
tailscale ip -4                      # note the 100.x address
sudo nano /etc/cueseek/config.yaml   # bind.address: "100.x.y.z:7777"
sudo systemctl edit --full cueseekd.service   # uncomment After=/Wants=tailscaled.service
sudo systemctl daemon-reload
sudo systemctl restart cueseekd
systemctl status cueseekd            # expect active, and "api listening address=100.x.y.z:7777"
sudo cueseekd pair                   # note the code; valid 5 minutes, single use
```

### On the main phone

1. Confirm Tailscale is connected and the host is reachable — the Tailscale app should
   list it.
2. Enable USB debugging, connect, and install: `./gradlew :app:installDebug`
3. Open CueSeek, enter the **tailnet address**, port `7777`, and the code.

### What to check, in order

| Check | Expected | Proves |
| --- | --- | --- |
| Pairing completes | dashboard appears, hostname is the Linux box | the tailnet hop and `POST /v1/pair` + `GET /v1/system` |
| Health is true | Jellyfin's state matches `systemctl status jellyfin` | the real adapter against real Jellyfin |
| Beat dot pulses, ages stay low | events arriving | SSE held open across Tailscale |
| **Restart Jellyfin** | confirm, then "Restart Jellyfin finished" | **polkit allows it, and the outcome returns as a stream event** |
| `systemctl status jellyfin` on the host | uptime reset | the restart actually happened |
| Lock the phone ~60s, unlock | `Unverified`, "Stream open" beside "no data Ns" | the freshness watchdog under **real** Doze |
| Turn Wi-Fi off, wait, on | reconnects and returns to live | reconnect over a real network change |
| Reboot the host | agent comes back bound to the tailnet address | `After=tailscaled.service` and the listen retry |

Anything that fails here is a P6 defect, not a new phase.
