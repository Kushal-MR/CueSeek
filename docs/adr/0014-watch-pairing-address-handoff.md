# ADR-0014: The phone carries the address; the watch mints its own token

- **Status:** Accepted
- **Date:** 2026-09-05

## Context

ADR-0006 established per-device scoped tokens: a code is minted on the host, redeemed once
by one client, and yields a token belonging to that client alone. It listed the price
explicitly:

> Cost: pairing is more friction than pasting a key, and the flow must be built on every
> client platform — **including a watch, where entry is awkward.**

M5 is where that bill arrives. The phone's pairing screen asks for four fields: address,
port, code, device name. On a 1.4-inch round screen with no usable keyboard, that is not
friction — it is the screen where somebody stops setting the app up.

The awkwardness is specifically the **address**. `100.92.18.125` is fifteen characters
including four separators, it is unguessable, and getting it wrong produces a timeout rather
than an error that names the mistake. The code is eight characters, case-insensitive, and
mistyping it says so immediately. One of those is worth asking a watch user for; the other
is not.

Three properties must survive whatever is chosen, because they are the whole of ADR-0006:

1. One token belongs to exactly one device.
2. Revoking one device affects no other.
3. The audit log can say *which device* performed an action.

## Decision

**The phone sends the agent's address to the watch. The watch pairs for itself.**

The phone app publishes `host` and `port` over the Wearable Data Layer. The watch reads
them, presents a code field alone, redeems the code against the agent directly, and stores
the resulting token in its own hardware-backed Keystore.

**No token is ever transferred between devices.** The watch holds a token the phone has
never seen, with its own scopes, its own row in the device list, and its own revocation.

The payload reuses the URI shape ADR-0006 Amendment 3 already recorded for QR, minus the
part that must not travel:

```
cueseek://pair?host=100.92.18.125&port=7777
```

Amendment 3 defined `code` as a third parameter for a QR flow, where the code is displayed
by the host and scanned by the client. Here the code does not originate on the phone and
must not pass through it.

**Default scopes for a watch: `read` and `service.control`.** Not `host.power`, and not
`devices.manage`.

ADR-0006 wrote that sentence before a watch existed:

> A paired watch can be issued `read` and `service.control` while being *structurally
> incapable* of powering off the machine — a policy expressible in one sentence.

A wrist is the worst place to hold an unbounded grant. `host.power` has no allowlist —
there is no target narrower than the machine — and a watch is worn against doorframes, on a
screen that wakes to a raised arm. An operator who wants it can ask for it by name, exactly
as on the phone, and press-and-hold still applies.

## Consequences

- **The watch is standalone at runtime but not for setup.** It talks to the agent directly,
  needs no phone afterwards, and keeps working if the phone is lost or switched off. But
  first-run requires a paired phone with CueSeek installed. This is a real reduction in
  independence and is the price of not asking somebody to type an IP address on a watch.
- **A fallback must exist**, because the Data Layer can fail: no phone, phone without the
  app, a channel that never delivers. The watch therefore keeps a manual address field,
  reached deliberately rather than presented first. The bad flow remains available; it stops
  being the default.
- **The address is not a secret**, so the channel does not need to be one. It is a private
  network address the user typed themselves and can read off their own terminal. The Data
  Layer is authenticated and encrypted between a paired phone and watch regardless, but
  nothing here depends on that being true — which is the property worth having.
- **A compromised phone cannot mint a watch token.** It can point a watch at the wrong
  address, and the watch will then fail to pair against an agent that never issued a code.
  It cannot produce a credential, because it never holds one.
- **Two devices, two rows, two revocations.** Revoking the phone does not disturb the watch,
  which is exactly the property ADR-0006 was written for and the reason the token is not
  shared.
- **Cost: a dependency on Google Play services on the watch**, which the Wearable Data Layer
  requires. CueSeek otherwise has no Google dependency at all, and this adds one to the
  client — not to the agent, and not to the contract, but it is a real narrowing of where
  the watch app can run.
- **Cost: the flow cannot be fully tested on an emulator.** A paired phone-and-watch pair is
  needed to exercise the handoff, which pushes M5.3a's verification onto real hardware.

## Alternatives considered

**Type the address on the watch.** The honest baseline, and what the fallback preserves. It
needs no new dependency, no new channel, and no ADR — and it is bad enough that the app
would be judged on it. Kept as an escape hatch precisely because it always works.

**Transfer the token from the phone.** Rejected before it was seriously proposed. It would
give two devices one credential, so revoking either revokes both, and the audit log would
attribute a watch's reboot to the phone. That is not a shortcut around ADR-0006; it is the
deletion of it.

**QR on the phone, scanned by the watch.** The Watch 2R has no camera, and most Wear devices
do not. Not viable on this form factor regardless of ADR-0006 Amendment 3's format.

**Discovery — mDNS or a broadcast.** Would remove the address problem for both clients, not
just the watch. Rejected for M5 as a genuinely new capability with its own security surface:
an agent that announces itself is a different threat model from one that must be named, and
ADR-0001 deliberately puts CueSeek behind a VPN rather than making it findable. Worth its own
ADR if it is ever wanted.
