# M4 verification

What was checked on real hardware, what was observed, and what did not work the first time.

Built up as phases land, in the shape of [`m3-verification.md`](m3-verification.md), rather
than written as a summary afterwards. M4.10 adds the part no earlier phase can cover: a
fresh machine that has never seen CueSeek, installed from the published artefacts following
only the published documentation.

## The host

| | |
| --- | --- |
| Machine | `kushal-HP-paviliong6`, Linux Mint 22.3 |
| systemd | 255 (255.4-1ubuntu8.16) |
| polkit | 124 |
| Live agent | `cueseekd m3.7-35a6998`, active, unmodified throughout |

**Nothing in this record touched the live install.** The agent under test was a separate
binary in `/tmp`, with its own configuration, its own state directory and its own port
(7788, loopback), run as the operator account rather than as `cueseek`. The live agent kept
running on 7777 for the duration and was still on its M3.7 build afterwards.

One restart of the live agent appears in the journal at 11:22 and was **a boot, not this
work**: the log carries the cold-boot bind sequence — `bind address is not present yet,
waiting for it … became available waited=8s attempts=5` — with Jellyfin and qBittorrent
still refusing connections at that moment. Incidentally that is ADR-0002's retry loop and
the M3.8 boot-ordering finding holding on a real boot, measured again without being asked
for.

---

## M4.4 — the `systemd` adapter, against real units

Four services configured against real units in four different states, served through the
real API and read back over HTTP.

| Unit | systemd state | status | reachable | reported_status | actions offered |
| --- | --- | --- | --- | --- | --- |
| `jellyfin.service` | active (running) | `healthy` | true | `active (running)` | Restart, Stop |
| `qbittorrent.service` | active (running) | `healthy` | true | `active (running)` | Restart, Stop |
| `alsa-state.service` | inactive (dead) | `unreachable` | false | `inactive (dead)` | **Start only** |
| `definitely-not-here.service` | not loaded | `unreachable` | false | **absent** | **Restart only** |

Every judgement call in the adapter is visible in that table and every one held:

- **A stopped unit is `unreachable`, not `degraded`**, agreeing with what Jellyfin's HTTP
  probe already reports for the same situation.
- **`reachable` tracks the unit being active**, so nothing renders "the agent reached it"
  beside a red dot.
- **`reported_status` is absent for a unit that does not exist** — there is no state to
  report, and absent is not empty.
- **Actions follow state.** Start alone for the stopped unit. Restart alone for the one
  whose state could not be read, which is `LifecycleActions(known: false)` choosing the one
  verb that is correct whichever state was missed.
- **Capabilities are honest.** `health` and `control` everywhere; `web_ui` only for the
  service configured with one; `now_playing` and `transfers` nowhere.

Reason codes arrived as designed: `unit_inactive` with `"alsa-state.service" is stopped.`,
and `unit_not_loaded` with the `systemctl list-units` hint.

## M4.3 — a unit-less service

Covered indirectly and deliberately: `qbittorrent` was configured with no `web_ui` and
advertised none, which is the same "absent capability rather than an error" path that
`unit` now takes. The agent started, polled and served with a mixed configuration.

---

## M4.5 — `cueseekd check`, and three defects it revealed

The command ran against real systemd, real units and a real copy of the shipped polkit
rule. It correctly reported healthy units, a stopped unit, a nonexistent unit, a writable
state directory, a loopback bind address, and an allowlist disagreement in both directions.

It also got three things wrong, all of which only a real host could have shown.

### 1. A rule it could not read, reported as a rule that was broken

**`/etc/polkit-1/rules.d` is `0750 root:polkitd`**, and the `cueseek` user is in no group
but its own. So the invocation `install.sh` printed —

```
sudo -u cueseek cueseekd check
```

— could read the configuration and **not** the polkit rule, and `check` answered:

```
FAIL  polkit rule  ... permission denied
                   -> without it polkit refuses every start, stop and restart.
                      Reinstall it from deploy/10-cueseek.rules
```

