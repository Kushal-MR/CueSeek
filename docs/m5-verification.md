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

### TalkBack, driven by a person

M5.14 could only read the accessibility tree with `uiautomator`; whether TalkBack actually
reaches the hold button's long-press action needed a finger. Kushal turned TalkBack on and
drove it:

- **The hold button announced itself as intended:** "Stop Cron. Press and hold to confirm."
- **Double-tap-and-hold stopped the service.** `action accepted service=cron action=stop …
  risk=destructive` at 14:21:32, from the watch.
- **A plain double-tap did not.** Exactly one stop appears in the journal for the session.
  This is the half that matters most: a screen-reader user's ordinary activation must not
  fire a destructive action any more than a sighted user's tap does.

A restart was also accepted at 14:21:13, while navigating; restart is `disruptive` and needs
its own confirming tap, so it took two deliberate activations rather than one stray one.

TalkBack was turned off over ADB afterwards, at Kushal's request
(`enabled_accessibility_services` cleared; nothing left bound).

### The complication's age, on real watch faces

M5.12 left one question open: whether a face draws the complication's **title**, where the
age lived. The answer, on the Watch 2R: **no face tried did.** The OnePlus "Arcs" face takes
CueSeek in its round slot as a `RANGED_VALUE` — an arc and the main text — and nothing else.

It mattered immediately. After the TalkBack test the slot read **`2/4`**: the last reading
the app had taken, while `cron` was stopped, shown with nothing to say it was old after
`cron` was running again. That was the documented cost of M5.12's design, seen on a wrist.

Kushal chose to **put the age into the main text**, which every face draws:
`TimeDifferenceComplicationText` with a `^1` template, so the slot reads **`3/4 1m`** and the
face ticks the age forward by itself — no fetch, no wake. Observed on the face: `3/4 1m`,
then `3/4 2m` a minute later with nothing from CueSeek in between. The title was dropped, so
a face that does draw titles does not show the age twice. The one-hour expiry stays as a
backstop on how long a visibly old reading can sit there.

Considered and not chosen: a 15-minute expiry with the bare count (honest only by being
blank most of the day), and keeping it as it was.

---

## Session 2 — the watch on the real server (2026-10-02)

Until now the watch had only ever talked to the VM. The HP server is the machine CueSeek
exists for, and it is always on, which makes it the honest target for the day-long checks
in session 3.

### Reaching it from the wrist

The HP's agent listened only on its Tailscale address, and the watch has no Tailscale.
Kushal needs the watch at home only — his phone covers away-from-home over Tailscale — so
the agent had to become reachable on the home network as well.

**The agent binds exactly one address**, so "also on the LAN" was not a config line. Three
ways were weighed: teach the agent a list of addresses (code, a release, an install), bind
every interface (two lines), or LAN only (breaks the phone). **Kushal chose to bind every
interface**: `address: "0.0.0.0:7777"` with `allow_unrestricted: true`. The agent logged
its own warning on start, which is the guard working as designed — widening is visible,
never accidental.

**The cost, stated:** on the home network the watch speaks plain HTTP. ADR-0001 delegates
transport security to the VPN, and the LAN is not the VPN; a compromised device on the home
network could in principle observe a token. Every request still needs one (`401` without),
and the watch was paired without `host.power`, so a stolen watch token cannot power the
machine off. This is a choice about one operator's server, not a change to the product's
default, so ADR-0001 is not amended; `config.example.yaml` still says leave it false.

| Check | Result |
| --- | --- |
| Agent | `LISTEN *:7777`, `api listening address=[::]:7777 agent_version=v0.1.0` |
| LAN, from the laptop | `192.168.1.12:7777` → `401` |
| Tailscale, from the HP | `100.92.18.125:7777` → `401` |
| The phone's release app | "kushal-HP-paviliong6 — Operational", live, over Tailscale — undisturbed |

The configuration was backed up first as `config.yaml.pre-m517`; one `cp` and a restart
undoes it.

