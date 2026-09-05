# M5 — the Wear OS client

M4 made CueSeek installable by a stranger. **M5 makes the architecture's central claim
visible**: that a second client, on a different form factor, renders the same capabilities
from the same agent without the agent changing at all.

M3 already produced a weaker version of that result — qBittorrent reached the phone with no
client release. M5 is the other half, and the harder one: a *client* that reaches the same
agent with no server release.

## The rule

**M5 adds no agent capability, no contract change and no ADR reversal.**

The same rule M4 carried, pointed the other way. If the watch needs an endpoint the phone
does not use, a field the contract does not carry, or a change to `agent/`, then either the
capability model is not doing what ADR-0005 and ADR-0007 claim, or the feature belongs in a
later milestone. **The exception is pairing**, which is a genuine gap — see M5.0.

Every commit that touches `agent/` or `api/openapi.yaml` during M5 is a signal worth
stopping for.

## What "good" means here

The user's requirement, recorded plainly: a **polished, Material 3 Wear UI with high
functionality, built to be publishable on Play**. Two of those three are engineering; the
third is mostly paperwork and calendar time, and is scoped separately below.

**Wear Material 3 is not phone Material 3.** It is a different library —
`androidx.wear.compose:compose-material3` — with different components, a different scaffold,
and layout rules driven by a round screen and a rotating side input. Reusing phone
components on a watch is the single most common way a Wear app looks wrong. Per ADR-0010 and
`DESIGN.md`: **the tokens are shared, the components are not.**

| Phone (`androidx.compose.material3`) | Wear (`androidx.wear.compose.material3`) |
| --- | --- |
| `Scaffold` | `AppScaffold` + `ScreenScaffold` |
| `LazyColumn` | `TransformingLazyColumn` |
| bottom `Button` | `EdgeButton` — hugs the screen's curve |
| `TopAppBar` | `TimeText` + `curvedText` |
| back gesture | `SwipeToDismissBox` |
| `AlertDialog` | `ConfirmationDialog` / Wear `AlertDialog` |

Also inherited, from ADR-0004: **the watch polls; it does not hold the SSE stream.** Nothing
background-critical may depend on SSE, and a watch radio is the case that rule was written
for.

---

## Phases

| Phase | Deliverable | Depends on | Status |
| --- | --- | --- | --- |
| M5.0 | Plan, and an ADR for watch pairing | — | ⬜ |
| M5.1 | `clients/wear/` module skeleton that builds and installs | M5.0 | ⬜ |
| M5.2 | Wear theme: shared tokens, Wear components | M5.1 | ⬜ |
| M5.3 | Pairing on the watch | M5.0–M5.2 | ⬜ |
| M5.4 | Dashboard: host vitals and the service list | M5.3 | ⬜ |
| M5.5 | Service detail and lifecycle actions | M5.4 | ⬜ |
| M5.6 | Host power actions | M5.5 | ⬜ |
| M5.7 | A Tile | M5.4 | ⬜ |
| M5.8 | A Complication | M5.4 | ⬜ |
| M5.9 | Golden tests at Wear sizes | M5.4–M5.6 | ⬜ |
| M5.10 | Release: signing, versioning, artefacts | M5.9 | ⬜ |
| M5.11 | Verification on a real watch | all | ⬜ |

Play Store is deliberately **not** in that table. See "Publishing" below.

---

### M5.0 — Plan, and the pairing decision

The one place M5 cannot avoid a design decision, and it needs an ADR before any code.

**Typing an address and an 8-character code on a watch is a genuinely bad experience.** The
phone's pairing screen asks for four fields. On a 45mm round screen with no keyboard worth
the name, that is not a polished flow — it is the flow that makes somebody uninstall the app.

Three options, and the ADR must choose one and say what it costs:

1. **Type it on the watch anyway.** Cheapest, honest, and bad. Wear's keyboard and voice
   input both exist; an IP address by voice is a coin flip.
2. **The phone sends the address; the watch asks only for the code.** The Wearable Data
   Layer carries `host:port` from the paired phone, so the watch shows a code field alone.
   **The token is never transferred** — the watch pairs separately and holds its own scopes,
   which is ADR-0006 intact. This is the recommended option and the one that keeps the
   security model honest.
3. **Transfer the token from the phone.** Rejected before it is proposed: it breaks
   per-device tokens, per-device revocation, and the audit log's ability to say which device
   did what. ADR-0006 exists to prevent exactly this.