Every word of that is wrong. The rule was correct, nothing was refusing anything, and the
suggested fix was to reinstall a file with nothing wrong with it. **A diagnostic that
manufactures a problem is worse than one that stays quiet**, because the operator now has
two problems and no way to tell which is real — and this is precisely the mistake the
parser was carefully designed not to make one layer further down.

Fixed in M4.5a: permission-denied is a **warning** that says the comparison could not be
made, and names the invocation that can make it. "Does not exist" remains a failure. The
installer and `deploy/README.md` now say `sudo cueseekd check`, with the reason.

### 2. A long subject broke the column, and the arrow pointed at nothing

`unit definitely-not-here.service` overran the subject column, pushed its detail out of
alignment, and left the `->` beneath it under empty space. A subject past the column now
takes its own line.

### 3. Subject-verb agreement

> `"alsa-state.service" and "definitely-not-here.service" is configured but not granted`

Small, and the kind of thing that makes an operator trust the rest of the output slightly
less. Both directions now agree in number, verified in one run: *"…**are** configured …
restart of **them**"* beside *"…**is** granted … Remove **it**"*.

### Also corrected

The generic fix line under a service-health warning read "check the service itself", which
misdirects when the reason above it already names the fix — as it does for a unit that does
not exist. It now says what the line actually is: what the phone will show.

---

## M4.5b — the defect the operator found, which was not in `check` at all

Reported as `sudo /usr/local/bin/cueseekd check` printing normal daemon startup logs and
then `bind: address already in use`, hanging until interrupted. `timeout 10s` reproduced it
with `exit=124`.

**The symptom pointed at `check`; the cause was the dispatch, and it predates M4.5.**

`/usr/local/bin/cueseekd` on the host is `m3.7-35a6998` — a build from before `check`
existed. `main` switched on `os.Args[1]`, matched nothing, and fell through to `runServe`.
`flag.Parse` **stops at the first non-flag argument and reports no error**, so:

- `check` was silently discarded;
- **every flag after it was discarded too**, including `-config`;
- the agent loaded the *default* `/etc/cueseek/config.yaml` — the real one;
- and tried to bind `100.92.18.125:7777`, which the running agent already held.

Every line the operator saw was the daemon starting correctly against their real
configuration. Reproduced locally on the current build: `cueseekd notacommand -config
/tmp/repro.yaml` ignored the flag and started against the default path.

**The near miss is the part worth recording.** The bind failure is what made this visible.
On a host where the port happened to be free — a fresh install, a different bind address, an
agent that was stopped — a mistyped subcommand would have **started a second agent against
the real configuration and the real SQLite database**, and reported nothing wrong.

Fixed in M4.5b: a first argument not beginning with `-` is a subcommand, and an unknown one
is an error with usage and exit 2. `runServe` also refuses leftover positional arguments,
so the serve path cannot be entered with arguments it did not understand however it was
reached.

Verified on the host: the old binary given `check` still emits `runServe`'s own config
error, proving which path it took; the new binary given the same word runs the check, and
given `chekc` prints usage and exits without starting anything.

`cmd/cueseekd` had no tests before this — which is why the dispatch was never exercised, and
why the decision was only observable by starting a daemon. `classify` exists to make it
observable, and the regression cases are the typo, the subcommand-from-the-future, and the
word-with-flags-after-it.

---

## M4.6 — the release tarball, extracted and run on the host

Built by `scripts/release-agent.sh`, copied over, verified, extracted and executed.

| Check | Result |
| --- | --- |
| `sha256sum -c SHA256SUMS` on the download | OK |
| Extracted modes | `cueseekd` and `install.sh` 0755, the rest 0644 |
| Payload | all 8 files: binary, installer, unit, rule, example config, checksum, LICENSE, NOTICE |
| `./cueseekd -version` | `cueseekd v0.0.0-test3 (linux/amd64, go1.26.5)` |
| `./cueseekd help` | the subcommand list, from the tarball |
| `./install.sh` without root | refused |
| `sha256sum -c cueseekd.sha256`, intact | OK |
| …after appending one byte | correctly rejected |

