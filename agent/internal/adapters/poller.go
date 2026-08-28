package adapters

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// Polling defaults. Every one of these can be overridden per service in configuration;
// they exist so a minimal config still behaves sensibly.
const (
	// DefaultPollInterval matches config's default.
	DefaultPollInterval = 30 * time.Second

	// DefaultTimeout bounds a single upstream request.
	//
	// Comfortably shorter than any sane poll interval, so a hung service cannot cause
	// polls to pile up on top of each other.
	DefaultTimeout = 5 * time.Second

	// DefaultStaleAfter is how long an observation stays trustworthy when the service
	// does not say otherwise.
	DefaultStaleAfter = 3 * DefaultPollInterval

	// staleMultiplier derives per-service staleness from its poll interval.
	//
	// Three missed polls rather than one: a single skipped cycle is normal — a slow
	// response, a moment of scheduling jitter — and flipping the dashboard to `unknown`
	// every time one poll runs late would train the operator to ignore the state that
	// exists precisely to be trusted.
	staleMultiplier = 3

	// MinRefreshInterval is the shortest gap between a nudged poll and the one before it.
	//
	// The floor exists because a nudge is reachable from a gesture: without it, a client
	// holding pull-to-refresh could turn one finger into a request amplifier against
	// Jellyfin. Two seconds is longer than any local poll takes and far shorter than a
	// human notices, so the bound costs the honest case nothing (ADR-0003 Amendment 1).
	MinRefreshInterval = 2 * time.Second
)

// Poller keeps the cache current.
//
// One goroutine per service, each with its own ticker and its own request deadline.
// Nothing is shared between them except the cache, which is mutex-guarded — so a service
// that takes the full timeout on every poll delays only itself. That isolation is the
// whole reason polling is structured this way rather than as one loop walking a list
// (ADR-0003).
type Poller struct {
	registry *Registry
	cache    *Cache
	settings map[string]pollSettings

	// nudges carries out-of-band poll requests, one channel per service.
	//
	// Written at construction and only read afterwards, so no lock guards the map itself;
	// the channels are the synchronisation. Buffered by one and sent to without blocking,
	// which is what conflates a burst: while a poll is running the buffer holds exactly
	// one pending request no matter how many arrive.
	nudges map[string]chan struct{}

	wg      sync.WaitGroup
	started bool
	mu      sync.Mutex
}

type pollSettings struct {
	interval time.Duration
	timeout  time.Duration
}

// NewPoller prepares polling for every built service and pre-registers them in the cache
// as unknown.
func NewPoller(registry *Registry, cfg config.Config) *Poller {
	p := &Poller{
		registry: registry,
		cache:    NewCache(),
		settings: make(map[string]pollSettings),
		nudges:   make(map[string]chan struct{}),
	}

	byID := make(map[string]config.Service, len(cfg.Services))
	for _, s := range cfg.Services {
		byID[s.ID] = s
	}

	for _, id := range registry.IDs() {
		svcCfg := byID[id]

		interval := svcCfg.PollInterval
		if interval <= 0 {
			interval = DefaultPollInterval
		}
		timeout := svcCfg.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		// A timeout longer than the interval would let polls overlap indefinitely.
		if timeout >= interval {
			timeout = interval / 2
		}

		p.settings[id] = pollSettings{interval: interval, timeout: timeout}
		p.nudges[id] = make(chan struct{}, 1)
		p.cache.Track(id, time.Duration(staleMultiplier)*interval)
	}
	return p
}

// Cache exposes the state the API serves from.
func (p *Poller) Cache() *Cache { return p.cache }

// Start launches one goroutine per service and returns immediately.
//
// Each service is polled once straight away rather than after one interval, so a freshly
// started agent has real state within a second instead of showing `unknown` for the first
// thirty.
func (p *Poller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return
	}
	p.started = true

	for _, service := range p.registry.Services() {
		settings := p.settings[service.ID()]
		p.wg.Add(1)
		go func(service Service, settings pollSettings) {
			defer p.wg.Done()
			p.run(ctx, service, settings)
		}(service, settings)
	}

	slog.Info("adapter polling started", "services", len(p.registry.IDs()))
}

// Wait blocks until every polling goroutine has exited.
//
// Called after the context is cancelled so shutdown does not race a poll that is midway
// through writing to the cache.
func (p *Poller) Wait() { p.wg.Wait() }

func (p *Poller) run(ctx context.Context, service Service, settings pollSettings) {
	lastPoll := time.Now()
	p.pollOnce(ctx, service, settings.timeout)

	ticker := time.NewTicker(settings.interval)
	defer ticker.Stop()

	nudges := p.nudges[service.ID()]

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			lastPoll = time.Now()
			p.pollOnce(ctx, service, settings.timeout)

		case <-nudges:
			// Rate-bounded, and silently: a dropped nudge is not an error the operator
			// needs to see. It means the answer being asked for is two seconds old, and
			// the honest response to that is to let the existing answer stand.
			if time.Since(lastPoll) < MinRefreshInterval {
				continue
			}
			// Deliberately does not reset the ticker. A nudge is an extra observation,
			// not a rescheduling: letting a gesture shift the regular cadence would make
			// the poll interval depend on how often someone opened the app.
			lastPoll = time.Now()
			p.pollOnce(ctx, service, settings.timeout)
		}
	}
}

