// Package adapters defines what a managed service is, and hosts one sub-package per
// supported service.
//
// # Capabilities, not a service interface
//
// There is no single fat Adapter interface, because there is no set of fields that
// honestly describes both a playback session and a torrent. Instead a minimal Service
// interface, plus narrow optional capabilities that an adapter opts into and the
// registry discovers by type assertion at registration time (ADR-0005):
//
//	Service             id, display name, health
//	Controllable        Actions() []Action; Invoke(ctx, actionID)
//	NowPlayingProvider  NowPlaying(ctx) []Session
//	TransferProvider    Transfers(ctx) []Transfer
//
// Adapters are compiled in and registered explicitly. There is no plugin loader: with
// two adapters there is no evidence about what a plugin API should look like, and
// freezing that guess into a public extension point is expensive to undo. Revisit at
// roughly five adapters.
//
// # Rules for adapter authors
//
//   - Distinguish "could not reach the service" from "the service reports a problem".
//     They are different facts and the console must not conflate them.
//
//   - Return actions as descriptors carrying an id, a display label and a risk level.
//     Clients gate destructive actions on that risk level without knowing what the
//     action does.
//
//   - Capability semantics belong to CueSeek, not to the upstream service. now_playing
//     is a contract that Jellyfin, Plex and Emby will all satisfy — do not shape it
//     around whichever one was implemented first.
//
//   - Never block indefinitely. Every upstream call takes a context with a timeout. A
//     hung service must degrade to "unreachable", never stall the agent.
//
// # Polling
//
// The agent polls adapters on its own schedule — one goroutine each, independent
// intervals — and serves cached last-known-good state with the timestamp it was
// observed. Client requests never trigger upstream calls synchronously. This is what
// stops one wedged service from hanging the dashboard, and what stops a watch glance
// from fanning out a request to every service on the box.
package adapters
