# M5 — the Wear OS client

M4 made CueSeek installable by a stranger. **M5 makes the architecture's central claim
visible**: that a second client, on a different form factor, renders the same capabilities
from the same agent without the agent changing at all.

M3 already produced the weaker half of that result — qBittorrent reached the phone with no
client release. M5 is the harder half: a *client* that reaches the same agent with no server
release.

## The rule

**M5 adds no agent capability, no contract change and no ADR reversal.**

M4's rule, pointed the other way. If the watch needs an endpoint the phone does not use, a
field the contract does not carry, or a change to `agent/`, then either the capability model
is not doing what ADR-0005 and ADR-0007 claim, or the feature belongs in a later milestone.

**One exception: pairing.** It is a genuine gap rather than a shortcut, and M5.0 decides it
with an ADR before any code is written.

Every commit that touches `agent/` or `api/openapi.yaml` during M5 is a signal worth
stopping for.

## What "polished" means here, concretely

The goal is not a port of the phone app onto a smaller screen. It is an app that a Wear user
would not immediately identify as a port. That means a specific, checkable list:

- The **rotating side button** scrolls everything scrollable.
- **Swipe-to-dismiss** works on every screen, because on Wear that is the back gesture.
- **Haptics** confirm destructive actions, because a watch is often used without looking.
- **Ambient mode** does something sensible instead of burning the panel.
- Every screen has a **loading, empty, error and stale** state that was designed rather than
  defaulted.
- A **Tile** and a **Complication**, because an app with neither is a phone app you can see
  from your wrist.
- **TalkBack** reads the screen in an order that makes sense.

Those are phases below, not aspirations.

### Wear Material 3 is not phone Material 3

A different library — `androidx.wear.compose:compose-material3` — with different components,
a different scaffold, and layout driven by a round screen and a rotating input. Reusing phone
components on a watch is the single most common way a Wear app looks wrong.

Per ADR-0010 and `DESIGN.md`: **the tokens are shared, the components are not.**

| Phone (`androidx.compose.material3`) | Wear (`androidx.wear.compose.material3`) |
| --- | --- |
| `Scaffold` | `AppScaffold` + `ScreenScaffold` |
| `LazyColumn` | `TransformingLazyColumn` |
| bottom `Button` | `EdgeButton` — hugs the screen's curve |
| `TopAppBar` | `TimeText` + `curvedText` |
| system back | `SwipeToDismissBox` |
| `AlertDialog` | `ConfirmationDialog` / Wear `AlertDialog` |
| — | `Picker`, `Stepper`, rotary-aware scroll state |

Inherited from ADR-0004: **the watch polls; it does not hold the SSE stream.** Nothing
background-critical may depend on SSE, and a watch radio is the case that rule was written
for.

---

## Phases

| Phase | Deliverable | Depends on | Status |
| --- | --- | --- | --- |
| M5.0 | Plan, and the pairing ADR | — | ⬜ |
| M5.1 | `clients/wear/` builds and installs | M5.0 | ⬜ |
| M5.2 | Theme: shared tokens, Wear components | M5.1 | ⬜ |
| M5.3a | Address handoff from the phone | M5.0 | ⬜ |
| M5.3b | Code entry, token minting, secure storage | M5.3a | ⬜ |
| M5.4a | Dashboard: the verdict and host vitals | M5.3b | ⬜ |
| M5.4b | Dashboard: the service list | M5.4a | ⬜ |
| M5.5 | Service detail | M5.4b | ⬜ |
| M5.6 | Lifecycle actions, with confirmation | M5.5 | ⬜ |
| M5.7 | Host power actions | M5.6 | ⬜ |
| M5.8 | Rotary, swipe-to-dismiss, haptics | M5.4b | ⬜ |
| M5.9 | Every state: loading, empty, error, stale | M5.4b | ⬜ |
| M5.10 | Ambient mode and battery behaviour | M5.9 | ⬜ |
| M5.11 | A Tile | M5.4b | ⬜ |
| M5.12 | A Complication | M5.4b | ⬜ |
| M5.13 | Identity: icon, name, launcher, splash | M5.2 | ⬜ |
| M5.14 | Accessibility pass | M5.8, M5.9 | ⬜ |
| M5.15 | Golden tests at real Wear geometries | M5.4–M5.13 | ⬜ |
| M5.16 | Release: signing, versioning, artefacts | M5.15 | ⬜ |
| M5.17 | Verification on the OnePlus Watch 2R | all | ⬜ |

