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

## Not verified, and not to be claimed

The older test phone **has no Tailscale**, so nothing here is evidence for the
Android → Tailscale → `cueseekd` path. These remain open:

1. **The tailnet hop itself.** A `100.64.0.0/10` address, Tailscale's MTU, and SSE held
   open across it.
2. **A real restart.** Host control has only ever been observed *failing* on the Windows
   stub. Whether polkit permits `RestartUnit jellyfin.service` for the `cueseek` user, and
   whether the outcome arrives as an `action_progress` event, is untested.
3. **A real Jellyfin.** Every health reading so far came from a stub of our own design.
4. **Doze on a real device.** P3 simulated the symptom — a connection held open while
   silent. Actual Doze, on cellular, has never run.
5. **Boot ordering in practice.** The `After=tailscaled.service` lines have never been
   enabled on a running host.

## The final acceptance test

Run when the main phone is available. This is the test ADR-0011 treats as the one that
counts.

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
