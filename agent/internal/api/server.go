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
	"syscall"
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

// Listen retry, for a bind address that does not exist yet.
//
// The window is 90s because that is how long a cold Tailscale login takes in the worst
// case observed on the target host, and because it is bounded: a genuinely wrong
// bind.address must eventually surface as a startup failure rather than a process that
// waits forever. systemd's Restart=on-failure then retries the whole cycle, so a VPN that
// is down for ten minutes still recovers on its own.
//
// It does not collide with systemd's DefaultTimeoutStartSec. Type=exec considers the
// service started once execve succeeds, which happens long before this runs.
const (
	listenRetryInterval = 2 * time.Second
	listenRetryWindow   = 90 * time.Second
)

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
	refresh  Refresher

	actions *actionTracker
	// hub fans stream events out to every connected client.
	hub *hub
	// heartbeat is the stream's idle pulse interval. A field rather than a constant so
	// tests can shorten it without mutating global state and interfering with each other.
	heartbeat time.Duration
	// background tracks detached goroutines resolving action outcomes, so shutdown can
	// wait for them instead of killing an in-flight restart's bookkeeping.
	background sync.WaitGroup

	// hostMetrics is the most recent collection, or nil before the first one.
	//
	// Held here rather than in the adapter cache because it is not an adapter's: the
	// cache is keyed by service id and every entry in it has health and actions, none of
	// which a machine has in the same sense. Guarded by its own mutex so a collection
	// never contends with a service poll.
	hostMetrics   *domain.HostMetrics
	hostMetricsMu sync.RWMutex

	requirements requirements
	pairLimiter  *rateLimiter

	hostID       string
	hostname     string
	agentVersion string
	apiVersion   string
	startedAt    time.Time

	// now is injectable so tests can assert on timestamps without racing the clock.
	now func() time.Time

	// listen is injectable so the retry loop can be tested deterministically, without
	// needing a network interface to appear and disappear on the test machine.
	listen listenFunc
	// listenWindow and listenInterval are fields rather than constants so a test can
	// exercise the give-up path in milliseconds instead of ninety seconds.
	listenWindow   time.Duration
	listenInterval time.Duration

	httpServer *http.Server
}

// listenFunc matches net.ListenConfig.Listen.
type listenFunc func(ctx context.Context, network, address string) (net.Listener, error)

// Options configures a Server. Everything not set falls back to a sensible default.
type Options struct {
	Store *store.Store

	// Registry holds the built adapters. An empty registry is valid — an agent managing
	// no services still pairs devices and reports system information.
	Registry *adapters.Registry

	// Cache is what the poller fills. Required whenever Registry has services in it.
	Cache *adapters.Cache

	// Refresher lets a client, or a completed action, ask for an observation ahead of
	// the next tick (ADR-0003 Amendment 1). Optional: without one the agent still works,
	// it simply only ever polls on its own schedule.
	Refresher Refresher

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

	server := &Server{
		store:          opts.Store,
		registry:       registry,
		cache:          cache,
		refresh:        opts.Refresher,
		actions:        newActionTracker(now),
		hub:            newHub(),
		heartbeat:      streamHeartbeat,
		requirements:   reqs,
		pairLimiter:    newRateLimiter(pairAttemptLimit, pairAttemptWindow),
		hostID:         opts.HostID,
		hostname:       hostname,
		agentVersion:   agentVersion,
		apiVersion:     APIVersion,
		startedAt:      now(),
		now:            now,
		listen:         (&net.ListenConfig{}).Listen,
		listenWindow:   listenRetryWindow,
		listenInterval: listenRetryInterval,
	}

	// Deltas leave the agent the moment the poller records a change, rather than the
	// stream re-reading this cache on a timer of its own.
	cache.SetObserver(server.onServiceUpdate)

	return server, nil
}

// OverallHealth is the agent's single answer for the whole host, computed from every
// service's cached state (ADR-0008).
//
// Currently called only by its own test. It was written for M1.7's stream snapshot, but
// the contract's Snapshot schema carries `system` and `services` and no host-level health
// field, so the stream has nothing to put it in.
//
// Kept rather than deleted because ADR-0008 commits to the agent computing this, and M3
// adds host metrics — the input this is missing — at which point it needs a contract
// field and a caller. If M3 arrives and still has no use for it, delete it: an unused
// exported method that looks load-bearing is worse than no method.
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
	listener, err := s.Listen(ctx, bind)
	if err != nil {
		return err
	}
	return s.Serve(ctx, listener)
}

