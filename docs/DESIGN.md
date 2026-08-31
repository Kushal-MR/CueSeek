# CueSeek Design System

The rules and the values, in one place. [ADR-0010](adr/0010-design-system-m3-expressive.md)
explains *why* this system is the way it is; this file states *what* it is.

**Using this with a design tool.** Paste this file, or point the tool at it, before asking
for anything. Everything under "Non-negotiables" is a constraint, not a preference — a
generated design that breaks one of them is wrong rather than different. "Open questions"
at the end lists what a tool is genuinely free to explore.

**Status of values.** Everything marked ✅ exists in `clients/android/core/design` and is
the source of truth. Everything marked 🔜 is guidance for work not yet built.

---

## 1. What is being designed

CueSeek is an **operations console for a self-hosted home server**. It monitors and
controls services — Jellyfin, qBittorrent, and more later — over a private Tailscale
network. Android phone first, Wear OS second.

It is an **instrument panel, not a content app**. The primary interaction is a two-second
glance answering *"is everything fine?"*. The secondary one is acting on the answer.

That single sentence decides most things. Legibility of state outranks richness of
presentation. There is no media to showcase, no feed to browse, no engagement to optimise.
The screen is read at arm's length, in a hurry, often to confirm nothing is wrong.

**Foundation:** Material 3 Expressive, with an owned token layer. Inherit the fundamentals —
accessibility, touch targets, state layers, motion physics, Wear parity. Own the identity —
colour, type, shape, motion, and the status language.

---

## 2. Non-negotiables

1. **Chroma means attention.** Healthy is nearly achromatic. Saturation is reserved for
   things that need a human. A dashboard where green is always present is one where green
   means nothing.
2. **Motion means something happened.** Every animation must be traceable to a cause: a
   touch, a state change, data arriving, a navigation. Nothing loops while the app idles.
   See §7 — this is a licence for *more* motion, not less, provided it has a reason.
3. **Status colours are not themeable.** They carry meaning. Dynamic colour is off, and the
   status roles live outside `ColorScheme` entirely so nothing can retheme them.
4. **Status is never colour alone.** Every state carries an icon or shape as well, and must
   survive dark mode, colour blindness, and a 1.4" always-on display at low brightness.
5. **Never show stale data as current.** If the agent has gone quiet, health degrades to
   `unknown` in the data layer, before any screen sees it. No screen may opt out.
6. **The UI never branches on which service it is.** Capabilities are looked up by id. A
   test enforces this; adding qBittorrent must not require touching a screen.

---

## 3. Colour ✅

A muted **sage / eucalyptus** family. Neither theme uses pure white or pure black — both
ends carry a green cast, so the app reads as one material rather than ink on paper.

### Light

| Role | Hex | Role | Hex |
|---|---|---|---|
| primary | `#4C6650` | background / surface | `#F1F4EE` |
| onPrimary | `#FFFFFF` | onBackground / onSurface | `#191D18` |
| primaryContainer | `#D3E3D2` | onSurfaceVariant | `#434840` |
| onPrimaryContainer | `#0A2010` | surfaceContainerLowest | `#FCFDFA` |
| secondary | `#5A6459` | surfaceContainerLow | `#F1F4EE` |
| secondaryContainer | `#DEE5DB` | surfaceContainer | `#EBEEE8` |
| tertiary | `#3B6664` | surfaceContainerHigh | `#E5E9E2` |
| tertiaryContainer | `#CDE3E0` | surfaceContainerHighest | `#E0E4DC` |
| outline | `#737870` | outlineVariant | `#C3C8BE` |
| error | `#8C3A31` | errorContainer | `#F0DCD8` |

### Dark

Not the light scheme inverted. Light puts brighter content on a tinted ground and separates
rows with hairlines; dark raises the container above a deeper page and separates with tone,
because a drop shadow does nothing on a dark ground. Container chroma is pulled back so
amber and clay do not glow at low brightness.

| Role | Hex | Role | Hex |
|---|---|---|---|
| primary | `#B1CDB2` | background / surface | `#0E1210` |
| onPrimary | `#1D3722` | onBackground / onSurface | `#E2E7DE` |
| primaryContainer | `#344E38` | onSurfaceVariant | `#B4BCB1` |
| onPrimaryContainer | `#CDE8CF` | surfaceContainerLowest | `#090C09` |
| secondary | `#C2CBBF` | surfaceContainerLow | `#161B15` |
| secondaryContainer | `#424A41` | surfaceContainer | `#191E18` |
| tertiary | `#A3CDC9` | surfaceContainerHigh | `#232821` |
| tertiaryContainer | `#224E4C` | surfaceContainerHighest | `#272D26` |
| outline | `#8C948A` | outlineVariant | `#333A32` |
| error | `#E0A79E` | errorContainer | `#52241E` |

