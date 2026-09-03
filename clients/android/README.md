# CueSeek for Android

The phone client. Talks to `cueseekd` over a tailnet; see
[`docs/m2-android-api.md`](../../docs/m2-android-api.md) for the API contract as the agent
actually implements it.

## Modules

```
clients/android/
├─ app/                 assembly, navigation, DI wiring, and the feature packages
│  └─ …/pairing/ …/dashboard/
└─ core/
   ├─ model/            Kotlin/JVM · domain types the UI speaks. Depends on nothing.
   ├─ api/              Kotlin/JVM · generated wire types + hand-written transport & SSE
   ├─ data/             Android   · repositories, token storage, live-state machine
   └─ design/           Android   · tokens + component catalogue (tokens shared with Wear)
```

Feature areas are packages inside `:app` rather than modules, and `:core:model` /
`:core:api` are plain Kotlin/JVM rather than Android libraries. Both choices, and their
costs, are in [ADR-0013](../../docs/adr/0013-android-client-architecture.md).

## Build

```bash
./gradlew build
```

Requires JDK 17+ and the Android SDK with platform 37. Gradle 9.7, AGP 9.3.1. CI runs the
same command over every module on each push.

### Two applications, on purpose

| Build | Application id | Signed by |
| --- | --- | --- |
| `debug` | `dev.cueseek.android.debug` | the local debug key |
| `release` | `dev.cueseek.android` | the release keystore, from the environment |

The suffix is what lets a development build and the published one sit on the same phone.
Without it they share an application id, Android refuses to install one over the other on
the signature mismatch, and uninstalling to get past that clears the credential store —
so testing a change would cost a re-pairing every time.

They keep separate storage, which follows from being separate applications: the debug build
pairs on its own and cannot see the release build's token.

### Version

`versionCode` and `versionName` come from `CUESEEK_VERSION`, which the release workflow sets
to the tag. A local build is `0.0.0-dev`; `v0.1.0` becomes `versionCode 100`, `versionName
0.1.0`. They were pinned at `1` and `"1.0"` until M4.7, so every build claimed to be the
same one — the same defect the agent carried until M4.6, where it cost a debugging session
(M4.5b).

### Signing

Four environment variables, none of them in the repository:

```bash
CUESEEK_KEYSTORE=/path/to/cueseek.jks \
CUESEEK_KEYSTORE_PASSWORD=... \
CUESEEK_KEY_ALIAS=... \
CUESEEK_KEY_PASSWORD=... \
./gradlew :app:assembleRelease
```

**All four or none.** With any missing, the signing configuration is not created at all and
the build produces `app-release-unsigned.apk` rather than failing — which is what keeps
`./gradlew build` working for CI on every pull request and for anyone without the key.

Three of the four are repository secrets. **The alias is not**, and was one briefly: a key
alias is a name rather than a credential, it protects nothing, and holding it as a secret
made Actions mask the string `cueseek` in every log the workflow produced — the first
signed build reported `package: name='dev.***.android'`. Redaction that hides nothing worth
hiding costs an hour the day something actually goes wrong.

The keystore is the one credential in this project that cannot be rotated. Losing it means
no future build can upgrade an existing install, for anybody. It belongs in a password
manager, not in a backup of this repository.

## Constraints

**`core:api` generates types, not calls.** Kotlin wire types come from
`api/openapi.yaml` and are committed with a drift gate, so the client cannot silently
disagree with the contract about shapes. The eight REST calls and the SSE reader are
hand-written — no generator models `text/event-stream`, and eight endpoints do not justify
a second codegen toolchain. Generated types stay internal to the module; no ViewModel ever
sees one. See [ADR-0004, Amendment 3](../../docs/adr/0004-contract-openapi-sse.md).

**No `when (serviceId)` anywhere in the UI.** The app holds a map of capability id to
composable renderer and looks it up. Branching on service identity discards the entire
point of capability discovery, and it is the single easiest way to quietly undo
ADR-0005. Treat it as a review-blocking defect.

**Unknown capabilities degrade visibly.** An app built before Immich support will meet
an `immich_jobs` capability it cannot render. It shows the capability's display label
with an "update CueSeek to view this" affordance — never an empty box, and never a
crash. This is a permanent condition, not a transitional one: the Wear app in
particular will routinely be older than the agent.

**The stream is a foreground affordance, and it lies.** A held SSE connection does not
survive Doze — but it does not disconnect either. It freezes for up to ~168s while still
reporting itself connected. Data is stale when nothing has arrived for roughly 2× the 15s
heartbeat, whatever the transport claims, and stale renders as `unknown` rather than as the
last known values. Anything that must be correct in the background is a poll or a
notification, never the stream.

**Multiple servers exist in the data layer from day one.** The MVP UI shows one, but
every repository is keyed by host id. Retrofitting that later is a navigation rewrite;
carrying it now is one field.

**Cleartext HTTP is permitted globally, because it cannot be narrowed.**
`network_security_config.xml` matches hostnames and single IP literals — not the CIDR range
Tailscale uses — and the agent's address is typed in by the user at runtime. The file
records this; the narrowing that actually holds is that the app only ever calls the address
the user entered. See [ADR-0001](../../docs/adr/0001-vpn-only-remote-access.md).

## Implementation choices

Decided for M2, listed here rather than as ADRs because none of them closes a door that is
expensive to reopen:

| Area | Choice | Why |
| --- | --- | --- |
| HTTP | OkHttp + Retrofit + kotlinx.serialization | `okhttp-sse` reads the stream; Ktor's multiplatform edge is worth nothing while every client is JVM |
| Credentials | DataStore sealed with an Android Keystore AES-GCM key | Jetpack Security is unmaintained — [ADR-0006, Amendment 2](../../docs/adr/0006-device-pairing-scoped-tokens.md) |
| DI | Manual constructor injection via `AppContainer` | Three screens and one data layer. Hilt earns its processor at M5, when Wear assembles the same repositories |
| Screenshot tests | Paparazzi, on `:core:design`'s status catalogue only | Status rendering is correctness-critical and stable; feature screens still being designed produce golden-image noise |
| Logout | Clears local credentials only | The CLI's default scopes exclude `devices.manage`, so a typical device cannot revoke itself. The app says so rather than failing quietly |
| Pairing | Typed host address + code | No QR producer exists — [ADR-0006, Amendment 3](../../docs/adr/0006-device-pairing-scoped-tokens.md) |

See also [ADR-0007](../../docs/adr/0007-client-capability-registry.md) and
[ADR-0010](../../docs/adr/0010-design-system-m3-expressive.md).
