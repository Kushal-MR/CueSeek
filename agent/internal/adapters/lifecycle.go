package adapters

import (
	"context"
	"fmt"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// Lifecycle action identifiers.
//
// Declared once rather than per adapter because "restart this unit" is the same operation
// for every unit-backed service, and three adapters each spelling `restart` themselves is
// three chances to spell it differently. ADR-0011 measures how much a new adapter costs;
// this is one of the places that cost is paid or avoided.
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
)

// LifecycleCopy carries the wording an adapter contributes to shared action descriptors.
//
// The mechanism is identical for every service; only the consequence differs. Jellyfin
// interrupts playback, qBittorrent pauses transfers, and the operator deserves to be told
// which one they are about to cause.
type LifecycleCopy struct {
	// DisplayName is the service's name as it appears in a label, e.g. "Jellyfin".
	DisplayName string

	// Interruption completes the sentence "…will be interrupted": what the user loses
	// when this service goes away for a moment. Optional.
	Interruption string
}

// LifecycleActions returns the actions available for a unit in the given state.
//
// This is why Controllable.Actions is no longer static. Offering Start on a running
// service is noise, and offering Stop on a stopped one is a lie; a client that has to
// work out which apply is a client re-deriving the agent's knowledge from health, badly.
// The agent knows the unit state, so the agent decides.
//
// `known` is false when the unit's state could not be read. Restart is still offered,
// because systemd's RestartUnit starts an inactive unit — it is the one verb that is
// correct whichever state we failed to observe.
func LifecycleActions(state host.UnitState, known bool, copy LifecycleCopy) []domain.Action {
	name := copy.DisplayName
	interruption := copy.Interruption
	if interruption == "" {
		interruption = "Anything using it will be interrupted."
	}

	restart := domain.Action{
		ID:    ActionRestart,
		Label: "Restart " + name,
		Description: "Restarts the " + name + " service. " + interruption +
			" It comes back on its own.",
		// Disruptive, not destructive: it interrupts people, but the service is running
		// afterwards whatever happens.
		Risk: domain.RiskDisruptive,
	}

	if !known {
		return []domain.Action{restart}
	}

	if !state.Active() {
		return []domain.Action{{
			ID:          ActionStart,
			Label:       "Start " + name,
			Description: "Starts the " + name + " service.",
			// Nothing is interrupted and nothing is lost: the service is not running.
			Risk: domain.RiskSafe,
		}}
	}

	return []domain.Action{
		restart,
		{
			ID:    ActionStop,
			Label: "Stop " + name,
			// The wording is load-bearing. A stop is the one lifecycle action that does
			// not undo itself, and the operator should not have to know that systemd
			// leaves the unit enabled in order to predict what happens next.
			Description: "Stops the " + name + " service. " + interruption +
				" It stays stopped until you start it again or the host reboots.",
			Risk: domain.RiskDestructive,
		},
	}
}

// InvokeLifecycle dispatches a lifecycle action id to the host layer.
//
// Every path goes through UnitControl, never through a service's own restart endpoint and
// never through systemd directly, so the request passes the configured unit allowlist and
// then polkit (ADR-0002). An adapter reaching for go-systemd itself would bypass both.
func InvokeLifecycle(
	ctx context.Context,
	units UnitControl,
	unit string,
	actionID string,
) (*host.Job, error) {
	switch actionID {
	case ActionRestart:
		return units.RestartUnit(ctx, unit)
	case ActionStart:
		return units.StartUnit(ctx, unit)
	case ActionStop:
		return units.StopUnit(ctx, unit)
	default:
		return nil, fmt.Errorf("unknown lifecycle action %q for unit %q", actionID, unit)
	}
}

// AvailableLifecycleActions reads the unit's state and returns what may be done to it.
//
// The state read happens here, on the poll path, and never on a request path: a client
// asking for the service list must not cause a D-Bus round trip (ADR-0003).
func AvailableLifecycleActions(
	ctx context.Context,
	units UnitControl,
	unit string,
	copy LifecycleCopy,
) []domain.Action {
	state, err := units.UnitState(ctx, unit)
	return LifecycleActions(state, err == nil, copy)
}