### The status palette

Deliberately **outside** `ColorScheme`. M3 sets the precedent: `error` is a static role that
does not follow the wallpaper, because it means something.

`unreachable` and `error` are the same fact wearing two names, so they are the same colour —
which means an M3 component reaching for the error role lands inside the status language
rather than beside it.

| State | Light | Light container | Dark | Dark container |
|---|---|---|---|---|
| healthy | `#3F5E46` | `#E3EBE0` | `#A8C4A6` | `#29392B` |
| degraded | `#7A5215` | `#F3E2C4` | `#E5BE84` | `#46320F` |
| unreachable | `#8C3A31` | `#F0DCD8` | `#E0A79E` | `#52241E` |
| unknown | `#5E6560` | outline `#848C82` | `#9AA298` | outline `#6B736B` |

**Tally-rule variants.** The 8dp tally rule uses *dimmed foregrounds*, not containers. The
containers read correctly behind a 17dp icon but measured 1.10 / 1.15 / 1.19 against the
page and made the rule effectively invisible — caught by a greyscale golden test, not by
eye. Now 3.35 / 3.57 / 3.56 in light, 3.93 / 4.33 / 3.69 in dark.

| | Light | Dark |
|---|---|---|
| tallyOnHealthy | `#6E8C70` | `#5A7A5C` |
| tallyOnDegraded | `#9E7A38` | `#9A722C` |
| tallyOnUnreachable | `#AE7068` | `#9A5E55` |

`beat` — the freshness pulse dot — is `#4C6650` light, `#A8C4A6` dark.

---

## 4. Typography ✅

**IBM Plex Sans**, with **IBM Plex Mono** for anything numeric. Plex was drawn for a
technology company's own system software and reads precise rather than friendly. The mono
sibling is why Plex was chosen over an equally legible neutral like Inter: timestamps and
ages need true tabular figures, and taking them from the same superfamily keeps the screen
one voice instead of two.

**Mono is confined to data** — ages, timestamps, counts. Interface language stays
proportional. That line is what separates instrumentation from terminal pastiche.

**Two weights only**, 400 and 500. Hierarchy comes from size, tracking and colour; reaching
for a third weight usually means the hierarchy is not working.

**Tracking tightens as size grows** — negative at 24sp, neutral mid-scale, open at 12sp.
Small type needs air; large type needs discipline.

| Role | Size / line | Weight | Tracking | Used for |
|---|---|---|---|---|
| headlineSmall | 24 / 30 | 500 | −0.3 | the verdict; the only headline on screen |
| titleMedium | 16 / 22 | 500 | −0.1 | service names |
| titleSmall | 14 / 20 | 500 | 0.0 | section labels |
| bodyMedium | 14 / 20 | 400 | 0.1 | dialog and sheet copy |
| bodySmall | 13 / 18 | 400 | 0.1 | supporting lines under a name |
| labelLarge | 14 / 20 | 500 | 0.1 | buttons |
| labelMedium | 12 / 16 | 500 | 0.5 | the host eyebrow |
| **Data.Small** | mono 12 / 16 | 400 | 0 | ages, timestamps in rows |
| **Data.Emphasis** | mono 12 / 16 | 500 | 0 | counts in the summary |

`bodySmall` is 13sp rather than M3's 12 — Plex has a more modest x-height than Roboto and
12sp goes weak beside a 16sp Medium name. `displaySmall` is defined but unused: an
operational screen has no room for 36sp, and defining it stops a stray role falling back to
Roboto and looking foreign.

---

## 5. Shape ✅

Roundness **alternates by level** rather than being applied everywhere, so shape carries
hierarchy instead of just friendliness.

| Level | Shape | Why |
|---|---|---|
| Screen | none | it is the page |
| Header | **no container at all** | flat and quiet; type alone carries it |
| Tally rule | full | reduced to a rule; the only fully-round strip |
| Roster | 28dp | one soft mass holding the work |
| Rows | square | quiet inside the container |
| Status mark | full circle | a closed circle means settled |
| Buttons, menus | M3 default | standard |

