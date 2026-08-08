package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/health"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// APIVersion is the contract version this build implements. Reported by GET /v1/system so
// a client can say honestly which side of a version skew is behind (ADR-0007).
const APIVersion = "0.1.0"

// Timeouts. Without them a stalled or malicious client holds a connection indefinitely,
// and enough of those exhaust the server without any traffic that looks like an attack.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 10 * time.Second
)

// Pairing attempts allowed per source address per window. Ten is generous for a human
// typing a code and negligible against 2^40 possibilities.
const (
	pairAttemptLimit  = 10
	pairAttemptWindow = time.Minute
)

// actionInvokeTimeout bounds how long the agent waits for an action to reach a terminal
// state before giving up on it.
//
// Generous: a systemd restart of a large media server can take a while, and abandoning a
// job that is still progressing would leave the action recorded as failed when it
// actually succeeded.
const actionInvokeTimeout = 5 * time.Minute

// actionDrainTimeout bounds how long shutdown waits for in-flight action bookkeeping.
//
// Much shorter than actionInvokeTimeout: systemd's default TimeoutStopSec is 90 seconds,
// and a graceful stop that blocked for five minutes would be escalated to SIGKILL long
// before it finished waiting.
const actionDrainTimeout = 5 * time.Second

// Server is the HTTP boundary: it implements gen.StrictServerInterface and owns the
// listener.
type Server struct {
	store *store.Store

	// registry and cache are the read path. Every services response is assembled from
	// these two and never from a live upstream call (ADR-0003).
	registry *adapters.Registry
	cache    *adapters.Cache

	actions *actionTracker
	// background tracks detached goroutines resolving action outcomes, so shutdown can
	// wait for them instead of killing an in-flight restart's bookkeeping.
	background sync.WaitGroup

	requirements requirements
	pairLimiter  *rateLimiter

	hostID       string
	hostname     string
	agentVersion string
	apiVersion   string
	startedAt    time.Time

	// now is injectable so tests can assert on timestamps without racing the clock.
	now func() time.Time

	httpServer *http.Server
}

// Options configures a Server. Everything not set falls back to a sensible default.
type Options struct {
	Store *store.Store

	// Registry holds the built adapters. An empty registry is valid — an agent managing
	// no services still pairs devices and reports system information.
	Registry *adapters.Registry

	// Cache is what the poller fills. Required whenever Registry has services in it.
	Cache *adapters.Cache

	AgentVersion string
	HostID       string
	Now          func() time.Time
}

// New builds a Server.
//
// Fails rather than degrading if the authorization table cannot be built from the
// embedded contract. An agent that starts without knowing what each endpoint requires is
// an agent serving endpoints whose authorisation nobody decided.
func New(opts Options) (*Server, error) {
	if opts.Store == nil {
		return nil, errors.New("api: Store is required")
	}

	reqs, err := loadRequirements()
	if err != nil {
		return nil, fmt.Errorf("api: build authorization table: %w", err)
	}

	now := opts.Now
	if now == nil {
		now = defaultNow
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	agentVersion := opts.AgentVersion
	if agentVersion == "" {
		agentVersion = "0.0.0-dev"
	}

	// Both default to empty rather than being required, so a Server can be constructed
	// before any adapter exists — and so a nil here is never a panic at request time.
	registry := opts.Registry
	if registry == nil {
		registry = adapters.NewRegistry()
	}
	cache := opts.Cache
	if cache == nil {
		cache = adapters.NewCache()
	}

	return &Server{
		store:        opts.Store,
		registry:     registry,
		cache:        cache,
		actions:      newActionTracker(now),
		requirements: reqs,
		pairLimiter:  newRateLimiter(pairAttemptLimit, pairAttemptWindow),
		hostID:       opts.HostID,
		hostname:     hostname,
		agentVersion: agentVersion,
		apiVersion:   APIVersion,
		startedAt:    now(),
		now:          now,
	}, nil
}

// OverallHealth is the agent's single answer for the whole host, computed from every
// service's cached state (ADR-0008).
//
// Exposed because M1.7's stream snapshot needs it, and because computing it in one place
// is the entire point — two callers deriving it separately would be the client-side
// divergence this decision exists to prevent.
func (s *Server) OverallHealth() domain.Health {
	services := s.registry.Services()
	inputs := make([]health.ServiceHealth, 0, len(services))
	for _, svc := range services {
		snapshot, ok := s.cache.Get(svc.ID())
		if !ok {
			continue
		}
		inputs = append(inputs, health.ServiceHealth{
			ServiceID: svc.ID(), Name: svc.Name(), Health: snapshot.Health,
		})
	}
	return health.Overall(inputs, s.now())
}

// waitForActions blocks until every detached action-resolution goroutine has finished,
// or until a bounded grace period expires.
//
// Bounded because an action's own deadline is generous — a large media server can take a
// while to restart — and a shutdown must not inherit that budget. If the wait expires,
// the outcome is simply never recorded, which is the same position the agent would be in
// had it crashed.
func (s *Server) waitForActions() {
	done := make(chan struct{})
	go func() {
		s.background.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(actionDrainTimeout):
		slog.Warn("shutting down with actions still in flight; " +
			"their outcomes will not be recorded")
	}
}

// Handler assembles the full middleware chain and returns the root http.Handler.
//
// Order matters and reads outside-in:
//
//	authenticate      resolves credentials, rejects nothing            (who?)
//	generated router  matches path+method, parses parameters
//	authorize         enforces the contract's x-required-scope         (may they?)
//	handler           business logic, no permission checks
//
// Authentication runs outside the router because it needs no knowledge of which operation
// was matched. Authorization runs inside it because it needs exactly that.
func (s *Server) Handler() http.Handler {
	strict := gen.NewStrictHandlerWithOptions(s,
		[]gen.StrictMiddlewareFunc{s.authorize},
		gen.StrictHTTPServerOptions{
			// Both default to plain text, which would make these the only responses in
			// the API not shaped like every other.
			RequestErrorHandlerFunc:  writeBadRequest,
			ResponseErrorHandlerFunc: writeProblem,
		})

	return gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		Middlewares:      []gen.MiddlewareFunc{s.authenticate},
		ErrorHandlerFunc: writeBadRequest,
	})
}

