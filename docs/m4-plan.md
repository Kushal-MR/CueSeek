# M4 plan — CueSeek on somebody else's machine

M3 made CueSeek a control panel. M4 makes it something a stranger can install.

Nothing here is a feature for the author. Every phase either removes an assumption about
one particular machine, or supplies something a person who has never seen that machine
needs in order to get running. The one exception is the `systemd` adapter, and it is here
because without it "self-hostable" means "self-hostable if you happen to run my two
services", which is not the claim being made.

Written down before any of it is built, for the reason M3's plan gives: M2's phase plan
lived only in a conversation, which made it impossible to review and easy to drift from.

## Naming

Phases are `M4.0` … `M4.10`, matching `M3.1`–`M3.8`.

**M4 was previously the Wear OS milestone.** Wear moves to M5 and the third-adapter
measurement to M6, recorded as [ADR-0011 Amendment 2](adr/0011-sequencing-spike-then-slice.md).
Records written before 2026-09-01 that mention "M4" mean Wear; they are left alone rather
than rewritten, which is the same treatment M2's `P0`–`P6` naming got.

## The rule, decided before any code

**M4 adds no capability, no contract change and no ADR reversal.** If a phase needs one of
those, it is the wrong phase and belongs in M5 or later.

That is not modesty. The value of this milestone is that somebody else can run what already
exists, and every feature added along the way is one more thing that has to work on a
machine nobody has tested. A productization milestone that ships features has no moment
where the thing being productized holds still.

Two things follow, and they are the whole shape of M4:

- **The default configuration must start.** Today `install.sh` copies `config.example.yaml`
  to `/etc/cueseek/config.yaml`, and that file has Jellyfin active with
  `api_key_file: /etc/cueseek/jellyfin.key`. On a fresh host that file does not exist, so
  the first `systemctl start` fails with an error about a service the operator may not run.
  The installer says the service is not running yet, which is true and is not the point: the
  first thing a new operator sees should not be somebody else's media server.
- **The acceptance test is a fresh virtual machine, not the development host.** M3's
  acceptance test was "verified on the real host". That cannot prove this milestone, because
  the real host already has a configuration, a database, a polkit rule and a paired phone.
  M4 is proven by installing from the published release, following only the published
  documentation, on a machine that has never seen CueSeek.

## Phases

| Phase | Scope | Depends on | Status |
| --- | --- | --- | --- |
| M4.0 | This plan, and the M4→M5 renumber | — | ✅ |
| M4.1 | Licence and security policy | — | ✅ |
| M4.2 | Make the documentation describe what the code does | — | ✅ |
| M4.3 | Neutral defaults: a first install that starts | — | ✅ |
| M4.4 | The `systemd` adapter | M4.3 | ✅ |
| M4.5 | `cueseekd check` | M4.3 | ✅ |
| M4.6 | Agent release: version stamping, artifacts, checksums | — | ✅ |
| M4.7 | Android release: coexisting builds, signing, versioning | — | ✅ |
| M4.8a | Documentation: requirements, install, pairing, README | M4.3–M4.7 | ✅ |
| M4.8b | Documentation: troubleshooting and the security model | M4.5 | ✅ |
| M4.8c | Documentation: configuration and per-service guides | M4.4 | ✅ |
| M4.9 | Website | M4.8 | ✅ |
| M4.10 | Fresh-VM verification, and `v0.1.1` | all | ✅ |

Each phase is independently verifiable, separately committed, and its own branch and pull
request — the M3 convention. `main` is installable after every one of them.

---

### M4.0 — This plan, and the renumber

No behaviour. This file, [ADR-0011 Amendment 2](adr/0011-sequencing-spike-then-slice.md),
the README roadmap, and the forward-looking references that said Wear lands in M4.

**What is deliberately not renumbered.** `docs/adr/0013-android-client-architecture.md` and
the two verification records mention M4 meaning Wear. ADR rule 3 is that an accepted
decision is never rewritten, and a verification record is dated evidence rather than a live
plan. The mapping is stated once, in the amendment, and old references decode through it.

---

### M4.1 — Licence and security policy

CueSeek has no `LICENSE` file. Default copyright therefore applies, and nobody may legally
use, modify or redistribute it — which makes every other phase in this milestone moot.
**Apache-2.0**, for the patent grant and the explicit contribution terms, both of which are
worth having in software that controls a machine.

`SECURITY.md` alongside it: how to report a vulnerability, and what is in scope. A daemon
that can power off a host should say where to send the report before somebody has to guess.