**The version is stamped.** `main.go` has carried the `-X main.version` hook since M1 and
nothing had ever used it, so every binary ever built said `0.0.0-dev` — including the one
on this host, which is how a `check` subcommand that did not exist yet came to be handed to
a daemon (M4.5b). A build that cannot say which build it is cannot answer the first
question anybody asks about a bug.

**Static, checked rather than assumed.** `file` reports `statically linked` and `ldd` says
`not a dynamic executable`. `CGO_ENABLED=0` is set explicitly in the script because a Linux
CI runner has a C toolchain and would otherwise link against its own glibc — producing a
binary that works on the runner, works on the maintainer's Ubuntu, and fails on somebody's
Alpine. `modernc.org/sqlite` being pure Go is what makes this available at all.

**Reproducible, measured.** Two builds of the same version produced byte-identical
tarballs: `f532f2ce70d1…`.

### The defect it found

The first tarball shipped **`cueseekd` as `-rw-r--r--`**. `go build -o` produced it on the
development machine, where `/tmp` is an NTFS mount with `noacl,posix=0` — the executable bit
is *inferred* from a shebang rather than stored, so `install.sh` had it and a Linux ELF did
not, and `chmod 0755` was a silent no-op.

On a Linux runner the mode would have been right **by luck**. The broken tarball would only
ever have been produced by a release cut outside CI, and would only have been noticed by
whoever downloaded it. It is the same class of defect as `deploy/install.sh` being committed
non-executable in M4.2: a mode that nothing states and nothing checks.

Fixed by stating the modes in the script — two `tar` passes with explicit `--mode` — so the
archive is a property of the script rather than of the machine that ran it. That also
removes the last host-dependent input to the byte-for-byte reproducibility above. The final
gate inspects the **archive** with `tar -tvzf` rather than the working tree, because the
working tree is exactly what cannot be trusted to carry a mode here.

---

## v0.1.0 — the release path, walked end to end

The first CueSeek release, installed on the development host from the published artefacts
rather than from a staged binary. This is the first time that machine has run anything but
a hand-copied build.

| Step | Result |
| --- | --- |
| Tag `v0.1.0` pushed; workflow published the release | ✅ |
| `sha256sum -c SHA256SUMS` on the host | OK |
| `sudo ./install.sh` from the extracted tarball | installed |
| `cueseekd -version` | `v0.1.0 (linux/amd64, go1.25.0)` — no longer `m3.7-35a6998` |
| `/etc/cueseek/config.yaml` after install | preserved untouched |
| Service | `active (running)`, enabled, listening on `100.92.18.125:7777` |

**The bind retry appears once more in the journal**, unprompted:
`bind address is not present yet, waiting for it … became available waited=10s attempts=6`.
Third independent observation of ADR-0002's retry loop doing the job `After=tailscaled.service`
was tested and rejected for in M3.8.

### The attestation, verified

`gh` on the host is 2.45.0 and has no `attestation` command, so this was done from the
development machine with 2.98.0:

```
repo     : github.com/Kushal-MR/CueSeek
workflow : .github/workflows/release.yml
commit   : cfbf7c3331dfa239ff73eb064ac0d32eaa2a225b
ref      : refs/tags/v0.1.0
runner   : github-hosted
```

Stronger than the checksum, and differently shaped: the checksum says the download is
intact, the attestation says the artefact was built by that workflow from that commit.
Neither is a substitute for the other.

**Worth recording as a limitation of the documented flow**: `deploy/README.md` and the
release notes both suggest `gh attestation verify`, and Ubuntu 24.04 ships a `gh` too old
to have it. A reader following the instructions on a stock distribution will find the
command missing rather than the verification failing.

### `cueseekd check` against the installed rule — M4.5's first open item, closed

Run as root, so it could read `/etc/polkit-1/rules.d/10-cueseek.rules` for the first time:

```
  ok    configuration       /etc/cueseek/config.yaml parsed; 2 services
  ok    bind address        100.92.18.125:7777 is assigned to tailscale0
  ok    state directory     /var/lib/cueseek is writable by root — this does not
                            prove the cueseek user can
  ok    polkit allowlist    agrees with the configuration on 2 units
  ok    power actions       all four logind actions granted
  ok    unit jellyfin.service      active (running)
  ok    unit qbittorrent.service   active (running)
  ok    jellyfin            healthy
  ok    qbittorrent         healthy — it reports "connected"

9 ok, 0 warnings, 0 failures
```

Every check that could only be exercised with root has now been exercised with root. The
allowlist genuinely agrees; it is no longer an assumption. Note the state-directory line
saying plainly what a root run does *not* prove — the caveat written for exactly this
invocation, doing its job.

### The phone, against a v0.1.0 agent

Captured over ADB, read-only. **Nothing on the phone was rebuilt or re-paired.**

- Verdict **Operational**, tally rule full, `✓ 2`
- **`● live`** — the stream is connected and the beat is arriving
- Vitals: CPU 1% of 4 cores · MEM 16%, 3.4 GB free · `/` 64%, 174 GB free · up 54m ·
  0 queued · 48°C coretemp
- Jellyfin **Running**, qBittorrent **Running**

Two things confirm this is live rather than a cached frame: between two captures a few
seconds apart the observed ages moved **10s → 3s** and the temperature **48°C → 50°C**.

Accessibility semantics survived intact — `"Jellyfin, Running"` as one merged node beside a
separate `"Actions for Jellyfin"`, which is M3.3's two-target row still announcing itself
correctly, and `"/, 174 GB free of 491 GB"` reading the vitals strip as a sentence.

**Two properties fall out of this, and they are the interesting ones.**

*Pairing survived a binary replacement.* No re-pair, no 401, no prompt. `install.sh`
replaces the binary, the unit and an unmodified rule, and never touches
`/var/lib/cueseek/cueseek.db` where the device row and token hash live. That is a designed
property rather than a lucky one, and it is now observed.

*The client is four milestones behind the agent and does not care.* The installed app is
the debug build from 2026-08-31 — before M4.3, M4.4, M4.5 and M4.6 existed. It is talking
to a v0.1.0 agent with no rebuild, no contract change and no visible difference. That is
ADR-0004 and ADR-0009's whole claim, observed rather than asserted, on the milestone where
the agent changed most.

---

## M4.7 — the Android release path

Built locally, since the workflow cannot be exercised until a keystore exists.

| Build | Result |
| --- | --- |
| `assembleDebug` | `dev.cueseek.android.debug`, `versionCode 1`, `versionName 0.0.0-dev-debug` |
| `assembleRelease`, no keystore | `app-release-**unsigned**.apk` — builds rather than fails |
| `assembleRelease`, `CUESEEK_VERSION=v0.1.0` | `dev.cueseek.android`, `versionCode 100`, `versionName 0.1.0` |
| `assembleRelease` with all four signing variables | `app-release.apk`, `apksigner verify` → **Verifies**, v2 scheme |
| `./gradlew build` with no keystore | passes — the case CI runs on every pull request |
| `:core:design:verifyPaparazziDebug` | passes |

The signing path was first proved with a **throwaway keystore generated for the purpose and
destroyed immediately afterwards** — `CN=CueSeek Signing Path Test, OU=Disposable`. Shipping
a signing configuration that had never signed anything would have moved the discovery of a
mistake to whoever first tried to cut a release.

### Then with the real key, and on the real phone

| Check | Result |
| --- | --- |
| Release keystore, RSA 4096, 10 000 days | created; password generated by a CSPRNG and never printed |
| Signed release APK | `dev.cueseek.android`, `versionCode 100`, `versionName 0.1.0` |
| `apksigner verify --print-certs` | `CN=Kushal M R, O=CueSeek, C=IN` |
| `adb install` of the debug build | **installed alongside** the existing app, not over it |
| Packages present afterwards | `dev.cueseek.android` **and** `dev.cueseek.android.debug` |
| The new debug build on launch | the pairing screen — its own storage, its own token, none |
| The existing paired app afterwards | **untouched**: Operational, `● live`, both services Running |