// Listen binds the configured address, waiting for it to appear if it has not yet.
//
// Separate from Serve so that a failure to claim the port is reported before anything
// else starts, and so tests can bind an ephemeral port and discover which one they got.
//
// # Why this waits
//
// ADR-0001 has the agent bind to a specific private-network address rather than to
// 0.0.0.0. On a VPN that address does not exist at boot: tailscaled creates the interface
// immediately but assigns the address only after it has authenticated, tens of seconds
// later. An unconditional bind therefore fails on a cold boot with EADDRNOTAVAIL.
//
// That is a normal condition, not a crash. Treating it as one — exiting and leaning on
// systemd's Restart=on-failure — logs a failure for something expected, and can exhaust
// StartLimitBurst if the VPN is slow enough. Ordering the unit after the VPN's service
// does not help either: it starts the daemon, not the address.
//
// Waiting here also keeps deploy/cueseekd.service generic. The alternative was an
// ExecStartPre polling loop, which would hard-code an interface name into a unit that
// should work for Tailscale, WireGuard or a plain LAN address alike.
func (s *Server) Listen(ctx context.Context, bind config.Bind) (net.Listener, error) {
	window, interval := s.listenWindow, s.listenInterval
	if window <= 0 {
		window = listenRetryWindow
	}
	if interval <= 0 {
		interval = listenRetryInterval
	}

	started := time.Now()
	deadline := started.Add(window)

	for attempt := 1; ; attempt++ {
		listener, err := s.listen(ctx, "tcp", bind.Address)
		if err == nil {
			if attempt > 1 {
				slog.Info("bind address became available",
					"address", bind.Address,
					"waited", time.Since(started).Round(time.Second),
					"attempts", attempt)
			}
			s.warnIfUnrestricted(bind)
			return listener, nil
		}

		// Only a missing address is worth waiting for. A port already in use, a
		// permission failure or a malformed address will never resolve on their own, and
		// retrying them would turn a clear startup error into a silent 90-second hang.
		if !isAddressUnavailable(err) {
			return nil, fmt.Errorf("listen on %s: %w", bind.Address, err)
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf(
				"listen on %s: address still not present after %s — is the VPN up, and is "+
					"bind.address correct for this host? %w",
				bind.Address, window, err)
		}

		if attempt == 1 {
			slog.Info("bind address is not present yet, waiting for it",
				"address", bind.Address,
				"giving_up_after", window,
				"hint", "normal at boot while a VPN interface is still coming up")
		}

		select {
		case <-ctx.Done():
			// Shutdown during the wait. Without this, SIGTERM at boot would be ignored
			// until the window expired and systemd would escalate to SIGKILL.
			//
			// Wrapped rather than returned bare: "context canceled" alone gives an
			// operator nothing to act on. errors.Is still matches through the wrap.
			return nil, fmt.Errorf("listen on %s: gave up waiting for the address: %w",
				bind.Address, ctx.Err())
		case <-time.After(interval):
		}
	}
}

func (s *Server) warnIfUnrestricted(bind config.Bind) {
	if !bind.AllowUnrestricted {
		return
	}
	// config.Validate already refused this unless explicitly opted into. Saying so at
	// every start means a machine left in that state cannot be quietly forgotten.
	slog.Warn("listening on ALL network interfaces",
		"address", bind.Address,
		"warning", "CueSeek can power off this machine and terminates no TLS; "+
			"it must not be reachable from untrusted networks (ADR-0001)")
}

// isAddressUnavailable reports whether a listen failed because the address is not
// present on this host yet.
//
// EADDRNOTAVAIL specifically. Deliberately not EADDRINUSE — a port conflict is a real
// misconfiguration that must surface immediately rather than after a long wait.
//
// The constant exists on every platform Go supports, so this compiles everywhere, but
// Windows reports WSAEADDRNOTAVAIL instead and will not match. That is acceptable: the
// agent deploys to Linux, and on a development machine the effect is simply that a bad
// bind fails fast rather than waiting.
func isAddressUnavailable(err error) bool {
	return errors.Is(err, syscall.EADDRNOTAVAIL)
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

	// An event stream is an in-flight request that never ends on its own, so Shutdown
	// would wait out its entire grace period on every restart. This closes every stream
	// the moment draining begins (ADR-0004, A7).
	s.httpServer.RegisterOnShutdown(s.hub.closeAll)

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

// Refresher triggers an out-of-band observation.
//
// An interface rather than a *adapters.Poller so this package keeps knowing nothing about
// how polling is implemented — and so a test can assert that a handler nudged without
// standing up a poller to watch.
//
// Both methods must return without waiting on an upstream service. That is the whole of
// ADR-0003 Amendment 1: the schedule may be nudged, but no request handler may block on
// the thing being observed.
type Refresher interface {
	PollNow(serviceID string)
	PollAllNow()
}

// nudge asks for an observation of one service, if a refresher was configured.
func (s *Server) nudge(serviceID string) {
	if s.refresh != nil {
		s.refresh.PollNow(serviceID)
	}
}