**Acceptance:** GitHub reports the licence in the sidebar and the policy in the Security tab.

---

### M4.2 — Make the documentation describe what the code does

The README and ADR-0008 both say overall status is derived from unit state, reachability
**and host metrics**. It is not: `health.Overall` takes `[]ServiceHealth` and nothing else,
and `server.go` passes it nothing else. ADR-0008 even uses "degraded: disk 94% full" as its
illustration.

**Amended, not implemented.** Feeding metrics into health means choosing thresholds, and
ADR-0008 already names threshold configurability as the thing that would force it to be
revisited. Doing that inside a phase whose purpose is documentation accuracy would smuggle a
policy decision in under a typo fix. The amendment says what is true today and leaves the
decision where it belongs.

Plus the README's own errors: a misspelling in the second sentence, a sentence that ends in
a dangling em dash, and a paragraph that trails off in an ellipsis.

**Why this is a phase rather than a chore.** Every claim this repository makes is checkable,
and that is the reason to believe the ones a reader cannot check. One place where the
documentation runs ahead of the code costs more than it appears to.

---

### M4.3 — Neutral defaults: a first install that starts

- `config.example.yaml` ships with no active service. Jellyfin and qBittorrent stay as
  annotated, commented worked examples, which is what they always were for everyone except
  the author.
- `install.sh`'s printed next steps stop opening with "Create a Jellyfin API key".
- `install.sh` stops overwriting a modified `10-cueseek.rules`. It already refuses to
  overwrite `config.yaml`; the polkit rule is hand-edited for exactly the same reason and
  currently loses those edits on every binary upgrade, silently.
- The pairing screen's placeholder is the development host's real tailnet address. So is
  the worked example in `docs/m2-android-api.md`.
- **`unit` becomes optional.** Configuration currently requires one for every service, but
  the adapters have never needed one: both `jellyfin.New` and `qbittorrent.New` already
  return a service that does not implement `Controllable` when no unit is configured, and
  capability discovery then simply does not advertise `control`. The validation is stricter
  than the code behind it, and the only thing it achieves is refusing a configuration the
  agent would serve correctly.

  The concrete case is a service installed from a container image rather than a package.
  It has an HTTP API and a web interface and no systemd unit, so today it cannot be
  configured at all — not even for health. With this change it appears in the roster with
  real health and a route to its own interface, and offers no lifecycle actions, because
  there is genuinely nothing to act on.

  **This is not container support.** Nothing here starts, stops or inspects a container,
  and `type: systemd` still requires a unit by definition — an adapter whose only source of
  health is a unit cannot work without one. What changes is that a missing unit becomes an
  absent capability rather than a startup failure, which is the same "absent is not empty"
  rule the rest of the system already follows.

**Acceptance:** on a machine with neither service installed, `install.sh` then
`systemctl start cueseekd` succeeds, and the client shows an empty roster rather than an
error. The empty case is already designed for — `health.Overall` answers `unknown` with a
`no_services` reason, and has since M1. A service configured without a unit starts, reports
health, and advertises no `control` capability.

---

### M4.4 — The `systemd` adapter

An adapter whose health comes from the unit and nothing else: `active` → healthy, `failed`
→ unreachable, `inactive` → degraded, not loaded → unreachable with a reason naming the
unit. No HTTP, no credentials, no `base_url`.

**No architectural change, and that is the point.** `base_url` is already optional because
whether a service needs one is a property of its adapter rather than of configuration.
`Controllable` is already conditional on a unit and a host layer being present.
`WebUIProvider` already reports what it was configured with. An adapter advertising
`health`, `control` and `web_ui` and nothing else is structurally what qBittorrent was
before M3.5. The contract does not move; no client is rebuilt.

**What it buys.** Two supported services becomes every service on the host. Plex, Sonarr,
Immich, Syncthing, Vaultwarden, a compose stack behind a unit — all of them get health,
lifecycle control and a route to their own interface on the day the operator installs.

**What it deliberately does not do.** No `now_playing`, no `transfers`, no reported status.
Those require translating a service's domain into a shared vocabulary, which is the work an
adapter exists to do and cannot be derived from a unit name. This is the honest boundary
between the two support tiers, and the documentation states it in those terms:

| Tier | What health means |
| --- | --- |
| Full — `jellyfin`, `qbittorrent` | The service answered its own API, and here is what it is doing |
| Basic — `systemd` | The process is running |

The gap between "running" and "answering" is real and is named in the docs rather than
papered over. Closing it is a generic HTTP-probe adapter, and that is M6's problem.

