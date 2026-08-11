package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// Server implements gen.StrictServerInterface.
//
// Every method here runs *after* authentication and authorization: the strict middleware
// in authz.go has already established that the caller holds the scope the contract
// requires. Handlers therefore contain no permission checks — if one appears, the
// requirement belongs in the contract instead.
var _ gen.StrictServerInterface = (*Server)(nil)

// ---------------------------------------------------------------- system

func (s *Server) GetSystem(ctx context.Context, _ gen.GetSystemRequestObject) (gen.GetSystemResponseObject, error) {
	return gen.GetSystem200JSONResponse(gen.System{
		HostId:       s.hostID,
		Hostname:     s.hostname,
		AgentVersion: s.agentVersion,
		ApiVersion:   s.apiVersion,
		StartedAt:    s.startedAt,
	}), nil
}

// ---------------------------------------------------------------- pairing

// PairDevice exchanges a pairing code for a device token. The only unauthenticated
// operation in the API.
func (s *Server) PairDevice(ctx context.Context, request gen.PairDeviceRequestObject) (gen.PairDeviceResponseObject, error) {
	// Rate limit before touching the database. The limit is the reason a 40-bit code is
	// safe at all, so it must apply to every attempt including malformed ones —
	// otherwise an attacker gets free guesses by sending slightly wrong shapes.
	caller := clientKeyFromContext(ctx)
	if !s.pairLimiter.Allow(caller) {
		slog.WarnContext(ctx, "pairing rate limit exceeded", "client", caller)
		return nil, errRateLimited
	}

	if request.Body == nil {
		return nil, errInvalidPairingCode
	}
	name := request.Body.DeviceName
	if name == "" {
		return nil, &Error{
			Status: http.StatusBadRequest,
			Type:   problemBase + "bad-request",
			Title:  "Bad request",
			Detail: "device_name must not be empty.",
		}
	}

	scopes, err := s.store.RedeemPairingCode(ctx, request.Body.Code)
	if errors.Is(err, store.ErrInvalidPairingCode) {
		// Unknown, expired and already-redeemed are one response. Distinguishing them
		// would tell a guesser which codes were ever real.
		s.audit(ctx, domain.Device{}, "device.pair", name, domain.OutcomeDenied,
			"invalid pairing code")
		return nil, errInvalidPairingCode
	}
	if err != nil {
		return nil, err
	}

	platform := domain.ParsePlatform(string(request.Body.Platform))
	device, token, err := s.store.CreateDevice(ctx, name, platform, scopes)
	if err != nil {
		return nil, err
	}

	s.audit(ctx, device, "device.pair", device.ID, domain.OutcomeSucceeded,
		"scopes: "+formatScopes(scopes))
	// The fingerprint is the first 16 hex characters of the stored token_hash. Logging it
	// lets an operator confirm that the token they are holding is the one this device was
	// issued, without reading the database and without the token itself ever appearing.
	slog.InfoContext(ctx, "device paired",
		"device_id", device.ID, "name", device.Name, "scopes", formatScopes(scopes),
		"token_fingerprint", store.TokenFingerprint(token))

	// The only time the plaintext token exists outside the client. Never logged.
	return gen.PairDevice201JSONResponse(gen.PairResponse{
		Token:  token,
		Device: toGenDevice(device),
	}), nil
}

// ---------------------------------------------------------------- devices

func (s *Server) ListDevices(ctx context.Context, _ gen.ListDevicesRequestObject) (gen.ListDevicesResponseObject, error) {
	devices, err := s.store.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ListDevices200JSONResponse(toGenDevices(devices)), nil
}

func (s *Server) RevokeDevice(ctx context.Context, request gen.RevokeDeviceRequestObject) (gen.RevokeDeviceResponseObject, error) {
	actor, err := deviceFromContext(ctx)
	if err != nil {
		return nil, err
	}

	target, err := s.store.GetDevice(ctx, request.DeviceId)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound.withDetail("no device with id %q", request.DeviceId)
	}
	if err != nil {
		return nil, err
	}

	if err := s.store.RevokeDevice(ctx, request.DeviceId); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errNotFound.withDetail("no device with id %q", request.DeviceId)
		}
		return nil, err
	}

	// Recorded against the actor, naming the target. Revoking your own device is
	// permitted — it is how a client logs itself out — and the log makes that legible.
	s.audit(ctx, actor, "device.revoke", target.ID, domain.OutcomeSucceeded,
		"revoked "+target.Name)
	slog.InfoContext(ctx, "device revoked",
		"device_id", target.ID, "name", target.Name, "by", actor.ID)

	return gen.RevokeDevice204Response{}, nil
}

// ---------------------------------------------------------------- services

// ListServices reports every managed service, served entirely from cache.
//
// No upstream call happens here. The poller refreshes each service on its own schedule
// and this reads what it last saw (ADR-0003). That is what stops a wedged qBittorrent
// from hanging the dashboard, and what stops a watch glance from fanning out one request
// per service.
func (s *Server) ListServices(ctx context.Context, _ gen.ListServicesRequestObject) (gen.ListServicesResponseObject, error) {
	services := s.registry.Services()
	out := make([]gen.Service, 0, len(services))
	for _, svc := range services {
		out = append(out, s.describeService(svc))
	}
	return gen.ListServices200JSONResponse(out), nil
}