---

### M5.0 — Plan, and the pairing decision

The one place M5 cannot avoid a design decision, and it needs an ADR before any code.

**Typing an address and an 8-character code on a watch is a genuinely bad experience.** The
phone asks for four fields. On a round screen with no usable keyboard, that is not a polished
flow — it is the flow that makes somebody uninstall the app.

Three options; the ADR chooses one and states its cost:

1. **Type it on the watch.** Cheapest, honest, and bad. Voice input for an IP address is a
   coin flip.
2. **The phone sends the address; the watch asks only for the code.** The Wearable Data Layer
   carries `host:port` from the paired phone. **The token is never transferred** — the watch
   pairs separately and holds its own scopes, so ADR-0006 stays intact. **Recommended.**
3. **Transfer the token from the phone.** Rejected before it is proposed: it breaks
   per-device tokens, per-device revocation, and the audit log's ability to say which device
   did what. ADR-0006 exists to prevent exactly this.

Option 2's real cost, to be written down rather than discovered: the watch is **standalone at
runtime but not for setup**. It talks to the agent directly and needs no phone afterwards —
but it needs one once.

**Also decided here:** the watch's default scopes. `read` alone is defensible; `service.control`
is the point of the thing; `host.power` on a wrist is a deliberate decision, and ADR-0006's
press-and-hold reasoning applies harder on a screen you brush against doorframes.

---

### M5.1 — The module skeleton

`clients/wear/` as its own Gradle project, mirroring `clients/android/`. It builds, installs,
and shows one screen naming itself.

Consumes `:core:model` and `:core:api` **unchanged**. ADR-0013 predicted this: both are plain
Kotlin/JVM specifically so a second consumer inherits them without an audit. **If that turns
out to be false, it is a finding, and it is more interesting than the phase.**

Manifest declares standalone operation:

```xml
<meta-data android:name="com.google.android.wearable.standalone" android:value="true" />
```

`minSdk` is chosen against **what the OnePlus Watch 2R actually runs**, read off the device,
not off a blog post.

**Acceptance:** installs on the watch and launches.

---

### M5.2 — Theme: shared tokens, Wear components

`DESIGN.md`'s palette expressed through Wear's `ColorScheme`, which has different roles from
the phone's and cannot be copied field for field.

The status palette (`healthy`, `degraded`, `unreachable`, `unknown`) already lives **outside**
`ColorScheme` by design, so it crosses unchanged. That is the design system paying for itself.

Type is where the watch diverges hardest. `DESIGN.md`'s scale targets a 6-inch screen; Wear
needs its own, and IBM Plex at small sizes on a round panel must be **read on the watch**, not
judged in a preview.

**Acceptance:** a test proving watch and phone resolve the same status colour from the same
domain value — the same class of check as the capability registry test.

---

### M5.3a — Address handoff from the phone

Implements the transport half of M5.0. The phone app publishes `host:port` over the Wearable
Data Layer; the watch reads it.

**No token crosses.** The payload is an address the user already typed once — the same
information printed on their own terminal.

**Acceptance:** the watch shows the HP host's address without anybody typing it there.

---

### M5.3b — Code entry, token minting, secure storage

The watch asks for the pairing code alone, redeems it, and seals its own token in its own
Keystore — reusing `:core:data`'s cipher approach rather than reimplementing it.

**If `:core:data` proves Android-library-bound in a way the watch cannot consume, that is a
finding for ADR-0013**, which claimed only `:core:model` and `:core:design` were shared.

