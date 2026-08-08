package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// Authentication answers "who is this?". Authorization — "may they do this?" — lives in
// authz.go.
//
// Splitting them is what lets POST /v1/pair be public without this layer knowing which
// operations are public. It resolves credentials if present, records the outcome, and
// never rejects anything. Only authorization, which knows the operation, decides.

type contextKey int

const (
	deviceContextKey contextKey = iota
	clientKeyContextKey
)

// clientKeyFromContext returns the caller's transport-level address, recorded by the
// authentication middleware.
//
// Handlers receive only a context, not the request, so anything they need about the
// connection has to be carried here. Returns "unknown" rather than an empty string so
// that a missing value cannot silently collapse every caller into one rate-limit bucket
// — which would be a shared bucket that any single client could exhaust for everyone.
func clientKeyFromContext(ctx context.Context) string {
	if key, ok := ctx.Value(clientKeyContextKey).(string); ok && key != "" {
		return key
	}
	return "unknown"
}

// authResult is what authentication puts in the context: either a device, or the reason
// there isn't one.
type authResult struct {
	device domain.Device
	err    error // nil when device is valid
}

var errNoCredentials = errors.New("no credentials presented")

// authenticate resolves a bearer token to a device and stores the outcome in the request
// context.
//
// It deliberately does not reject unauthenticated requests. A middleware that rejected
// here would have to carry a list of public paths, duplicating knowledge that already
// exists in the contract and drifting from it the first time an endpoint is added.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), clientKeyContextKey, clientKey(r))

		token, ok := bearerToken(r)
		if !ok {
			ctx = context.WithValue(ctx, deviceContextKey, authResult{err: errNoCredentials})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		device, err := s.store.AuthenticateToken(r.Context(), token)
		result := authResult{device: device, err: err}
		if err != nil && !errors.Is(err, store.ErrInvalidToken) {
			// A database failure is not an authentication failure. Reporting it as one
			// would tell a legitimate client its token was rejected and send it to
			// re-pair, when the real problem is the agent.
			result.err = err
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, deviceContextKey, result)))
	})
}

// bearerToken extracts the credential from an Authorization header.
//
// The scheme match is case-insensitive because RFC 7235 says it is; clients in the wild
// send "Bearer", "bearer" and occasionally "BEARER".
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	scheme, credentials, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "bearer") {
		return "", false
	}
	credentials = strings.TrimSpace(credentials)
	if credentials == "" {
		return "", false
	}
	return credentials, true
}

// deviceFromContext returns the authenticated device, or an *Error describing why there
// isn't one.
func deviceFromContext(ctx context.Context) (domain.Device, error) {
	result, ok := ctx.Value(deviceContextKey).(authResult)
	if !ok {
		// The authentication middleware did not run. That is a wiring bug, and treating
		// it as "unauthenticated" would hide it — every request would 401 and the cause
		// would look like a credential problem.
		return domain.Device{}, errInternal.withDetail("authentication middleware not installed")
	}
	switch {
	case result.err == nil:
		return result.device, nil
	case errors.Is(result.err, errNoCredentials), errors.Is(result.err, store.ErrInvalidToken):
		return domain.Device{}, errUnauthorized
	default:
		return domain.Device{}, errInternal
	}
}