**Upgrade path, designed rather than discovered.** When a `plex` adapter is written later,
an operator moves from `type: systemd` to `type: plex` in one line and gains activity. The
`id` is unchanged, so the service keeps its identity. M3.4 established that an adapter
reaches the phone with no client release, so this costs the operator nothing but an edit.

---

### M4.5 — `cueseekd check`

The configuration and the polkit rule carry the same unit allowlist and are deliberately
not generated from each other (ADR-0002). Nothing currently tells an operator that they
disagree, and the symptom is an authorisation error that reads like a broken installation.

`cueseekd host restart <unit>` already distinguishes the three refusals that look identical
through the API. `check` is that diagnosis applied to the whole configuration at once,
before anything is tapped: every configured unit resolved against systemd, the installed
polkit rule parsed and compared in both directions, every `base_url` probed, the bind
address checked against the interfaces that actually exist, and the state directory's
ownership confirmed.

**Verifies, never generates.** Emitting the polkit rule from the configuration would
collapse two independent checks into one and remove the defence in depth ADR-0002 asks for.
Reporting the disagreement costs nothing and keeps both copies.

---

### M4.6 — Agent release

`main.go` has carried the `-ldflags -X main.version` hook since M1 and nothing has ever
used it, so every binary ever built reports `0.0.0-dev`. A tagged workflow builds
`linux/amd64`, stamps the version from `git describe`, publishes `SHA256SUMS`, and
`install.sh` learns to verify one.

**`linux/arm64` only if it is tested.** A Raspberry Pi is a plausible host and an untested
architecture is a claim this project does not make elsewhere.

**No `.deb` or `.rpm`.** `deploy/README.md` already names `install.sh` as the supported
path. Two packaging formats for one installer is maintenance with no reader.

---

### M4.7 — Android release

The app is debug-signed and its `applicationId` has no debug suffix, so a release-signed
build cannot install over it: Android refuses on the signature mismatch, and uninstalling
to get past that clears the credential store and unpairs the device.

- `applicationIdSuffix = ".debug"` on the debug build type, so both builds coexist on one
  phone permanently. This lands first, in this phase, so nothing has to be uninstalled to
  test the release.
- A release signing configuration driven by environment variables, with the keystore never
  in the repository.
- `versionCode` and `versionName` derived from the tag rather than pinned at `1` / `1.0`.

**The keystore is generated once and kept forever.** Losing it means no future build can
upgrade an existing install, for anyone.

---

### M4.8a — Documentation: requirements, install, pairing, README

`docs/requirements.md`, `docs/install.md`, `docs/pairing.md`, and a README rewritten around
a reader who has never seen this machine — what it is, what it is not, screenshots, the
support matrix, five commands, links out.

**The support matrix is deliberately narrow.** Linux with systemd and polkit ≥ 0.106; the
distribution and version actually tested, named; `linux/amd64`; Tailscale tested, WireGuard
and LAN untested and said to be; Jellyfin and qBittorrent at the full tier and anything with
a unit at the basic tier; Android from API 26, on the device it was tested on. Everything
else is listed as unsupported rather than left ambiguous.

---

### M4.8b — Documentation: troubleshooting and the security model

`docs/troubleshooting.md` is the highest-value document in this milestone, because it turns
what took a milestone to learn into somebody else's five minutes. The rows already exist as
evidence in this repository: a unit name that differs from what the service calls itself, a
polkit allowlist that disagrees with the configuration, an agent bound to loopback that no
phone can reach, a missing or empty key file, a polkit too old to enforce the allowlist, and
a device paired before M3.7 that cannot power the machine off.

`docs/security.md` is assembly rather than authorship: `deploy/README.md`, the polkit rule's
own comments and ADR-0001/0002/0006 already contain it. What it must state plainly is what
CueSeek can do to a machine, what it cannot, and how a reader verifies both without trusting
this sentence.

---

### M4.8c — Documentation: configuration and per-service guides

`docs/configure.md`, largely a route into the annotated `config.example.yaml`, which is
already a reference. `docs/services/jellyfin.md` and `docs/services/qbittorrent.md` — the
API key, the unit name that is not what the process calls itself, the localhost
authentication bypass. Also the ADRs published so they can be read without a clone.

---

### M4.9 — Website

One static page. What it is, what it is **not**, two screenshots, the architecture diagram
unchanged, the support matrix, the install, a paragraph on the security model, and links to
GitHub and the release.