Scale: `extraSmall 4` · `small 8` · `medium 12` · `large 16` · `extraLarge 28`.

**The header having no surface is load-bearing.** It is what makes the roster read as the
one substantial object on screen, and it is why adding a summary card would undo the
hierarchy rather than reinforce it.

**Rows are square on purpose.** A rounded row inside a rounded container produces two
competing radii and reads as a card that failed to separate. The container is the shape;
rows are its contents.

---

## 6. Layout and spacing ✅

The 8dp grid M3 Expressive asks for.

| Token | Value | Notes |
|---|---|---|
| screenMargin | 16dp | |
| rosterInset | 8dp | the roster sits inside the screen margin |
| rowHorizontal | 16dp | |
| rowVertical | 12dp light / **14dp dark** | dark separates by tone, and low-contrast type needs air more than it needs lines |
| markGap | 16dp | status mark to text column |
| textStart | 64dp | 16 margin + 32 mark + 16 gap; dividers inset to exactly where text begins |
| menuInset | 4dp | trailing button to roster edge |

**One surface holding many rows, never a card per item.** A card each pays for padding twice
and caps a phone at three visible services; the roster fits eight or nine and still reads as
a deliberate object.

**Depth differs by theme.** Light gets a 2dp shadow because it has a ground to cast onto.
Dark gets none, because a shadow on a dark page is invisible and the tonal step does the
work there.

---

## 7. Motion

The rule people get wrong: **motion must be caused**. That is not a budget on how much
motion there is — it is a rule about where it comes from. Two categories.

### Responsive motion — encouraged, should be everywhere 🔜

Motion with a cause. Press states, releases, expansions, dismissals, values changing,
navigation, data landing. This should feel continuous, spring-based, and unhurried, in the
manner of M3 Expressive. If an element changes at all, it should change *smoothly*; snapping
is the exception and needs a reason.

Use spring physics for anything that changes **size or position**; use the easing curves for
anything **entering or leaving**.

| Event | Treatment |
|---|---|
| Press on a row or button | state-layer tint fades in under the ripple, `DurationExit` |
| Release | tint fades out; the ripple finishes on its own |
| A value changing in place | `tallySpring` — low stiffness, no bounce |
| A status mark changing | `markSpring` — medium stiffness, no bounce |
| Content entering | fade + expand, `DurationEnter`, `EmphasizedDecelerate` |
| Content leaving | fade + shrink, `DurationExit`, `EmphasizedAccelerate` |
| Crossfading a verdict | `AnimatedContent`, enter/exit as above |
| A sheet or dialog | M3 defaults; predictive back handles dismissal |
| An operation in flight | one bounded indicator, running one direction |

### Ambient motion — banned

Motion with no cause. Shimmer skeletons, looping placeholders, breathing glows, decorative
parallax, anything that moves while nothing is happening.

A shimmering skeleton means "we are working". On this screen the honest signal for "we do
not know yet" is stillness plus `unknown`. An operations console that looks busy while idle
is lying about the thing it exists to report.

### Tokens ✅

| Token | Value |
|---|---|
| `Emphasized` | `cubic-bezier(0.2, 0, 0, 1)` |
| `EmphasizedDecelerate` | `cubic-bezier(0.05, 0.7, 0.1, 1)` |
| `EmphasizedAccelerate` | `cubic-bezier(0.3, 0, 0.8, 0.15)` |
| `DurationEnter` | 400ms |
| `DurationExit` | 200ms |
| `DurationStandard` | 300ms |
| `DurationStateChange` | 220ms — colour and alpha crossfades |
| `tallySpring` | no bounce, `StiffnessMediumLow` |
| `markSpring` | no bounce, `StiffnessMedium` |

**Never bounce on a status change.** A service going degraded must not feel playful.

### Two motions that carry meaning ✅

**The beat.** One pulse per received event — roughly every 15 seconds against a live agent,
which is deliberately slow. A dot that blinks constantly is noise; a dot that moves twice a
minute is a pulse you notice the absence of. Keyed to the last event's *arrival*, so it never
ticks through a frozen stream. Expand 180ms, settle 520ms, peak scale 1.55, trough alpha
0.55.

