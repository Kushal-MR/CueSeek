package adapters

import (
	"context"
	"net/http"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// Service is the whole of what every adapter must implement.
//
// Deliberately tiny. Everything beyond identity and health is an optional capability,
// because there is no honest set of fields describing both a playback session and a
// torrent (ADR-0005).
type Service interface {
	// ID is the stable identifier used in API paths. Comes from configuration, so two
	// instances of the same software can be managed on one host.
	ID() string

	// Name is the display name.
	Name() string

	// Health observes the service now.
	//
	// Implementations must not block indefinitely: ctx always carries a deadline, and a
	// hung service must degrade to unreachable rather than stall the agent.
	//
	// Returning an error and returning an unhealthy Health are different things. An
	// error means the adapter could not form an opinion; an unreachable Health means it
	// formed the opinion that the service is down. Adapters should almost always do the
	// latter — "I could not connect" is information, not a failure.
	Health(ctx context.Context) (domain.Health, error)
}

// Controllable is the optional capability of having actions that can be invoked.
type Controllable interface {
	// Actions returns the descriptors that currently apply.
	//
	// No longer static, as of ADR-0002 Amendment 1: Start applies only to a stopped unit
	// and Stop only to a running one. Takes a context because answering may require
	// reading the unit's state, and that read must happen on the poll path rather than
	// on a request path (ADR-0003).
	Actions(ctx context.Context) []domain.Action

	// Invoke starts an action and returns a job whose outcome arrives later.
	//
	// Asynchronous because the underlying operations are: M0 established that systemd's
	// RestartUnit returns once the job is queued, not once the service is back. This is
	// the mechanism behind the API's 202-plus-action-id design (ADR-0004).
	Invoke(ctx context.Context, actionID string) (*host.Job, error)
}

// NowPlayingProvider and TransferProvider are declared here, unimplemented by any adapter
// yet, because their existence is what makes capability discovery meaningful rather than
// a single-branch formality. They land in M3.
//
// Their payload types are deliberately absent: shaping now_playing before a second
// media server exists would bake Jellyfin's DTOs into a contract that Plex and Emby also
// have to satisfy, which ADR-0005 explicitly warns against.
type (
	// NowPlayingProvider reports what a media service is currently playing.
	NowPlayingProvider interface {
		NowPlayingCapability()
	}
	// TransferProvider reports in-flight transfers, e.g. torrents or downloads.
	TransferProvider interface {
		TransfersCapability()
	}
)

// UnitControl is the slice of the host layer that adapters need.
//
// Declared here, in the consumer, and satisfied by *host.Controller without that package
// knowing adapters exist. This is how an adapter restarts its own service without ever
// touching systemd or D-Bus: it asks the host layer, which enforces the allowlist and is
// in turn bounded by polkit (ADR-0002).
//
// An adapter that imported go-systemd directly would bypass the allowlist entirely, so
// the narrowness of this interface is a security property, not a style preference.
// WebUIProvider is implemented by an adapter whose service has its own interface.
//
// Returning false is normal, not a failure: whether a service has a web UI is an operator
// configuration, not a property of the adapter's code. An adapter implements this and then
// reports what it was configured with, which is why discovery below checks the second
// return rather than only the type assertion.
type WebUIProvider interface {
	WebUI() (domain.WebUI, bool)
}

type UnitControl interface {
	// UnitState reads the unit's current properties, so an adapter can decide which
	// lifecycle actions currently apply.
	UnitState(ctx context.Context, unit string) (host.UnitState, error)

	RestartUnit(ctx context.Context, unit string) (*host.Job, error)
	StartUnit(ctx context.Context, unit string) (*host.Job, error)
	StopUnit(ctx context.Context, unit string) (*host.Job, error)
}

// Deps are what a Factory may use to build an adapter.
//
// Passed as a struct rather than as parameters so that adding a dependency later does not
// change the signature of every adapter constructor in the tree.
type Deps struct {
	// HTTPClient is shared across adapters for connection reuse. Per-request deadlines
	// come from the context, not from Client.Timeout, so one slow service cannot consume
	// another's budget.
	HTTPClient *http.Client

	// Units is the host layer. May be nil in tests and on platforms without systemd; an
	// adapter must degrade rather than panic.
	Units UnitControl
}

// capabilityProbe pairs a capability with the test for it.
//
// The one type-assertion site in the codebase, and the reason it is acceptable: this list
// is O(capabilities), not O(services). Adding an adapter touches it zero times; adding a
// capability touches it once, here. That is precisely the property `switch serviceID`
// fails to have (ADR-0005, ADR-0007).
var capabilityProbes = []struct {
	capability domain.Capability
	implements func(Service) bool
}{
	{domain.CapabilityControl, func(s Service) bool { _, ok := s.(Controllable); return ok }},
	{domain.CapabilityWebUI, func(s Service) bool {
		// Two conditions, unlike the others: the adapter must implement the interface
		// *and* have been configured with somewhere to point. Advertising the capability
		// without a destination would make a client offer to open nothing.
		provider, ok := s.(WebUIProvider)
		if !ok {
			return false
		}
		_, configured := provider.WebUI()
		return configured
	}},
	{domain.CapabilityNowPlaying, func(s Service) bool { _, ok := s.(NowPlayingProvider); return ok }},
	{domain.CapabilityTransfers, func(s Service) bool { _, ok := s.(TransferProvider); return ok }},
}

// CapabilitiesOf discovers what a service supports.
//
// Health is unconditional: it is part of Service, so every adapter has it by
// construction. The rest are opt-in and discovered by type assertion at registration.
func CapabilitiesOf(s Service) []domain.Capability {
	caps := []domain.Capability{domain.CapabilityHealth}
	for _, probe := range capabilityProbes {
		if probe.implements(s) {
			caps = append(caps, probe.capability)
		}
	}
	return caps
}

// HasCapability reports whether a service advertises the given capability id.
func HasCapability(s Service, id string) bool {
	for _, c := range CapabilitiesOf(s) {
		if c.ID == id {
			return true
		}
	}
	return false
}
