# CueSeek agent API — Android client handoff

Everything an Android developer needs to talk to the CueSeek agent as it exists today.

**Every detail here is derived from the current implementation**, not from the plan. The
source of truth is [`api/openapi.yaml`](../api/openapi.yaml); the agent embeds that
same document and derives its authorization table from it at startup, so the contract and
the running server cannot disagree. Behaviour not visible in the contract was read out of
`agent/internal/api/`, `agent/internal/store/` and `agent/internal/domain/`.

Anything not established by the code is marked **NOT YET DEFINED** rather than guessed.

- Agent version verified against: `m1.8-listenretry`
- `api_version` reported by the agent: `0.1.0`
- Stream `schema_version`: `1`

---

## 1. Network assumptions

The agent binds to **one specific address**, never `0.0.0.0`. On the reference deployment
that is a Tailscale address, e.g. `http://100.92.18.125:7777`.

- **Plain HTTP. No TLS.** The agent terminates none and has no certificate. Transport
  security is delegated entirely to the VPN (ADR-0001).
- The phone must be on the same tailnet. If Tailscale is off, the host is simply
  unreachable — there is no fallback path, no relay, no cloud.
- Android blocks cleartext HTTP by default from API 28. The app will need a
  `network_security_config.xml` permitting cleartext to the agent's address, or to the
  `100.64.0.0/10` CGNAT range Tailscale uses.
- Port is whatever `bind.address` in the agent's config says. `7777` on the reference
  deployment, but it is configuration, not a constant — the user must be able to enter it.

There is **no discovery mechanism**. The user supplies the host address. Multi-host is
modelled in the data layer from day one (ADR-0008); each host has a stable `host_id` from
`GET /v1/system`, which survives restarts, hostname changes and IP changes.

---

## 2. Pairing

Pairing is the only unauthenticated operation.

### The flow

1. **On the server**, the operator runs a CLI command that mints a single-use code:
   ```
   cueseekd pair -config /etc/cueseek/config.yaml -scopes read,service.control
   ```
   It prints a code like `D8JT-HUPV`.
2. **The app** sends that code to `POST /v1/pair`.
3. The agent returns a **device token**, exactly once. It is never retrievable again.

### Pairing code format

- 8 characters from the alphabet `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` — 32 symbols,
  excluding `I`, `O`, `0` and `1` because they are misread when copied off a screen.
- Displayed grouped as `XXXX-XXXX`.
- **Redemption is forgiving**: the server strips anything outside that alphabet and
  uppercases before matching. `d8jt-hupv`, `D8JT HUPV` and `  D8JTHUPV\n` all work. The app
  does not need to normalise, though it may.
- **Single use.** A second redemption of the same code returns `403`.
- Default TTL **5 minutes** (`store.DefaultPairingTTL`), overridable by the operator's
  `-ttl` flag.

### QR codes

`clients/android/README.md` plans "pair by QR". **NOT YET DEFINED**: the agent does not
emit a QR code, and no QR payload format exists anywhere in the codebase. The CLI prints
plain text only. Whoever builds the QR path must define that payload — presumably host
address plus code — and it is a client-side decision until the agent grows an equivalent.

### `POST /v1/pair`

No `Authorization` header. Content-Type `application/json`.

**Request**

```json
{
  "code": "D8JT-HUPV",
  "device_name": "Pixel 8",
  "platform": "android"
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `code` | string | yes | the pairing code |
| `device_name` | string | yes | must be non-empty; shown in the device list |
| `platform` | string | yes | see enum below |

`platform` accepts `android`, `wearos`, `ios`, `web`, `desktop`, `cli`, `unknown`.
An unrecognised value is **not rejected** — it is stored as `unknown`. It is a display
label with no security meaning.

**Response `201`**

```json
{
  "device": {
    "id": "217f2f3dbf991996",
    "name": "Pixel 8",
    "platform": "android",
    "scopes": ["read", "service.control"],
    "created_at": "2026-08-08T05:38:14Z"
  },
  "token": "csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU"
}
```

`device.last_seen_at` is **absent** until the device makes its first authenticated
request; thereafter present as RFC 3339.

**Failures**

| Status | When | `type` |
|---|---|---|
| `400` | `device_name` empty, or malformed JSON | `.../bad-request` |
| `403` | code unknown, expired, or already redeemed — **deliberately indistinguishable** | `.../invalid-pairing-code` |
| `429` | more than 10 attempts per source address per minute | `.../rate-limited` |

The `403` cases are merged on purpose: telling a caller a code "expired" reveals it was
once real. Do not build UI that claims to know which it was.

---

## 3. Authentication

### Token format

- Prefix `csk_` followed by 43 characters of base64url (32 random bytes, no padding).
- **Total length exactly 47 characters.** Alphabet after the prefix: `A–Z a–z 0–9 - _`.
- Opaque. Not a JWT. Carries no claims, no expiry, nothing to parse.
- The server stores only a SHA-256 hash. There is **no refresh token and no expiry** — a
  token is valid until the device is revoked.

### Supplying it

```
Authorization: Bearer csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU
```

- The scheme match is **case-insensitive** (`Bearer`, `bearer`, `BEARER` all work).
- Leading/trailing whitespace around the credential is trimmed.
- Store it in **Android Keystore / EncryptedSharedPreferences**, per ADR-0006. It is a
  long-lived credential that can restart services on a real machine.

### The two 401s are different, and the difference is useful

| Detail | Meaning |
|---|---|
| `No bearer token was presented. Send \`Authorization: Bearer <device token>\`.` | no header, or header with an empty credential |
| `The device token was not accepted. Pair the device again to obtain a new one.` | a token was sent and rejected |