// PollNow asks one service to be observed as soon as the bound allows.
//
// Never blocks and never fails: the caller is on a request path or an action's completion
// path, and neither may wait on an upstream service (ADR-0003 Amendment 1). An unknown id
// is ignored rather than reported, because every caller learned the id from the registry.
func (p *Poller) PollNow(serviceID string) {
	nudge, ok := p.nudges[serviceID]
	if !ok {
		return
	}
	select {
	case nudge <- struct{}{}:
	default:
		// One already pending. Two requests to look now are one observation.
	}
}

// PollAllNow nudges every service. What a client's manual refresh reaches.
func (p *Poller) PollAllNow() {
	for id := range p.nudges {
		p.PollNow(id)
	}
}

// pollOnce performs one observation and records it.
//
// Never returns an error: a failed poll is itself information, and the cache is where it
// belongs. Propagating it upward would leave the caller with nothing useful to do and the
// cache holding a stale success.
func (p *Poller) pollOnce(ctx context.Context, service Service, timeout time.Duration) {
	// Derived from the poller's context so shutdown cancels an in-flight poll, but with
	// its own deadline so one service's budget is its own.
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	started := time.Now().UTC()
	health, err := service.Health(pollCtx)

	// Read on the poll path so a client request never causes a D-Bus round trip
	// (ADR-0003), and cached with the health so the list a client is shown is the list
	// the API validates against.
	var actions []domain.Action
	if controllable, ok := service.(Controllable); ok {
		actions = controllable.Actions(pollCtx)
	}

	if err != nil {
		// The adapter could not form an opinion at all. Distinct from it reporting
		// "unreachable", which is an opinion.
		if ctx.Err() != nil {
			// Shutdown, not a service problem. Recording unknown here would leave a
			// misleading final state in a cache nobody will read again anyway.
			return
		}
		slog.Warn("adapter health check failed",
			"service", service.ID(), "error", err, "elapsed", time.Since(started))

		// Activity is deliberately not collected here. A service the agent cannot form
		// an opinion about has no activity worth reporting, and asking would spend
		// another timeout on something already known to be unresponsive.
		p.cache.Put(service.ID(), Observation{
			Health: domain.Health{
				Status:     domain.StatusUnreachable,
				Reachable:  false,
				ObservedAt: time.Now().UTC(),
				Reasons: []domain.HealthReason{{
					Code:    domain.ReasonUnreachable,
					Message: "The agent could not determine this service's state: " + err.Error(),
				}},
			},
			Actions: actions,
		})
		return
	}

	if health.ObservedAt.IsZero() {
		health.ObservedAt = time.Now().UTC()
	}
	if !health.Status.Valid() {
		// An adapter returning a status outside the closed set is a bug in that adapter.
		// Passing it through would put an unrenderable value in front of every client.
		slog.Error("adapter returned an invalid health status",
			"service", service.ID(), "status", health.Status)
		health = domain.UnknownHealth(health.ObservedAt, domain.HealthReason{
			Code:    domain.ReasonInvalidResponse,
			Message: "The adapter reported a status this agent does not recognise.",
		})
	}

	p.cache.Put(service.ID(), Observation{
		Health:     health,
		Actions:    actions,
		NowPlaying: p.nowPlaying(pollCtx, service),
		Transfers:  p.transfers(pollCtx, service),
	})
}

// nowPlaying and transfers read the activity capabilities, on the poll path.
//
// Both return nil rather than an error, and neither can change the service's health. A
// media server that answers /System/Info and then refuses /Sessions is **up**; reporting
// it as unhealthy would send an operator hunting an outage that is not happening. What is
// lost is the activity payload, and nil says exactly that — "not known" rather than
// "nothing is playing".
//
// They share the poll's deadline rather than taking their own. That is the point: the
// whole observation of one service is bounded by one budget, so a slow /Sessions delays
// this service's next poll and nothing else's.
func (p *Poller) nowPlaying(ctx context.Context, service Service) *domain.NowPlaying {
	provider, ok := service.(NowPlayingProvider)
	if !ok {
		return nil
	}
	playing, err := provider.NowPlaying(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("now_playing unavailable", "service", service.ID(), "error", err)
		}
		return nil
	}
	playing.Items = domain.Bounded(playing.Items)
	return &playing
}

func (p *Poller) transfers(ctx context.Context, service Service) *domain.Transfers {
	provider, ok := service.(TransferProvider)
	if !ok {
		return nil
	}
	moving, err := provider.Transfers(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("transfers unavailable", "service", service.ID(), "error", err)
		}
		return nil
	}
	moving.Items = domain.Bounded(moving.Items)
	return &moving
}
