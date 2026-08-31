# M3 plan — service control, a second adapter, and the host itself

M2 proved the path works. M3 makes CueSeek a control panel rather than a monitor: a second
service, real lifecycle control, the host's own vitals, and the ability to power it down.

Written down because M2's phase plan lived only in a conversation, which made it impossible
to review and easy to drift from.

## Naming

Phases are `M3.1` … `M3.8`, matching `M1.1`–`M1.8`. (M2 used `M2 P0`–`M2 P6`, an earlier
variant of the same idea; that history is merged and public and is left alone.)

## Interaction model, decided before any code

Two rules, applied to every service rather than to Jellyfin specially:

- **The main row body means "tell me about this thing."** Tapping it opens the service's
  detail — health, what it is playing, what it is transferring, everything the agent knows.
- **The trailing ⋮ menu is everything else.** Opening the service's own web interface, plus
  whichever lifecycle actions the agent advertises right now, gated by the risk level the
  agent assigns.

**Reversed in M3.5, after using it.** The body originally opened the web interface and the
sheet was the fallback for a service without one. Two things were wrong with that. On a
host where every service has a `web_ui` the detail sheet became unreachable entirely, which
is how the M3.5 activity renderers shipped with no route to the screen. And more
fundamentally it put the gesture that *leaves the app* on the largest, easiest target: the
reason to open a console is to find out, and going to the service is what you do once the
answer warrants it. Leaving is a deliberate act and now takes a deliberate tap.

Nothing is prioritised or hardcoded in either direction — no native-app preference, no
per-service knowledge in the client.

The URL is **composed client-side** from the paired host address plus a scheme, port and
path supplied by the agent. Never a whole URL from the server: composing it locally means a
compromised agent cannot hand back `javascript:` or point the browser at a host the user
never paired with, and it means the same configuration works over Tailscale and on the LAN
without the user maintaining two addresses.

## Phases

| Phase | Scope | Depends on | Status |
| --- | --- | --- | --- |
| M3.1 | Service lifecycle: Start, Stop, Restart | — | ✅ verified |
| M3.2 | Contract + agent: `web_ui` | — | ✅ verified |
| M3.3 | Android: row interaction model and ⋮ menu | M3.1, M3.2 | ✅ verified |
| M3.3a | On-demand refresh: nudged polls and pull-to-refresh | M3.3 | ✅ verified |
| M3.4 | qBittorrent adapter | M3.1, M3.2 | ✅ verified |
| M3.5 | Activity capabilities: `transfers`, `now_playing` | M3.4 | ✅ verified |
| M3.6 | Host metrics: CPU, memory, storage, thermals | M3.2 | ✅ verified |
| M3.7 | Host power actions | M3.1, M3.3 | ⏳ built |
| M3.8 | Verification, documentation, ADR closure | all | ⬜ |

Each phase is independently verifiable and separately committed.

---

### M3.1 — Service lifecycle: Start, Stop, Restart ✅

Verified on the real host; see [`m3-verification.md`](m3-verification.md).

**No contract change.** `Action` is `{id, label, risk, description}`, and actions are data,
so `stop` and `start` are new values rather than new schema. That is the point of ADR-0005,
and it makes the acceptance test unusually strong.

- ADR-0002 **Amendment 1**: the privilege ceiling widens to `start`/`stop`, with the cost
  written down. Recorded *before* the code.
- `deploy/10-cueseek.rules`: add `start` and `stop` to the verb allowlist. The **unit**
  allowlist is unchanged — wider verbs, same targets.
- `host.Backend` / `host.Controller`: `StartUnit`, `StopUnit` beside `RestartUnit`.
- `adapters.UnitControl`: the same three.
- `Controllable.Actions()` becomes state-derived: Start only when the unit is inactive,
  Stop only when active. Derived from `host.UnitState` per poll.
- `stop` is classified **destructive**, routing it to press-and-hold in the client.

**Acceptance**

- Stopping the unit from the host changes the action list the API serves, and the change
  arrives over SSE.
