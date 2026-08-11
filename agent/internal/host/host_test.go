package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// These tests run on every platform, including Windows, because they exercise the policy
// layer rather than D-Bus. That is the point of splitting Controller from Backend: the
// allowlist is the most security-relevant rule in this package, and it would be
// untestable off a systemd host if it lived inside the Linux implementation.

// fakeBackend records what it was asked to do, so tests can assert that a refused
// operation never reached it at all — not merely that it returned an error.
type fakeBackend struct {
	restartCalls []string
	startCalls   []string
	stopCalls    []string
	stateCalls   []string
	closed       bool

	restartErr error
	stateErr   error
	state      UnitState
	results    chan string
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{results: make(chan string, 1)}
}

func (f *fakeBackend) StartUnit(_ context.Context, unit string) (*Job, error) {
	f.startCalls = append(f.startCalls, unit)
	if f.restartErr != nil {
		return nil, f.restartErr
	}
	return newJob(1, unit, f.results), nil
}

func (f *fakeBackend) StopUnit(_ context.Context, unit string) (*Job, error) {
	f.stopCalls = append(f.stopCalls, unit)
	if f.restartErr != nil {
		return nil, f.restartErr
	}
	return newJob(1, unit, f.results), nil
}

func (f *fakeBackend) Platform() string { return "fake" }

func (f *fakeBackend) Close() error {
	f.closed = true
	return nil
}

func (f *fakeBackend) UnitState(_ context.Context, unit string) (UnitState, error) {
	f.stateCalls = append(f.stateCalls, unit)
	if f.stateErr != nil {
		return UnitState{}, f.stateErr
	}
	state := f.state
	state.Name = unit
	return state, nil
}

func (f *fakeBackend) RestartUnit(_ context.Context, unit string) (*Job, error) {
	f.restartCalls = append(f.restartCalls, unit)
	if f.restartErr != nil {
		return nil, f.restartErr
	}
	return newJob(42, unit, f.results), nil
}

func newTestController(t *testing.T, units ...string) (*Controller, *fakeBackend) {
	t.Helper()
	if len(units) == 0 {
		units = []string{"jellyfin.service", "qbittorrent.service"}
	}
	backend := newFakeBackend()
	c, err := NewWithBackend(backend, units)
	if err != nil {
		t.Fatalf("NewWithBackend: %v", err)
	}
	return c, backend
}

// ---------------------------------------------------------------- allowlist

// TestRestartRefusesUnmanagedUnitBeforeBackend is the core security assertion of this
// package.
//
// It is not enough that an unlisted unit produces an error: ADR-0002 requires the agent
// to refuse before the request reaches the system bus, so that a misconfiguration cannot
// name an arbitrary unit to systemd. The fake backend records every call, so this asserts
// the absence of the call rather than the presence of an error.
func TestRestartRefusesUnmanagedUnitBeforeBackend(t *testing.T) {
	c, backend := newTestController(t)

	for _, unit := range []string{
		"ssh.service",                   // exists on the host, deliberately unlisted
		"cron.service",                  // ditto
		"jellyfin",                      // right service, missing suffix
		"jellyfin.service.evil",         // prefix attack
		"Jellyfin.service",              // systemd unit names are case-sensitive
		" jellyfin.service",             // leading whitespace
		"jellyfin.service ",             // trailing whitespace
		"",                              // empty
		"../../etc/passwd",              // path traversal shape
		"jellyfin.service\nssh.service", // injection shape
	} {
		t.Run(fmt.Sprintf("%q", unit), func(t *testing.T) {
			_, err := c.RestartUnit(t.Context(), unit)
			if !errors.Is(err, ErrUnitNotManaged) {
				t.Errorf("err = %v, want ErrUnitNotManaged", err)
			}
		})
	}

	if len(backend.restartCalls) != 0 {
		t.Errorf("backend was called %d times for unmanaged units: %v",
			len(backend.restartCalls), backend.restartCalls)
	}
}

func TestRestartAllowsManagedUnit(t *testing.T) {
	c, backend := newTestController(t)

	job, err := c.RestartUnit(t.Context(), "jellyfin.service")
	if err != nil {
		t.Fatalf("RestartUnit: %v", err)
	}
	if job.Unit != "jellyfin.service" {
		t.Errorf("job.Unit = %q", job.Unit)
	}
	if len(backend.restartCalls) != 1 || backend.restartCalls[0] != "jellyfin.service" {
		t.Errorf("backend calls = %v", backend.restartCalls)
	}
}