**Two things went wrong on the way, neither CueSeek's.** The HP's LAN address had moved
from `.9` to `.12` while it was off for two weeks — the same DHCP drift that cost the VM a
re-pairing in M5.9 — so SSH timed out until the address was found. The host key was
compared before trusting the new address: identical at both. And Windows PowerShell strips
inner double quotes from arguments to native programs, so a quoted `grep` pattern arrived
on the HP with its `|` read as shell pipes; commands for PowerShell are now written with no
inner quotes at all.

**Not done, and worth doing:** a DHCP reservation for the HP on the router. The watch stores
`192.168.1.12`, and another drift would cut it off until it is re-paired.

### The watch paired to the HP

The watch's VM pairing was cleared (`pm clear`) — and the phone's debug app, still paired to
the VM, immediately handed the watch the VM's address. That is ADR-0014's handoff working as
designed, pointed at the wrong server; **Change address** took the HP's instead.

Kushal generated the code with `cueseekd pair` and entered it himself. The agent recorded
`device paired … name=OPWWE234 scopes="read, service.control"` — the documented default,
**without `host.power`**: reboot and shut down were proven on the VM, and nothing in a
day-long wear test needs the power to switch off the real server.

What the watch then showed, all of it a first on real hardware:

- "kushal-HP-paviliong6 — Operational, 2/2 healthy", Jellyfin and qBittorrent both Running
- **a real thermal sensor**, `acpitz 46°C` — the VM exposes none, so this line of the vitals
  had never rendered outside a test
- **no Machine button**, correctly, for a watch without `host.power`

---

## Session 3 — a day on the wrist, against the HP (2026-10-03)

Worn from **08:34 to 22:01**, paired to the HP over the home network. Battery statistics
reset at 100% at the start; wireless debugging **off all day**, so the radio cost is the
watch's own and CueSeek's.

### Battery

| | |
| --- | --- |
| On battery | 13h 27m |
| Drain | 100% → 70%, **145–150 mAh** of 500 |
| Screen on | 35m, across 401 wake-ups |
| **CueSeek, all causes** | **3.64 mAh** — about **2.4%** of the day's drain, **0.7%** of the battery |

CueSeek's share, from `batterystats` for its UID:

| | mAh | what it is |
| --- | --- | --- |
| screen | 1.45 | 5m 53s with CueSeek on screen — the operator looking at it |
| cpu | 1.63 | almost all while open; **4.5 seconds** in the background for the whole day |
| wifi | 0.56 | 611 KB received, 221 KB sent; the radio asleep 99.8% of the day |

Opened 24 times, 7m 22s in the foreground. For scale, the watch face's always-on display
cost **42 mAh** — more than ten times CueSeek's total. The cost of not holding a stream
(ADR-0004) is visible here as its absence.

### The tile and the complication

| | runs over the day | |
| --- | --- | --- |
| Tile | **65**, about 4.8 an hour | consistent with its own 15-minute refresh (4 an hour) plus glances |
| Complication | 28 | reads only, never fetches |

The split between the tile's self-refreshes and glances could not be read, because the logs
that would show it were lost (below). The rate is what a working 15-minute refresh predicts,
and a tile that only ran on glances could not have reached it.

**The complication's expiry is ignored by this face.** At 08:35 the slot read **"2/2 12h"**
— last night's reading, twelve hours past its one-hour `validTimeRange`. The OnePlus Arcs
face does not honour the range. The age moved into the main text in session 1 is therefore
the only thing keeping the slot honest on this watch, and it did: the number said exactly
how old it was.

### Ambient

**Ambient engages on a wrist.** `onEnterAmbient burnIn=false lowBit=false` at 08:34:27,
seconds after the watch went on — the first time ambient ever fired for CueSeek, after
M5.10 never saw it on a desk or a dock. How often it engaged during the day, and whether an
hour of it left any mark, could not be measured.

**Why, recorded so it is not repeated:** the watch keeps a 64 KB log buffer by default,
which a day overwrites in minutes. It was raised to 64 MB at 08:34 — and was back at 64 KB
by evening **without a reboot** (up since 06:00). The system reverts the setting on its own.
A future measurement of ambient on this watch needs the app to keep its own small record,
not the system log.

### Readable outdoors

Yes — the verdict read at a glance in daylight.

