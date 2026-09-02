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

## Not yet verified, and why

Stated rather than left to be assumed from an absence.

- **The allowlist comparison against the *installed* rule.** Verified against a copy of the
  shipped rule placed where it could be read. The installed one needs root, and this work
  was done over a key-only SSH session with no interactive sudo.
- **Lifecycle control through polkit for a `systemd`-type service.** The polkit rule grants
  the `cueseek` user, and the test agent ran as the operator account. Property reads are not
  polkit-gated (an M0 finding), which is why health worked; a real start/stop/restart needs
  the packaged install. `RestartUnit` through this exact path is already verified for
  Jellyfin and qBittorrent in M3.1, and `systemd` shares `adapters.InvokeLifecycle` with
  both rather than reimplementing it.
- **Anything on the phone.** No client change was made in M4.3–M4.5, and M3.4 established
  that an adapter reaches the phone through capabilities the client already renders.
- **A fresh machine.** That is M4.10, and it is the only thing that can prove the milestone.
