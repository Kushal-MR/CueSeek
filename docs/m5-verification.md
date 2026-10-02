# M5 verification

What was checked on a real wrist, what was observed, and what did not work the first time.

M5.17's record, in the shape of [`m4-verification.md`](m4-verification.md). Everything before
this phase was verified on the watch at a desk, on its dock; this is the part that needed the
watch worn, the phone in hand, and eventually the real server. Earlier per-phase records live
in [`m5-plan.md`](m5-plan.md) under each phase and are not repeated here.

## The hardware

| | |
| --- | --- |
| Watch | OnePlus Watch 2R (`OPWWE234`), Wear OS 5 / API 34, 466×466px round, no rotary encoder |
| Phone | OnePlus `CPH2707`, Android 16 |
| Agent | the test VM `cueseek-vm` at 192.168.1.250, four `systemd` services (`cron`, `ssh`, `dbus`, and a deliberately missing unit) |
| Watch build | `dev.cueseek.android.debug`, paired to the VM as `platform=wearos` |

---

## Session 1 — at a desk, against the VM (2026-10-02)

### A restart from the wrist, confirmed on the host

Not "the screen said asked": the process was replaced. `cron` read before and after a
Restart from the watch, which correctly asked for confirmation first because the agent
classes restart as `disruptive`.

| | `MainPID` | `ActiveEnterTimestamp` |
| --- | --- | --- |
| before | 689 | 13:37:25 UTC |
| after | **1378** | **13:39:23 UTC** |

The journal carries `action accepted service=cron action=restart … device=7e458e5ef9998a51
risk=disruptive` — the watch's device id, not the phone's.

### The empty state

The agent's configuration was backed up and its services replaced with `services: []` —
what every new install ships with. The watch showed the shared verdict "No services", the
machine's vitals (which need no configuration), then *No services configured. Add them on the
host.*, with the Machine button still reachable below. It reads as an instruction, not a
fault, which was the intent. The configuration was restored byte for byte (13053 bytes, four
services) and the agent restarted cleanly.

### The phone's hold, with animations off

M5.14 fixed the hold-to-confirm defect on both clients but could only prove it on the watch.
Proved on the phone here, with "Remove animations" on, against `Stop Cron`:

| press | result |
| --- | --- |
| 300ms | nothing sent; `cron` still active |
| 1600ms | stopped, `device=333e6e0b15679197` — the phone |

Animations were restored afterwards.

### The hold on a raised wrist

Kushal held "Stop Cron — hold" with the watch on his raised arm. **1.2 seconds felt right**
and the threshold haptic fired. `HOLD_MILLIS` stays at 1200, and the comment that called it
unmeasured can now cite this.

**The wrist found two defects the desk could not.**

**1. The last button could not be seen whole.** On a round screen the list stopped scrolling
with "Stop Cron — hold" on the lower curve, clipped at both corners, and it could go no
further. His words: the UI should look built for the watch. Two causes:

- The scaffold's bottom padding only clears the bezel. Screens that end in actions now pad
  to 40% of the screen height (`withRoomToCentre`), so the last button can scroll up to the
  middle, where the circle is widest. Measured on the watch: Stop now sits centred and whole.
- An idle outcome line still reserved 10dp, leaving a visible empty band between the
  header and the first button. It now takes no space until there is an outcome to report.
  The gap fell from about 29dp to about 19dp.

**2. A held button could be held again.** The journal showed two stops from the watch four
seconds apart, then a start. After an acceptance, the screen waits two seconds and re-reads
the agent — and the controls were re-enabled the moment the agent accepted, so for that
window the screen still offered "Stop Cron — hold" for a service already stopping.
Harmless for stop, which is idempotent; not something a control panel should allow. The
controls now stay disabled until the re-read has come back. Read back from the watch's
accessibility tree: immediately after a hold, Stop is `enabled=false` with no long-press
action; four seconds later the screen offers **Start Cron**, and the journal holds exactly
one stop.

That fix had a trap in it, caught before it shipped: the outcome haptic is keyed on the
action state, and settling → settled is a state change, so every acceptance would have
buzzed twice. The haptic is now keyed on the outcome with `settling` erased.

### Along the way: why the VM kept hanging at boot

The VM's intermittent hang at `Begin: Loading essential drivers` (recorded since M5.7) had
become the usual outcome — three boots in a row. VirtualBox's own log names the cause:
`HM: HMR3Init: Attempting fall back to NEM: VT-x is not available`. Windows' hypervisor holds
the CPU's virtualisation extensions (Memory Integrity or WSL turn it on), so VirtualBox runs
the guest through the Hyper-V platform API, where multi-processor guests are known to stall.

Dropped to **one vCPU**, it booted first time. The proper fix is turning Hyper-V off on the
laptop, which is a Windows security setting and the owner's call, not this project's. The
agent is light enough that one CPU costs nothing measurable.

A ping answered at 192.168.1.250 while the guest was frozen before networking, which looked
like an address conflict. Duplicate-address detection from inside the VM (`arping -D`)
received no response, so nothing else holds the address; the earlier reply is most likely
VirtualBox's Wi-Fi bridge.