func (s *Server) GetService(ctx context.Context, request gen.GetServiceRequestObject) (gen.GetServiceResponseObject, error) {
	svc, ok := s.registry.Service(request.ServiceId)
	if !ok {
		return nil, errNotFound.withDetail("no service with id %q", request.ServiceId)
	}
	return gen.GetService200JSONResponse(s.describeService(svc)), nil
}

// describeService joins three sources: identity and capabilities from the adapter, health
// from the cache, and actions from the Controllable capability if the adapter has it.
func (s *Server) describeService(svc adapters.Service) gen.Service {
	out := gen.Service{
		Id:           svc.ID(),
		Name:         svc.Name(),
		Capabilities: toGenCapabilities(adapters.CapabilitiesOf(svc)),
		Actions:      []gen.Action{},
	}

	// Actions come from the same snapshot as the health, not from a fresh call to the
	// adapter. They are state-dependent now (ADR-0002 Amendment 1), so reading them
	// separately would let this response offer Start while the invoke path — reading a
	// different observation — rejected it.
	if snapshot, ok := s.cache.Get(svc.ID()); ok {
		out.Health = toGenHealth(snapshot.Health)
		out.Actions = toGenActions(snapshot.Actions)
	} else {
		// Registered but untracked. Should not happen — the poller tracks everything the
		// registry built — so say so honestly rather than inventing a status.
		out.Health = toGenHealth(domain.UnknownHealth(s.now(), domain.HealthReason{
			Code:    domain.ReasonNotPolled,
			Message: "This service is not being polled.",
		}))
	}
	return out
}

// InvokeServiceAction starts an action and returns immediately.
//
// The response is 202 plus an action id, never a result, because the agent has no result
// to give: M0 established that systemd's RestartUnit returns once the job is *queued*.
// The outcome is resolved by a background goroutine and will be published over the stream
// in M1.7 (ADR-0004).
func (s *Server) InvokeServiceAction(ctx context.Context, request gen.InvokeServiceActionRequestObject) (gen.InvokeServiceActionResponseObject, error) {
	actor, err := deviceFromContext(ctx)
	if err != nil {
		return nil, err
	}

	svc, ok := s.registry.Service(request.ServiceId)
	if !ok {
		return nil, errNotFound.withDetail("no service with id %q", request.ServiceId)
	}

	controllable, ok := svc.(adapters.Controllable)
	if !ok {
		return nil, errNotFound.withDetail(
			"service %q does not support actions", request.ServiceId)
	}

	// The action must be one the adapter currently advertises. Invoking something absent
	// from that list would let a client reach behaviour that never appeared in the
	// descriptor list a UI gates on — including a Stop on a service that is already
	// stopped, or a Start on one that is running.
	//
	// Read from the cache so this agrees with what the service listing showed. The
	// fallback covers the window before the first poll: an invocation is already going
	// to D-Bus, so one state read on this path costs nothing that matters, and ADR-0003's
	// concern is a client *read* hanging on an upstream call.
	available := controllable.Actions(ctx)
	if snapshot, ok := s.cache.Get(request.ServiceId); ok && snapshot.Actions != nil {
		available = snapshot.Actions
	}

	var descriptor *domain.Action
	for _, candidate := range available {
		if candidate.ID == request.ActionId {
			action := candidate
			descriptor = &action
			break
		}
	}
	if descriptor == nil {
		return nil, errNotFound.withDetail(
			"service %q advertises no action %q", request.ServiceId, request.ActionId)
	}

	tracked, started := s.actions.begin(request.ServiceId, request.ActionId)
	if tracked == nil {
		return nil, errInternal
	}
	if !started {
		s.audit(ctx, actor, "service.action", request.ServiceId, domain.OutcomeDenied,
			fmt.Sprintf("%s already in progress", request.ActionId))
		return nil, errActionInProgress.withDetail(
			"%q is already running on %q; wait for it to finish",
			request.ActionId, request.ServiceId)
	}

	// Invoked with a context that is NOT the request's. The request ends the moment the
	// 202 is written, and cancelling the systemd job at that point is exactly the wrong
	// behaviour — the whole design is that the work outlives the call.
	invokeCtx, cancel := context.WithTimeout(context.Background(), actionInvokeTimeout)
	job, err := controllable.Invoke(invokeCtx, request.ActionId)
	if err != nil {
		cancel()
		s.actions.finish(tracked.ID, err)
		s.audit(ctx, actor, "service.action", request.ServiceId, domain.OutcomeFailed,
			fmt.Sprintf("%s: %v", request.ActionId, err))
		return nil, s.actionError(request.ServiceId, request.ActionId, err)
	}

	s.actions.markRunning(tracked.ID)
	s.audit(ctx, actor, "service.action", request.ServiceId, domain.OutcomeAccepted,
		fmt.Sprintf("%s (action %s)", request.ActionId, tracked.ID))
	slog.InfoContext(ctx, "action accepted",
		"service", request.ServiceId, "action", request.ActionId,
		"action_id", tracked.ID, "device", actor.ID, "risk", descriptor.Risk)

	s.awaitAction(invokeCtx, cancel, tracked.ID, actor, request.ServiceId, request.ActionId, job)

	return gen.InvokeServiceAction202JSONResponse(gen.ActionAccepted{
		ActionId:   tracked.ID,
		ServiceId:  request.ServiceId,
		Action:     request.ActionId,
		Status:     gen.ActionStatus(actionRunning),
		AcceptedAt: tracked.AcceptedAt,
	}), nil
}

