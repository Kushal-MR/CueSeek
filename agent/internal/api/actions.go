package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Action lifecycle states, matching the contract's ActionStatus enum.
type actionState string

const (
	actionPending   actionState = "pending"
	actionRunning   actionState = "running"
	actionSucceeded actionState = "succeeded"
	actionFailed    actionState = "failed"
)

// trackedAction is one invocation.
type trackedAction struct {
	ID          string
	ServiceID   string
	ActionID    string
	State       actionState
	AcceptedAt  time.Time
	CompletedAt time.Time
	Err         string
}

// terminal reports whether the action has finished, either way.
func (a trackedAction) terminal() bool {
	return a.State == actionSucceeded || a.State == actionFailed
}

// actionTracker remembers in-flight and recently finished actions.
//
// In memory rather than in SQLite, deliberately. An action id correlates an HTTP response
// with a later stream event, and both live inside one agent process — an agent that
// restarts has nothing useful to say about a restart that was in flight when it died, and
// would be reporting on a job whose systemd JobRemoved signal it can no longer receive.
//
// The permanent record is the audit log, which is in the database. This is the live view.
type actionTracker struct {
	mu      sync.Mutex
	actions map[string]*trackedAction

	// retain is how long a finished action stays queryable, so a client that reconnects
	// shortly after can still learn how its request ended.
	retain time.Duration
	now    func() time.Time
}

const defaultActionRetention = 10 * time.Minute

func newActionTracker(now func() time.Time) *actionTracker {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &actionTracker{
		actions: make(map[string]*trackedAction),
		retain:  defaultActionRetention,
		now:     now,
	}
}

// begin records a newly accepted action.
//
// Returns false when an identical action is already running on that service. The contract
// describes 409 as "not currently possible, e.g. already in progress", and queuing a
// second restart behind the first is never what the person tapping twice wanted.
func (t *actionTracker) begin(serviceID, actionID string) (*trackedAction, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	t.pruneLocked(now)

	for _, existing := range t.actions {
		if existing.ServiceID == serviceID && existing.ActionID == actionID && !existing.terminal() {
			return existing, false
		}
	}

	id, err := newActionID()
	if err != nil {
		// crypto/rand failing is not a condition worth a second code path; the caller
		// turns this into a 500.
		return nil, false
	}

	action := &trackedAction{
		ID:         id,
		ServiceID:  serviceID,
		ActionID:   actionID,
		State:      actionPending,
		AcceptedAt: now,
	}
	t.actions[id] = action
	return action, true
}

// markRunning records that the underlying job actually started.
func (t *actionTracker) markRunning(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if action, ok := t.actions[id]; ok && !action.terminal() {
		action.State = actionRunning
	}
}

// finish records a terminal outcome.
func (t *actionTracker) finish(id string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	action, ok := t.actions[id]
	if !ok {
		return
	}
	action.CompletedAt = t.now()
	if err != nil {
		action.State = actionFailed
		action.Err = err.Error()
		return
	}
	action.State = actionSucceeded
}

// get returns a copy of a tracked action.
func (t *actionTracker) get(id string) (trackedAction, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	action, ok := t.actions[id]
	if !ok {
		return trackedAction{}, false
	}
	return *action, true
}

// pruneLocked drops finished actions past their retention window.
//
// Without it the map grows for the life of the process — slowly, but an operations agent
// is expected to run for months.
func (t *actionTracker) pruneLocked(now time.Time) {
	for id, action := range t.actions {
		if action.terminal() && now.Sub(action.CompletedAt) > t.retain {
			delete(t.actions, id)
		}
	}
}

// newActionID returns an opaque identifier.
//
// Random rather than sequential: the id appears in URLs and stream events, and a
// predictable counter would leak how many actions the agent has performed.
func newActionID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate action id: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
