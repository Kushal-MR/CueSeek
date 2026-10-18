# M6 — the website

M5 finished the product. **M6 explains it** — to someone who has never run a home server,
in the time it takes to scroll a page, and without claiming anything the product does not
do.

The page shipped in M4.9 (`docs/index.html`, live at
[kushal-mr.github.io/CueSeek](https://kushal-mr.github.io/CueSeek/)) is a deliberate
placeholder: plain, no framework, no analytics, no third-party requests. M6 replaces it.

## The rule

**The site claims nothing the README does not.** Every service named, every feature shown,
every screen captured is something a stranger could install today — or is labelled, in
words, as planned. A promotional page is the easiest place in a project to lie by accident,
and this one is read by people deciding whether to trust an agent with root on their
machine.

**M6 changes nothing in `agent/`, `api/` or `clients/`.** The site is a new top-level
`site/` directory. If showing something requires changing the product, that is a product
milestone, not a website phase.

## Decided before the plan

Settled with Kushal on 2026-10-17, from references he collected on a design gallery:

| | Decision |
| --- | --- |
| Arrival | The Swell **flies in** with squash and stretch, lands, then a glowing wave rolls through the wordmark — a liquid, smeared distortion in CueSeek greens, with small HUD labels. Plays once, settles, idles with a slight breath |
| Scroll | Each section **triggers** a camera move — zooming in on what it is about. Never scroll-*scrubbed*: a scroll starts a transition, which then plays at its own pace |
| Theme | A **knob** instead of a dark/light switch, turning the page from Night to Day through **detented stops**. Each stop is a palette checked for contrast, status colours included; blending happens only mid-turn. Starts at the visitor's system preference; arrow keys turn it |
| The watch | Hangs **on its strap** with real physics — grab it, swing it. Its face shows the live verdict and the complication. A generic round watch drawn in code, not any manufacturer's design |
| Demo | A **separate page**, reached after a short intro. Not the whole site — the product's surface is deliberately small, and a whole-site demo would run out of things to do |
| The reel | Plays **in the page, as code** — the renderer and synthesiser ported to the browser, with a "view source" toggle. Sound **off** until asked for. Exported to MP4 separately, for sharing |
| "Why" notes | The site explains why things are built the way they are, linking to the ADRs and verification records. It does not discuss how the project was built |
| Stack | React + TypeScript, built with Vite to **static files**. Effects are plain WebGL where one shader is the whole job — the arrival's wordmark is one quad and one fragment shader, and Three.js would have added ~150 KB to the first thing a visitor loads. Three.js comes in only for a phase that is genuinely 3D |
| Hosting | **Vercel**, free `*.vercel.app` domain for now; a preview link per pull request. Kushal connects the account |
| Self-contained | Everything — fonts, scripts, shaders, images — is bundled and served by the site itself. No analytics, no CDNs, no third-party requests. The placeholder's promise survives |

### Which services the site shows

Three tiers, labelled as such, because all three are true:

| Tier | Services | What CueSeek gives you |
| --- | --- | --- |
| Full support | Jellyfin, qBittorrent | health from the service's own API, plus what is playing or what is moving |
| Works today, basic | anything run as a systemd unit — Plex, Sonarr, Radarr, Immich, Syncthing, Vaultwarden, Samba, … | running or not, and start, stop, restart |
| Planned | full Plex support | labelled "coming later" |

Services are named in text. Other projects' logos are not used.

### Screenshots of the services themselves

Captured from the HP against a **staged library**, never the real one: legally free films in
Jellyfin (Big Buck Bunny, Sintel, Tears of Steel) and Linux ISOs in qBittorrent. Staged,
captured, then removed. Phone and watch captures come from the real devices over adb.

## The page

One showpiece per section. Doubling up is how a page like this turns into noise.

| # | Section | Showpiece |
| --- | --- | --- |
| 0 | Arrival | the Swell flies in; the wave through the wordmark; the knob in a corner |
| 1 | The problem — rewritten | kinetic type |
| 2 | The answer | zoom into the phone's dashboard |
| 3 | What it works with | the three tiers |
| 4 | On your wrist | the watch on its strap |
| 5 | The reel | playing as code, with "view source" |
| 6 | Why it is built this way | notes linking to the ADRs |
| 7 | Try it → | the entry to the demo page |
| 8 | Download | both APKs and the agent, checksums, attestation, and the plain statement that the Wear app installs over ADB |

**Most visitors arrive on a phone** — a link opened from a feed. So everything works by
touch (drag the watch, swipe the knob), and the camera moves stay light. Anyone with reduced
motion set gets a still version of every section, not a broken one.

## Phases

| Phase | Deliverable | Depends on | Status |
| --- | --- | --- | --- |
| M6.0 | This plan | — | ✅ |
| M6.1 | Storyboard: every section as stills, the knob's stops — **approval gate** | M6.0 | ✅ |
| M6.2 | `site/` builds; Vercel preview per PR; CI builds it | M6.1 | ✅ |
| M6.3 | The knob and its palettes | M6.2 | ✅ |
| M6.4 | Arrival: the fly-in and the wave | M6.3 | ✅ |
| M6.5 | Problem and answer, with triggered camera moves | M6.4 | ✅ |
| M6.5b | The dot travels: it leaves the logo, hops to each section and cues it, and comes home | M6.5 | ✅ |
| M6.6 | Staging on the HP, and every capture | M6.1 | ⬜ |
| M6.7 | What it works with | M6.5, M6.6 | ⬜ |
| M6.8 | The watch on its strap | M6.5 | ⬜ |
| M6.9 | The reel, ported to the browser | M6.6 | ⬜ |
| M6.10 | Why it is built this way | M6.5 | ⬜ |
| M6.11 | The demo page | M6.5 | ⬜ |
| M6.12 | The download page | M6.2 | ⬜ |
| M6.13 | Phones, reduced motion, accessibility, weight | all above | ⬜ |
| M6.14 | Sharing: the MP4, a captioned 4:5 cut, the link preview image | M6.9 | ⬜ |
| M6.15 | Launch: the placeholder points to the new site; README links; the arrival back to once per visit (`ONCE_PER_VISIT` in `site/src/arrival/arriving.ts`) | all | ⬜ |

### M6.1 — Storyboard

The same rule as the identity and the reel: **show, get an OK, then build.** Every section
as a still in CueSeek's identity, at desktop and phone widths, plus the knob at each stop.
Nothing in `site/` is written until this is approved.

**Approved 2026-10-17, at draft 2.** The one change from draft 1: section 01 now ends on the
real pain — *"You can't keep an app for every one of them — let alone reach them all from
outside your home"* — and 02 answers it: *"CueSeek answers for you."*

### M6.5 — Problem and answer

One change to the approved copy: the storyboard said restarting was "one tap". Restart is
classed `disruptive`, so the app asks for confirmation first — two taps. The site says "a
couple of taps" rather than promise a gesture the app does not offer. The dashboard picture
is the README's for now; the real capture replaces it in M6.6.

### M6.5b — The travelling dot

Kushal's idea, after seeing 01 and 02: the Swell's dot leaves the logo and follows the reader
down the page. Each section marks a station with `data-station`; the dot hops there along an
arc and its landing is the section's cue (`useCue`). Never the only cue — without the dot
(reduced motion) a section starts when in view, and with it, a late dot is not waited for.
Scrolling back up brings it home into the logo's gap. Every later section adds a station.

### M6.14 — Sharing

The MP4 needs an encoder (ffmpeg, via `imageio-ffmpeg`). Approved in principle; installed
only when this phase starts. The 4:5 cut has captions burned in, because feeds autoplay
muted.

### Still to come from Kushal

- The links for the footer credit (GitHub, LinkedIn) — during the build.

## Background: the motion reel proof

Built at the end of M5 (2026-10-04): a 20-second reel made **entirely from code** — every
frame drawn by a program, the soundtrack synthesised by another, no editing software, no
stock assets. CueSeek's own identity only: the near-black page, the greens, the status
palette, IBM Plex, and the Swell regenerated from `docs/brand/icon.py`. Ten scenes at 120
BPM: the question, the verdict, vitals, a service needing attention, a restart that says
"asked", the hold-to-confirm, phone and watch agreeing, the end card. Kushal's verdict:
*"this is exactly what I'm looking for"* — it needs polish, not a new direction.

M6's version adds real captures of the services and of both devices, more polished text, and
more imagery. The proof's renderer and synth are the starting point.