The contract emits `https://cueseek.dev/problems/…` in the `type` field of every error
response. Those URIs are a promise the API already makes on every 403, and this is the phase
that either honours them or changes them.

**No framework, no comparison table, no analytics.** The restraint is not laziness; a
marketing site for a single-maintainer homelab tool reads as overclaiming, and the thing
worth showing is the ADRs.

---

### M4.10 — Fresh-VM verification, and `v0.1.0`

Install on a virtual machine that has never seen CueSeek, from the published artefacts,
following only the published documentation, with nothing borrowed from the development host.
Configure a service. Pair a phone. Record every place the documentation was wrong, fix it,
and repeat until a clean run needs no outside knowledge.

Recorded in `docs/m4-verification.md`, in the shape of `m3-verification.md`: what was done,
what was observed, and what did not work the first time.

**Then tag `v0.1.0`.** Not 1.0 — the version says what it is.

---

## What M4 deliberately excludes

Recorded so it is not relitigated phase by phase. None of these is rejected; none is in M4.

- **QR pairing.** The URI format is specified in ADR-0006 Amendment 3 and unimplemented.
  Real friction, but typing an address once is survivable and this milestone is already
  wide.
- **A generic HTTP-probe adapter.** The natural successor to M4.4, and the thing that closes
  the gap between "running" and "answering". It needs the per-adapter options map that
  ADR-0011 Amendment 1 requires, which makes it a configuration change and therefore M6.
- **Container support.** The largest audience beyond this milestone's reach, and the one
  place where the privilege model gets worse rather than better: a Docker socket grant has
  no allowlist and no polkit equivalent. It needs its own ADR. Note that a compose stack
  behind a systemd unit already works through M4.4 with no new code.
- **Alerting.** Analysed and deferred in ADR-0012, whose reasoning has not changed.
- **Multi-host UI.** The data layer has been keyed by host id since M2. Building the
  navigation before there is a second host is solving a problem nobody has.
- **A web client, `.deb`/`.rpm` packaging, `CONTRIBUTING.md`, an adapter authoring guide,
  macOS/BSD host backends, arm64 if it cannot be tested.**

The adapter authoring guide is the interesting exclusion: writing one invites adapter
contributions, and inviting contributions is ecosystem building. This project is not doing
that yet, and saying so in the README is more honest than a guide nobody maintains.

## Inherited open items

- **A CI check that the ADR index matches the files.** Raised in `m3-verification.md` after
  an unanchored `sed` silently incremented the wrong record's amendment count, and
  deliberately not built during a closure phase. It is repository hygiene, which makes M4 its
  home; it lands with M4.1 or M4.2, whichever touches the index first.

---

## M4 is closed — 2026-09-05

Released as **`v0.1.1`**, not `v0.1.0`. `v0.1.0` had already been published and installed,
and moving a tag would have left two different artefacts claiming one version — including
the one running on the development host. Release history stays append-only.

**What the milestone actually proved.** Not that the code works — M0–M3 established that —
but that a stranger can install it. The test was a machine that had existed for under an
hour, holding none of the development host's configuration, credentials or habits.

It found things nothing else could:

- **The shipped polkit rule granted two units a fresh install never configures.** M4.3
  neutralised the configuration and left the rule beside it naming `jellyfin.service` and
  `qbittorrent.service`. On the development host both are configured, the two lists agree,
  and `cueseekd check` stays silent — the defect is invisible there by construction.
- **`install.md` gave no download command.** It said "From the releases page:" and jumped to
  `sha256sum -c`. A server has no browser.
- **The `gh` caveat covered the wrong error.** It anticipated `unknown command` from an old
  `gh`; a stock Ubuntu Server has none at all and says `command not found`.
- **It pointed at an APK that did not exist**, and then, once one did, at an upgrade Android
  refuses: a client built from source is debug-signed and cannot be replaced by a
  release-signed package. Everyone following the build-from-source instruction would have
  hit that wall.

**Two long-open items closed.** Lifecycle control through polkit for a `type: systemd`
service — restart, stop and start, confirmed from systemd rather than from the agent's own
report, and driven from a phone. And the release-signed APK on a real device, where the
on-device bytes were checked against the published artefact rather than assumed.

**One demonstration that could not have been planned.** The same client showed `48°C
coretemp` against the HP host and no temperature row at all against the VM. Absent versus
present, one build, two machines — "absent is not zero" shown rather than argued.

The verification record is [`m4-verification.md`](m4-verification.md). What M4 deliberately
excluded is listed above and unchanged; none of it was smuggled in.