**The refresh band.** While a manual refresh is genuinely outstanding, a band runs the
indicator track — head leading on a decelerating curve, tail following on an accelerating
one, so it stretches out of the left edge, sweeps, and gathers into the right. One pass per
second. It always runs **one direction**: the first version bounced, and reversal reads as
waiting rather than progress. Admissible under the motion rule because it is bounded by a
real request and stops when the request does.

### Reduced motion 🔜

Read `Settings.Global.ANIMATOR_DURATION_SCALE`. If zero, replace looping motion with a
static equivalent — never with nothing. The honest alternative to a moving indicator is a
still one, not silence.

---

## 8. Components built so far ✅

**Status mark.** A 32dp circle carrying the status icon. Closed circle when settled; dashed
ring for `unknown`, whose outline meets 3:1 on both surfaces.

**Tally rule.** The whole fleet's composition as an 8dp rule, encoding **proportion only**.
When stale it stops being segments and becomes a dashed hairline: the distribution is no
longer knowable, so it is no longer claimed. This is the one place colour stands alone, and
it is acceptable precisely because the verdict above and the rows below both carry icon and
text.

**Provenance line.** When live: status counts, plus a beat dot and "live". When stale:
"Stream open" beside "no data 34s". Stating both at once is the clearest way to say that a
connection being alive is not evidence the data is.

**Service row — two targets.** The body *inspects* the service; the trailing ⋮ is
everything else. Body tap opens the detail sheet — health, activity, everything observed.
The ⋮ carries "Open web interface" plus lifecycle actions gated by risk: safe fires,
disruptive asks, destructive must be held. The two targets are announced separately to
screen readers.

Reversed in M3.5 after use: the body used to open the web interface, which put the gesture
that leaves the app on the easiest target and made the detail sheet unreachable on a host
where every service has one.

**Risk ladder.** `safe` → tap. `disruptive` → dialog with a button. `destructive` and
anything unrecognised → dialog with a hold-to-confirm bar, 1200ms, where the fill *is* the
progress so releasing early visibly abandons it. Effort in proportion to consequence.

Three levels, and it stays three. Powering the machine off is genuinely worse than stopping
a service — a stop is undone from the same screen, a power-off needs somebody in the room —
but the vocabulary is public API shared with clients that already exist, and a fourth level
would force every one of them to handle a value it has no interaction for. **The difference
is carried in words instead**, which is where it is actually read.

**Host power** (M3.7). Reboot and shut down live in the host menu, never in a service ⋮ —
the machine is not one of its own services, and putting "restart the host" in the same list
as "restart Jellyfin" would make the two look like the same kind of act. The menu only
asks; the confirmation decides, so a machine is never one tap inside a list somebody is
scrolling.

The confirmation **names what it will interrupt** — "Right now: 2 streams playing and 1
transfer running" — coloured, because that is the sentence that should give somebody pause.
It does not block. The operator owns the machine and may have good reasons to shut it down
mid-transcode; a console that argued with them would be worse than one that stayed quiet,
and one that knows and says nothing is wasting the only thing it is uniquely placed to say.

Items grey out when the device's token lacks `host.power`, with a line saying so. Showing
nothing would be indistinguishable from an agent too old to offer them, and those two
problems have completely different fixes.

**Detail sheet.** Detail only — no actions. With actions in both, one screen offered the
same verbs three times, and a second entry point to a destructive action is a second thing
to keep gated correctly forever.

**Host vitals strip** (M3.6). A **3×3 grid**: CPU, memory and the fullest filesystem as
three columns, each with a label and percentage, an 8dp rule, the absolute figure beneath it,
and a machine-level fact on the last row — uptime, load, temperature. Everything left-aligns
within its column except the percentage, which anchors right.

The grid is the correction, not the starting point. The first version put the absolute
numbers in one full-width dot-separated line under a three-column row, which belonged to
none of the columns and read as text that had overflowed. Alignment is what turns a row of
numbers into a table somebody trusts.

The same rule motif as the tally, at the same height, because the screen has already taught
what a rule means — proportion, nothing else. Not a card: the roster is the one substantial
object on screen, and a second surface would flatten the hierarchy the header exists to make.

Its position down the column is the order of the questions: is anything wrong, can I believe
this, then how is the machine. A CPU figure above the line that says whether to trust it
would be the wrong way round.

