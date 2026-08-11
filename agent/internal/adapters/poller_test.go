package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// ---------------------------------------------------------------- cache

func TestCacheReportsUnknownBeforeFirstPoll(t *testing.T) {
	c := NewCache()
	c.Track("jellyfin", time.Minute)

	snapshot, ok := c.Get("jellyfin")
	if !ok {
		t.Fatal("tracked service is absent from the cache")
	}
	if snapshot.Health.Status != domain.StatusUnknown {
		t.Errorf("status = %q, want unknown before the first poll", snapshot.Health.Status)
	}
	if len(snapshot.Health.Reasons) == 0 ||
		snapshot.Health.Reasons[0].Code != domain.ReasonNotPolled {
		t.Errorf("reasons = %v, want not_polled", snapshot.Health.Reasons)
	}
	if snapshot.Health.Reachable {
		t.Error("an unpolled service reports reachable")
	}
}

func TestCacheGetUnknownService(t *testing.T) {
	c := NewCache()
	if _, ok := c.Get("never-tracked"); ok {
		t.Error("Get returned true for a service that was never tracked")
	}
}

func TestCacheStoresObservation(t *testing.T) {
	c := NewCache()
	c.Track("jellyfin", time.Minute)

	observed := time.Now().UTC()
	c.Put("jellyfin", domain.Health{
		Status: domain.StatusHealthy, Reachable: true, ObservedAt: observed,
	}, nil)

	snapshot, _ := c.Get("jellyfin")
	if snapshot.Health.Status != domain.StatusHealthy {
		t.Errorf("status = %q", snapshot.Health.Status)
	}
	if !snapshot.Health.ObservedAt.Equal(observed) {
		t.Errorf("ObservedAt = %v, want %v", snapshot.Health.ObservedAt, observed)
	}
	if snapshot.ServiceID != "jellyfin" {
		t.Errorf("ServiceID = %q", snapshot.ServiceID)
	}
}

// TestCacheDowngradesStaleStateToUnknown is ADR-0008's central claim made executable:
// showing stale green while the agent cannot reach a service is worse than showing
// nothing, because it is confidently wrong.
func TestCacheDowngradesStaleStateToUnknown(t *testing.T) {
	now := time.Now().UTC()
	c := NewCache()
	c.now = func() time.Time { return now }
	c.Track("jellyfin", 30*time.Second)

	c.Put("jellyfin", domain.Health{
		Status: domain.StatusHealthy, Reachable: true, ObservedAt: now,
	}, nil)

	if snapshot, _ := c.Get("jellyfin"); snapshot.Health.Status != domain.StatusHealthy {
		t.Fatalf("status = %q immediately after a poll, want healthy", snapshot.Health.Status)
	}

	// Inside tolerance.
	now = now.Add(20 * time.Second)
	if snapshot, _ := c.Get("jellyfin"); snapshot.Health.Status != domain.StatusHealthy {
		t.Errorf("status = %q at 20s of a 30s tolerance, want healthy", snapshot.Health.Status)
	}

	// Past tolerance: the stored value did not change, but what we report does.
	now = now.Add(15 * time.Second)
	snapshot, _ := c.Get("jellyfin")
	if snapshot.Health.Status != domain.StatusUnknown {
		t.Errorf("status = %q at 35s of a 30s tolerance, want unknown", snapshot.Health.Status)
	}
	if len(snapshot.Health.Reasons) == 0 || snapshot.Health.Reasons[0].Code != domain.ReasonStale {
		t.Errorf("reasons = %v, want stale", snapshot.Health.Reasons)
	}
	// The observation timestamp is preserved so a client can say how old the data is.
	if snapshot.Health.ObservedAt.IsZero() {
		t.Error("ObservedAt was cleared when the entry went stale")
	}
}

// TestCacheStatusSinceTracksTransitions: "unreachable for 4m" needs to know when the
// current status began, and repeated identical polls must not keep resetting it.
func TestCacheStatusSinceTracksTransitions(t *testing.T) {
	now := time.Now().UTC()
	c := NewCache()
	c.now = func() time.Time { return now }
	c.Track("jellyfin", time.Hour)

	c.Put("jellyfin", domain.Health{Status: domain.StatusHealthy, ObservedAt: now}, nil)
	first, _ := c.Get("jellyfin")

	now = now.Add(time.Minute)
	c.Put("jellyfin", domain.Health{Status: domain.StatusHealthy, ObservedAt: now}, nil)
	second, _ := c.Get("jellyfin")

	if !second.StatusSince.Equal(first.StatusSince) {
		t.Errorf("StatusSince moved on an unchanged status: %v -> %v",
			first.StatusSince, second.StatusSince)
	}

	now = now.Add(time.Minute)
	c.Put("jellyfin", domain.Health{Status: domain.StatusUnreachable, ObservedAt: now}, nil)
	third, _ := c.Get("jellyfin")

	if !third.StatusSince.Equal(now) {
		t.Errorf("StatusSince = %v on a status change, want %v", third.StatusSince, now)
	}
}

