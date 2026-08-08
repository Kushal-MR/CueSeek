package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// Authorization requirements are derived from the embedded contract at startup, not
// hardcoded here.
//
// api/openapi.yaml already declares x-required-scope on every operation, and a spec test
// fails the build if one is missing. Reading that same declaration at runtime means the
// documented rule and the enforced rule are the same bytes — they cannot drift, because
// there is only one of them.
//
// The alternative, a map[string]Scope maintained by hand, is one forgotten line away from
// an endpoint that authorises differently than it documents.

// requirement is what an operation demands of a caller.
type requirement struct {
	public bool // declared `security: []` in the contract
	scope  domain.Scope
}

// requirements maps an operation to its requirement, keyed by lowercased operation id.
//
// Case folding is deliberate. The generated strict middleware passes the Go method name
// ("ListDevices"), while the contract declares "listDevices". Matching case-insensitively
// keeps this working if the generator's naming convention ever changes — a mismatch would
// otherwise fail closed and break every endpoint at once.
type requirements map[string]requirement

func (r requirements) lookup(operationID string) (requirement, bool) {
	req, ok := r[strings.ToLower(operationID)]
	return req, ok
}

// loadRequirements reads the embedded contract and builds the authorization table.
//
// Fails rather than defaulting if an operation declares neither a scope nor public
// access. An endpoint whose authorisation nobody decided must not start; defaulting to
// open would be a silent hole, and defaulting to closed would produce a confusing
// permanent 403 that looks like a client bug.
func loadRequirements() (requirements, error) {
	spec, err := gen.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load embedded spec: %w", err)
	}

	reqs := make(requirements)
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			id := op.OperationID
			if id == "" {
				return nil, fmt.Errorf("%s %s has no operationId", method, path)
			}
			key := strings.ToLower(id)
			if _, dup := reqs[key]; dup {
				return nil, fmt.Errorf(
					"operationId %q collides with another when compared case-insensitively", id)
			}

			// security: [] overrides the document-level requirement, marking the
			// operation public.
			if op.Security != nil && len(*op.Security) == 0 {
				reqs[key] = requirement{public: true}
				continue
			}

			raw, ok := op.Extensions["x-required-scope"]
			if !ok {
				return nil, fmt.Errorf(
					"%s %s (%s) declares no x-required-scope and is not public; "+
						"refusing to start with an endpoint whose authorisation is undecided",
					method, path, id)
			}
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("%s: x-required-scope is %T, want string", id, raw)
			}
			scope, err := domain.ParseScope(text)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			reqs[key] = requirement{scope: scope}
		}
	}
	return reqs, nil
}

// authorize is the strict middleware that enforces requirements.
//
// It runs after the generated router has matched the request, so it knows the operation
// by name and needs no path matching of its own.
func (s *Server) authorize(f gen.StrictHandlerFunc, operationID string) gen.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		req, ok := s.requirements.lookup(operationID)
		if !ok {
			// The generated code routed to an operation the contract does not describe.
			// Impossible unless generated and embedded specs disagree; fail closed.
			slog.ErrorContext(ctx, "no authorization requirement for operation",
				"operation", operationID)
			return nil, errInternal
		}

		if req.public {
			return f(ctx, w, r, request)
		}

		device, err := deviceFromContext(ctx)
		if err != nil {
			return nil, err
		}

		if !device.HasScope(req.scope) {
			// Denied attempts are the most interesting rows in the audit log: they are
			// how an operator learns a device is being used beyond its remit.
			s.audit(ctx, device, "authz.denied", operationID, domain.OutcomeDenied,
				fmt.Sprintf("missing scope %s", req.scope))
			return nil, errForbidden.withDetail(
				"this operation requires the %q scope; this device holds %s",
				req.scope, formatScopes(device.Scopes))
		}

		return f(ctx, w, r, request)
	}
}

func formatScopes(scopes []domain.Scope) string {
	if len(scopes) == 0 {
		return "none"
	}
	parts := make([]string, len(scopes))
	for i, s := range scopes {
		parts[i] = string(s)
	}
	return strings.Join(parts, ", ")
}
