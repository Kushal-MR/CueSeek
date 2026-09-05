# Pairing

How a phone gets permission to see and control your server, and how to take it away.

## Why there is pairing at all

Being on the VPN proves a request came from a device on your private network. It does not
prove that *this application on this device* may power off the machine — and a tailnet can
include a work laptop, a shared node, or a phone that gets lost.

The usual answer in self-hosted tooling is one shared API key in a config file. It takes ten
minutes and cannot be revoked for one device without rotating every client, carries no
notion of what a device may do, and leaves no record of which device did what. It also ends
up in a screenshot.

So: each device pairs separately, holds its own token, and carries its own permissions
([ADR-0006](adr/0006-device-pairing-scoped-tokens.md)).

## Pairing a device

On the host:

```bash
sudo -u cueseek cueseekd pair
```

```
  Pairing code:  7F2K-90QX
  Scopes:        read,service.control
  Valid for:     5m

  Single use. Enter it in the CueSeek app, or:

    curl -sX POST http://100.64.0.5:7777/v1/pair \
      -H 'Content-Type: application/json' \
      -d '{"code":"7F2K-90QX","device_name":"My Phone","platform":"cli"}'
```

In the app, type:

| Field | Value |
| --- | --- |
| Address | the host's VPN or LAN address — the same one in `bind.address` |
| Port | `7777` unless you changed it |
| Pairing code | as printed |
| This device | a name you will recognise in the device list |

Spacing and case in the code do not matter; the agent strips and uppercases before matching.

The code is **single use**, expires in five minutes, and is rate limited. Unknown, expired
and already-redeemed codes are deliberately indistinguishable, so guessing tells an attacker
nothing.

Once redeemed the app holds a long-lived token, stored as ciphertext sealed with a key
generated in — and never leaving — the device's hardware Keystore. The agent stores only a
hash of it. Neither the plaintext token nor the code is recoverable afterwards.

## There is no QR code

You type the address and the code. That is the whole flow.

QR is written down but not built: the agent emits no QR payload, the app has no scanner, no
camera dependency and no `cueseek://` handler. The URI format is recorded in
[ADR-0006 Amendment 3](adr/0006-device-pairing-scoped-tokens.md) so that both halves can be
built together later, because implementing a scanner without a producer would be a client
feature waiting on a server that does not exist.

The pairing screen says so in its own comment rather than pretending otherwise: *"There is
no discovery and no QR. The agent emits neither, so the honest first screen asks for what
the operator already has in front of them."*

## Scopes

A token carries **independent grants, not tiers**. A device can hold `service.control`
without ever being able to power the machine off.

| Scope | Grants | Default |
| --- | --- | --- |
| `read` | System, devices, services, host metrics, the live stream | ✅ |
| `service.control` | Start, stop and restart the configured services | ✅ |
| `host.power` | Reboot and shut down the machine | ❌ |
| `devices.manage` | Revoke paired devices | ❌ |

To grant more, ask by name:

```bash
sudo -u cueseek cueseekd pair -scopes read,service.control,host.power
```

It prints a warning before it hands you the code, because that grant is the only one with no
allowlist behind it — a unit grant is bounded to named units, and there is no target
narrower than the machine.

**`devices.manage` is withheld separately and deliberately.** Revoking is arguably the most
destructive operation in the API: powering off a machine is recoverable by walking to it,
but revoking every device removes the means to fix anything remotely, including from the
device you would reach for. A watch paired with `read` and `service.control` must not be
able to lock out your phone.

**Scopes are enforced in the agent, not in the app.** A token without `host.power` is
refused by the API regardless of what UI produced the request. The client greys those items
out and says why — that is user experience; the agent's check is the control.

## Devices paired before a scope existed

`host.power` has existed since M1 and authorised nothing until M3.7. **A device paired
before then cannot power the machine off and must pair again to gain it.** That is the scope
model working rather than a bug: a token carries what it was granted, and nothing widens it
retroactively.

## Seeing and revoking devices

The device list shows every paired device, its platform, its scopes and when it was last
seen. Revoking one takes effect immediately and affects nothing else.

Revocation needs `devices.manage`, which the default pairing does not include — so a typical
device cannot revoke anything, including itself. "Forget this host" in the app removes the
token locally and does **not** revoke it on the agent; the app says so rather than implying
the server forgot.

If you lose a phone and no device holds `devices.manage`, pair something that does:

```bash
sudo -u cueseek cueseekd pair -scopes read,devices.manage
```

If you lose every device, delete `/var/lib/cueseek/cueseek.db` and pair again. That is the
last resort and it is meant to be: it also erases the audit log.

## What is recorded

An append-only audit log answers *which device did this, and when*, including refusals. The
device name is stored as a snapshot rather than a reference, so revoking a device does not
erase the record of what it did — which is often exactly the incident being investigated.

## Re-pairing

Normal, and not a failure. Re-pairing an already-paired host replaces its record rather than
colliding, which is how a device recovers a token it can no longer decrypt — after the
screen lock changes, for instance, which invalidates the Keystore key. The app drops the
token and asks you to pair again rather than failing every request with a 401 it cannot
explain.

**Upgrading the agent does not require re-pairing.** The installer never touches the
database. Verified across an M3.7 → v0.1.0 upgrade.

**Reinstalling the app does.** The token is in app-private storage, excluded from cloud
backup and device-to-device transfer on purpose — a restored copy would be sealed by a key
that no longer exists, producing a phone that believes it is paired and fails every request.
Honestly unpaired is the better state.

**Moving from a source build to the published APK counts as a reinstall**, and it is not
optional: Android will not replace a debug-signed package with a release-signed one, so the
old build has to be uninstalled first ([install](install.md)). Once you are on published
APKs this stops happening — they share a signing key, so later versions upgrade in place and
the pairing survives. Verified across a `1.0` debug build to `v0.1.1`, which required it,
and an M3.7 → v0.1.0 agent upgrade, which did not.