**The suffix was verified without spending the re-pairing it exists to prevent.** Installing
the debug build could not disturb the paired app precisely because they are now different
applications, which is the property under test — so the test costs nothing, which is the
sort of test worth arranging.

One incidental confirmation: the new build's pairing screen shows the placeholder
`100.64.0.1`. That is M4.3's change — the development host's real tailnet address removed
from the shipped UI — appearing in a built application for the first time.

### And in CI, before merging

The four signing secrets were set and the workflow dispatched **from the M4.7 branch**, so
the job that had never run got to run before the branch was merged rather than after. Both
jobs succeeded; `apksigner verify` reported `Verifies` on a runner-built APK. The
dispatch produced `versionCode 1` / `0.0.0-dev`, which is correct — `CUESEEK_VERSION` is
set only for a tag.

**It also found something.** The signed build reported:

```
package: name='dev.***.android'
```

The key alias had been stored as a repository secret, so Actions was masking the literal
string `cueseek` in every line of every log the workflow produced. A key alias is a name,
not a credential: it protects nothing, and redacting it makes every future CI log harder to
read for no gain. The secret was deleted and the alias is now written plainly in the
workflow.

Worth noting as a general shape rather than a one-off: **making a non-secret a secret is not
free.** It buys nothing and it silently degrades the diagnosability of everything the
workflow prints — a cost that only shows up later, while something else is going wrong.

**The unsigned case is the one worth having a test for.** `./gradlew build` runs
`assembleRelease`, so a signing block that referenced a missing keystore file would fail
every pull request from anyone without the key — which is everyone except the maintainer.
The configuration is therefore created only when all four variables are present, and its
absence produces an unsigned APK rather than an error.

### What this does not fix, and cannot

The currently-installed app on the development phone is `dev.cueseek.android` **debug-signed**
— it predates the suffix. The published APK carries the same application id with a different
signature, so Android will refuse to install one over the other.

**Moving to the released build therefore costs one uninstall and one re-pairing, once.** The
suffix does not avoid that; it ensures it is the last time, because from here a development
build and the published one are different applications. Said plainly rather than discovered
by whoever tries it.

---

## Not yet verified, and why

Stated rather than left to be assumed from an absence.

- **Lifecycle control through polkit for a `systemd`-type service.** The one item M4.5 left
  open that is still open. Health works because property reads are not polkit-gated (an M0
  finding); a real start/stop/restart of a `type: systemd` entry needs one configured on a
  machine whose polkit rule names its unit, and the development host runs neither of its
  two services that way. `RestartUnit` through this exact path is verified for Jellyfin and
  qBittorrent in M3.1, and the `systemd` adapter shares `adapters.InvokeLifecycle` with both
  rather than reimplementing it — so what is untested is the configuration, not the code
  path. It should be closed deliberately, with a service somebody is willing to restart.
- **A published APK.** The workflow signs one; no tag has yet carried one to a release page,
  because M4.7 is not merged.
- **A release-signed APK installed on a phone.** Built and signature-verified, not
  installed. Installing it means uninstalling the app that has been paired since M2, and
  that is a decision to take deliberately rather than in passing. The signature, the
  application id and the version are all confirmed; what is untested is Android accepting
  the install, which is the least interesting part.
- **A fresh machine.** That is M4.10, and it is the only thing that can prove the milestone.
  Everything above was observed on a host that has run CueSeek since M1 — which is exactly
  why it cannot stand in for a stranger's.

### Closed since

- ~~The allowlist comparison against the *installed* rule.~~ Closed by the v0.1.0 install:
  `sudo cueseekd check` read the real rule and reported `9 ok, 0 warnings, 0 failures`.
- ~~Anything on the phone.~~ Closed by the ADB capture above — and it turned out to be worth
  doing rather than assuming, because it observed two properties nothing else could: that
  pairing survives a binary replacement, and that a client four milestones behind the agent
  is unaffected by it.
- ~~`gh attestation verify` on a stock distribution.~~ Not fixed, but no longer a trap: the
  release notes now state that the command needs `gh` 2.49 or later and that Ubuntu 24.04
  ships 2.45, so a reader meets the limitation in the instructions rather than in a shell.