func TestUnitStateRefusesUnmanagedUnit(t *testing.T) {
	c, backend := newTestController(t)

	if _, err := c.UnitState(t.Context(), "ssh.service"); !errors.Is(err, ErrUnitNotManaged) {
		t.Errorf("err = %v, want ErrUnitNotManaged", err)
	}
	if len(backend.stateCalls) != 0 {
		t.Errorf("backend was consulted for an unmanaged unit: %v", backend.stateCalls)
	}
}

// TestErrorDoesNotLeakAllowlist: the error reaches an API client, so it must not let an
// unauthorised caller enumerate which units exist on the host by guessing.
func TestErrorDoesNotLeakAllowlist(t *testing.T) {
	c, _ := newTestController(t, "jellyfin.service", "qbittorrent.service", "secret-vpn.service")

	_, err := c.RestartUnit(t.Context(), "ssh.service")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, leaked := range []string{"jellyfin", "qbittorrent", "secret-vpn"} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("error names a managed unit (%q): %v", leaked, err)
		}
	}
}

// ---------------------------------------------------------------- construction

func TestNewWithBackendValidatesUnits(t *testing.T) {
	cases := map[string][]string{
		"empty name":     {"jellyfin.service", ""},
		"whitespace":     {"   "},
		"missing suffix": {"jellyfin"},
	}
	for name, units := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewWithBackend(newFakeBackend(), units); err == nil {
				t.Errorf("accepted invalid unit list %v", units)
			}
		})
	}

	if _, err := NewWithBackend(nil, []string{"jellyfin.service"}); err == nil {
		t.Error("accepted a nil backend")
	}
}

func TestManagedUnits(t *testing.T) {
	c, _ := newTestController(t, "jellyfin.service", "qbittorrent.service", "jellyfin.service")

	units := c.ManagedUnits()
	if len(units) != 2 {
		t.Fatalf("ManagedUnits() = %v, want duplicates collapsed", units)
	}
	// Configuration order is preserved, so a device list and a config file read the same.
	if units[0] != "jellyfin.service" || units[1] != "qbittorrent.service" {
		t.Errorf("order not preserved: %v", units)
	}

	// The returned slice is a copy: a caller must not be able to edit the allowlist.
	units[0] = "tampered.service"
	if c.ManagedUnits()[0] != "jellyfin.service" {
		t.Error("ManagedUnits returned a mutable view of the allowlist")
	}

	if !c.IsManaged("jellyfin.service") || c.IsManaged("ssh.service") {
		t.Error("IsManaged disagrees with the configured allowlist")
	}
}

// TestEmptyAllowlistRefusesEverything: an agent configured with no services must control
// nothing, rather than defaulting to unrestricted.
func TestEmptyAllowlistRefusesEverything(t *testing.T) {
	backend := newFakeBackend()
	c, err := NewWithBackend(backend, nil)
	if err != nil {
		t.Fatalf("NewWithBackend: %v", err)
	}
	if _, err := c.RestartUnit(t.Context(), "jellyfin.service"); !errors.Is(err, ErrUnitNotManaged) {
		t.Errorf("err = %v, want ErrUnitNotManaged", err)
	}
	if len(backend.restartCalls) != 0 {
		t.Error("backend was called with an empty allowlist")
	}
}

func TestCloseClosesBackend(t *testing.T) {
	c, backend := newTestController(t)
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !backend.closed {
		t.Error("backend was not closed")
	}
}

// ---------------------------------------------------------------- error mapping

// TestClassifyErrorMapsAuthorizationFailures covers the M0 finding that made this
// necessary: polkit refuses a daemon with "Interactive authentication required", not
// "Access denied". Classifying that as an unexpected failure turns a clean authorisation
// result into a 500 — a mistake the spike's own probe script made.
func TestClassifyErrorMapsAuthorizationFailures(t *testing.T) {
	cases := map[string]struct {
		dbusName string
		message  string
	}{
		"polkit interactive, by name": {
			"org.freedesktop.DBus.Error.InteractiveAuthorizationRequired", "boom"},
		"polkit interactive, by message": {
			"", "Call failed: Interactive authentication required."},
		"access denied, by name": {
			"org.freedesktop.DBus.Error.AccessDenied", "boom"},
		"access denied, by message": {
			"", "Call failed: Access denied"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := classifyError(tc.dbusName, errors.New(tc.message))
			if !errors.Is(got, ErrUnauthorized) {
				t.Errorf("classifyError = %v, want ErrUnauthorized", got)
			}
			// The original text is preserved for the log, even though the API will not
			// show it to a client.
			if !strings.Contains(got.Error(), tc.message) {
				t.Errorf("original error text lost: %v", got)
			}
		})
	}
}