Option 2 has a real cost worth stating: it makes the watch app **not fully standalone** for
setup, even though it is standalone at runtime. That contradicts nothing — the watch still
talks to the agent directly and needs no phone afterwards — but it must be written down
rather than discovered.

**Also decided here:** the watch's default scopes. `read` alone is defensible for a first
release; `service.control` is the point of the thing. `host.power` on a wrist is a decision
to take deliberately, and ADR-0006's reasoning about press-and-hold applies harder on a
screen you brush against.

---

### M5.1 — The module skeleton

`clients/wear/` as its own Gradle project, mirroring `clients/android/`'s layout. It builds,
installs on an emulator, and shows one screen saying what it is.

Consumes `:core:model` and `:core:api` unchanged. ADR-0013 predicted this: both are plain
Kotlin/JVM specifically so a second consumer inherits them without an audit — **if that
turns out to be false, it is a finding, and it is more interesting than the milestone.**

`compileSdk`/`targetSdk` track the phone app. `minSdk` is Wear-specific: Wear OS 4/5 is a
sensible floor, and the number gets chosen against what the test device actually runs rather
than against a blog post.

The manifest declares standalone operation:

```xml
<meta-data android:name="com.google.android.wearable.standalone" android:value="true" />
```

**Acceptance:** `./gradlew :app:installDebug` puts something on a watch or emulator that
launches.

---

### M5.2 — Theme: shared tokens, Wear components

`DESIGN.md`'s palette, unchanged, expressed through Wear's `ColorScheme` — which has
different roles from the phone's and cannot be copied field for field.

The status palette (`healthy`, `degraded`, `unreachable`, `unknown`) is already outside
`ColorScheme` by design, so it crosses as-is. That is the design system paying for itself.

Type is where the watch diverges hardest: `DESIGN.md`'s scale is built for a 6-inch screen.
Wear needs its own scale, and IBM Plex at 12sp on a round screen must be **read on a real
watch**, not judged in a preview.

**Acceptance:** a token round-trip test proving the watch and phone resolve the same status
colour from the same domain value — the same class of test as the client capability registry
check.

---

### M5.3 — Pairing on the watch

Implements M5.0's decision. Address arrives from the phone; the code is entered on the
watch; the token is minted for the watch alone and sealed in its own Keystore.

Reuses `:core:data`'s cipher approach rather than reimplementing it — or, if `:core:data`
proves to be Android-library-bound in a way the watch cannot consume, **that is a finding
for ADR-0013**, which claimed only `:core:model` and `:core:design` were shared.

**Acceptance:** a watch paired against the VM agent, holding its own token, visible as a
separate device in the phone's device list with its own scopes.

---

### M5.4 — Dashboard

The screen that justifies the app: overall status, host vitals, the service list.

Not a port. The phone shows CPU, memory, storage and temperature as a four-up grid; a watch
that tried would be unreadable. The watch shows **the verdict first** — `Operational` and a
count — with vitals reachable by scrolling. `TransformingLazyColumn` and the rotating side
input do the work.

`DESIGN.md` §12 already lists host-metric layout as an open question for the phone. The
watch forces an answer, and whatever it produces should feed back.

Capabilities render through the same registry pattern ADR-0007 mandates. **Branching on
service id is a review-blocking defect here exactly as it is on the phone**, and the test
that enforces it gets a Wear sibling.

**Acceptance:** the VM's `Cron` and the HP host's `Jellyfin`/`qBittorrent` both render, with
the temperature row present on one and absent on the other — the same absent-is-not-zero
behaviour M4.10 observed on the phone.

---

### M5.5 — Service detail and actions

Restart, stop and start, state-dependent exactly as the API reports them. Risk classes carry
across: `disruptive` confirms, `destructive` needs press-and-hold.

**Press-and-hold matters more on a wrist**, and the phone's timing is not automatically
right for a device you knock against a doorframe. Whatever is chosen is measured, not
guessed.

**Acceptance:** a service restarted from the watch, confirmed by `MainPID` changing on the
host — the standard this project has used since M3.1.

---

### M5.6 — Host power actions

Reboot and shut down, if M5.0 granted the scope. Same press-and-hold, same audit trail.

If M5.0 decided the watch does not get `host.power` by default, this phase is **one
paragraph of documentation and no code** — and that is a complete outcome, not a skipped one.

---

### M5.7 — A Tile