**Colour here is unusually restrained, and deliberately.** Memory and storage tint at 85%
and again at 95%, because filling them is a state somebody must act on. **CPU is never
tinted** — a processor at 95% is a transcode doing its job, and colouring it would spend the
attention this palette reserves for decisions. Temperature is judged against the sensor's
*own* reported threshold rather than a number this app invented, so a laptop CPU and an NVMe
drive at the same 85°C are read differently, correctly.

**Words are chosen so they cannot be misread.** "load 0.1 of 4" was replaced because it
read as cores in use, which load average is not — it counts processes waiting to run or
blocked on disk and can exceed the core count, which is exactly why it appears next to a
percentage rather than instead of one. The temperature shows the sensor **closest to its own
stated limit**, not the hottest: on real hardware a chassis sensor and a CPU sensor sat a few
degrees apart and traded places minute to minute, so the strip kept changing what it was
talking about while nothing was happening.

**Nothing absent is drawn.** No placeholder, no dash, no empty meter. A machine with no
sensors shows no footnote; a first collection with no CPU figure shows two meters rather
than three. A placeholder would imply something was measured and found blank, which is the
one claim this payload is shaped never to make. And when the data goes stale the strip
**leaves** rather than greying out — a service keeps its timestamps because "healthy three
minutes ago" is information, and a three-minute-old CPU percentage is not.

---

## 9. Accessibility floor

- **WCAG AA.** 4.5:1 for text, 3:1 for meaningful non-text. Measured, not eyeballed — two
  real defects (a rule at 1.10, a ring at 2.00) were found by a greyscale golden test and
  were invisible by eye.
- **48dp minimum touch target**, even where the visible glyph is 18dp.
- **Never colour alone.** Every status carries icon or shape too.
- **Merged row semantics**, so a screen reader hears one node per row — and the merge must
  preserve the click action. `clearAndSetSemantics` erases it; that shipped once.
- **Announce the destination when there is more than one.** A row that says "Open Jellyfin"
  and then shows a sheet is lying to the users who most depend on accuracy.
- **Every gesture needs a non-gesture equivalent.** Pull-to-refresh also exists as a custom
  accessibility action.
- **Test in both themes and at 1.4".**

---

## 10. Anti-patterns

Rejected on this project, with reasons. A proposal that lands here is not a fresh idea.

- **A card per service.** Pays for padding twice, caps a phone at three items.
- **Generic web-dashboard layouts.** Sidebar-and-widgets, KPI tiles, chart-first grids. This
  is Android-native, read one-handed.
- **Several palettes over an identical layout, presented as different directions.** Rejected
  twice. Directions must differ in structure and hierarchy.
- **A summary card.** Undoes the hierarchy the surface-less header creates.
- **Rounded rows inside the rounded roster.** Two competing radii.
- **Green as the resting state.** Violates the chroma rule directly.
- **Shimmer skeletons and any idle animation.** See §7.
- **A third font weight.** Usually a symptom of hierarchy that is not working.
- **A completion flourish after data arrives.** The beat dot and the tally already say it;
  a tick would be a second, louder answer to a question already answered.

---

## 11. Applying this to new features 🔜

Screens still to come: host metrics (CPU, memory, storage, thermals), transfers, now-playing,
host power actions, multi-host.

- **Ask what question the screen answers in two seconds.** Design that answer first; put
  everything else below it.
- **Metrics are data — mono, tabular, aligned.** A CPU percentage that jitters its own column
  width is a defect.
- **Prefer a rule or a bar to a chart.** The tally rule is the house pattern for "proportion
  at a glance". A full chart needs to earn its space by answering something a rule cannot.
- **A new capability gets a renderer, never a branch.** Unknown capability ids render the
  agent's own label plus "Update CueSeek to view this".
- **Thermals will tempt you into red.** Resist until it is actually hot. Chroma means
  attention.
- **Wear gets the same tokens, different components.** The same capability is deliberately
  rendered differently per form factor.

---

## 12. Open questions

Genuinely unsettled, and where exploration is wanted:

- Layout and hierarchy for the **host metrics** screen — four values with very different
  update rates and very different failure meanings.
- How **transfers** (a list that changes constantly) coexists with a motion rule built around
  rare, meaningful change.
- Whether the roster needs **grouping** once there are more than about eight services.
- An **empty and first-run state** that is not a shrug.
- **Multi-host** switching, without a nav drawer.
- Whether a **second accent** — the tertiary eucalyptus `#3B6664` — should carry a role of
  its own, or stay incidental.

Anything in §2 is not open.
