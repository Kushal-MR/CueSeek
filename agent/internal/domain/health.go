package domain

import (
	"slices"
	"time"
)

// HealthStatus is the closed set of states a service or host can be in.
//
// Shared by the API, both clients and the design system's status language (ADR-0008).
// Adding a fifth value is a contract change, not an implementation detail.
type HealthStatus string

const (
	// StatusHealthy means reachable and reporting no problems.
	StatusHealthy HealthStatus = "healthy"

	// StatusDegraded means reachable, but something is wrong.
	//
	// Reachability and reported status are deliberately separate facts. "We could not
	// reach qBittorrent" and "qBittorrent says it is unhappy" have different causes and
	// different fixes, and a console that conflates them lies to its operator (ADR-0005).
	StatusDegraded HealthStatus = "degraded"

	// StatusUnreachable means the agent could not talk to the service at all.
	StatusUnreachable HealthStatus = "unreachable"

	// StatusUnknown means the agent has no current information.
	//
	// A real state, not an error: before the first poll completes, and whenever cached
	// state has aged past tolerance, "I don't know" is the honest answer. Showing stale
	// green while the agent cannot reach a service is worse than showing nothing,
	// because it is confidently wrong (ADR-0008).
	StatusUnknown HealthStatus = "unknown"
)

// AllHealthStatuses is the closed set, in decreasing order of confidence.
var AllHealthStatuses = []HealthStatus{
	StatusHealthy, StatusDegraded, StatusUnreachable, StatusUnknown,
}

// Valid reports whether s is a recognised status.
func (s HealthStatus) Valid() bool { return slices.Contains(AllHealthStatuses, s) }

// HealthReason explains a status.
//
// Reasons travel with the status because "degraded" alone is not actionable, while
// "degraded: authentication rejected" is. The code is stable and safe to branch on; the
// message is for humans and may be reworded freely.
type HealthReason struct {
	Code    string
	Message string
}

// Reason codes emitted by the agent. Stable identifiers: a client may branch on these,
// so renaming one is a breaking change even though the message beside it is not.
const (
	ReasonNotPolled       = "not_polled"       // no observation yet
	ReasonStale           = "stale"            // last observation aged out
	ReasonUnreachable     = "unreachable"      // transport failure
	ReasonTimeout         = "timeout"          // upstream did not answer in time
	ReasonAuthFailed      = "auth_failed"      // credentials rejected
	ReasonUpstreamError   = "upstream_error"   // upstream returned an error status
	ReasonInvalidResponse = "invalid_response" // reachable, but the answer made no sense
	ReasonShuttingDown    = "shutting_down"    // service says it is going away
	ReasonPendingRestart  = "pending_restart"  // service says it needs a restart

	// ReasonPeerConnectivity covers a service that is running and answering but cannot
	// reach the network it exists to use — a torrent client that is firewalled or
	// disconnected from its peers.
	//
	// Distinct from ReasonUnreachable, which is about the agent's own hop. Here the agent
	// reached the service perfectly well; the service is the one that cannot reach
	// anything. Conflating the two would send the operator to look at the wrong network.
	ReasonPeerConnectivity = "peer_connectivity"
)

// Health is one observation of a service's state.
type Health struct {
	Status HealthStatus

	// Reachable records whether the agent could talk to the service at ObservedAt.
	Reachable bool

	// ReportedStatus is what the service says about itself, verbatim and unmapped.
	// Empty when the service was unreachable or reports nothing of the kind.
	ReportedStatus string

	Reasons []HealthReason

	// ObservedAt is when this state was actually seen — not when it was served. Clients
	// render staleness from this rather than presenting cached data as current.
	ObservedAt time.Time
}

// UnknownHealth returns an unknown observation carrying a reason.
//
// A constructor rather than a zero value, because the zero value of Health has an empty
// status string, and an empty status is not one of the four. Making the honest answer
// the easy one to write is the point.
func UnknownHealth(at time.Time, reason HealthReason) Health {
	return Health{
		Status:     StatusUnknown,
		Reachable:  false,
		Reasons:    []HealthReason{reason},
		ObservedAt: at,
	}
}

// RiskLevel says how much care an action warrants.
//
// Clients gate on this without knowing what the action does — which is what lets a new
// action ship to an existing client with a confirmation prompt already attached.
type RiskLevel string

const (
	// RiskSafe: read-only or trivially reversible.
	RiskSafe RiskLevel = "safe"
	// RiskDisruptive: interrupts service, but the service comes back. Restarting Jellyfin
	// while somebody is watching a film.
	RiskDisruptive RiskLevel = "disruptive"
	// RiskDestructive: may lose data or require physical access to undo. Powering off a
	// machine you are not standing next to.
	RiskDestructive RiskLevel = "destructive"
)

var allRiskLevels = []RiskLevel{RiskSafe, RiskDisruptive, RiskDestructive}

// Valid reports whether r is a recognised risk level.
func (r RiskLevel) Valid() bool { return slices.Contains(allRiskLevels, r) }

// Action is a descriptor, not a function.
//
// Deliberately data: a client renders and gates an action it has never heard of, using
// the label and risk level alone. Shipping a new action therefore requires no client
// release (ADR-0005).
type Action struct {
	ID          string
	Label       string
	Description string
	Risk        RiskLevel
}

// Capability is something a service can do.
//
// Clients hold a map of capability id to renderer and look it up; they must never branch
// on service id (ADR-0007). The label exists so a client that predates a capability can
// render "Immich Jobs — update CueSeek to view this" rather than an empty box.
type Capability struct {
	ID    string
	Label string
}

// The capability vocabulary. Ids are public API: a client keys its renderer map on them,
// so renaming one silently breaks every client that has not been updated.
var (
	CapabilityHealth     = Capability{ID: "health", Label: "Health"}
	CapabilityControl    = Capability{ID: "control", Label: "Controls"}
	CapabilityWebUI      = Capability{ID: "web_ui", Label: "Web interface"}
	CapabilityNowPlaying = Capability{ID: "now_playing", Label: "Now Playing"}
	CapabilityTransfers  = Capability{ID: "transfers", Label: "Transfers"}
)

// WebUI locates a service's own interface, as parts rather than as a URL.
//
// Deliberately not a URL, and the reason is worth stating where the type is defined. The
// agent reaches Jellyfin at 127.0.0.1 because they share a host; that address is useless
// to a phone. The client already holds one that works — whatever it paired with — so it
// composes scheme://{paired host}:{port}{path} itself.
//
// The security property is the same shape as the allowlist duplication in ADR-0002: by
// never supplying an origin, the agent cannot send a client somewhere the operator never
// pointed it, even if the agent is wrong or compromised.
type WebUI struct {
	// Scheme is "http" or "https".
	Scheme string
	// Port the service's own interface listens on.
	Port int
	// Path to open, including the leading slash.
	Path string
}
