# ADR-0006: Device pairing with per-device scoped tokens

- **Status:** Accepted
- **Date:** 2026-08-08
- **Amended:** 2026-08-08 — `devices.manage` added to the scope set (see [Amendments](#amendments))

## Context

ADR-0001 delegated transport security to the VPN. That proves a request came from a
device on the private network. It does not prove that *this application on this device*
may power off the machine — and a tailnet can include a work laptop, a shared node, or a
phone that gets lost.

The default in self-hosted tooling is a single shared API key in a config file. It takes
ten minutes and has three problems that matter here: it cannot be revoked for one device
without rotating every client, it carries no notion of what a device may do, and it
leaves no record of which device performed an action. It also reliably ends up in a
screenshot.

Full OIDC via an identity provider was considered. It is the correct enterprise answer
and the wrong one for a single-user tool that would then require running Authelia.

## Decision

The agent issues a single-use, short-lived pairing code (with QR) only when explicitly
invoked. A client redeems it once and receives a long-lived, per-device token stored in
platform-secure storage.

Tokens carry **independent scopes**: `read`, `service.control`, `devices.manage`,
`host.power` (the last added by Amendment 1). Devices are listed, named, last-seen and
individually revocable.

Tokens are persisted as hashes in SQLite under `/var/lib/cueseek`, alongside an
append-only audit log.

## Consequences

- Losing a device means deleting one row, not re-provisioning every client.
- A paired watch can be issued `read` and `service.control` while being *structurally
  incapable* of powering off the machine — a policy expressible in one sentence.
- The audit log answers "who rebooted the server, from which device, when". A console
  with that capability should be able to answer that question.
- SQLite rather than a JSON file: small data, but written concurrently, and a crash
  mid-write must not corrupt the registry every client depends on to authenticate.
- **Scopes are enforced in the agent.** Client-side biometric confirmation is user
  experience and can be bypassed by anything that is not the client. Both ship, but only
  one is a control.
- Pairing codes are single-use, short-lived and rate-limited with backoff. A short code
  is guessable given unlimited attempts.
- Cost: pairing is more friction than pasting a key, and the flow must be built on every
  client platform — including a watch, where entry is awkward.
- Cost: long-lived tokens do not rotate. Refresh tokens were judged unnecessary given
  revocation is immediate and the network is already private; revisit if the agent is
  ever reachable more broadly.

## Amendments

### Amendment 1 — 2026-08-08: `devices.manage` added

**What changed.** The scope set gains a fourth member, `devices.manage`, required by
`DELETE /v1/devices/{deviceId}`. `GET /v1/devices` remains `read`.

**Why.** M1.3 implemented revocation against the contract as originally written, which
required only `read`. Review caught that this was wrong twice over.

First, revoking a device is destructive, and `read` is the scope every client holds. A
token issued purely to display a dashboard could lock every other device out.

Second — and this is why the obvious fix was not simply promoting it to
`service.control` — that would have left the real hazard in place. A watch is routinely
paired with `read` and `service.control` so it can restart a service from the wrist. Under
that fix it could still revoke the phone. Revocation would have travelled with an
unrelated permission, which is precisely the tiering this ADR set out to avoid.

**Why it is a separate scope rather than a reuse.** Revocation is arguably the most
destructive operation in the API. Powering off a machine is recoverable by walking to it;
revoking every device removes the means to fix the problem remotely, including the device
an operator would reach for. It deserves to be withheld on its own.

**Cost.** The scope enum is public API. Four platforms' generated clients now carry a
value that did not exist, and a fourth grant is one more thing an operator must reason
about at pairing time. Accepted because the alternative is a permission that cannot be
withheld independently of one every client needs.

**Guard.** `TestRevokeRequiresDevicesManage` pairs a watch with `read` +
`service.control`, asserts it can list devices but is refused revocation, and asserts a
holder of `devices.manage` succeeds. Had revocation reused `service.control`, that test
would fail.

**What did not change.** Everything else in this record: pairing codes, per-device tokens,
hash-only storage, immediate revocation, the audit log, and the principle that scopes are
independent grants rather than tiers. This amendment applies that principle more
faithfully than the original scope list did.