// awaitAction resolves the job's outcome after the HTTP response has been sent.
//
// Detached on purpose. Audit entries are written with a fresh context for the same
// reason: the request's context is cancelled the instant the 202 is written, and using it
// here would abort the very record that explains what happened.
func (s *Server) awaitAction(
	ctx context.Context, cancel context.CancelFunc,
	trackedID string, actor domain.Device, serviceID, actionID string, job *host.Job,
) {
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		defer cancel()

		if job == nil {
			// An adapter may complete an action synchronously and return no job.
			s.actions.finish(trackedID, nil)
			s.audit(context.Background(), actor, "service.action", serviceID,
				domain.OutcomeSucceeded, actionID)
			s.publishActionProgress(trackedID, serviceID, actionID, actionSucceeded, nil)
			return
		}

		result, err := job.Wait(ctx)
		switch {
		case err != nil:
			s.actions.finish(trackedID, err)
			slog.Error("action did not complete",
				"service", serviceID, "action", actionID, "action_id", trackedID, "error", err)
			s.audit(context.Background(), actor, "service.action", serviceID,
				domain.OutcomeFailed, fmt.Sprintf("%s: %v", actionID, err))
			s.publishActionProgress(trackedID, serviceID, actionID, actionFailed, err)

		case !result.Succeeded():
			failure := fmt.Errorf("systemd reported %q", result)
			s.actions.finish(trackedID, failure)
			slog.Warn("action failed",
				"service", serviceID, "action", actionID, "action_id", trackedID, "result", result)
			s.audit(context.Background(), actor, "service.action", serviceID,
				domain.OutcomeFailed, fmt.Sprintf("%s: %v", actionID, failure))
			s.publishActionProgress(trackedID, serviceID, actionID, actionFailed, failure)

		default:
			s.actions.finish(trackedID, nil)
			slog.Info("action completed",
				"service", serviceID, "action", actionID, "action_id", trackedID)
			s.audit(context.Background(), actor, "service.action", serviceID,
				domain.OutcomeSucceeded, actionID)
			s.publishActionProgress(trackedID, serviceID, actionID, actionSucceeded, nil)
		}
	}()
}

// actionError maps a host-layer failure onto the contract's declared responses.
func (s *Server) actionError(serviceID, actionID string, err error) error {
	switch {
	case errors.Is(err, host.ErrUnitNotManaged):
		// The adapter offered an action whose unit is not in the allowlist — a
		// configuration mistake on this host, not a client error.
		return errActionUnavailable.withDetail(
			"%q cannot be performed: the unit behind service %q is not in this agent's "+
				"managed list", actionID, serviceID)

	case errors.Is(err, host.ErrUnauthorized):
		// The caller did everything right; the agent is not permitted. Reporting 403
		// would blame the client's token, and a bare 500 would hide a problem the
		// operator can fix in one file.
		return errActionUnavailable.withDetail(
			"the agent is not authorised to perform %q on this host; check that the "+
				"polkit rule in deploy/ is installed and names this unit", actionID)

	case errors.Is(err, host.ErrUnitNotFound):
		return errActionUnavailable.withDetail(
			"the unit behind service %q does not exist on this host", serviceID)

	case errors.Is(err, host.ErrUnsupportedPlatform):
		return errActionUnavailable.withDetail(
			"host control is not supported on this platform")
	}
	return errInternal
}

// StreamEvents lives in stream.go.

// ---------------------------------------------------------------- audit

// audit records an action, and never fails the request it describes.
//
// A restart that worked must not be reported as failed because the log write did. The
// failure is logged loudly instead: an audit trail that is quietly incomplete is worse
// than one that is obviously broken.
func (s *Server) audit(ctx context.Context, actor domain.Device, action, target string, outcome domain.Outcome, detail string) {
	entry := domain.AuditEntry{
		At:         s.now(),
		DeviceID:   actor.ID,
		DeviceName: actor.Name,
		Action:     action,
		Target:     target,
		Outcome:    outcome,
		Detail:     detail,
	}
	if err := s.store.AppendAudit(ctx, entry); err != nil {
		slog.ErrorContext(ctx, "failed to write audit entry",
			"action", action, "target", target, "outcome", outcome, "error", err)
	}
}

func defaultNow() time.Time { return time.Now().UTC() }