func TestClassifyErrorMapsMissingUnits(t *testing.T) {
	for name, tc := range map[string]struct{ dbusName, message string }{
		"by name":    {"org.freedesktop.systemd1.NoSuchUnit", "boom"},
		"by message": {"", "Unit nope.service not loaded."},
	} {
		t.Run(name, func(t *testing.T) {
			if got := classifyError(tc.dbusName, errors.New(tc.message)); !errors.Is(got, ErrUnitNotFound) {
				t.Errorf("classifyError = %v, want ErrUnitNotFound", got)
			}
		})
	}
}

// TestClassifyErrorPassesThroughUnknownFailures: a connection failure is not an
// authorisation failure, and reporting it as one would send an operator to check their
// polkit rule when the real problem is that D-Bus is down.
func TestClassifyErrorPassesThroughUnknownFailures(t *testing.T) {
	original := errors.New("dial unix /run/dbus/system_bus_socket: connect: connection refused")

	got := classifyError("", original)
	if errors.Is(got, ErrUnauthorized) || errors.Is(got, ErrUnitNotFound) {
		t.Errorf("unrelated failure was misclassified: %v", got)
	}
	if !errors.Is(got, original) {
		t.Errorf("original error not preserved: %v", got)
	}
	if classifyError("", nil) != nil {
		t.Error("classifyError(nil) should be nil")
	}
}

