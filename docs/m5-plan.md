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

- The **rotating side button** scrolls everything scrollable. (True of the code; **not
  checkable on this project's watch**, which has no rotary encoder — see M5.8.)
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
| M5.0 | Plan, and the pairing ADR | — | ✅ |
| M5.1 | `clients/wear/` builds and installs | M5.0 | ✅ |
| M5.2 | Theme: shared tokens, Wear components | M5.1 | ✅ |
| M5.3a | Address handoff from the phone | M5.0 | ✅ |
| M5.3b | Code entry, token minting, secure storage | M5.3a | ✅ |
| M5.4a | Dashboard: the verdict and host vitals | M5.3b | ✅ |
| M5.4b | Dashboard: the service list | M5.4a | ✅ |
| M5.5 | Service detail | M5.4b | ✅ |
| M5.6 | Lifecycle actions, with confirmation | M5.5 | ✅ |
| M5.7 | Host power actions | M5.6 | ✅ |
| M5.8 | Rotary, swipe-to-dismiss, haptics | M5.4b | ✅ |
| M5.9 | Every state: loading, empty, error, stale | M5.4b | ✅ |
| M5.10 | Ambient mode and battery behaviour | M5.9 | ✅ |
| M5.11 | A Tile | M5.4b | ⬜ |
| M5.12 | A Complication | M5.4b | ⬜ |
| M5.13 | Identity: icon, name, launcher, splash | M5.2 | ⬜ |
| M5.14 | Accessibility pass | M5.8, M5.9 | ⬜ |
| M5.15 | Golden tests at real Wear geometries | M5.4–M5.13 | ⬜ |
| M5.16 | Release: signing, versioning, artefacts | M5.15 | ⬜ |
| M5.17 | Verification on the OnePlus Watch 2R | all | ⬜ |

---

### M5.0 — Plan, and the pairing decision ✅

**Decided in [ADR-0014](adr/0014-watch-pairing-address-handoff.md).** The phone publishes
`host` and `port` over the Wearable Data Layer; the watch presents a code field alone,
redeems it against the agent itself, and stores a token the phone has never seen. Default
scopes are `read` and `service.control` — not `host.power`.

A manual address field survives as a fallback, reached deliberately rather than shown first,
because the Data Layer can fail and the bad flow always works.

The reasoning below is what led there, kept because the alternatives matter.

---

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

### M5.1 — The module skeleton ✅

Built and installed on the OnePlus Watch 2R. **The device, read off the device:**

| | |
| --- | --- |
| Model | `OPWWE234` — OnePlus Watch 2R |
| OS | Android 14, **API 34**, Wear SDK 5 |
| Display | 466 × 466 px @ 320dpi = **233 × 233 dp**, fully circular |

`minSdk = 30` (Wear OS 3, the first with the standalone app model), `targetSdk = 34` to
match the device. The library floor is lower — Wear Compose Material 3 declares minSdk 25 —
but a Wear OS 2 companion app is a different product.

**Not its own Gradle project.** The plan said it would be; it is a project in the existing
`clients/android` build instead, with `projectDir` pointing at `clients/wear/app`. Two
separate builds cannot share a project, and making them would need a composite build with
dependency substitution — a second build system to understand before reading any application
code, which is the cost ADR-0013 already refused for convention plugins. The directory
layout ADR-0009 specified is unchanged; only the build root differs. Reasoning is in
`settings.gradle.kts`, with the trigger to revisit.

**`applicationId` is shared with the phone** (`dev.cueseek.android`), namespace is not
(`dev.cueseek.wear`). The Wearable Data Layer that ADR-0014's handoff depends on expects one
identity across form factors, and it is the only shape that could ever be delivered from a
single listing. The cost is that the two artefacts must coordinate `versionCode` — M5.16.

**ADR-0013's claim held.** `:core:model` was consumed with no change and no audit, and is
exercised rather than merely linked: the watch renders `HealthStatus.fromWire("healthy")`,
so the companion object and the wire mapping both run in a Wear process. `:core:api` is
declared and resolves — which proves its Retrofit/OkHttp/serialization graph is compatible
with a Wear build — but nothing calls it until M5.3b.

The original description follows.

---

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

### M5.2 — Theme: shared tokens, Wear components ✅

**The sharing did not work as recorded, and fixing it is the phase's main result.**
`:core:design` declared `api(libs.androidx.compose.material3)`, so every consumer inherited
the *phone's* Material 3 — meaning ADR-0010's "tokens shared, components not" was enforced by
review rather than by the compiler. Demoted to `implementation`; the watch now cannot see it,
verified by adding the import deliberately and watching the build fail.
[ADR-0013 Amendment 1](adr/0013-android-client-architecture.md).

**Wear's `ColorScheme` is confirmed incompatible field-for-field**: 29 roles, including
`primaryDim`/`secondaryDim`/`tertiaryDim`/`errorDim` with no phone equivalent, and no plain
`surface`. Read off the AAR rather than assumed.

**The status palette crossed unchanged**, which is ADR-0010 paying for itself somewhere
nobody aimed: status roles live outside `ColorScheme` so meaning could not be themed, and
that is exactly why they survived a scheme that turned out to be form-factor-specific.

**Dark only.** An OLED panel, all day, on a fraction of a phone's battery — and DESIGN.md's
dark palette is the one whose contrast was tuned at low brightness.

**Two gaps DESIGN.md could not answer** and now records as open: the four `*Dim` roles
(derived here as `base * 0.65 + background * 0.35`, with a test proving the committed
literals match the formula) and the `on` roles for secondary/tertiary/error (reused from
`background`/`onBackground`, contrast pinned at ≥ 4.5:1).

**One thing the watch settled rather than opened:** Wear Material 3 has five first-class
`numeral*` type roles, so "mono is confined to data" stops being a convention the codebase
maintains and becomes configuration. Fed back into DESIGN.md §12.

Five unit tests, 0 skipped. The original description follows.

---

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

### M5.3a — Address handoff from the phone ✅

**Observed on hardware: `192.168.1.10:7777` appeared on the watch with nobody typing it.**
The negative case was confirmed first — the watch read "no address from a phone" while the
phone still ran the release build — so the address appeared because of the handoff rather
than from anything cached.

The wire format lives in `:core:model`, so both halves share it by construction and it is
tested with no device on either end. It is ADR-0006 Amendment 3's QR URI minus `code`, and
**`decode` refuses any payload carrying one**, in either parameter position. ADR-0014 turns
on no credential crossing between devices; a pairing code is one redemption away from being
one; so the constraint is enforced by the parser rather than by a sentence in a record.

**One defect found by rendering it, and one claim of mine corrected.** The address was
styled `numeralSmall` and wrapped mid-octet as `192.168.1.1` / `0:7777`. Wear's numeral
scale is 24–60sp, built for a single glanceable magnitude — not for a fifteen-character
identifier. M5.2 concluded that Wear's numeral roles turn DESIGN.md's "mono is confined to
data" rule into configuration; that is true for magnitudes and false for identifiers, and
the watch needed a mono-at-body-size role at exactly the 12sp the phone's `Data.Small`
already uses. DESIGN.md §12 now carries the correction rather than the original claim.

The original description follows.

---

Implements the transport half of M5.0. The phone app publishes `host:port` over the Wearable
Data Layer; the watch reads it.

**No token crosses.** The payload is an address the user already typed once — the same
information printed on their own terminal.

**Acceptance:** the watch shows the HP host's address without anybody typing it there.

---

### M5.3b — Code entry, token minting, secure storage ✅

The watch paired against the VM agent. From the agent's own device list:

```
M5.3b verifier   platform=cli      scopes=read,devices.manage
OPWWE234         platform=wearos   scopes=read,service.control
CPH2707          platform=android  scopes=read,service.control,host.power
```

Three devices, three platforms, three scope sets. The watch holds its own token with its
own fingerprint — **no `host.power`**, exactly as ADR-0014 decided — and the phone's is
untouched. That is ADR-0006's per-device model and ADR-0014's handoff, both verified at once
rather than argued.

**`:core:data` is shared after all.** ADR-0013 named only `:core:model` and `:core:design`.
It carries nothing phone-specific — DataStore, the Android Keystore and coroutines all exist
on Wear — so the watch reuses `PairingRepository` and the same `TokenCipher` rather than
growing a second implementation of "seal a credential".

**One latent bug in the phone client, found by adding a second one.**
`PairingRepository` hardcoded `Platform.Android`. `Platform.WearOs` had existed unused in the
model since M1, and a watch reporting itself as a phone would have made the device list and
the audit log — whose entire job is saying *which device did this* — both lie.

**One bug of my own, and a false negative that hid it.** The Wear app shipped with no
`networkSecurityConfig`, so Android blocked cleartext from API 28 and it could not reach any
agent. `curl` on the same watch succeeded, because a shell is not subject to an app's network
policy — which is exactly the check that gave false confidence. The config now lives in
`:core:data` with the module that owns agent connectivity, referenced by both manifests,
because the drift *was* the bug.

**RemoteInput cannot be driven by `adb shell input tap`** — the chooser opens, the keyboard
accepts text, and send never returns a result. Under a real finger it completes immediately.
Synthetic taps are not equivalent to touch for that system UI, so this is a checklist item
M5.17 has to hand to a person.

The original description follows.

---

The watch asks for the pairing code alone, redeems it, and seals its own token in its own
Keystore — reusing `:core:data`'s cipher approach rather than reimplementing it.

**If `:core:data` proves Android-library-bound in a way the watch cannot consume, that is a
finding for ADR-0013**, which claimed only `:core:model` and `:core:design` were shared.

**Acceptance:** the watch appears as its own device in the phone's device list, with its own
scopes, and revoking it does not affect the phone.

---

### M5.4a — Dashboard: the verdict and host vitals ✅

On the watch, against the VM agent:

```
cueseek-vm
Operational
1/1 healthy
CPU 0%   MEM 11%   / 17%
```

**No temperature row**, because a VM exposes no sensors — absent is not zero, observed on a
third surface now.

**The verdict had to be shared, and was not.** `verdict`, `hostConcern`, the pressure
thresholds and `Tally` all lived inside the phone app's dashboard package, three of them
`internal`. Layout is per form factor and M5.3b already gave the watch its own error copy on
exactly that reasoning — but this is not layout. "Is everything fine?" is one question about
one machine, and a watch that answered it differently from the phone in your pocket would be
a console contradicting itself. All four moved to `:core:model`, with the phone's 10 verdict
tests and 12 vitals tests passing unchanged against the new home.

`verdict` also gained an overload taking `(stale, services, hostMetrics, tally)`. It required
an `AgentState`, which is shaped by the phone's event stream; a polling watch holds none of
that, so the alternative was fabricating a stream state or writing a second verdict.

**Polling, not streaming** (ADR-0004): `ServicesRepository.snapshot()` fetches system,
services, metrics and actions in one round trip. There is no timer — the screen refreshes
when it appears, and that is the whole schedule.

**One bug, found by launching it twice.** Routing used a `remember { mutableStateOf(false) }`
set when pairing succeeded, so an already-paired watch showed the pairing screen on every
launch — the token was in the store and nothing ever asked. Routing now derives from the
store, with `Deciding` as a distinct state rather than defaulting to `Pairing` while it
loads: flashing "Pair with an agent" at somebody who paired last week is a wrong answer shown
confidently, which is the same thing this project refuses everywhere else.

Staleness is computed from a clock on a 5s ticker scoped to the screen, not from the fetch —
6 tests including the boundary and an 8-hour sleep.

The original description follows.

---

The screen that justifies the app.

Not a port. The phone shows CPU, memory, storage and temperature as a four-up grid; a watch
that tried would be unreadable. The watch shows **the verdict first** — `Operational` and a
count — with vitals below it.

`DESIGN.md` §12 lists host-metric layout as an open question for the phone. The watch forces
an answer, and whatever it produces should feed back.

### M5.4b — Dashboard: the service list ✅

Four services on the watch, against a VM configured with three real units and one
deliberately wrong one:

```
1 needs attention        3/4 healthy
● Cron         Running
● SSH          Running
● D-Bus        Running
● Nonexistent  Unreachable
```

The verdict moved from `Operational` to `1 needs attention` on its own, from the function
`:core:model` shares with the phone — so the two clients cannot disagree about the machine.
The status colours are the phone's, verbatim.

**The guard got a Wear twin rather than being assumed to cover both.**
`WearCapabilityTest` scans the watch's UI source for `when (service.id)` and its variants,
exactly as the phone's `CapabilityAndCopyTest` does. A second client is where that rule
decays, because the shortcut is cheapest in the file nobody has reviewed yet.

**The activity line is per form factor, and that is a different answer from M5.4a's.** The
verdict *had* to be shared — two clients disagreeing about "is everything fine?" is a
contradiction. An activity line is a phrasing of the same fact, and the phone's does not fit:
`3 of 12 active · ↓ 4.2 MB/s` is 28 characters, and lifting it would have dragged `byteSize`
and its decimal-places rule into the domain module. So the watch says less, on purpose, while
the *decisions* stay identical — idle says nothing, transcoding is named only when nonzero, a
service doing both gets both.

**One test I had to rewrite because it was wrong.** The width assertion picked 24 characters
out of the air and failed a line that fits perfectly well. It now asserts what is actually
being decided — that no transfer rate and no total reach the watch row — with a length bound
as a rail rather than the claim.

The original description follows.

---

Capabilities render through the same registry pattern ADR-0007 mandates. **Branching on
service id is a review-blocking defect here exactly as on the phone**, and the test enforcing
it gets a Wear sibling.

**Acceptance:** the VM's `Cron` and the HP host's `Jellyfin`/`qBittorrent` both render, with
the temperature row present on one and absent on the other — the absent-is-not-zero behaviour
M4.10 observed on the phone.

---

### M5.5 — Service detail ✅

On the watch, tapping a roster row:

```
Cron                          Nonexistent
Running                       Unreachable
active (running)              systemd has no unit called
                              "definitely-not-a-unit.service".
                              Check the exact name with
                              `systemctl list-units --type=service`.
```

`active (running)` is systemd's own word, carried verbatim; the second is the agent's own
diagnostic reaching a wrist. That is the whole point of the screen — the row says a service
is unhealthy, this says **why**.

**One item, not a list, and *which* one is a decision.** The phone shows every session and
every torrent because a thumb flicks through twelve rows in a second; twelve rotations of a
crown is not the same interaction. So the screen shows one and the count says how many it
stands for. A transcoding session outranks a direct play — one 4K transcode saturates the CPU
every other service on that host shares — and an active transcode outranks a paused one.
Transfers rank by speed among those actually moving, which is the finding the qBittorrent
adapter already recorded when it stopped sorting the whole list by `dlspeed`. 14 tests.

**Navigation arrived early.** `SwipeDismissableNavHost` rather than hand-rolled back, because
on Wear the back gesture *is* a swipe and an app that implements it by hand gets the edge
behaviour subtly wrong. One `DashboardViewModel` is shared by both destinations, so opening a
service costs no radio round trip. M5.8 still owns making rotary and haptics systematic.

**Not verified on hardware, and stated rather than implied:** the `now_playing` and
`transfers` rendering. The VM runs only `type: systemd` units, and the HP host — which has
Jellyfin and qBittorrent — is unreachable from the watch, which has no Tailscale. The focus
rules are covered by unit tests; the *drawing* of them is not, and M5.17 should carry it.

**A second thing synthetic input cannot do.** Swipe-to-dismiss does not fire from
`adb shell input swipe`, exactly as RemoteInput does not fire from `input tap`. Back via
`KEYCODE_BACK` confirmed the navigation itself is correct, so this is an automation limit
rather than a defect — and it is the second item M5.17 has to hand to a person.

The original description follows.

---

One service, full height: health, reported status, reasons, and what it is doing. Activity
capabilities (`now_playing`, `transfers`) get watch-shaped renderers — a wrist shows *one*
session, not five.

---

### M5.6 — Lifecycle actions, with confirmation ✅

**The acceptance criterion, met from the wrist:** `MainPID 9739 → 11833` on a restart, read
from systemd rather than from the agent's own report — the standard used since M3.1.

The audit log attributes it to the watch, not the phone:

```
action accepted   service=cron action=restart device=207470f9dc3048cb risk=disruptive
action accepted   service=cron action=stop    device=207470f9dc3048cb risk=destructive
```

`207470f9dc3048cb` is the device id M5.3b minted. ADR-0006's per-device attribution, working
end to end from a watch.

**Both ceremonies observed, including the negative case.** A 600ms press on the destructive
control did **not** fire — cron stayed `active`. A 1800ms hold did: `inactive`, `MainPID=0`,
logged `risk=destructive`. A guard nobody has tried to defeat is not a guard.

**State-dependence came from the agent, not from the client.** After the stop the screen
offered `Start Cron` alone, with `Restart` and `Stop` gone, and read `inactive (dead)` plus
the agent's reason `"cron.service" is stopped.` The watch decided none of that — ADR-0002
Amendment 1 did, and this is that behaviour arriving on a second form factor with no server
change.

**"Asked", not "Done".** The agent answers an invocation with an acceptance; the terminal
outcome arrives on a stream this client deliberately does not hold (ADR-0004). So the banner
says `Restart Cron — asked` and the screen then shows what it observed. Claiming success from
an acceptance would assert something nobody saw — the same class of error as rendering stale
green.

**`Unrecognised` risk is treated as destructive.** A level this build has never heard of came
from a newer agent, and guessing it is mild is the one mistake that cannot be undone by
asking again.

**The hold duration is NOT yet measured**, and the plan said it would be. It is the phone's
1200ms, with the reasoning recorded in `ActionControls.kt`: the risk a watch adds is not
longer accidental contact — a sleeve brush does not sustain 1.2 seconds — but a *harder
deliberate hold*, on a small target, on a raised arm. Lengthening it would trade a sufficient
safety margin for worse ergonomics. The watch-specific answers are a bigger target, which is
done, and haptics, which are M5.8. **M5.17 decides whether 1200ms is holdable on a wrist.**

Unlike swipe-to-dismiss and RemoteInput, a synthetic long press *does* drive this, which is
why both the fire and no-fire cases could be checked here rather than deferred.

The original description follows.

---

Restart, stop and start, state-dependent exactly as the API reports them. Risk classes carry
across: `disruptive` confirms, `destructive` needs press-and-hold.

**Press-and-hold timing is measured, not inherited.** The phone's duration is not
automatically right for a device you knock against a doorframe.

**Acceptance:** a service restarted from the watch, confirmed by `MainPID` changing on the
host — the standard used since M3.1.

---

### M5.7 — Host power actions ✅

**It was not the one-paragraph outcome, and the reasoning is in
[ADR-0014 Amendment 1](adr/0014-watch-pairing-address-handoff.md).** M5.0 made `host.power`
**not a default** — which is not the same as making it unavailable, and the ADR had already
written the sentence that decides it: *"An operator who wants it can ask for it by name,
exactly as on the phone, and press-and-hold still applies."* Press-and-hold only applies to a
control that exists.

Two facts closed the question:

- The agent returns the power actions to **every** caller holding `read`, deliberately, so a
  client can tell *this agent cannot* apart from *this device was not allowed*
  (`agent/internal/api/hostpower.go`). The watch's snapshot had been carrying reboot and shut
  down since M5.4 and dropping them on the floor — an accident of nobody having written the
  screen, not a policy.
- Nothing on a watch face distinguishes "correctly withheld" from "accidentally dropped". So
  the gate is a named function with a case-by-case test rather than a condition buried in a
  composable.

**What shipped**

| | |
| --- | --- |
| Gate | `powerAccess(scopes, hostActions)` → `Ungranted` / `NoneOffered` / `Offered` |
| Entry | an `EdgeButton` pinned to the bottom bezel, **absent entirely** without the grant |
| Screen | `HostPowerScreen` — what the machine is busy with, then the agent's actions |
| Gesture | `ActionButton`, unchanged from M5.6, so `destructive` is the same press-and-hold |

**Three decisions worth keeping.**

*The entry point is hidden, not greyed.* This is the one place the two clients present the
same permission differently. On the phone a greyed row with an explaining sentence costs
nothing inside a menu somebody opened on purpose. On 233dp it would occupy the most valuable
pixels for an explanation that cannot fit beside it, on the device least able to act on it.

*It is an `EdgeButton`, not a row in the roster.* A control that ends a machine must not be
reachable by momentum — the last flick of a scroll should never land a thumb on it. It is
also not a service, and a roster containing "Shut down" would present the machine as one of
its own services that could be restarted and come back.

*`invokePower` is not `invoke` with a different endpoint.* `invoke` waits two seconds and
re-reads the agent, because observing a result is how a polling client reports one honestly.
Doing that here would be a defect: a power action that **worked** takes the agent down with
the machine, so the refresh would fail and the screen would say "Could not reach the agent"
at the moment everything went right. The success case would be the one that looked broken.
So it stops at the acceptance and says *"Reboot — asked. The agent will go quiet now."*
Nothing ever claims "Rebooted"; no client can, because the connection that would have
reported it is gone.

**Tests:** `HostPowerAccessTest`, 12 cases — the gate per scope combination, and the busy
line, including that a capability the agent could not read contributes nothing rather than
zero and that finished transfers are not counted as work a shutdown would interrupt. 236
client tests, 0 failures.

## Verified on the OnePlus Watch 2R — 2026-09-18

Deferred for two days because the test VM's bridged adapter is attached to a Wi-Fi NIC the
host was not connected to; done on the first evening it was. **Run as an A/B on one build,
against one agent, with the token's scopes as the only variable.**

| | |
| --- | --- |
| Ungranted | scrolled to the end of the roster — **no `Machine` button at the bezel** |
| Granted | re-paired with `read,service.control,host.power` — **the button is there** |
| 600ms press | nothing fired; `boot_id` unchanged |
| 1800ms hold | *"Restart machine — asked. The agent will go quiet now."* |
| The machine | `boot_id 974761d4… → 933e7416…`, down at t+6s, back at t+18s |

```
host power action accepted       action=reboot action_id=c81840443c71bd98 device=OPWWE234
host power action handed to logind  action=reboot action_id=c81840443c71bd98
```

The reboot is confirmed by the kernel's own `boot_id` changing, not by the agent's report —
the standard used since M3.1, and the only one available here, because the process that would
have reported success is the one that went down.

### The defect it found, which is the point of doing this at all

**The first build rendered both action descriptions and neither button.** Every unit test
passed; the gate was correct; the screen was wrong. In `HostPowerScreen` the button and its
description were emitted as **two siblings inside one lazy item slot**, and a
`TransformingLazyColumn` item takes a single composable — so the description was drawn and the
control was not. Fixed by wrapping them in a `Column`.

Nothing short of running it could have caught that. It is not a logic error, it produces no
warning, and the screen it produces looks deliberate: two calm sentences about what reboot and
shut down do, on a screen titled "Machine", with no way to do either. **A watch granted
`host.power` would have had no way to use it, and the app would have looked finished.**

### Also confirmed along the way

- **The address handoff still works.** After clearing the app, the watch showed
  `192.168.1.10:7777` *"from your phone"* with nothing typed — ADR-0014's flow, and the mono
  `DataSmall` style holding the address on one line without breaking mid-octet (M5.2's fix).
- **The watch recovers on its own.** After the reboot it found the agent again on the next
  poll with no intervention.
- **RemoteInput still cannot be driven synthetically.** `input text` fills the field; tapping
  send **clears it instead**. One real tap completes it immediately. Third confirmation, and
  the reason a person is still needed for any pairing test.

**Correction to an earlier reading in this file's spirit:** the `Nonexistent` fixture's status
mark is *filled*, not hollow. `unreachable` is a fact the agent established, so it is drawn as
one; the hollow ring is reserved for `unknown`. An earlier screenshot appeared to show a ring,
which was the lazy column's edge transform clipping the item, not the status encoding.

**Still owed to M5.17:** whether `HOLD_MILLIS = 1200` is holdable on a raised wrist by a
person rather than by `input swipe`, and the `now_playing` / `transfers` rendering.

---

### M5.8 — Rotary, swipe-to-dismiss, haptics

The phase that separates a Wear app from a shrunk phone app.

**The audit came first, and it changed what this phase is.** Two of the three were already
true — by library default rather than by decision, which is the same condition M5.7 found and
the same reason to write it down: an absence nobody chose looks exactly like one somebody
did.

| | state before this phase | what M5.8 did |
| --- | --- | --- |
| Rotary | `TransformingLazyColumn` wires `RotaryScrollableDefaults.behavior(state)` and `requestFocusOnHierarchyActive()` itself, and **all four screens use one** | recorded it, and recorded what cannot be verified here |
| Swipe-to-dismiss | `SwipeDismissableNavHost` covers detail and power; `Theme.DeviceDefault` sets `windowSwipeToDismiss` for the root | recorded it; needs a finger to confirm |
| Haptics | **nothing, anywhere** | the whole of the code below |

#### Haptics: three events, deliberately only three

A watch is often operated without looking at it — most of why the app exists on one. So the
wrist carries the facts a glance would otherwise have to: **that a control committed**, and
**whether the agent took it or refused it**.

| | when | why it is felt |
| --- | --- | --- |
| `committed()` | a hold crosses its threshold, or a disruptive action is confirmed | the decision is made; you can let go |
| `accepted()` | the agent took the request | it is on its way |
| `refused()` | the agent declined it, or the call failed | it is *not* on its way |

Everything else stays silent — taps, scrolls, navigation, arriving at a screen. A device that
buzzes at everything communicates nothing: the signal stops being information and becomes
texture, and then the one buzz that mattered is indistinguishable from the twenty that did
not.

`refused()` is the row that earns the file. Without it a failed action on a screen nobody is
looking at is indistinguishable from a successful one, and the watch becomes a thing you have
to verify on your phone — the opposite of the point. It matters most on the **power** screen,
where success is silence by design, so a refusal is the only thing there is to report.

**The threshold buzz fires at the threshold, not at the lift.** It is the signal that says
*stop pressing now*, which is worth nothing if it arrives after you already have. The cost,
stated rather than hidden: a hold that reaches the threshold and is then cancelled will have
buzzed for something that did not happen. The alternative is a control you must watch to use,
on the device least suited to being watched.

The constants are semantic (`GestureThresholdActivate`, `Confirm`, `Reject`), not durations
chosen here. Hand-rolling amplitudes would mean overruling a vendor's tuning for their own
motor — and `aw-haptic-hv` on the Watch 2R is not the motor it would have been tuned against.

#### The acceptance criterion was falsified by the hardware

It read: *every screen reachable and dismissable using only the crown and a swipe.*

**The OnePlus Watch 2R has no crown to rotate.** Its input devices, read off the device:

```
Device 4: sec_touchscreen    Device 3: qpnp_pon
Device 2: gpio-keys          Device 5: aw-haptic-hv
```

No `ROTARY_ENCODER` source. The side button is a button. So the criterion as written cannot
be met on the only watch this project has, and pretending otherwise would put an untestable
claim in the record.

**Amended acceptance, split by what can actually be established:**

| | how |
| --- | --- |
| Every screen scrolls by rotary | **not verifiable on this hardware.** Needs a Wear emulator, which has a rotary control |
| ~~Every screen dismissable by swipe~~ | ✅ **2026-09-20** — swiped right from a service detail back to the dashboard |
| ~~The three haptics fire, and are distinguishable~~ | ✅ **2026-09-20** — see below |

None of the three can be closed by automation, which is unusual for this project and worth
saying plainly rather than quietly downgrading.

#### Felt on the wrist — 2026-09-20

Performed by hand rather than by `input swipe`, which is better evidence: the gestures were
real, and the judgement being made is subjective by nature.

| gesture | ceremony | felt |
| --- | --- | --- |
| Restart Cron | tap → confirm | **two buzzes**, second different |
| Stop Cron | press-and-hold | one at the threshold *while holding*, one **on release** |
| Restart Nonexistent | tap → confirm, **refused** | two buzzes, *"2nd one stronger"* |
| Stop Cron, polkit grant removed | hold, **refused** | *"sharper buzz… I felt that it failed"* |

**Swipe-to-dismiss works**, confirmed the same way and for the same reason — a right swipe
from a service detail returned to the dashboard. `input swipe` cannot fire it, so this was
never going to be closed by anything but a finger. That leaves rotary as the only part of
M5.8 still unverified, and it is unverifiable here rather than untested: there is no encoder
to turn.

**The refusals were real, not simulated.** `Nonexistent` points at a unit that does not exist,
and the second was produced by removing `cron.service` from the polkit allowlist — exactly the
missing-grant failure ADR-0002 describes — leaving the agent healthy throughout. A faked
transport error would have tested the error path without testing the thing that matters: that
a refusal the *host layer* produced reaches the wrist.

**"I felt that it failed", without looking.** That is the whole claim this phase makes.

##### A hypothesis the hardware corrected

After the first run — two buzzes on the tap path, *"one longer buzz"* on the hold path — the
reading was that the threshold and acceptance buzzes were **fusing**, and the fix being
considered was to delay the outcome buzz so it could not.

**That was wrong, and the second run falsified it.** The operator's own description:

> when i hold it down one buzz comes but when i leave the red button another buzz comes

The two events were always separable. What differs is the *waveform*: `Confirm` is soft
enough to read as one longer pulse when it follows the threshold closely, and `Reject` is
sharp enough to stand alone. So the success case feels like one event and the failure case
feels like two — an asymmetry that was never designed, and is kept because it points the
right way. **The failure is the one that needs attention, and it is the one that announces
itself.**

The delay was not implemented. It would have fixed nothing and cost the thing that makes the
threshold buzz worth having — its immediacy.

---

### M5.9 — Every state, designed rather than defaulted ✅

Four states per screen, each written on purpose:

| State | What it must not do | outcome |
| --- | --- | --- |
| Loading | show a blank screen | a spinner **and a sentence** — a bare spinner does not say whether the app is thinking or the agent is slow |
| Empty | look like an error | *"No services configured. Add them on the host."* — an instruction, not a fault |
| Error | show a stack trace or a bare code | the agent's own words, shortened for a wrist, plus **Try again** |
| **Stale** | show confident green while the agent is unreachable | **it was doing exactly that — see below** |

#### The defect this phase existed to catch

`ServiceDetailScreen` had been receiving **`stale = false`, hardcoded**, since M5.5. The
dashboard computed staleness correctly and kept it to itself; every screen reached *through*
the dashboard was told the reading was fresh, forever.

So a service detail could sit on screen showing a confident green **Running** with the agent
dead and nothing to say so — the precise failure the whole staleness design exists to
prevent, reintroduced one screen down.

**It is worse there than on the dashboard, because the detail screen is where you act.**
Deciding to restart something from a reading two minutes dead is a different class of mistake
from merely reading one. The power screen had no notion of staleness at all, and it is the
screen where a wrong reading costs the most.

The clock is now hoisted to `rememberStaleness`, owned by the one composable all three
screens share. Still composable scope rather than the ViewModel — it stops when the app is
off screen, because a ticker behind a dark panel spends battery correcting a display nobody
is looking at. The old comment justifying its old home is quoted in the new file, because it
was *right* when the dashboard was the only screen, and it stopped being right silently.

#### Stale on the power screen withdraws the claim rather than blocking the action

`busyClaim(services, stale)` returns one of three things, and the middle one is why it is a
function rather than an `if`:

| | |
| --- | --- |
| `Unknown` | the reading aged out — *"Last reading is out of date — what this would interrupt is unknown."* |
| `Busy` | something is running, and here is what |
| `Quiet` | asked, and nothing is running — say nothing |

**Stale is not the same as quiet, and collapsing them is the bug worth a test.** Both would
render as an empty line, and on a screen whose buttons end a machine, *"nothing is running"*
and *"I have no idea what is running"* are the two sentences that must never be confused. The
first invites the button; the second should give pause.

The buttons still work either way. The operator owns the machine and may have excellent
reasons to act on a reading they know is old; refusing would make the tool argue with the
person it exists to serve (ADR-0002 Amendment 2).

#### Verified on the watch — 2026-09-25

The agent was stopped so nothing could refresh the reading, then the clock was allowed to run
out.

| | before | after 90s |
| --- | --- | --- |
| Service detail | **Running**, green | **Unverified**, grey |
| Roster rows | filled marks, "Running" | **hollow** marks, "Unverified" |
| Machine screen | *"Right now: …"* | *"Last reading is out of date…"* |

All three encodings move together — colour, shape and word — which is what `DESIGN.md` §3
requires and why the roster is still readable by somebody who cannot separate the hues.

**Error and recovery**: relaunched cold against a dead agent → *"Could not reach the agent"*
with **Try again**; agent restarted, button tapped, dashboard live again.

#### A layout defect only the device could show

The error state first drew as **`ould not reach the agen`** — clipped at both ends.

A round screen is not a rectangle with the corners missing. At the top of a 466px circle the
chord is far shorter than the screen is wide, and a full-width line drawn there runs off the
glass. **Horizontal padding did not fix it** — the content had to move down to where the
circle is wide. Now `fillMaxWidth(0.78f)` with a 40dp top inset, measured on the device
rather than derived.

Every state test so far had been a logic question. This one was pure geometry, and no unit
test, screenshot test at rectangular sizes, or amount of reading would have produced it.

**Not verified on hardware: the empty state.** Producing it needs an agent with no services,
and the attempt to edit the VM's config into that shape broke the YAML — the agent refused
to start with `parse config: yaml: line 22: did not find expected key`, which is itself the
config validation behaving correctly. The fixture was restored rather than cut at further.
The code path is two lines and reads correctly; it belongs to **M5.17**, and it matters more
than its size suggests, because `services: []` is what ships and therefore what every new
operator sees first.

---

### M5.10 — Ambient mode and battery behaviour ✅ (built; **not observed on this watch**)

A watch app that keeps a screen bright and a radio awake is a bad app regardless of how it
looks.

**What shipped**

| | |
| --- | --- |
| Ambient screen | hostname, verdict, and **the age of the reading** — three lines of unfilled text on black |
| Polling in ambient | none, by construction: the ambient branch returns before the nav host |
| Returning to interactive | refreshes, because ambient deliberately did not |
| Plumbing | `AmbientLifecycleObserver`, `WAKE_LOCK`, and the `com.google.android.wearable` shared library |

**Ambient says *when*, because it cannot say *now*.** It does not poll, so everything on it
is by definition the last thing known. A dimmed screen reading `Operational` with no age
would be confident green with nothing behind it — the failure this project keeps legislating
against, and worse here, because ambient can sit on a wrist for an hour. So the age is the
second line, and it is what makes the first line honest. `readingAge` rounds coarsely (`now`,
`4m ago`, `2h ago`) because a glance is asking *current or old*, not for seconds.

**The status colour is dropped in ambient, and that is a deliberate loss.** Colour is the
weakest of the three encodings `DESIGN.md` §3 requires; the word survives without it, and a
coloured block is exactly the shape of thing that burns into an OLED panel over a wear-day.

**Ambient replaces the whole navigation graph** rather than dimming whichever screen was
open. A dimmed detail screen would leave a service's controls on a lowered wrist, and
ambient answers a different question anyway: interactive asks *what is going on with this
service*, ambient asks *is everything still fine* — which is the dashboard's question and the
only one worth keeping a panel lit for.

**A double-poll removed while here.** `DashboardScreen` triggered its own refresh on
composition. Once ambient gave the app a second way to become visible, that would have meant
two fetches on every wrist-raise landing on the dashboard — on the one phase whose subject is
not spending battery. The poll now lives at `PairedApp`, the boundary every destination
enters through.

#### It does not engage on the OnePlus Watch 2R, and the evidence is recorded

The callback never fired. **The app asked correctly** — this is established, not assumed:

| | |
| --- | --- |
| Shared library present | `cmd package list libraries` → `library:com.google.android.wearable` |
| Declared | `uses-library` + `WAKE_LOCK` in the manifest |
| Registered | `CueSeekWear: ambient observer registered`, logged at startup |
| Result | `mWakefulness` went **Awake → Asleep**; `onEnterAmbient` never called |

And the system said what it did with the app:

```
AmbientTaskStackManager: Moving task [dev.cueseek.android.debug/…MainActivity] to the back of activity stack!
AmbientTaskStackManager: Task [#1|home|…SysUiActivity] should stay in the front.
```

**What is not established** is *why*, and it is worth two hypotheses rather than one
confident sentence:

1. OnePlus's Wear system does not grant third-party apps an ambient state, backgrounding
   them in favour of the watch face.
2. The watch was **off-wrist and on a cable** throughout. Wear's always-on behaviour can
   depend on the device believing it is worn, and every test here ran on a desk.

Telling those apart needs the watch on a wrist for a day, which is **M5.17** — where the
battery measurement already lives. Until then this phase is *built and unobserved*, not
*working*.

The startup log line is kept in the shipped code for exactly this reason: ambient is the one
behaviour with no visible evidence when it fails. The screen goes dark either way, and
"declined by the system" and "never asked" look identical from the outside.

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
- ~~No `Machine` button without `host.power`, and a reboot from the wrist with it~~ — both
  done 2026-09-18, recorded under M5.7
- ~~Swipe dismisses; haptics fire~~ — done 2026-09-20, recorded under M5.8. **Rotary is
  not checkable on this watch at all** — it has no encoder
- Tile and complication both installed and updating
- **Whether ambient engages at all on this device**, on a wrist rather than on a desk — it
  never fired while cabled and off-wrist, and M5.10 records two hypotheses for why
- Ambient behaves for a full hour without the screen burning
- **The empty state** — an agent with `services: []`, which is what every new operator sees
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