- **The unmodified M2 client renders Start when the service is stopped.** No client rebuild.
  If this holds, capability-driven action discovery is proven empirically rather than
  asserted.
- The agent still refuses a unit outside the allowlist, and polkit refuses it again behind
  that.

**Verify on the host:** `systemctl stop jellyfin` → app offers Start → Start from the app →
`ActiveEnterTimestamp` moves and health returns to healthy.

---

### M3.2 — Contract and agent: `web_ui`

Spec-first, per ADR-0009. Follows the precedent already in the contract, where the `control`
capability declares support and the sibling `actions` array carries the payload.

- `api/openapi.yaml`: a `web_ui` capability id, and `web_ui: {scheme, port, path}` on
  `Service`.
- `config`: a per-service `web_ui` block.
- Adapters advertise the capability when it is configured.
- Regenerate Go and Kotlin; the drift gate proves both sides agree.

**Acceptance:** `GET /v1/services` carries `web_ui` for Jellyfin, and the **unmodified M2
client** renders "Update CueSeek to view this" for it — forward compatibility observed
rather than assumed.

---

### M3.3 — Android: the row interaction model

- Row body → `ACTION_VIEW` with the composed URL; falls back to the detail sheet when the
  service has no `web_ui`.
- Trailing ⋮ → the agent's advertised actions, risk-gated: `safe` fires, `disruptive`
  confirms, `destructive` holds.
- Stop's confirmation states the consequence plainly: the service stays down until it is
  started again or the host reboots.
- Two touch targets in one row, each with its own semantics and no double ripple.

**Tests:** URL composition across tailnet, LAN, custom scheme/port/path; refusal of
malformed values; the existing no-service-id-branching guard still passing.

---

### M3.3a — On-demand refresh: nudged polls and pull-to-refresh

Unplanned, and numbered `3a` rather than taking a slot so that M3.4–M3.8 keep the numbers
they were given. It exists because M3.3 exposed a gap the plan had not anticipated.

The agent had one clock. The poller ticked every 30s, wrote a cache, and every read served
that cache — so the console could be knowingly wrong twice over. After stopping a service it
went on reporting the state observed *before* it did the stopping, and an operator wanting to
confirm anything had no way to ask.