**Acceptance:** the watch appears as its own device in the phone's device list, with its own
scopes, and revoking it does not affect the phone.

---

### M5.4a — Dashboard: the verdict and host vitals

The screen that justifies the app.

Not a port. The phone shows CPU, memory, storage and temperature as a four-up grid; a watch
that tried would be unreadable. The watch shows **the verdict first** — `Operational` and a
count — with vitals below it.

`DESIGN.md` §12 lists host-metric layout as an open question for the phone. The watch forces
an answer, and whatever it produces should feed back.

### M5.4b — Dashboard: the service list

Capabilities render through the same registry pattern ADR-0007 mandates. **Branching on
service id is a review-blocking defect here exactly as on the phone**, and the test enforcing
it gets a Wear sibling.

**Acceptance:** the VM's `Cron` and the HP host's `Jellyfin`/`qBittorrent` both render, with
the temperature row present on one and absent on the other — the absent-is-not-zero behaviour
M4.10 observed on the phone.

---

### M5.5 — Service detail

One service, full height: health, reported status, reasons, and what it is doing. Activity
capabilities (`now_playing`, `transfers`) get watch-shaped renderers — a wrist shows *one*
session, not five.

---

### M5.6 — Lifecycle actions, with confirmation

Restart, stop and start, state-dependent exactly as the API reports them. Risk classes carry
across: `disruptive` confirms, `destructive` needs press-and-hold.

**Press-and-hold timing is measured, not inherited.** The phone's duration is not
automatically right for a device you knock against a doorframe.

**Acceptance:** a service restarted from the watch, confirmed by `MainPID` changing on the
host — the standard used since M3.1.

---

### M5.7 — Host power actions

Reboot and shut down, if M5.0 granted the scope. Same press-and-hold, same audit trail.

If M5.0 decided the watch does not get `host.power` by default, this phase is **one paragraph
of documentation and no code** — a complete outcome, not a skipped one.

---

### M5.8 — Rotary, swipe-to-dismiss, haptics

The phase that separates a Wear app from a shrunk phone app.

- **Rotary input** scrolls every scrollable surface, with the correct fling behaviour.
- **Swipe-to-dismiss** on every screen — on Wear this *is* back, and an app that swallows it
  feels broken.
- **Haptics** on confirmation and on action completion, because a watch is often operated
  without looking at it.

**Acceptance:** every screen reachable and dismissable using only the crown and a swipe.

---

### M5.9 — Every state, designed rather than defaulted

Four states per screen, each written on purpose:

| State | What it must not do |
| --- | --- |
| Loading | show a blank screen |
| Empty | look like an error |
| Error | show a stack trace or a bare code |
| **Stale** | show confident green while the agent is unreachable |

Stale matters most. The client degrades to `unknown` from its own clock rather than trusting
the connection to notice its own death — the phone's behaviour, and a watch radio sleeps far
more aggressively.

---

### M5.10 — Ambient mode and battery behaviour

A watch app that keeps a screen bright and a radio awake is a bad app regardless of how it
looks.

Ambient shows the last known verdict and its age, dimmed, and **does not poll**. Coming back
to interactive refreshes. Polling backs off when the app is not visible.

**Acceptance:** measured battery impact over a wear-day in M5.17, not asserted here.

---

### M5.11 — A Tile

The first thing that makes a watch app feel native rather than installed.

**Tiles are not Compose.** They render through `androidx.wear.tiles` and ProtoLayout, in a
separate process, from a snapshot — genuinely new UI code, not a reuse of M5.4, and the phase
most likely to be underestimated.

One tile: overall status, service count, and the age of the reading. It must be honest about
staleness; a tile is glanceable, and a stale green is worse there than anywhere else in the
product. Tapping it opens the app.

---

### M5.12 — A Complication

A watch face slot: one number or one state. The smallest surface in the project and the one
with the least room to be wrong.

Scope discipline applies hardest here. A complication that tries to show four services shows
none of them. Supports the handful of complication types that actually suit a status value,
and declines the rest rather than rendering them badly.

