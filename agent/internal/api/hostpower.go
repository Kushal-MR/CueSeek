package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// Host power actions.
//
// The one endpoint in this package that must answer before it acts. Every other handler
// can do the work and then describe it; a reboot handler that reboots first never writes
// its response, because the process is gone. So: acknowledge, then act on a timer.
//
// That inversion is also why success is unobservable here. A power action that worked
// takes the stream, the agent and the machine with it, so the only outcome that can ever
// reach a client is a **failure** — which is genuinely useful information, since it means
// the machine is still up and the button did nothing.

const (
	// powerActionDelay is how long the agent waits after answering before it acts.
	//
	// Long enough for the 202 to be written and flushed to a phone on a tailnet, short
	// enough that nobody watching the screen wonders whether the tap registered. It is a
	// send window, not a change-your-mind window: there is no cancel, and pretending
	// otherwise by making it longer would be worse than not offering one.
	powerActionDelay = 750 * time.Millisecond

	// powerInvokeTimeout bounds the D-Bus call itself.
	//
	// Only reached when logind is wedged or polkit is deliberating. A power action that
	// has not been accepted in ten seconds is not going to be.
	powerInvokeTimeout = 10 * time.Second
)

// HostPower is the machine-level half of host control, as this package needs it.
//
// An interface rather than *host.Controller so the API can be tested without a D-Bus
// connection, and so an agent built without power support simply supplies nothing.
type HostPower interface {
	// PowerActions lists what this agent offers. Empty on a platform that cannot.
	PowerActions() []domain.Action

	// InvokePower requests the action. A nil error means accepted, not done.
	InvokePower(ctx context.Context, action string) error
}

// ListHostActions returns the power actions this agent offers.
//
// Requires `read`, not `host.power`, and returns the same list to every caller. What the
// agent *offers* is a property of the agent; what a device may *do* is a property of its
// token. Filtering the list by scope would leave a client unable to distinguish "this
// agent is too old to power off" from "this device was not granted permission" — two
// problems with completely different fixes, and the client already knows its own scopes.
func (s *Server) ListHostActions(
	_ context.Context, _ gen.ListHostActionsRequestObject,
) (gen.ListHostActionsResponseObject, error) {
	return gen.ListHostActions200JSONResponse(s.hostActions()), nil
}

// hostActions renders the offered actions in the wire shape. Never nil: a platform with
// no power support returns an empty array, which says "none" rather than "unknown".
func (s *Server) hostActions() []gen.Action {
	out := []gen.Action{}
	if s.power == nil {
		return out
	}
	for _, action := range s.power.PowerActions() {
		out = append(out, gen.Action{
			Id:    action.ID,
			Label: action.Label,
			// Optional on the wire, and always present here: the consequence is the
			// whole reason a reader pauses before holding the button down.
			Description: optionalString(action.Description),
			Risk:        gen.ActionRisk(action.Risk),
		})
	}
	return out
}

// InvokeHostAction accepts a power action and performs it shortly afterwards.
//
// Scope is enforced here rather than in the client. A press-and-hold confirmation is user
// experience; this is the control, and it holds for any client ever written including one
// that skips the gesture entirely (ADR-0002 Amendment 2).
func (s *Server) InvokeHostAction(
	ctx context.Context, request gen.InvokeHostActionRequestObject,
) (gen.InvokeHostActionResponseObject, error) {
	if s.power == nil || len(s.power.PowerActions()) == 0 {
		return nil, errNotFound.withDetail("this agent offers no host power actions")
	}

	if !s.offersHostAction(request.ActionId) {
		return nil, errNotFound.withDetail("unknown host power action")
	}

	// One at a time. A second reboot arriving while the first is in its send window is
	// not a second reboot, and letting both through would mean two goroutines racing to
	// end the same machine.
	if !s.claimPower() {
		return nil, errActionInProgress.withDetail("a host power action is already in flight")
	}

	actionID, err := newActionID()
	if err != nil {
		s.releasePower()
		return nil, err
	}

	// Logged at warn, with the device named. This is the most consequential thing the
	// agent can be asked to do, and the journal is the only place it is recorded once the
	// machine goes down.
	name := "unknown"
	if device, err := deviceFromContext(ctx); err == nil {
		name = device.Name
	}
	slog.Warn("host power action accepted",
		"action", request.ActionId, "action_id", actionID, "device", name)

	s.schedulePower(actionID, request.ActionId)

	return gen.InvokeHostAction202JSONResponse(gen.HostActionAccepted{
		ActionId:   actionID,
		Action:     request.ActionId,
		Status:     gen.Pending,
		AcceptedAt: s.now(),
	}), nil
}

func (s *Server) offersHostAction(id string) bool {
	for _, action := range s.power.PowerActions() {
		if action.ID == id {
			return true
		}
	}
	return false
}

// schedulePower performs the action after the response has gone out.
//
// Detached from the request context on purpose — that context is cancelled the instant the
// 202 is written, which is precisely the moment this work begins. Registered with the
// server's background group so a shutdown racing a reboot is at least observed in order.
func (s *Server) schedulePower(actionID, action string) {
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		defer s.releasePower()

		time.Sleep(powerActionDelay)

		ctx, cancel := context.WithTimeout(context.Background(), powerInvokeTimeout)
		defer cancel()

		err := s.power.InvokePower(ctx, action)
		if err == nil {
			// Nothing follows this in the ordinary case. If the machine is going down,
			// this line is the last thing the agent ever logs.
			slog.Warn("host power action handed to logind", "action", action, "action_id", actionID)
			return
		}

		// The machine is still here, which makes this worth telling somebody about.
		slog.Error("host power action failed", "action", action, "action_id", actionID, "error", err)
		s.publishHostActionProgress(actionID, action, actionFailed, err)
	}()
}

// publishHostActionProgress emits the only outcome a power action can deliver.
//
// Its own event type rather than `action_progress`, whose `service_id` is required by the
// contract. Relaxing that would have been the smaller change to the specification and much
// the worse one on a phone: a client built before this shipped would fail to parse a field
// it was promised, and it would fail while some *other* device pressed the button. An
// unrecognised event type is ignored safely; a malformed known one is not.
func (s *Server) publishHostActionProgress(actionID, action string, state actionState, failure error) {
	progress := gen.HostActionProgress{
		ActionId: actionID,
		Action:   action,
		Status:   gen.ActionStatus(state),
		At:       s.now(),
	}
	if failure != nil {
		message := failure.Error()
		if errors.Is(failure, host.ErrUnauthorized) {
			// The most likely failure by far, and the least self-explanatory: it means
			// the polkit rule is missing or does not cover this action, not that the
			// device's token was wrong.
			message = "The host refused the request. Check that the CueSeek polkit rule " +
				"grants the power actions: " + message
		}
		progress.Error = &message
	}
	s.hub.publish(streamEvent{
		typ:              gen.StreamEventTypeHostActionProgress,
		emittedAt:        progress.At,
		hostActionResult: &progress,
	})
}