### Now playing and transfers, from a real server

Kushal played media on Jellyfin and ran a torrent; both appeared on the watch, on the
service screens that had only ever rendered fixtures and goldens. **They took a while to
appear**, and the reason is a design decision meeting a real use: the watch reads the agent
when it opens and on returning from ambient, and holds no stream — so anything started while
CueSeek is on screen stays invisible until it is left and reopened.

### What the day changed

**Pull to refresh**, which Kushal asked for by name from the phone. Wear Compose has none and
the phone's lives in phone Material 3, which this module cannot see (ADR-0010), so it is
written for the watch: a nested-scroll connection that collects the downward drag a list at
its top cannot use, a threshold felt as the app's commitment haptic, a small ring at the top
while pulled and while reading, and a "Refresh" accessibility action as its non-gesture
equivalent. **The first version drew its ring on top of the clock**, which Kushal caught on
the wrist; the list now slides down while pulled, and the ring sits in that gap, below the
time. The ring's line is set thin explicitly — the default stroke, sized for full-screen
indicators, filled a 24dp circle and read as a solid dot. On the dashboard and the service screens. Verified on the watch: with the reading
aged to "2m" on the complication, a pull brought it to "1m" — an age can only fall if a new
reading was taken.

**The dashboard's last row reaches the middle.** Kushal found qBittorrent clipped by the
round screen. Session 1 gave the action screens room to centre their last item and left the
dashboard out, assuming it always ends in the Machine button — but a watch without
`host.power`, which the HP pairing deliberately is, has no Machine button. It now gets the same
room. On the watch: qBittorrent sits whole in the middle of the screen.

**Items morph at the edges.** Wear's own list treatment (`rememberTransformationSpec`,
`transformedHeight`) had never been applied, so a row at the top or bottom kept its full
width and the circle cut it. Rows now narrow and fade into the curve and return to full
width in the middle; the vitals were split into one item per row so each does this on its
own. Driven by scroll position, so the reduced-motion preference has nothing to turn off.

**A small finding:** a reading seconds old shows as "1m", not "now". This face rounds the
age up to the minute, so `setDisplayAsNow` never shows. Harmless — it is never younger than
it says.

### Scrolling felt laggy — and it was the build, not the code

Kushal felt a stutter scrolling the dashboard and asked for it to be as smooth as the system
launcher. Measured with `dumpsys gfxinfo` over twelve swipes on the watch, before changing
anything:

| build | janky frames | 50th | 90th | 99th | slow UI-thread frames |
| --- | --- | --- | --- | --- | --- |
| debug — what he was wearing | **9.6%** | 14 ms | 44 ms | 150 ms | 53 |
| release code, just installed | 1.2% | — | 17 ms | 30 ms | 9 |
| release code, profile-compiled | **0.6%** | 9 ms | **14 ms** | **17 ms** | 4 |

GPU time stayed at 6–10 ms throughout; the jank was all on the UI thread, which is exactly
where a debug build is slowest — no optimisation, extra runtime checks, nothing
ahead-of-time compiled. With release code and the hot paths compiled, 99% of frames land
within one 60 Hz frame (16.7 ms): launcher-smooth. Two runs agreed.

**Nothing in the app was changed for it.** The edge morph and pull-to-refresh added in this
session cost nothing visible once compiled. The release APK already carries Compose's
baseline profiles (`assets/dexopt/baseline.prof`, with `profileinstaller`), so a sideloaded
install reaches the compiled state on its own, typically after the watch's first idle charge;
the "just installed" row is the first day.

**The lesson, and the tool for it:** smoothness judged on a debug build is judged on the wrong
app. The watch module now has a `benchmark` build type — release code, debug key, debug
application id — which installs over a paired debug build without losing the pairing. It is
what was measured above, and what the watch was left running.

---

## M5 closed — 2026-10-03

Every item on M5.17's checklist was either verified on the wrist or is recorded in
[`m5-plan.md`](m5-plan.md) as a known gap with its reason: an hour of ambient (not
measurable from the system log on this watch) and rotary input (no encoder). Nothing
was marked done that was not seen.
