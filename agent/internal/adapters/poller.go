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
	p.pollOnce(ctx, service, settings.timeout)

	ticker := time.NewTicker(settings.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollOnce(ctx, service, settings.timeout)
		}
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

		p.cache.Put(service.ID(), domain.Health{
			Status:     domain.StatusUnreachable,
			Reachable:  false,
			ObservedAt: time.Now().UTC(),
			Reasons: []domain.HealthReason{{
				Code:    domain.ReasonUnreachable,
				Message: "The agent could not determine this service's state: " + err.Error(),
			}},
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

	p.cache.Put(service.ID(), health)
}