The first thing that makes a watch app feel native rather than ported.

Tiles are **not Compose**. They render through `androidx.wear.tiles` and ProtoLayout, in a
separate process, from a snapshot — so this is genuinely new UI code, not a reuse of M5.4,
and it is the phase most likely to be underestimated.

One tile: overall status, the service count, and the age of the reading. It must be honest
about staleness, because a tile is glanceable and a stale green is worse there than anywhere
else in the product.

---

### M5.8 — A Complication

A watch face slot: one number or one state. Smallest surface in the project and the one with
the least room to be wrong.

Scope discipline applies hardest here. A complication that tries to show four services shows
none of them.

---

### M5.9 — Golden tests at Wear sizes

Paparazzi already covers `:core:design`. Wear needs its own goldens at real device
geometries — small round, large round — because a layout that survives 45mm can break at
41mm, and neither is the phone.

The greyscale check from `DESIGN.md`'s tally-rule finding applies: contrast measured, not
eyeballed. That finding was caught by a golden test and not by a person.

---

### M5.10 — Release

Extends `.github/workflows/release.yml` rather than forking it. Same keystore, same signing
job shape, a second artefact.

**The versionCode scheme needs a decision.** `versionCodeFrom` yields `101` for `v0.1.1`. If
the watch APK ships from the same tag with the same code, Play treats them as one app with
two form-factor deliverables — which is what we want, but it constrains how they are built
and uploaded. This is decided here, before the first upload, because changing it afterwards
is painful.

---

### M5.11 — Verification on a real watch

**This phase is why the milestone is credible or is not.**

Everything above can be built against an emulator. An emulator cannot show you that 12sp
Plex is unreadable in sunlight, that press-and-hold fires when you flex your wrist, that the
tile is stale because the radio slept, or that the app drains the battery.

M4's lesson, stated in its closing note: the defects that mattered were only visible on a
machine that had never run the software. The watch equivalent is a watch on a wrist for a
day.

Recorded in `docs/m5-verification.md`, in the shape of `m4-verification.md`.

---

## Publishing to Google Play

**Recommended: not part of M5.** It is distribution, not engineering, and folding it in
would stop M5 from ever closing — the same trap M4.9's website nearly became.

It also contains a hard gate that is **calendar time, not work**:

- A **new personal developer account must run a closed test with at least 12 testers, opted
  in for 14 continuous days**, before it can apply for production access. Twelve real
  accounts, two weeks, before the first public release is even possible.
- **$25** one-time registration.
- **AAB, not APK.** Play has not accepted APKs for new apps since 2021, so `release.yml`
  needs a bundle target. The existing APK stays for sideloading — it is what `install.md`
  documents.
- **Play App Signing**, which means Google holds the signing key. That interacts with the
  keystore already generated: it becomes the *upload* key rather than the app signing key.
- **Privacy policy URL**, **data safety form**, **content rating**, store listing assets.

Two honest problems specific to this app:

**A reviewer cannot test it.** CueSeek needs a self-hosted agent on a VPN. A Play reviewer
has neither. Without a demo mode and clear reviewer notes, this is a plausible rejection —
and the app already has a demo mode used for screenshots, which is most of the answer.

**The data safety form must disclose plaintext HTTP.** The app talks cleartext to a
user-entered address, for reasons ADR-0001 sets out. That is defensible and must be declared
accurately rather than glossed.

If Play is wanted, it should be **M7** or later, after the third adapter, and its own plan.

---

## What M5 deliberately excludes

- **Multi-host on the watch.** The phone does not have it either.
- **A watch face.** Complications are a slot in someone else's watch face; building one is a
  different product.
- **Standalone LTE / off-VPN access.** ADR-0001 is unchanged.
- **Notifications and alerting.** ADR-0012 deferred it and its reasoning has not changed. A
  watch makes alerting *more* tempting and no more correct.
- **Voice input.** An IP address by voice is a coin flip.
- **iOS-paired Wear.** Not a supported configuration.

## Open questions, to be answered rather than assumed

1. **Is there a real Wear device?** M5.11 is not optional, and without hardware the milestone
   ships emulator-verified — which this project has never accepted for anything else.
2. **Which scopes does the watch get by default?** M5.0.
3. **Does `:core:data` survive the move**, or is ADR-0013's sharing claim narrower than
   stated? Answered by attempting M5.3.
4. **Does the watch change `DESIGN.md`'s open question on host metrics**, or fork it?
