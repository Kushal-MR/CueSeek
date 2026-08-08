package api

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func newTestTracker(now *time.Time) *actionTracker {
	return newActionTracker(func() time.Time { return *now })
}

func TestActionTrackerLifecycle(t *testing.T) {
	now := time.Now().UTC()
	tracker := newTestTracker(&now)

	action, started := tracker.begin("jellyfin", "restart")
	if !started || action == nil {
		t.Fatal("begin refused the first action")
	}
	if action.ID == "" {
		t.Error("no action id was minted")
	}
	if action.State != actionPending || action.terminal() {
		t.Errorf("state = %q, want a non-terminal pending", action.State)
	}

	tracker.markRunning(action.ID)
	if got, _ := tracker.get(action.ID); got.State != actionRunning {
		t.Errorf("state = %q, want running", got.State)
	}

	tracker.finish(action.ID, nil)
	got, ok := tracker.get(action.ID)
	if !ok {
		t.Fatal("action disappeared after finishing")
	}
	if got.State != actionSucceeded || !got.terminal() {
		t.Errorf("state = %q, want a terminal succeeded", got.State)
	}
	if got.CompletedAt.IsZero() {
		t.Error("CompletedAt was not recorded")
	}
}

func TestActionTrackerRecordsFailure(t *testing.T) {
	now := time.Now().UTC()
	tracker := newTestTracker(&now)

	action, _ := tracker.begin("jellyfin", "restart")
	tracker.finish(action.ID, errors.New("systemd reported \"failed\""))

	got, _ := tracker.get(action.ID)
	if got.State != actionFailed {
		t.Errorf("state = %q, want failed", got.State)
	}
	if got.Err == "" {
		t.Error("the failure reason was not kept")
	}
}

// TestDuplicateActionRefused is the contract's 409 condition, at the level that decides
// it. Queuing a second restart behind the first is never what a double-tap meant.
func TestDuplicateActionRefused(t *testing.T) {
	now := time.Now().UTC()
	tracker := newTestTracker(&now)

	first, started := tracker.begin("jellyfin", "restart")
	if !started {
		t.Fatal("first action refused")
	}

	existing, started := tracker.begin("jellyfin", "restart")
	if started {
		t.Error("a duplicate action was accepted while the first was still running")
	}
	if existing == nil || existing.ID != first.ID {
		t.Error("the refusal did not identify the action already in flight")
	}

	// A different action on the same service is not a duplicate.
	if _, started := tracker.begin("jellyfin", "reload"); !started {
		t.Error("a different action on the same service was refused")
	}
	// Nor is the same action on a different service.
	if _, started := tracker.begin("qbittorrent", "restart"); !started {
		t.Error("the same action on a different service was refused")
	}
}

// TestFinishedActionCanRunAgain: the block is on concurrency, not on repetition.
func TestFinishedActionCanRunAgain(t *testing.T) {
	now := time.Now().UTC()
	tracker := newTestTracker(&now)

	first, _ := tracker.begin("jellyfin", "restart")
	tracker.finish(first.ID, nil)

	second, started := tracker.begin("jellyfin", "restart")
	if !started {
		t.Fatal("a finished action blocked a new one")
	}
	if second.ID == first.ID {
		t.Error("the second invocation reused the first action id")
	}
}

// TestFinishedActionsArePruned: the map would otherwise grow for the life of a process
// that is expected to run for months.
func TestFinishedActionsArePruned(t *testing.T) {
	now := time.Now().UTC()
	tracker := newTestTracker(&now)

	action, _ := tracker.begin("jellyfin", "restart")
	tracker.finish(action.ID, nil)

	// Still queryable inside the retention window, so a client that reconnects shortly
	// after can learn how its request ended.
	now = now.Add(tracker.retain / 2)
	if _, ok := tracker.get(action.ID); !ok {
		t.Error("action was dropped while still within its retention window")
	}

	now = now.Add(tracker.retain * 2)
	tracker.begin("other", "restart") // pruning happens on the next begin
	if _, ok := tracker.get(action.ID); ok {
		t.Error("a long-finished action was not pruned")
	}
}

// TestInFlightActionsAreNeverPruned: an action that never completes must not be silently
// forgotten, or a repeat invocation would slip past the duplicate check.
func TestInFlightActionsAreNeverPruned(t *testing.T) {
	now := time.Now().UTC()
	tracker := newTestTracker(&now)

	action, _ := tracker.begin("jellyfin", "restart")
	tracker.markRunning(action.ID)

	now = now.Add(24 * time.Hour)
	tracker.begin("other", "restart")

	if _, ok := tracker.get(action.ID); !ok {
		t.Error("an action still in flight was pruned")
	}
}

func TestActionIDsAreUniqueAndOpaque(t *testing.T) {
	now := time.Now().UTC()
	tracker := newTestTracker(&now)

	seen := make(map[string]bool)
	for i := range 200 {
		action, started := tracker.begin("service", "action")
		if !started {
			// Duplicate guard: finish it so the next iteration may begin.
			tracker.finish(action.ID, nil)
			continue
		}
		if seen[action.ID] {
			t.Fatalf("duplicate action id at iteration %d", i)
		}
		seen[action.ID] = true
		tracker.finish(action.ID, nil)
	}
	if len(seen) < 100 {
		t.Fatalf("only %d ids generated", len(seen))
	}
	for id := range seen {
		// Random, not sequential: a counter in a URL would leak how many actions the
		// agent has performed.
		if len(id) != 16 {
			t.Errorf("id %q is not a 16-character hex value", id)
		}
	}
}

// TestActionTrackerIsConcurrencySafe: actions are begun on request goroutines and
// finished on detached ones. Run with -race.
func TestActionTrackerIsConcurrencySafe(t *testing.T) {
	now := time.Now().UTC()
	tracker := newTestTracker(&now)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for range 50 {
				action, started := tracker.begin("service", "action")
				if !started || action == nil {
					continue
				}
				tracker.markRunning(action.ID)
				tracker.get(action.ID)
				tracker.finish(action.ID, nil)
			}
		}(i)
	}
	wg.Wait()
}

func TestGetUnknownAction(t *testing.T) {
	now := time.Now().UTC()
	tracker := newTestTracker(&now)

	if _, ok := tracker.get("nope"); ok {
		t.Error("get returned true for an unknown action id")
	}
	// Operations on unknown ids are no-ops rather than panics: the detached goroutine
	// that finishes an action may outlive a prune.
	tracker.markRunning("nope")
	tracker.finish("nope", nil)
}