---

### M5.13 — Identity: icon, name, launcher, splash

The unglamorous phase that decides whether the app looks finished.

A Wear launcher icon is not the phone icon scaled down. Plus the app name as it appears in the
launcher, and a splash that does not flash white on a dark watch face.

---

### M5.14 — Accessibility pass

The floor `DESIGN.md` §9 already sets, applied to a screen where it is harder.

TalkBack reads each screen in a sensible order; every control has a content description that
says what it does rather than what it is; touch targets meet the Wear minimum; the reduced-
motion preference is honoured.

---

### M5.15 — Golden tests at real Wear geometries

Paparazzi already covers `:core:design`. Wear needs its own goldens at **real device
geometries** — small round and large round — because a layout that survives 45mm can break at
41mm, and neither is the phone.

The greyscale check from `DESIGN.md`'s tally-rule finding applies: contrast measured, not
eyeballed. That finding was caught by a golden test and not by a person.

---

### M5.16 — Release

Extends `.github/workflows/release.yml` rather than forking it. Same keystore, same signing
job shape, one more artefact — `cueseek-wear_<version>.apk`, checksummed and attested exactly
like the other two.

**The versionCode scheme needs a decision** and gets it here, before the first artefact ships.

---

### M5.17 — Verification on the OnePlus Watch 2R

**This phase is why the milestone is credible or is not.**

Everything above can be built against an emulator. An emulator cannot show that small-size
Plex is unreadable in sunlight, that press-and-hold fires when you flex your wrist, that the
tile went stale because the radio slept, or that the app costs 15% of the battery in a day.

M4's lesson, from its closing note: the defects that mattered were only visible on a machine
that had never run the software. The watch equivalent is a watch on a wrist for a day.

Checklist, recorded in `docs/m5-verification.md` in the shape of `m4-verification.md`:

- Paired against the VM **and** the HP host
- A service restarted, confirmed by `MainPID` on the host
- Rotary scrolls; swipe dismisses; haptics fire
- Tile and complication both installed and updating
- Ambient behaves for a full hour without the screen burning
- Battery cost over a working day, measured
- Readable outdoors
- TalkBack pass

---

## Distribution

**Play Store is not the plan.** It is a distribution channel with a hard calendar gate — a new
personal developer account must run a closed test with 12 testers opted in for 14 continuous
days before it can apply for production access — and it would gate the app on paperwork rather
than readiness. It stays available as a later option and is not designed against.

**The download page is M6's job**, and it is the one that matters: the website carries the
signed APKs for both the phone and the watch, with checksums and attestation instructions, the
same way `install.md` already handles the agent.

One honest constraint M6 must state rather than gloss: **sideloading a Wear app is awkward.**
Without Play, installation is ADB over Wi-Fi — pairing the watch to a computer, enabling
developer options, `adb connect`. That is fine for the audience CueSeek already has and it
should be written down as what it is, not dressed up.

---

## What M5 deliberately excludes

- **Multi-host on the watch.** The phone does not have it either.
- **A watch face.** Complications are a slot in someone else's watch face; building one is a
  different product.
- **Standalone LTE / off-VPN access.** ADR-0001 is unchanged.
- **Notifications and alerting.** ADR-0012 deferred it and its reasoning has not changed. A
  watch makes alerting *more* tempting and no more correct.
- **Voice input for the address.** A coin flip, and M5.3a removes the need.
- **iOS-paired Wear.** Not a supported configuration.

## Open questions, to be answered rather than assumed

1. **Which scopes does the watch get by default?** M5.0.
2. **Does `:core:data` survive the move**, or is ADR-0013's sharing claim narrower than
   stated? Answered by attempting M5.3b.
3. **Does the watch change `DESIGN.md`'s open question on host metrics**, or fork it?
4. **What does the Watch 2R actually run** — Wear OS version, screen geometry, API level?
   Read off the device in M5.1, and it sets `minSdk` and the golden-test sizes.