func TestCacheSnapshots(t *testing.T) {
	c := NewCache()
	c.Track("a", time.Minute)
	c.Track("b", time.Minute)
	c.Put("a", domain.Health{Status: domain.StatusHealthy, ObservedAt: time.Now().UTC()}, nil)

	snapshots := c.Snapshots()
	if len(snapshots) != 2 {
		t.Fatalf("Snapshots() returned %d, want 2 (including the unpolled one)", len(snapshots))
	}
	byID := map[string]Snapshot{}
	for _, s := range snapshots {
		byID[s.ServiceID] = s
	}
	if byID["a"].Health.Status != domain.StatusHealthy {
		t.Errorf("a = %q", byID["a"].Health.Status)
	}
	if byID["b"].Health.Status != domain.StatusUnknown {
		t.Errorf("b = %q, want unknown", byID["b"].Health.Status)
	}
}

// TestCacheIsConcurrencySafe: the cache is the one thing every polling goroutine shares,
// so it is the one place a data race could exist. Run with -race.
func TestCacheIsConcurrencySafe(t *testing.T) {
	c := NewCache()
	for _, id := range []string{"a", "b", "c"} {
		c.Track(id, time.Minute)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 100 {
				id := []string{"a", "b", "c"}[i%3]
				c.Put(id, domain.Health{Status: domain.StatusHealthy, ObservedAt: time.Now().UTC()}, nil)
				c.Get(id)
				c.Snapshots()
			}
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------- poller

func newTestPoller(t *testing.T, services map[string]*fakeService, interval, timeout time.Duration) *Poller {
	t.Helper()

	r := NewRegistry()
	_ = r.RegisterFactory("fake", func(cfg config.Service, _ Deps) (Service, error) {
		if svc, ok := services[cfg.ID]; ok {
			svc.id = cfg.ID
			return svc, nil
		}
		return &fakeService{id: cfg.ID}, nil
	})

	var cfgServices []config.Service
	for id := range services {
		cfgServices = append(cfgServices, config.Service{
			ID: id, Type: "fake", PollInterval: interval, Timeout: timeout,
		})
	}
	cfg := config.Config{Services: cfgServices}
	if err := r.Build(cfg, Deps{}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return NewPoller(r, cfg)
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestPollerPollsImmediatelyOnStart: waiting a full interval would leave a freshly
// started agent reporting unknown for thirty seconds when it could have real state in
// under one.
func TestPollerPollsImmediatelyOnStart(t *testing.T) {
	svc := &fakeService{}
	p := newTestPoller(t, map[string]*fakeService{"jellyfin": svc}, time.Hour, time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	waitFor(t, "the first poll", func() bool {
		snapshot, _ := p.Cache().Get("jellyfin")
		return snapshot.Health.Status == domain.StatusHealthy
	})

	cancel()
	p.Wait()
}

// TestPollerUpdatesCacheOnEachTick covers the cache-update requirement: a service that
// changes state must be reflected without restarting the agent.
func TestPollerUpdatesCacheOnEachTick(t *testing.T) {
	svc := &fakeService{}
	p := newTestPoller(t, map[string]*fakeService{"jellyfin": svc}, 20*time.Millisecond, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	waitFor(t, "the first healthy observation", func() bool {
		s, _ := p.Cache().Get("jellyfin")
		return s.Health.Status == domain.StatusHealthy
	})

	svc.set(domain.Health{Status: domain.StatusDegraded, Reachable: true}, nil)

	waitFor(t, "the degraded observation", func() bool {
		s, _ := p.Cache().Get("jellyfin")
		return s.Health.Status == domain.StatusDegraded
	})

	cancel()
	p.Wait()
}

// TestOneSlowServiceDoesNotBlockAnother is requirement 4's central claim.
//
// The slow service hangs until its per-request deadline expires on every poll. With a
// single loop walking a list, the fast service would be starved behind it; with one
// goroutine each, it is unaffected.
func TestOneSlowServiceDoesNotBlockAnother(t *testing.T) {
	slow := &fakeService{delay: time.Hour}
	fast := &fakeService{}

	p := newTestPoller(t, map[string]*fakeService{"slow": slow, "fast": fast},
		20*time.Millisecond, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	// The fast service polls repeatedly while the slow one is still stuck on its first.
	waitFor(t, "the fast service to poll several times", func() bool {
		return fast.callCount() >= 3
	})

	slowSnapshot, _ := p.Cache().Get("slow")
	if slowSnapshot.Health.Status == domain.StatusHealthy {
		t.Error("the hung service reported healthy")
	}

	fastSnapshot, _ := p.Cache().Get("fast")
	if fastSnapshot.Health.Status != domain.StatusHealthy {
		t.Errorf("fast service = %q; it was blocked by the slow one", fastSnapshot.Health.Status)
	}

	cancel()
	p.Wait()
}

// TestPollerAppliesPerRequestTimeout: the deadline reaches the adapter through its
// context, which is what lets an adapter abandon a hung request rather than the poller
// abandoning the adapter and leaking the goroutine.
func TestPollerAppliesPerRequestTimeout(t *testing.T) {
	svc := &fakeService{delay: time.Hour}
	p := newTestPoller(t, map[string]*fakeService{"slow": svc}, time.Hour, 20*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	started := time.Now()
	p.Start(ctx)

	waitFor(t, "the timeout to be recorded", func() bool {
		s, _ := p.Cache().Get("slow")
		return s.Health.Status == domain.StatusUnreachable
	})

	// Bounded by the timeout, not by the one-hour delay.
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("poll took %v; the per-request deadline was not applied", elapsed)
	}

	cancel()
	p.Wait()
}

// TestPollerRecordsAdapterErrorsAsUnreachable: an adapter that cannot form an opinion is
// not the same as a healthy service, and the cache must not keep the last success.
func TestPollerRecordsAdapterErrorsAsUnreachable(t *testing.T) {
	svc := &fakeService{}
	p := newTestPoller(t, map[string]*fakeService{"x": svc}, 20*time.Millisecond, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	waitFor(t, "a healthy observation", func() bool {
		s, _ := p.Cache().Get("x")
		return s.Health.Status == domain.StatusHealthy
	})

	svc.set(domain.Health{}, errors.New("connection refused"))

	waitFor(t, "the failure to be recorded", func() bool {
		s, _ := p.Cache().Get("x")
		return s.Health.Status == domain.StatusUnreachable
	})

	snapshot, _ := p.Cache().Get("x")
	if snapshot.Health.Reachable {
		t.Error("a failed poll reported reachable")
	}
	if len(snapshot.Health.Reasons) == 0 {
		t.Error("a failed poll recorded no reason")
	}

	cancel()
	p.Wait()
}

// TestPollerRejectsInvalidStatusFromAdapter: an adapter returning a status outside the
// closed set is a bug in that adapter, and passing it through would put an unrenderable
// value in front of every client.
func TestPollerRejectsInvalidStatusFromAdapter(t *testing.T) {
	svc := &fakeService{}
	svc.set(domain.Health{Status: "totally-fine", Reachable: true}, nil)

	p := newTestPoller(t, map[string]*fakeService{"x": svc}, time.Hour, time.Second)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p.Start(ctx)

	waitFor(t, "an observation", func() bool {
		s, _ := p.Cache().Get("x")
		return s.Health.Status != ""
	})

	snapshot, _ := p.Cache().Get("x")
	if snapshot.Health.Status != domain.StatusUnknown {
		t.Errorf("status = %q, want unknown for an unrecognised value", snapshot.Health.Status)
	}

	cancel()
	p.Wait()
}

// TestPollerStopsOnContextCancel: every goroutine must exit, or `systemctl stop` leaves
// the process alive until systemd escalates.
func TestPollerStopsOnContextCancel(t *testing.T) {
	p := newTestPoller(t, map[string]*fakeService{
		"a": {}, "b": {}, "c": {},
	}, 10*time.Millisecond, 5*time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())
	p.Start(ctx)

	waitFor(t, "polling to begin", func() bool {
		s, _ := p.Cache().Get("a")
		return s.Health.Status == domain.StatusHealthy
	})

	cancel()

	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("polling goroutines did not exit after cancellation")
	}
}

// TestTimeoutIsClampedBelowInterval: a request budget at or beyond the poll interval lets
// polls overlap and accumulate.
func TestTimeoutIsClampedBelowInterval(t *testing.T) {
	p := newTestPoller(t, map[string]*fakeService{"x": {}}, time.Second, 10*time.Second)

	settings := p.settings["x"]
	if settings.timeout >= settings.interval {
		t.Errorf("timeout %v is not shorter than interval %v", settings.timeout, settings.interval)
	}
}

func TestPollerAppliesDefaultsForUnsetValues(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterFactory("fake", fakeFactory)
	cfg := config.Config{Services: []config.Service{{ID: "x", Type: "fake"}}}
	if err := r.Build(cfg, Deps{}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	p := NewPoller(r, cfg)
	settings := p.settings["x"]
	if settings.interval != DefaultPollInterval {
		t.Errorf("interval = %v, want %v", settings.interval, DefaultPollInterval)
	}
	if settings.timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want %v", settings.timeout, DefaultTimeout)
	}
	// Tracked in the cache before any poll, so the service reports unknown rather than
	// being absent.
	if snapshot, ok := p.Cache().Get("x"); !ok || snapshot.Health.Status != domain.StatusUnknown {
		t.Errorf("service not tracked as unknown before polling: %+v, %v", snapshot, ok)
	}
}

func TestStartIsIdempotent(t *testing.T) {
	p := newTestPoller(t, map[string]*fakeService{"x": {}}, time.Hour, time.Second)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	p.Start(ctx)
	p.Start(ctx) // must not launch a second set of goroutines

	waitFor(t, "a poll", func() bool {
		s, _ := p.Cache().Get("x")
		return s.Health.Status == domain.StatusHealthy
	})

	cancel()
	done := make(chan struct{})
	go func() { p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return; Start probably launched duplicate goroutines")
	}
}