Branch on this. The first means a client bug — an empty variable, a header not attached.
The second means the token is genuinely dead and the user must re-pair. *Why* it was
rejected (unknown vs revoked) is deliberately not disclosed.

All `401` responses carry `WWW-Authenticate: Bearer realm="cueseek"`. `403` deliberately
does not — re-authenticating would be the wrong advice.

### Scopes

Granted per device at pairing time, from the operator's `-scopes` flag.

| Scope | Grants |
|---|---|
| `read` | `/v1/system`, `/v1/devices`, `/v1/services`, `/v1/services/{id}`, `/v1/stream` |
| `service.control` | `POST /v1/services/{id}/actions/{action}` |
| `devices.manage` | `DELETE /v1/devices/{id}` |
| `host.power` | reserved; **no endpoint uses it yet** |

Scopes are **independent grants, not tiers**. A device with `service.control` does not
have `read`. Enforcement is entirely server-side — client-side gating is UX, not a
control.

The token's scopes are in the pairing response and in `GET /v1/devices`. Use them to
hide actions the device cannot perform, but always handle a `403` anyway.

---

## 4. Errors

Every failure is `application/problem+json` (RFC 9457). One error shape across the whole
API, so one mapper suffices.

```json
{
  "type": "https://cueseek.dev/problems/insufficient-scope",
  "title": "Insufficient scope",
  "status": 403,
  "detail": "this operation requires the \"service.control\" scope; this device holds read",
  "instance": "/v1/services/jellyfin/actions/restart"
}
```

`type`, `title`, `status` and `instance` are always present. `detail` is optional —
present on most errors, but treat it as nullable.

**Branch on `type`, not on `detail`.** Detail strings are human-facing and may be reworded.

| `type` (after `https://cueseek.dev/problems/`) | Status | Meaning |
|---|---|---|
| `unauthorized` | 401 | missing or rejected credential — see §3 |
| `insufficient-scope` | 403 | authenticated, but lacks the scope |
| `invalid-pairing-code` | 403 | code unknown/expired/used |
| `rate-limited` | 429 | pairing attempts exceeded |
| `not-found` | 404 | no such service, action or device |
| `action-in-progress` | 409 | that action is already running on that service |
| `action-unavailable` | 409 | the host cannot perform it (unit not in allowlist, polkit refused, unit missing, unsupported platform) |
| `bad-request` | 400 | malformed request or parameters |
| `internal` | 500 | server fault; detail is deliberately generic |
| `not-implemented` | 503 | see below |

`not-implemented` is currently reachable in exactly one place: `GET /v1/stream` returns it
if the agent is **shutting down** when the request arrives. It is not a permanent state of
any endpoint.

**`action-unavailable` (409) deserves special UI.** It means the *agent* is not permitted
or able — most often a polkit rule that does not name the unit — not that the user did
anything wrong. The `detail` explains which, and is worth surfacing verbatim to an
operator.

---

## 5. Endpoints

### `GET /v1/system` — scope `read`

```json
{
  "host_id": "664917f8b739290c57d971481accef0e",
  "hostname": "kushal-HP-paviliong6",
  "agent_version": "m1.8-listenretry",
  "api_version": "0.1.0",
  "started_at": "2026-08-08T14:56:49.7878093Z"
}
```

