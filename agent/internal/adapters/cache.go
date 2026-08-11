package adapters

import (
	"fmt"
	"sync"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// Snapshot is the agent's current belief about one service.
type Snapshot struct {
	ServiceID string
	Health    domain.Health

	// Actions are those that applied at ObservedAt.
	//
	// Cached with the health rather than recomputed per request for two reasons: reading
	// unit state is a D-Bus call, and the list a client is shown must be the same list
	// the agent validates an invocation against. Deriving them separately would let a UI
	// offer Start while the API rejected it.
	Actions []domain.Action

	// StatusSince is when the current status was first observed.
	//
	// Carried so reasons can say "unreachable for 4 minutes" rather than just
	// "unreachable" — ADR-0008's example of the difference between a status and an
	// actionable one. Updated only when the status changes, not on every poll.
	StatusSince time.Time
}

// Cache holds the last observation for each service.
//
// Clients read from here; nothing they do triggers an upstream call (ADR-0003). That is
// what stops a wedged qBittorrent from hanging the dashboard, and what stops a watch
// glance from fanning out one request per service.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry

	// now is injectable so staleness tests advance a clock instead of sleeping.
	now func() time.Time

	// observer is notified after every Put, so the event stream can emit a delta the
	// moment state changes rather than polling this cache on its own timer.
	//
	// One observer, not a list: there is exactly one stream hub, and a second
	// broadcaster layered on this one would be a second place for fan-out bugs to live.
	observer func(Snapshot)
}

type cacheEntry struct {
	snapshot Snapshot
	observed bool
	// staleAfter is how long an observation stays trustworthy for this service.
	// Per-service because poll intervals differ: 5 seconds is stale for a service polled
	// every 2 seconds and fresh for one polled every minute.
	staleAfter time.Duration
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{entries: make(map[string]*cacheEntry), now: time.Now}
}

// Track registers a service before any observation exists.
//
// Without this, an unpolled service would simply be absent from the cache, and absence is
// ambiguous: it could mean "not configured" or "not yet polled". Registering up front
// makes the answer explicit and makes `unknown` the state a service starts in rather than
// a gap in a map (ADR-0008).
func (c *Cache) Track(serviceID string, staleAfter time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}
	c.entries[serviceID] = &cacheEntry{staleAfter: staleAfter}
}

// SetObserver registers a callback invoked after every Put.
//
// The callback must not block: it runs on the polling goroutine, so a slow observer would
// delay the next poll of that service.
func (c *Cache) SetObserver(fn func(Snapshot)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.observer = fn
}

// Put records a fresh observation.
func (c *Cache) Put(serviceID string, health domain.Health, actions []domain.Action) {
	snapshot, observer := c.put(serviceID, health, actions)

	// Called outside the lock. Notifying while holding it would let an observer that
	// touches this cache — which the stream's own snapshot builder does — deadlock
	// against the poller that triggered it.
	if observer != nil {
		observer(snapshot)
	}
}

func (c *Cache) put(
	serviceID string,
	health domain.Health,
	actions []domain.Action,
) (Snapshot, func(Snapshot)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, tracked := c.entries[serviceID]
	if !tracked {
		entry = &cacheEntry{staleAfter: DefaultStaleAfter}
		c.entries[serviceID] = entry
	}

	if health.ObservedAt.IsZero() {
		health.ObservedAt = c.now().UTC()
	}

	// StatusSince only moves when the status itself changes, so "how long has it been
	// like this" survives repeated identical polls.
	statusSince := entry.snapshot.StatusSince
	if !entry.observed || entry.snapshot.Health.Status != health.Status {
		statusSince = health.ObservedAt
	}

	entry.snapshot = Snapshot{
		ServiceID:   serviceID,
		Health:      health,
		Actions:     actions,
		StatusSince: statusSince,
	}
	entry.observed = true
	return entry.snapshot, c.observer
}

// Get returns the current belief about a service, downgrading it to unknown if the
// observation has aged out.
//
// Staleness is computed here rather than stored, because a stored status silently becomes
// wrong with the passage of time and nothing wakes up to correct it. Deriving it on read
// means a cache nobody is updating reports `unknown` by itself — which is exactly what
// should happen if the poller has died (ADR-0008).
func (c *Cache) Get(serviceID string) (Snapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, tracked := c.entries[serviceID]
	if !tracked {
		return Snapshot{}, false
	}
	now := c.now().UTC()

	if !entry.observed {
		return Snapshot{
			ServiceID: serviceID,
			Health: domain.UnknownHealth(now, domain.HealthReason{
				Code:    domain.ReasonNotPolled,
				Message: "No observation yet; the first poll has not completed.",
			}),
			StatusSince: now,
		}, true
	}

	snapshot := entry.snapshot
	if age := now.Sub(snapshot.Health.ObservedAt); age > entry.staleAfter {
		snapshot.Health = domain.UnknownHealth(snapshot.Health.ObservedAt, domain.HealthReason{
			Code: domain.ReasonStale,
			Message: fmt.Sprintf(
				"Last observed %s ago, which exceeds the %s tolerance for this service.",
				age.Round(time.Second), entry.staleAfter),
		})
	}
	return snapshot, true
}

// Snapshots returns the current belief about every tracked service.
//
// Order is unspecified: the registry owns presentation order, and duplicating that here
// would be a second place to get it wrong.
func (c *Cache) Snapshots() []Snapshot {
	c.mu.RLock()
	ids := make([]string, 0, len(c.entries))
	for id := range c.entries {
		ids = append(ids, id)
	}
	c.mu.RUnlock()

	out := make([]Snapshot, 0, len(ids))
	for _, id := range ids {
		if snapshot, ok := c.Get(id); ok {
			out = append(out, snapshot)
		}
	}
	return out
}
