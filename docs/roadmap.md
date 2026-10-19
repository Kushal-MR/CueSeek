# Roadmap

Where CueSeek is, milestone by milestone. Each finished milestone links to its plan and to the
record of what was verified on real hardware.

| Milestone | Scope | Status |
| --- | --- | --- |
| **Setup** | Repository skeleton, ADRs, contract placeholder | ✅ Done |
| **M0** | Architecture validation spike: polkit + D-Bus, and SSE over a tailnet | ✅ Done — [findings](m0-findings.md). A7 closed: SSE viable, but Doze freezes it silently rather than killing it (ADR-0004 Amendment 2) |
| **M1** | Agent: pairing, scoped tokens, Jellyfin health + restart | ✅ Done — contract, store, API, host control, adapters, SSE stream and [deployment](../deploy/) |
| **M2** | Android client: pair by entering host address + code, capability-driven dashboard, one action | ✅ Done — verified end to end over Tailscale against the real agent ([record](m2-p6-verification.md)) |
| **M3** | qBittorrent, `web_ui`, activity, host metrics, power actions | ✅ Done — nine phases, each verified on hardware as it landed ([plan](m3-plan.md) · [record](m3-verification.md)). A second adapter reached the phone with **zero client changes**, and the reboot was confirmed by a changed kernel boot id |
| **M4** | Productization: licence, neutral defaults, the `systemd` adapter, `cueseekd check`, released artefacts, documentation | ✅ Done — [plan](m4-plan.md) · [record](m4-verification.md). Proven by installing on a fresh VM that had never seen CueSeek, which found a defect the development host was structurally incapable of showing |
| **M5** | Wear OS standalone client, tiles and complications | ✅ Done — [plan](m5-plan.md) · [record](m5-verification.md). Verified for a day on a OnePlus Watch 2R against the real server, and released from `v0.1.2`. The wrist found what the desk could not: a button the round screen clipped, a hold that could be held twice, a complication age no face drew |
| **M6** | The website — the real one, replacing M4.9's deliberately plain placeholder, with a motion reel built from code | ✅ Done — live at [cueseek.vercel.app](https://cueseek.vercel.app) ([plan](m6-plan.md)) |
| **M7** | A third adapter, used to measure whether the abstraction held | ⬜ |

The numbering has moved twice. M4 was once the Wear milestone
([ADR-0011 Amendment 2](adr/0011-sequencing-spike-then-slice.md)), and the website was
inserted as M6 with the third adapter pushed to M7
([Amendment 4](adr/0011-sequencing-spike-then-slice.md)). That ADR states plainly that
a third slip of the adapter measurement would mean the sequencing rule is failing at its one
job.