// ListenAndServe binds and serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) ListenAndServe(ctx context.Context, bind config.Bind) error {
	listener, err := s.Listen(bind)
	if err != nil {
		return err
	}
	return s.Serve(ctx, listener)
}

// Listen binds the configured address.
//
// Separate from Serve so that a failure to claim the port is reported before anything
// else starts, and so tests can bind an ephemeral port and discover which one they got.
func (s *Server) Listen(bind config.Bind) (net.Listener, error) {
	listener, err := net.Listen("tcp", bind.Address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", bind.Address, err)
	}

	if bind.AllowUnrestricted {
		// config.Validate already refused this unless explicitly opted into. Saying so at
		// every start means a machine left in that state cannot be quietly forgotten.
		slog.Warn("listening on ALL network interfaces",
			"address", bind.Address,
			"warning", "CueSeek can power off this machine and terminates no TLS; "+
				"it must not be reachable from untrusted networks (ADR-0001)")
	}
	return listener, nil
}

// Serve handles requests on listener until ctx is cancelled, then drains gracefully.
//
// Note for M1.7: Shutdown waits for in-flight requests, and an SSE stream is an in-flight
// request that never ends on its own. The event stream handler must register with
// httpServer.RegisterOnShutdown so it closes when draining begins — otherwise every
// connected client holds shutdown open until the grace period expires and connections are
// forcibly closed, turning a clean stop into a 10-second stall on every restart.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	// serveCtx is the parent of every request context, and it is deliberately NOT
	// derived from ctx.
	//
	// Deriving it from the signal context looks harmless and defeats the entire point of
	// graceful shutdown: SIGTERM would cancel every in-flight request's context at the
	// same moment Shutdown starts politely waiting for those requests to finish. A
	// request that arrived a millisecond earlier would see context.Canceled from the
	// database and fail, while the logs still reported a clean shutdown. The failure is
	// invisible unless a request happens to be in flight.
	//
	// Rooted at Background, in-flight requests run to completion during the drain window.
	// They are cancelled only when their connection closes — which is what Close() below
	// does if the drain deadline expires.
	serveCtx, cancelServe := context.WithCancel(context.Background())
	defer cancelServe()

	s.httpServer = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		BaseContext:       func(net.Listener) context.Context { return serveCtx },
	}

	slog.Info("api listening",
		"address", listener.Addr().String(),
		"agent_version", s.agentVersion,
		"api_version", s.apiVersion,
		"services", len(s.registry.IDs()))

	errCh := make(chan error, 1)
	go func() {
		err := s.httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			// Expected: Shutdown or Close was called deliberately.
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		// Serve stopped on its own — the listener broke. Not a shutdown.
		return err

	case <-ctx.Done():
		slog.Info("shutdown signal received, draining", "grace_period", shutdownTimeout)

		// Background, not ctx: ctx is already cancelled, so deriving the drain deadline
		// from it would expire instantly and skip the drain entirely.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		shutdownErr := s.httpServer.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			// Requests were still running when the grace period expired. Close forces
			// their connections shut so the process can exit rather than hanging until
			// systemd escalates to SIGKILL.
			slog.Warn("grace period expired with requests still in flight; closing connections",
				"error", shutdownErr)
			if err := s.httpServer.Close(); err != nil {
				slog.Error("force close failed", "error", err)
			}
		}

		cancelServe() // release anything still holding a request context
		serveErr := <-errCh

		// Actions outlive the request that started them, so they also outlive the drain.
		// Waiting here means a restart accepted moments before shutdown still gets its
		// outcome written to the audit log instead of vanishing.
		s.waitForActions()

		if shutdownErr != nil {
			return fmt.Errorf("graceful shutdown incomplete: %w", shutdownErr)
		}
		if serveErr != nil {
			return serveErr
		}
		slog.Info("api stopped cleanly")
		return nil
	}
}
