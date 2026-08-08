# M0 — Spike results

**Date:** 2026-08-08 · **Status:** polkit/D-Bus assumptions validated; SSE test outstanding

This is not an ADR. M0 made no decision — it tested decisions already recorded in
[ADR-0002](adr/0002-host-privilege-dbus-polkit.md) and
[ADR-0004](adr/0004-contract-openapi-sse.md). Filing validation results as an ADR would
blur what that directory means.

The spike code itself was throwaway and has been deleted, as
[ADR-0011](adr/0011-sequencing-spike-then-slice.md) committed. This document is what
survives it.

## Host under test

```
Linux Mint 22.3 "Zena"  (Ubuntu 24.04 noble), kernel 7.0.0-28-generic
systemd 255 (255.4-1ubuntu8.16)
polkit  124             /etc/polkit-1/rules.d present; localauthority absent
units   jellyfin.service, qbittorrent.service   — both native systemd system units
        no Docker, no snap, no flatpak
```

Two findings from preparation, before any test ran, each of which would have forced an
architecture change:

- **Had polkit been 0.105** (Ubuntu 22.04 / Mint 21.x), `.pkla` rules could not inspect
  action details, and the per-unit allowlist in ADR-0002 would have been unenforceable.
- **Had either service been a Docker container**, `RestartUnit` would have been the wrong
  mechanism entirely, and Docker socket access is approximately root — undermining the
  premise of an unprivileged agent.

Neither applies here. This is worth stating because both were live possibilities, and the
architecture's viability on this host was contingent on facts nobody had checked.

## Method

Authorisation was tested as a sequence, not a single check. A grant that "works" on a host
which was never denying anything proves nothing:

1. Create an unprivileged `cueseek` user, install **no** rule → confirm denial
2. Install the polkit rule → confirm the allowlisted unit is now permitted
3. Confirm a **non**-allowlisted unit is still refused, via the same polkit action
4. Repeat 2 and 3 in a **session-less daemon context** (`systemd-run --uid=cueseek`)
5. Execute a real reboot

Step 4 exists because polkit authorises a *subject*, not a uid. A process started with
`sudo -u` inherits an active login session; the real agent has none. This host's implicit
defaults treat those differently — `auth_admin_keep` for active, `auth_admin` for
inactive — so a rule can pass interactive testing and fail in production.

## Results

| | Assumption | Result | Evidence |
| --- | --- | --- | --- |
| A1 | polkit supports a per-unit allowlist | ✅ | polkit 124, JS `.rules` |
| A2 | unprivileged user denied by default | ✅ | `Call failed: Interactive authentication required.` |
| A3 | rule grants the allowlisted unit | ✅ | `rc=0  o "/org/freedesktop/systemd1/job/4217"` |
| A3b | non-allowlisted unit stays denied | ✅ | `cron.service` refused in both contexts |
| A4 | grant survives a session-less daemon | ✅ | `XDG_SESSION_ID=<none> XDG_SEAT=<none>`, `rc=0` |
| A5 | unit state readable over D-Bus | ✅ | `ActiveState`, `SubState`, `ActiveEnterTimestamp` |
| A6 | power actions grantable | ✅ | `CanReboot`/`CanPowerOff` → `"yes"` |
| A6b | real reboot executes | ✅ | `Broadcast message from cueseek@…: The system will reboot now!` |
| A7 | SSE survives a tailnet on mobile data | ⬜ | not run — Tailscale not installed |

A6b's broadcast names `cueseek@`, not root: an unprivileged system user with no session,
no shell and no sudo rights power-cycled the machine through one polkit rule.

## Consequences for ADRs

**None.** ADR-0002 stands as written. Its central claim — that a single short polkit rule
can state the complete per-unit ceiling for an unprivileged agent — is now demonstrated on
the target host rather than argued.

## Constraints this imposes on M1

These are not decisions; they are facts the implementation must accommodate.

### `rc=0` means enqueued, not finished

`RestartUnit` returns a job object path immediately. The restart completes later, and the
D-Bus call carries no information about whether it succeeded. During the spike this was
confirmed only indirectly: `ActiveEnterTimestamp` advanced by 1619 s across reads, proving
the unit genuinely re-entered active state rather than the job silently failing.

**This independently validates ADR-0004's `202 Accepted` + action-id design.** The agent
cannot report a synchronous result because systemd does not give it one. M1 must subscribe
to `JobRemoved` on `org.freedesktop.systemd1.Manager` to learn the terminal state and
publish it over the stream.

### Denials arrive as "Interactive authentication required"

Not "Access denied". polkit is requesting a password that a daemon can never supply. The
agent must map this to *unauthorised*, not to an unexpected internal error. This exact
mistake occurred in the spike's own probe script and made a correct denial print as a
failure — a cheap error to make and a confusing one to debug.

### Timestamps are epoch microseconds

`ActiveEnterTimestamp` is a D-Bus `t` (uint64) in microseconds since the epoch. Directly
usable for health and staleness; note the unit, since seconds and nanoseconds are the more
common conventions and the error is silent.

### Managed unit names

`jellyfin.service` and `qbittorrent.service`. The latter is **not** `qbittorrent-nox.service`,
despite the unit's description being "qBittorrent-nox". The allowlist must be configuration,
not a compiled-in guess.

## Limitations

Stated so the results are not over-read:

- **One host, one distribution, one polkit version.** Nothing here generalises to Mint 21.x,
  Debian, Fedora or a non-systemd host. It establishes that the design works on the target,
  not that it works everywhere.
- **polkit's decision-level logging never surfaced.** `polkit.log()` calls in the rule did
  not appear in the journal at default verbosity. Conclusions rest on the observed results
  of the calls, which is stronger evidence anyway, but polkit's own account was expected and
  not obtained.
- **Two active sessions were present throughout.** The rule granted both
  `login1.reboot` and `login1.reboot-multiple-sessions`; which one logind actually consulted
  is not determinable from the evidence. Including both was defensive, and cannot be
  claimed to have been necessary.
- **A7 is untested.** ADR-0004's stream design remains unvalidated on a real mobile network.
  Worth closing before M1 commits to the stream shape.

## Corrections made during the run

Both were defects in the spike, not the architecture, and both are recorded because the
second could have gone badly:

1. The probe script matched denials against `Access denied` and did not recognise polkit
   124's `Interactive authentication required`, so a correct baseline denial reported
   `FAIL`.
2. The negative test originally targeted `ssh.service`. Had the allowlist *not* held, that
   test would have restarted sshd and severed the session running the spike. Changed to
   `cron.service` before execution. A test that can destroy the experiment measuring it is
   a badly designed test.