`host_id` is the stable key for the multi-host data model. `hostname` is display-only and
may change. Compare `api_version` against what the app was built for and say honestly
which side is behind (ADR-0007) — the app must not silently mis-render.

### `GET /v1/devices` — scope `read`

Returns a JSON **array** (`[]` when empty, never `null`).

```json
[
  {
    "id": "217f2f3dbf991996",
    "name": "Pixel 8",
    "platform": "android",
    "scopes": ["read", "service.control"],
    "created_at": "2026-08-08T05:38:14Z",
    "last_seen_at": "2026-08-08T05:38:27Z"
  }
]
```

Ordered newest first. `last_seen_at` absent if never used.

### `DELETE /v1/devices/{deviceId}` — scope `devices.manage`

`204` with an empty body on success. `404` if no such device.

A device **may revoke itself** — that is how a client logs out — but it still needs
`devices.manage`. A device paired with only `read, service.control` (the CLI default)
**cannot** revoke anything, including itself. Revocation is immediate.

### `GET /v1/services` — scope `read`

Array, in configuration order.

```json
[
  {
    "id": "jellyfin",
    "name": "Jellyfin",
    "capabilities": [
      { "id": "health",  "label": "Health" },
      { "id": "control", "label": "Controls" }
    ],
    "actions": [
      {
        "id": "restart",
        "label": "Restart Jellyfin",
        "description": "Restarts the Jellyfin service. Anyone currently watching will be interrupted and will need to resume playback.",
        "risk": "disruptive"
      }
    ],
    "health": {
      "status": "healthy",
      "reachable": true,
      "reasons": [],
      "observed_at": "2026-08-08T14:56:51.7940666Z"
    }
  }
]
```

`capabilities`, `actions` and `reasons` are always arrays, never `null`.

### `GET /v1/services/{serviceId}` — scope `read`

Same object, unwrapped. `404` if unknown.

**Nothing you request triggers an upstream call.** The agent polls each service on its own
schedule and serves cached state (ADR-0003). Requests are always fast; a wedged Jellyfin
cannot hang the app.

---

## 6. Service state

### Capabilities

Hold a `Map<String, @Composable renderer>` keyed on capability `id` and look it up.
**Never branch on `service.id`** — `when (serviceId)` in UI is a review-blocking defect
(ADR-0007), because it discards the whole point of capability discovery.

Currently emitted:

| `id` | `label` | Meaning |
|---|---|---|
| `health` | Health | always present on every service |
| `control` | Controls | the service has invokable actions |

`now_playing` and `transfers` exist as identifiers in the server's domain vocabulary but
**no adapter implements them**, so they never appear today. They land in M3.

An unknown capability must render its `label` with an "update CueSeek to view this"
affordance — never an empty box, never a crash. This is a permanent condition: the Wear
app will routinely be older than the agent.

### Health

| Field | Notes |
|---|---|
| `status` | `healthy` \| `degraded` \| `unreachable` \| `unknown` — a closed set |
| `reachable` | whether the agent could contact the service at `observed_at` |
| `reasons` | array of `{code, message}`; may be empty |
| `observed_at` | when this was **observed**, not when it was served |
| `reported_status` | what the service says about itself, verbatim. **Absent for Jellyfin** — it publishes no self-assessment |

`status` and `reachable` are **separate facts**. `degraded` + `reachable: true` means the
service answered and something is wrong (a rejected API key, for instance). `unreachable`
means no contact at all. Conflating them sends the user to the wrong place.

`unknown` is a real state, not an error — before the first poll, and when cached state has
aged past tolerance. **Render it as "I don't know", never as healthy.** Showing stale
green is worse than showing nothing (ADR-0008).

**Render staleness from `observed_at`, not from arrival time.**

Reason codes currently emitted: `not_polled`, `stale`, `unreachable`, `timeout`,
`auth_failed`, `upstream_error`, `invalid_response`, `shutting_down`, `pending_restart`.
`message` is human-facing prose; branch on `code`.

Reasons are **not always problems** — a `healthy` service can carry `pending_restart`.

---

## 7. Actions

### `POST /v1/services/{serviceId}/actions/{actionId}` — scope `service.control`

No request body. Returns immediately.

**Response `202`**

```json
{
  "action_id": "2944a731d4a8af63",
  "service_id": "jellyfin",
  "action": "restart",
  "status": "running",
  "accepted_at": "2026-08-09T03:48:17.000890051Z"
}
```

