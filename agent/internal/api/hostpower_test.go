package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// fakePower stands in for the host controller, so these tests exercise the handler without
// a D-Bus connection and — more to the point — without rebooting the machine running them.
type fakePower struct {
	mu       sync.Mutex
	calls    []string
	err      error
	disabled bool

	// invoked is closed on the first call, so a test can wait for the delayed goroutine
	// rather than sleeping and hoping.
	invoked chan struct{}
	once    sync.Once
}

func newFakePower() *fakePower { return &fakePower{invoked: make(chan struct{})} }

func (f *fakePower) PowerActions() []domain.Action {
	if f.disabled {
		return nil
	}
	return []domain.Action{
		{ID: host.ActionReboot, Label: "Restart machine", Description: "Comes back.", Risk: domain.RiskDestructive},
		{ID: host.ActionPowerOff, Label: "Shut down machine", Description: "Stays off.", Risk: domain.RiskDestructive},
	}
}

func (f *fakePower) InvokePower(_ context.Context, action string) error {
	f.mu.Lock()
	f.calls = append(f.calls, action)
	f.mu.Unlock()
	f.once.Do(func() { close(f.invoked) })
	return f.err
}

func (f *fakePower) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// waitForInvoke blocks until the delayed goroutine has called through.
func (f *fakePower) waitForInvoke(t *testing.T) {
	t.Helper()
	select {
	case <-f.invoked:
	case <-time.After(5 * time.Second):
		t.Fatal("the power action was never performed")
	}
}

// ---------------------------------------------------------------- listing

func TestHostActionsAreListedToAnyReader(t *testing.T) {
	env := newTestEnv(t)
	// A read-only token. What the agent *offers* is a property of the agent; what this
	// device may *do* is a property of its token, and a client that could not see the
	// list would be unable to tell "too old to power off" from "not allowed to".
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	resp := env.do(t, http.MethodGet, "/v1/host/actions", token, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var actions []gen.Action
	if err := json.NewDecoder(resp.Body).Decode(&actions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions = %+v, want reboot and power-off", actions)
	}
	for _, a := range actions {
		if a.Risk != gen.Destructive {
			t.Errorf("%s risk = %q, want destructive", a.Id, a.Risk)
		}
	}
}

func TestHostActionsEmptyWhenUnsupported(t *testing.T) {
	env := newTestEnv(t)
	env.power.disabled = true
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	resp := env.do(t, http.MethodGet, "/v1/host/actions", token, nil)
	defer resp.Body.Close()

	var actions []gen.Action
	if err := json.NewDecoder(resp.Body).Decode(&actions); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// An empty array, not null: "this machine offers none" rather than "unknown".
	if actions == nil || len(actions) != 0 {
		t.Errorf("actions = %+v, want an empty list", actions)
	}
}

// ---------------------------------------------------------------- scope

// TestPowerRequiresItsOwnScope is the control. A client's press-and-hold gesture is user
// experience; this holds for any client ever written, including one that skips the gesture.
func TestPowerRequiresItsOwnScope(t *testing.T) {
	env := newTestEnv(t)
	// service.control is enough to stop Jellyfin, and must not be enough to stop the
	// machine Jellyfin runs on.
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeServiceControl)

	resp := env.do(t, http.MethodPost, "/v1/host/actions/reboot", token, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if got := env.power.recorded(); len(got) != 0 {
		t.Errorf("a refused request still reached the host: %v", got)
	}
}

func TestPowerRejectsAnUnauthenticatedCaller(t *testing.T) {
	env := newTestEnv(t)

	resp := env.do(t, http.MethodPost, "/v1/host/actions/reboot", "", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// ---------------------------------------------------------------- invoking

// TestPowerAcknowledgesBeforeActing is the property this endpoint exists to get right. A
// handler that reboots and then writes its response never writes the response.
func TestPowerAcknowledgesBeforeActing(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeHostPower)

	resp := env.do(t, http.MethodPost, "/v1/host/actions/reboot", token, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}

	var accepted gen.HostActionAccepted
	if err := json.NewDecoder(resp.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if accepted.ActionId == "" {
		t.Error("no action id to correlate a failure with")
	}
	if accepted.Action != host.ActionReboot {
		t.Errorf("action = %q", accepted.Action)
	}
	if accepted.Status != gen.Pending {
		t.Errorf("status = %q, want pending — nothing has happened yet", accepted.Status)
	}

	// The response came first; the machine is touched afterwards.
	if got := env.power.recorded(); len(got) != 0 {
		t.Errorf("the host was touched before the client was answered: %v", got)
	}

	env.power.waitForInvoke(t)
	if got := env.power.recorded(); len(got) != 1 || got[0] != host.ActionReboot {
		t.Errorf("host calls = %v, want one reboot", got)
	}
}

func TestUnknownPowerActionIs404(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeHostPower)

	resp := env.do(t, http.MethodPost, "/v1/host/actions/self-destruct", token, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if got := env.power.recorded(); len(got) != 0 {
		t.Errorf("an unknown action reached the host: %v", got)
	}
}

// TestOnlyOnePowerActionInFlight stops two goroutines racing to end the same machine.
func TestOnlyOnePowerActionInFlight(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeHostPower)

	first := env.do(t, http.MethodPost, "/v1/host/actions/reboot", token, nil)
	first.Body.Close()
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first status = %d, want 202", first.StatusCode)
	}

	second := env.do(t, http.MethodPost, "/v1/host/actions/power-off", token, nil)
	defer second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Errorf("second status = %d, want 409", second.StatusCode)
	}

	env.power.waitForInvoke(t)
	if got := env.power.recorded(); len(got) != 1 {
		t.Errorf("host calls = %v, want exactly one", got)
	}
}

// ---------------------------------------------------------------- failure

// TestFailureReachesTheStream covers the only outcome a power action can report. A reboot
// that worked took the stream with it; this event means the machine is still here.
func TestFailureReachesTheStream(t *testing.T) {
	env := newTestEnv(t)
	env.power.err = host.ErrUnauthorized
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeHostPower)

	frames, _ := openStream(t, env, token)
	nextFrame(t, frames, "the snapshot")

	resp := env.do(t, http.MethodPost, "/v1/host/actions/power-off", token, nil)
	resp.Body.Close()

	event := nextFrame(t, frames, "a host_action_progress event")
	if event.name != string(gen.StreamEventTypeHostActionProgress) {
		t.Fatalf("event = %q, want host_action_progress", event.name)
	}

	progress := event.envelope.HostActionProgress
	if progress == nil {
		t.Fatal("the event carried no payload")
	}
	if progress.Status != gen.Failed {
		t.Errorf("status = %q, want failed", progress.Status)
	}
	if progress.Error == nil {
		t.Fatal("a failure with no explanation")
	}
	// The likeliest failure and the least self-explanatory: it means the polkit rule is
	// missing or does not cover this action, not that the device's token was wrong.
	if !strings.Contains(*progress.Error, "polkit") {
		t.Errorf("error does not point at the polkit rule: %q", *progress.Error)
	}
	if event.envelope.ActionProgress != nil {
		t.Error("a host failure was also delivered as a service action")
	}
}

// TestSnapshotCarriesHostActions means a client that never calls the REST endpoint still
// knows what this machine can be asked to do.
func TestSnapshotCarriesHostActions(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	frames, _ := openStream(t, env, token)
	first := nextFrame(t, frames, "the snapshot")

	actions := first.envelope.Snapshot.HostActions
	if actions == nil || len(*actions) != 2 {
		t.Fatalf("snapshot host_actions = %+v, want two", actions)
	}
}
