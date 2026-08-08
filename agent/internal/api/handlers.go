package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
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
	slog.InfoContext(ctx, "device paired",
		"device_id", device.ID, "name", device.Name, "scopes", formatScopes(scopes))

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

// ListServices reports the configured services.
//
// Health is `unknown` for all of them, which is the honest answer rather than a
// placeholder: no adapter exists yet, so nothing has been observed. ADR-0008 makes
// `unknown` a first-class state precisely for this — reporting `healthy` before the first
// poll would be confidently wrong, which is worse than admitting ignorance.
//
// Capabilities and actions are empty until M1.5 registers adapters.
func (s *Server) ListServices(ctx context.Context, _ gen.ListServicesRequestObject) (gen.ListServicesResponseObject, error) {
	out := make([]gen.Service, 0, len(s.services))
	for _, svc := range s.services {
		out = append(out, s.unpolledService(svc.ID, svc.Name))
	}
	return gen.ListServices200JSONResponse(out), nil
}

func (s *Server) GetService(ctx context.Context, request gen.GetServiceRequestObject) (gen.GetServiceResponseObject, error) {
	for _, svc := range s.services {
		if svc.ID == request.ServiceId {
			return gen.GetService200JSONResponse(s.unpolledService(svc.ID, svc.Name)), nil
		}
	}
	return nil, errNotFound.withDetail("no service with id %q", request.ServiceId)
}

func (s *Server) unpolledService(id, name string) gen.Service {
	return gen.Service{
		Id:           id,
		Name:         name,
		Capabilities: []gen.Capability{},
		Actions:      []gen.Action{},
		Health: gen.Health{
			Status:     gen.HealthStatusUnknown,
			Reachable:  false,
			ObservedAt: s.now(),
			Reasons: []gen.HealthReason{{
				Code:    "not_polled",
				Message: "No adapter is configured for this service yet.",
			}},
		},
	}
}

// InvokeServiceAction is reachable but has nothing to invoke: no service advertises
// actions until adapters land in M1.5, so every action id is genuinely unknown. A 404
// here is accurate rather than a stub.
func (s *Server) InvokeServiceAction(ctx context.Context, request gen.InvokeServiceActionRequestObject) (gen.InvokeServiceActionResponseObject, error) {
	for _, svc := range s.services {
		if svc.ID == request.ServiceId {
			return nil, errNotFound.withDetail(
				"service %q advertises no action %q", request.ServiceId, request.ActionId)
		}
	}
	return nil, errNotFound.withDetail("no service with id %q", request.ServiceId)
}

// StreamEvents is declared in the contract but not implemented.
//
// Deliberate: assumption A7 of the M0 spike — that an SSE stream survives a tailnet on
// mobile data — has not been tested. ADR-0004 marks the transport provisional and the
// contract carries x-provisional on this operation. Building it now would put the agent's
// primary read path on the one architectural assumption still unproven, which is exactly
// the mistake M0 exists to prevent.
//
// A7 runs immediately before M1.7, which replaces this.
func (s *Server) StreamEvents(ctx context.Context, _ gen.StreamEventsRequestObject) (gen.StreamEventsResponseObject, error) {
	return nil, errNotImplemented.withDetail(
		"the event stream lands in M1.7, after its transport is validated on a real mobile network")
}

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