The agent **cannot** report a synchronous result: systemd's `RestartUnit` returns once the
job is *queued*, not once the service is back. The outcome arrives later, over the stream.

- `action_id` — 16 hex characters, random (not sequential). Correlates this call with a
  later `action_progress` event. **Keep it.**
- `status` in the 202 is always `running` in the current implementation.
- Only actions listed in that service's `actions` array may be invoked; anything else is
  `404`.

### Risk levels

Every `Action` carries a `risk`. Gate confirmation UI on this value **without knowing what
the action does** — that is what lets a new action ship to an existing client with an
appropriate prompt already attached (ADR-0005).

| `risk` | Meaning | Suggested treatment |
|---|---|---|
| `safe` | read-only or trivially reversible | invoke directly |
| `disruptive` | interrupts service, but it comes back on its own | confirm |
| `destructive` | may lose data or need physical access to undo | confirm emphatically; biometric step-up |

Only `disruptive` is emitted today — Jellyfin's `restart`. `safe` and `destructive` are
part of the closed set and must be handled; `destructive` will appear with host power
actions in M3. An unrecognised value should be treated as at least as dangerous as
`destructive`, never as `safe`.

**Failures**

| Status | Meaning |
|---|---|
| `403` | device lacks `service.control` |
| `404` | unknown service, unknown action, or service has no actions |
| `409` `action-in-progress` | the same action is already running on that service |
| `409` `action-unavailable` | the host cannot do it — allowlist, polkit, missing unit, unsupported platform |

### `action_id` semantics

1. `POST` returns `action_id` with `status: "running"`.
2. Some time later, a single `action_progress` stream event arrives carrying **the same
   `action_id`** and a terminal status.
3. Terminal statuses actually published are **`succeeded`** and **`failed`** only.
   `pending` and `running` exist in the enum but are never emitted in an
   `action_progress` event by the current server.
4. On `failed`, `error` is present with a message.

**There is no way to query an action's status.** No `GET /v1/actions/{id}` exists. The
stream is the only delivery mechanism. If the app is not streaming when the outcome
arrives, **that outcome is lost** — a reconnect delivers a fresh snapshot, not missed
events.

Practical consequence: after invoking an action, either hold the stream open until the
matching `action_progress` arrives, or fall back to observing the service's `health`
changing. The server retains action records in memory for 10 minutes, but exposes no
endpoint to read them.

---

## 8. The event stream

### `GET /v1/stream` — scope `read`

Server-Sent Events. Response headers:

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

### Frame format

```
event: <type>
data: <one StreamEnvelope as JSON>

```

Each frame is `event:` then `data:` then a blank line. The `data:` payload is always a
complete `StreamEnvelope` on a single line.

**No `id:` field is ever sent.** This is deliberate — an id would make clients send
`Last-Event-ID` on reconnect, implying a replay buffer that does not exist. Reconnecting
yields a fresh snapshot instead.

### Envelope

```json
{
  "type": "service_updated",
  "seq": 1,
  "emitted_at": "2026-08-08T14:56:53.7967252Z",
  "schema_version": "1",
  "service": { }
}
```

| Field | Always | Notes |
|---|---|---|
| `type` | yes | `snapshot` \| `service_updated` \| `action_progress` \| `heartbeat` |
| `seq` | yes | monotonic **per connection**, starting at `0` for the snapshot |
| `emitted_at` | yes | RFC 3339 |
| `schema_version` | yes | currently `"1"` |
| `snapshot` | only on `snapshot` | |
| `service` | only on `service_updated` | a full `Service` object |
| `action_progress` | only on `action_progress` | |

`seq` is **not a resume token.** It detects gaps within one connection and resets to 0 on
every reconnect.

`schema_version` is versioned independently of the URL path, because a Wear build will
routinely be older than the agent it talks to.

### Event types

**`snapshot`** — always the first event on every connection, `seq: 0`.

```json
{
  "type": "snapshot", "seq": 0, "emitted_at": "...", "schema_version": "1",
  "snapshot": {
    "system": { },
    "services": [ ]
  }
}
```

The services in the snapshot are the **same shape** `GET /v1/services` returns, including
capabilities and actions. One source of truth, not two.

The snapshot is what removes the need for a replay buffer: every connection is told
everything.

**`service_updated`** — carries one full `Service` object, not a diff. Emitted **after
every poll**, whether or not anything changed; identical consecutive events differing only
in `observed_at` are normal and expected. Replace the service in local state by `id`.

**`action_progress`** — see §7.