func TestRestartPropagatesBackendErrors(t *testing.T) {
	c, backend := newTestController(t)
	backend.restartErr = fmt.Errorf("%w: polkit said no", ErrUnauthorized)

	if _, err := c.RestartUnit(t.Context(), "jellyfin.service"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

// ---------------------------------------------------------------- jobs

// TestJobWaitReturnsSystemdResult models the asynchronous contract M0 established:
// RestartUnit returns when the job is queued, and the outcome arrives later via
// JobRemoved. This is the mechanism behind ADR-0004's 202-plus-action-id design.
func TestJobWaitReturnsSystemdResult(t *testing.T) {
	for _, result := range []JobResult{
		JobDone, JobFailed, JobCanceled, JobTimeout, JobDependency, JobSkipped,
	} {
		t.Run(string(result), func(t *testing.T) {
			c, backend := newTestController(t)
			job, err := c.RestartUnit(t.Context(), "jellyfin.service")
			if err != nil {
				t.Fatalf("RestartUnit: %v", err)
			}

			backend.results <- string(result)

			got, err := job.Wait(t.Context())
			if err != nil {
				t.Fatalf("Wait: %v", err)
			}
			if got != result {
				t.Errorf("Wait() = %q, want %q", got, result)
			}
			if got.Succeeded() != (result == JobDone) {
				t.Errorf("%q.Succeeded() = %v", result, got.Succeeded())
			}
		})
	}
}

// TestJobWaitRespectsContext: a job whose result never arrives — systemd wedged, D-Bus
// gone — must not block a request forever.
func TestJobWaitRespectsContext(t *testing.T) {
	c, _ := newTestController(t)
	job, err := c.RestartUnit(t.Context(), "jellyfin.service")
	if err != nil {
		t.Fatalf("RestartUnit: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	if _, err := job.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait err = %v, want DeadlineExceeded", err)
	}
}

// TestJobResultChannelIsBuffered guards a real deadlock.
//
// go-systemd delivers a job's outcome by writing to this channel from its signal
// dispatcher while holding an internal lock, and its documentation states that until the
// write completes the connection cannot handle other jobs. If the channel were
// unbuffered, a caller that never waited — a client that fires a restart and disconnects
// — would stall every subsequent job on the agent's systemd connection.
//
// The send below must not block even though nothing ever calls Wait.
func TestJobResultChannelIsBuffered(t *testing.T) {
	c, backend := newTestController(t)
	if _, err := c.RestartUnit(t.Context(), "jellyfin.service"); err != nil {
		t.Fatalf("RestartUnit: %v", err)
	}

	done := make(chan struct{})
	go func() {
		backend.results <- string(JobDone) // must not block
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("send to an abandoned job's result channel blocked; " +
			"an unread result would stall every later job on the connection")
	}
}

func TestJobWaitWithoutResultChannel(t *testing.T) {
	var job *Job
	if _, err := job.Wait(t.Context()); err == nil {
		t.Error("Wait on a nil job should error, not panic")
	}
	if _, err := (&Job{}).Wait(t.Context()); err == nil {
		t.Error("Wait with no result channel should error")
	}
}

// ---------------------------------------------------------------- unit state

func TestUnitStateHelpers(t *testing.T) {
	cases := map[string]struct {
		state                  UnitState
		loaded, active, failed bool
	}{
		"running": {
			UnitState{LoadState: "loaded", ActiveState: "active", SubState: "running"},
			true, true, false},
		"failed": {
			UnitState{LoadState: "loaded", ActiveState: "failed", SubState: "failed"},
			true, false, true},
		"stopped": {
			UnitState{LoadState: "loaded", ActiveState: "inactive", SubState: "dead"},
			true, false, false},
		"not installed": {
			UnitState{LoadState: "not-found", ActiveState: "inactive"},
			false, false, false},
		"masked": {
			UnitState{LoadState: "masked", ActiveState: "inactive"},
			false, false, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.state.Loaded() != tc.loaded {
				t.Errorf("Loaded() = %v, want %v", tc.state.Loaded(), tc.loaded)
			}
			if tc.state.Active() != tc.active {
				t.Errorf("Active() = %v, want %v", tc.state.Active(), tc.active)
			}
			if tc.state.Failed() != tc.failed {
				t.Errorf("Failed() = %v, want %v", tc.state.Failed(), tc.failed)
			}
		})
	}
}

func TestUnitStatePassesThroughBackend(t *testing.T) {
	c, backend := newTestController(t)
	when := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	backend.state = UnitState{
		LoadState: "loaded", ActiveState: "active", SubState: "running",
		ActiveEnterTime: when,
	}

	got, err := c.UnitState(t.Context(), "jellyfin.service")
	if err != nil {
		t.Fatalf("UnitState: %v", err)
	}
	if got.Name != "jellyfin.service" || !got.Active() || !got.ActiveEnterTime.Equal(when) {
		t.Errorf("state = %+v", got)
	}
}

// TestStartAndStopRefuseUnmanagedUnitBeforeBackend extends the package's core security
// assertion to the two verbs added in ADR-0002 Amendment 1.
//
// The widening was to the *verbs*, never to the *targets*. If the allowlist gated restart
// but not stop, a misconfiguration could stop an arbitrary unit — and stop is the verb
// that does not undo itself. Asserting the absence of the backend call, not merely the
// presence of an error, is what proves the refusal happened before the system bus.
func TestStartAndStopRefuseUnmanagedUnitBeforeBackend(t *testing.T) {
	unmanaged := []string{
		"ssh.service",
		"jellyfin",
		"jellyfin.service.evil",
		"Jellyfin.service",
		" jellyfin.service",
		"",
	}

	t.Run("start", func(t *testing.T) {
		c, backend := newTestController(t)
		for _, unit := range unmanaged {
			if _, err := c.StartUnit(context.Background(), unit); !errors.Is(err, ErrUnitNotManaged) {
				t.Errorf("StartUnit(%q) error = %v, want ErrUnitNotManaged", unit, err)
			}
		}
		if len(backend.startCalls) != 0 {
			t.Errorf("an unmanaged unit reached the backend: %v", backend.startCalls)
		}
	})

	t.Run("stop", func(t *testing.T) {
		c, backend := newTestController(t)
		for _, unit := range unmanaged {
			if _, err := c.StopUnit(context.Background(), unit); !errors.Is(err, ErrUnitNotManaged) {
				t.Errorf("StopUnit(%q) error = %v, want ErrUnitNotManaged", unit, err)
			}
		}
		if len(backend.stopCalls) != 0 {
			t.Errorf("an unmanaged unit reached the backend: %v", backend.stopCalls)
		}
	})
}

// TestStartAndStopReachBackendForManagedUnit is the other half: the allowlist must permit
// what it is supposed to permit, or the widening did nothing.
func TestStartAndStopReachBackendForManagedUnit(t *testing.T) {
	c, backend := newTestController(t)

	if _, err := c.StartUnit(context.Background(), "jellyfin.service"); err != nil {
		t.Fatalf("StartUnit on a managed unit: %v", err)
	}
	if _, err := c.StopUnit(context.Background(), "jellyfin.service"); err != nil {
		t.Fatalf("StopUnit on a managed unit: %v", err)
	}

	if len(backend.startCalls) != 1 || backend.startCalls[0] != "jellyfin.service" {
		t.Errorf("startCalls = %v", backend.startCalls)
	}
	if len(backend.stopCalls) != 1 || backend.stopCalls[0] != "jellyfin.service" {
		t.Errorf("stopCalls = %v", backend.stopCalls)
	}
}
