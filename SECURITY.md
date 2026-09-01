# Security Policy

CueSeek runs a daemon on a machine you own that can restart services, reboot the host and
power it off. That is a higher bar than most home-lab tooling, and this document states
what it does to earn it, what it does not defend against, and how to report a problem.

## Reporting a vulnerability

**Use GitHub's private vulnerability reporting**, from the Security tab of
[this repository](https://github.com/Kushal-MR/CueSeek/security/advisories/new). It goes
straight to the maintainer and stays private until there is a fix.

Please do not open a public issue for a security problem first.

This is a single-maintainer project with no on-call rotation. A realistic expectation:

| | |
| --- | --- |
| Acknowledgement | Within a week |
| Assessment and a plan | Within two weeks |
| Fix, or a written reason there will not be one | Depends on severity — you will be told which |

There is no bounty. Credit in the release notes and the advisory, unless you would rather
not be named.

## Supported versions

| Version | Supported |
| --- | --- |
| `main` | Yes — fixes land here first |
| Tagged releases | None yet |

**CueSeek has not had a release.** `v0.1.0` is the first, and lands at the end of M4
(see [`docs/m4-plan.md`](docs/m4-plan.md)). Until then the only supported thing is `main`,
and anyone running CueSeek is running a development build knowingly.

## The security model, in brief

The full reasoning is in the ADRs; this is the summary a reader needs before deciding to
trust the thing.

**The agent never runs as root.** `cueseekd` runs as its own unprivileged system user with
an empty capability bounding set, no login shell and no home directory. It holds no
privilege of its own and never elevates — no `sudo`, no shell, no `systemctl` invocation.

**Privilege is granted by a file you can read.**
[`deploy/10-cueseek.rules`](deploy/10-cueseek.rules) is the complete statement of what
CueSeek may do to a machine, and it is short enough to read in two minutes without knowing
any Go. It grants the `cueseek` user `start`, `stop` and `restart` on an **allowlist of
named units**, plus `reboot` and `power-off` via logind. Everything else returns
`NOT_HANDLED`, so the rule only ever adds permission and never removes one another rule
granted. See [ADR-0002](docs/adr/0002-host-privilege-dbus-polkit.md).

**The unit allowlist is enforced twice**, in the agent's configuration and in the polkit
rule, deliberately not generated from each other. If the two disagree the narrower wins,
which is the safe direction.

**There is nothing exposed to the internet.** CueSeek terminates no TLS, does no NAT
traversal, runs no relay, and refuses at startup to bind to a wildcard address unless an
operator sets `allow_unrestricted: true` on purpose. Reachability is the VPN's job —
Tailscale, WireGuard or the LAN. See [ADR-0001](docs/adr/0001-vpn-only-remote-access.md).

**Network reachability is not authorisation.** A tailnet can contain a work laptop or a
phone that gets lost, so every device pairs separately and carries its own token with
independent scopes — `read`, `service.control`, `devices.manage`, `host.power`. Scopes are
grants, not tiers: a device can be structurally incapable of powering off the machine
regardless of what its UI offers. **Scopes are enforced in the agent**, never in the
client. See [ADR-0006](docs/adr/0006-device-pairing-scoped-tokens.md).

**Tokens are stored as hashes.** The agent keeps only a hash; the plaintext is returned
once at pairing and is never recoverable. Pairing codes are single-use, short-lived and
rate-limited, and unknown, expired and already-redeemed codes are deliberately
indistinguishable. On Android the token is held as ciphertext sealed with an AES-GCM key
generated in, and never leaving, the device Keystore, and is excluded from cloud backup
and device-to-device transfer.

**Actions are recorded.** An append-only audit log answers "which device did this, and
when", including refusals. The device name is denormalised on purpose, so revoking a
device does not erase the record of what it did.

**The agent never sends a URL.** A service's web interface travels as scheme, port and
path; the client composes the origin from the address it already paired with. A
compromised or buggy agent therefore cannot redirect a client to an arbitrary origin or
hand back a `javascript:` URL, because it never supplies an origin at all.

## Scope

**In scope:**

- `cueseekd` — authentication, scope enforcement, pairing, the SSE stream, the HTTP API,
  configuration parsing, the SQLite store.
- The host layer — unit allowlist enforcement, D-Bus calls, anything that could widen what
  the agent can do to a machine.
- `deploy/` — the polkit rule, the systemd unit's hardening, `install.sh`.
- The Android client — token storage, what it does with the address it was paired with.
- The adapters, where a malicious or compromised upstream service could affect the agent.

**Out of scope:**

- **Exposing the agent directly to the internet.** This is explicitly unsupported
  (ADR-0001). Reports that amount to "it has no TLS" describe a documented design decision,
  not a vulnerability.
- Vulnerabilities in Jellyfin, qBittorrent, systemd, polkit or Tailscale themselves. Report
  those upstream. A way for CueSeek to *make* one of them exploitable is in scope.
- Anything requiring an attacker who is already root on the host. CueSeek is unprivileged;
  root outranks it by construction.
- Physical access to an unlocked, paired phone. Client-side confirmation is user experience,
  not a control — the control is the scope on the token.

## Known and accepted risks

Every one of these is a decision with its cost written down, not an oversight. They are
listed here so nobody has to discover them.

**`host.power` has no allowlist.** A unit grant is bounded to named units; there is no
target narrower than the machine. A stolen token carrying `host.power` can take the host
off the network until somebody physically walks to it. Bounded by: the scope is separate,
never granted by default, enforced in the agent, and classified `destructive` so every
client routes it through press-and-hold. See
[ADR-0002 Amendment 2](docs/adr/0002-host-privilege-dbus-polkit.md).

**`stop` is not self-healing.** A `restart` grant leaves the service running whatever
happens; a `stop` grant does not. Bounded by: the unit allowlist is unchanged, and
`enable`, `disable` and `mask` remain absent — so a stopped unit stays enabled and returns
on the next boot. The damage is one reboot, not permanent. See
[ADR-0002 Amendment 1](docs/adr/0002-host-privilege-dbus-polkit.md).

**Device tokens are long-lived and do not rotate.** There are no refresh tokens. Accepted
because revocation is immediate and per-device and the network is already private. This
should be revisited if the agent ever becomes reachable more broadly. See
[ADR-0006](docs/adr/0006-device-pairing-scoped-tokens.md).

**Traffic is plaintext HTTP.** By design — transport security is the VPN's, and the client
permits cleartext because Android's network security configuration cannot express a CIDR
range and the agent's address is typed in at runtime. The narrowing is behavioural: the app
only ever issues requests to the address the user entered.

**The configuration file may contain credentials.** `install.sh` installs it `0640`,
`root:cueseek`. Prefer `api_key_file` and `password_file`, so the configuration *names* a
secret rather than containing one.

**polkit below 0.106 cannot enforce the allowlist.** Older polkit uses `.pkla` files that
cannot inspect action details, which would silently degrade the rule to "may restart any
unit". `install.sh` refuses to install on such a host rather than installing a rule that
lies about what it grants.

## What CueSeek deliberately cannot do

Absences that are load-bearing. Adding any of them is an architectural decision requiring
an ADR, not a feature request.

- **Run an arbitrary command.** There is no shell, no command execution and no
  operator-supplied script anywhere in the agent. Service names never become part of a
  command string, so there is no command-injection class to defend against.
- **`enable`, `disable` or `mask` a unit.** A stopped unit stays enabled.
- **`suspend` or `hibernate` the host.** A suspended host is unreachable over the VPN and
  cannot be woken by the thing that suspended it, which turns a remote console into a way
  to lock yourself out.
- **Touch a unit outside the allowlist.** Refused by the agent before D-Bus is reached, and
  refused again by polkit behind it.
- **Reach a service that is not in its configuration.**
- **Phone home.** There is no telemetry, no analytics, no update check and no cloud
  component of any kind.

## Verifying this yourself

None of the above should be taken on trust. On a host with CueSeek installed:

```bash
# What CueSeek is permitted to do, in full
cat /etc/polkit-1/rules.d/10-cueseek.rules

# That polkit can actually enforce the per-unit allowlist
pkaction --version

# That the agent is unprivileged and holds no capabilities
systemctl show cueseekd -p User -p CapabilityBoundingSet -p NoNewPrivileges

# Which units it is configured to manage
grep -A2 'unit:' /etc/cueseek/config.yaml

# Which devices are paired, and with which scopes
sudo -u cueseek cueseekd pair --help
```