```json
{
  "type": "action_progress", "seq": 7, "emitted_at": "...", "schema_version": "1",
  "action_progress": {
    "action_id": "2944a731d4a8af63",
    "service_id": "jellyfin",
    "action": "restart",
    "status": "succeeded",
    "at": "..."
  }
}
```

**`heartbeat`** — carries **no payload**; its existence is the message. Emitted on a fixed
15-second ticker that is *not* reset by other events, so heartbeats interleave with
deltas rather than appearing only during silence.

### Android SSE lifecycle — the part that matters

This was measured on real hardware over a cellular tailnet (assumption A7,
[`docs/m0-findings.md`](m0-findings.md)). The results changed the design.

**Foreground on mobile data is flawless.** Zero missed events, zero reconnects over a
3-minute test.

**With the screen off, the stream does not disconnect — it freezes silently.** Measured:
normal for ~60–75 seconds, then **108 seconds (Wi-Fi) and 168 seconds (cellular) of total
silence**, while the connection still reported itself as connected. The error surfaced
only when the queue flushed on wake. No events were lost; they arrived late, in a burst.

Therefore:

1. **Never trust the connection state.** Treat data as stale when no event of any kind has
   arrived within roughly **2 × the heartbeat interval (~30s)**, regardless of what the
   transport claims, and render `unknown` (ADR-0008) rather than the last known values.
   This is the single most important requirement in this document.
2. **Reconnect on your own schedule**, not when told to.
3. **Foreground only.** Nothing background-critical may depend on the stream. Alerting is a
   separate unsolved problem (ADR-0012).
4. **Every reconnect gives a full snapshot.** Replace local state wholesale; do not attempt
   to merge or resume. Measured reconnect cost: 3–4 seconds.

Server-side behaviour worth knowing:

- Each connection has a 16-event buffer. **A client too slow to drain it is disconnected**
  rather than having events dropped, so it reconnects and gets correct state.
- Each write has a 10-second deadline; a frozen client is closed rather than parking a
  server goroutine.
- On agent shutdown every stream is closed promptly and cleanly.

---

## 9. Known gaps

Explicitly not defined by the current server. Do not assume any of these exist.

| Gap | Status |
|---|---|
| QR pairing payload format | **NOT YET DEFINED** — agent emits no QR |
| Querying an action's status | **No endpoint.** Stream only |
| Host-level aggregate health | Computed internally but **not exposed** — `Snapshot` has `system` and `services` only |
| Host power actions (reboot/shutdown) | `host.power` scope exists; **no endpoint**. M3 |
| `now_playing`, `transfers` capabilities | Identifiers exist; **no adapter implements them**. M3 |
| Host metrics (CPU/RAM/disk) | Not implemented. M3 |
| Push notifications / alerting | Not implemented; reopens ADR-0001 (see ADR-0012) |
| Pagination on any list | None. Lists are complete and small |
| `ETag` / conditional requests | Not implemented |
| CORS headers | Not sent |
| Server-initiated pairing (agent shows QR on demand) | Not implemented |

---

## 10. Generating a client

`api/openapi.yaml` is **OpenAPI 3.0.3** — not 3.1. The version is pinned because
`oapi-codegen` v2.4.1 does not support 3.1 (ADR-0004 Amendment 1), and a build-time test
fails if it changes. Any Kotlin generator must handle 3.0.3.

Per ADR-0009, `:core:api` is generated but **wrapped**: a thin hand-written layer maps
generated types to sealed domain types and a `Result`-style error model, so no ViewModel
ever sees a generated type. Without it, changing generators later is a rewrite rather than
a swap.

**The generated client will not handle the SSE endpoint.** Most OpenAPI generators cannot
model `text/event-stream`; the Go server had to hand-write its side for the same reason.
Expect to write the stream client by hand (OkHttp's `EventSource`, or a raw streaming
response reader) against the envelope shapes in §8.

---

## Appendix: quick reference

```
GET    /v1/system                                   read              200
POST   /v1/pair                                     (public)          201 400 403 429
GET    /v1/devices                                  read              200 401
DELETE /v1/devices/{deviceId}                       devices.manage    204 401 403 404
GET    /v1/services                                 read              200 401
GET    /v1/services/{serviceId}                     read              200 401 404
POST   /v1/services/{serviceId}/actions/{actionId}  service.control   202 401 403 404 409
GET    /v1/stream                                   read              200 401 (503 if shutting down)
```

All timestamps are RFC 3339 in UTC. All list fields are `[]` when empty, never `null`.