Pull-to-refresh was built first and did not fix it: re-reading a cache returns the same
answer. Verification needs the agent to **look**, not to repeat itself. That is recorded as
[ADR-0003 Amendment 1](adr/0003-agent-runtime-go.md#amendments).

- The poller's `select` gained a nudge channel beside its ticker.
- `POST /v1/refresh` nudges every service; an action nudges its own service once it reaches
  a terminal state, deferred so a Stop that systemd rejected still gets re-read.
- The Android gesture calls the endpoint, waits a bounded moment, then reads.

**What did not change, and is the point.** No request handler waits on an upstream service.
`/v1/refresh` returns `202` having only nudged; the poll runs on the service's own goroutine
under its own timeout, and the result reaches clients as an ordinary `service_updated` event.
The 30s cadence is untouched, and a nudge deliberately does not reset the ticker — letting it
would make the poll interval depend on how often someone opened the app.

**Bounds:** a nudge inside 2s of the last poll is dropped, nudges conflate per service, and
the endpoint requires `read` rather than `service.control` — asking the agent to look is not
acting on anything.

**Tests:** 8 agent (nudge observes ahead of the tick, rate bound, never blocks on a wedged
service, unknown ids ignored; endpoint nudges, needs only `read`, rejects the unauthenticated;
action completion nudges) and 7 Android (clock resets on success, unmoved on failure, no
overlap under a burst, does not claim the stream is open, ends reconnect backoff, nudges
before reading, and degrades when the agent is too old to know the endpoint).

---

### M3.4 — qBittorrent adapter ✅ verified

New adapter package plus one line in `builtin.go`. This is **ADR-0011's stated
measurement**: how many files change outside the new adapter's own package? The intended
answer is two — that file and the config.

**Result: four production files, and the two extra ones are worth naming.**

| File | Change | Anticipated? |
| --- | --- | --- |
| `adapters/builtin/builtin.go` | one line in the factory map | ✅ |
| `config/config.go` | `username` / `password` / `password_file` | ✅ "the config" |
| `domain/health.go` | one reason code, `peer_connectivity` | ❌ |
| `deploy/config.example.yaml` | the commented block, now real | ❌ (documentation) |

**Zero contract changes and zero Android changes.** `go generate` produced no diff and no
file under `clients/` was touched — qBittorrent reaches the phone through the same
`control` and `web_ui` capabilities Jellyfin uses, which is the acceptance criterion met
literally rather than approximately.

**On the two unanticipated files.** Neither is a capability-model failure, but one is worth
watching:

- `domain/health.go` gained `peer_connectivity`, for a service that is running and
  answering but cannot reach its peers. Reason codes are a shared vocabulary, so this grows
  with the *kinds of thing that can be wrong*, not with the number of services — the same
  O(capabilities) property `capabilityProbes` has. A fourth adapter that is another HTTP
  service will add nothing here.
- `config.Service` gaining credential fields is the one that would become a smell. It grew
  because qBittorrent authenticates with a login rather than a key, which is a genuinely
  different shape. **If a third adapter needs a third credential shape, the fix is a
  per-adapter options map rather than a fourth pair of fields** — and that is the decision
  ADR-0011's step 4 exists to trigger.

**What the adapter contributes, and what it inherits.** The whole lifecycle implementation
is two sentences of copy: `DisplayName` and `Interruption`. Start/Stop availability, the
risk levels, the hold-to-confirm classification and the state-dependence all come from
`adapters.AvailableLifecycleActions`. On-demand refresh, the freshness watchdog and the ⋮
menu required nothing at all.

qBittorrent is also the **first adapter to populate `reported_status`** — Jellyfin publishes
no self-assessment and leaves it empty. `connection_status` crosses verbatim and unmapped,
and `firewalled` is deliberately a *reason without a status change*: it still transfers, it
is often the operator's own choice, and this palette treats chroma as attention.

**Acceptance:** qBittorrent appears in the app with **zero client changes**, including its
web UI and its lifecycle menu. Managing torrents happens in qBittorrent's own Web UI, which
is what the `web_ui` capability is for; CueSeek handles the service, not the torrents.

---

### M3.5 — Activity: `transfers` and `now_playing` ✅ verified

Deliberately after M3.4. `adapters/adapter.go` warns that shaping `now_playing` before a
second media server exists bakes Jellyfin's DTOs into a contract Plex and Emby must also
satisfy. Two adapters first, then the shape.

**The shape, and why it is this one.** Both capabilities carry **aggregate counts plus a
bounded sample**, and the counts are the truth. A seedbox with four hundred torrents must
not put four hundred objects on an SSE frame every thirty seconds, so `items` is capped at
ten by the agent while `active` and `total` stay correct. A client that rendered
`items.size` as the total would understate a busy server at exactly the moment the number
mattered.

Neither payload borrows its vocabulary from the service that implemented it first:

- `now_playing` breaks out **`transcoding`** as its own count. On a self-hosted box direct
  play is nearly free and one 4K transcode can saturate the CPU every other service shares,
  so a session count alone would hide the fact that explains a hot machine. What is
  deliberately *not* carried is Jellyfin's transcode reasons and codec detail — those are
  Jellyfin's, and this name belongs to Plex and Emby too.
- `transfers` keeps the service's own **`state` word verbatim** — `stalledDL`, `queuedDL`,
  `seeding`. Flattening those into a shared enum would discard the distinction between
  "stalled" and "queued", which is precisely what tells an operator whether to care.

**Absent is not empty.** A capability the agent could not read is omitted entirely rather
than sent as a zero. `null` means "we could not ask"; `{"sessions": 0}` means "we asked, and
nothing is playing". Collapsing them would let a failed request render as an idle server,
and the distinction is asserted at three layers — the poller, the wire, and the client
mapping.

**Activity never affects health.** A media server that answers `/System/Info` and refuses
`/Sessions` is up. Reporting it as unhealthy would send an operator hunting an outage that
is not happening, which is the same class of error as conflating "unreachable" with
"degraded". The payload goes missing; the status does not move.

**Structural change.** `Cache.Put` now takes an `Observation` struct rather than a growing
parameter list. Its arity had already changed once in M3.1 and broke every fake in the tree;
M3.6 adds host metrics behind it.

**Client.** Two renderers in the existing capability registry — no new screen, no parallel
pattern. Both reuse the rule motif for progress rather than introducing a component, since
the dashboard has already taught the reader what a rule means.

**Tests:** 31 new. Agent — Jellyfin session mapping including the idle-session and
transcode-detection judgement calls, qBittorrent state and ETA normalisation, the poller's
collection and bounding, and the wire's absent-versus-zero encoding. Client — formatters,
the model's null-progress rules, and the mapping's forward tolerance of unknown states.

---

### M3.6 — Host metrics ✅ verified

The HP server's own vitals: **CPU usage, memory usage, storage usage, and thermals where
the hardware exposes them.**

These are not a service capability — they belong to the host, and they are deliberately not
modelled as a pseudo-service. That would have reused the poller, the cache and the row
renderer almost for free, and put the machine into the dashboard's own tally, so "two of
three healthy" would count the computer as one of its own services. It also gets worse with
time: once M3.7 lands, "restart the host" and "restart Jellyfin" would sit in the same list
as the same affordance.

**Where the original plan was wrong.** This section said metrics "extend the system surface"
because `SystemInfo` already carries host identity. Right about ownership, silent about
delivery. `System` is sent twice — once from `GET /v1/system` and once in the snapshot — and
never again, because nothing in it changes. Metrics change every few seconds, so inside
`System` a CPU figure would have frozen at the moment the client connected while sitting
under a live indicator. So metrics travel as their **own `host_updated` stream event**, with
`host_metrics` on the snapshot and `GET /v1/host/metrics` for clients not holding a stream
(ADR-0004 Amendment 4).

The read endpoint is not symmetry for its own sake. Without it a manual refresh composes its
snapshot from REST, carries no metrics, and blanks the vitals off the screen on every pull —
which would look exactly like the pull having broken something.

**Cadence.** 10s, against the service poll's 30s, on its own goroutine. Nothing in the
adapter poller applies: no adapter, no upstream timeout, no health, no nudge. And
utilisation averaged over thirty seconds hides the five-second spike that explains a hot
machine.

Read from `/proc` and `/sys` directly. No new privilege: those are world-readable, this is
reading, and the agent already runs on the machine. No polkit change.

**Absent is not zero, harder than in M3.5.** Every field but `collected_at` is optional, and
`null` and `[]` are distinguished at every layer. Hardware differs more than services do: a
virtual machine has no sensors, a container may see no usable `/proc/stat`, and **the first
collection after a restart cannot report utilisation at all** — the kernel counts
cumulatively, so one sample is not a measurement. Zero would have claimed an idle machine
nobody measured. The collector takes a baseline sample a second before its first publish so
that window is one second rather than one interval.

**Client.** A vitals strip under the summary header, reusing the tally's rule motif rather
than introducing a gauge or a card. Colour only where it means something: memory and storage
tint at 85% and 95%, thermals against the sensor's **own** reported threshold, and CPU never
— a processor at 95% is a transcode doing its job, and colouring it would spend the
attention this palette reserves for decisions.

Stale metrics are **dropped**, not degraded. A service keeps its timestamps through
staleness because "healthy three minutes ago" is still information; a three-minute-old CPU
percentage is not, and there is no last-known value worth preserving.

**Tests:** 41 new. Agent — `/proc/stat` differencing including the first-sample, wrapped and
zero-elapsed cases, `meminfo` parsing and the available-versus-free distinction, hwmon
labelling and the zero-sentinel, mount unescaping, ordering stability, the nil-versus-empty
wire encoding, the event fan-out and the 200/204 endpoint. Client — the full mapping
including 204 and forward tolerance, the live state's replace/clear/drop-on-stale rules, and
the strip's judgement calls.

---

### M3.7 — Host power actions ⏳ built, awaiting real-host verification

Reboot and shut down the machine from the phone. Mostly a matter of **connecting three
things that were each built expecting it**: the polkit power block, written and commented
out since M1; the `host.power` scope, which has existed just as long and guarded nothing;
and the client's press-and-hold, which until now only `stop` used.

- Polkit block enabled, **all four actions together**. The rule warned that omitting the
  `*-multiple-sessions` variants is a classic "works alone, fails in practice", and it is
  right: logind consults them the moment another user is logged in.
- `destructive` risk for both, so both route to press-and-hold. Recorded as
  [ADR-0002 Amendment 2](adr/0002-host-privilege-dbus-polkit.md).

**Acknowledge before acting.** The one handler in the agent that must answer before it does
its work: a reboot handler that reboots and then writes its response never writes it. It
returns `202` and calls logind 750 ms later. The consequence is that **success is
unobservable** — a power action that worked takes the stream, the agent and the machine with
it, so the only outcome a client can ever receive is a failure, which usefully means the
machine is still up.

**Its own shapes, not the service ones.** `HostActionAccepted` and `HostActionProgress`
rather than reusing `ActionAccepted`/`ActionProgress`, whose `service_id` is required. A
machine is not one of its own services, and relaxing that field would have been smaller in
the specification and much worse on a phone — an M3.6 client would fail to parse a field it
was promised, and it would fail while some *other* device pressed the button. The same
argument gave `host_action_progress` its own event type: an unknown type is ignored safely,
a malformed known one is not.

**Where it lives.** The host menu, not the service ⋮ — which is what M3.6's refusal to model
the host as a pseudo-service bought. Had the machine been a row in the roster, "restart the
host" and "restart Jellyfin" would now sit in the same list as the same affordance.

**Scope, listed but enforced.** `GET /v1/host/actions` needs only `read` and returns the
same list to everyone; invoking needs `host.power`. What the agent *offers* and what a
device *may do* are different questions, and a client that could not see the list would be
unable to tell "this agent is too old" from "this device was not granted permission". The
menu greys the items and says which it is. **Devices paired before M3.7 must pair again.**

**The confirmation names what it will interrupt** — "Right now: 2 streams playing and 1
transfer running" — from the activity M3.5 already collects, at no extra request. It does
not block: the operator owns the machine and may have good reasons to shut it down
mid-transcode. Blocking would make the tool argue with the person it exists to serve.

**Tests:** 21 new. Agent — the offered list and its risk levels, power-off's description
naming physical access, unsupported platforms offering nothing, unknown ids never reaching
the backend, scope enforcement, acknowledge-before-acting, the single-flight latch, and the
failure event. Client — the interrupted-work summary, including that finished transfers do
not count as work.

---

### M3.8 — Verification, documentation, ADR closure

Real-device end-to-end over Tailscale, as M2 P6 did. README and roadmap updated. ADR
amendments closed. The `After=tailscaled.service` deployment item left open by M2 is
finished here.

## Motion and polish

The design system's rule is **motion means data arrived**, which sits awkwardly with "make
it as expressive as possible" until the distinction is named: *interaction feedback is not
ambient motion.* A menu opening because a finger touched it is a response to input, not a
claim about liveness. What stays banned is decorative loops and shimmer, because on this
screen anything that moves on a timer is a thing that lies about freshness.

So M3's motion budget goes to: the ⋮ menu's enter and exit, correct state layers on two
touch targets in one row, sheet transitions, spring-driven changes when Start replaces Stop,
and — the one place motion genuinely carries data — M3 Expressive's wavy linear progress
indicator for qBittorrent transfers.
