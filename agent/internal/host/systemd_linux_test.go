//go:build linux

package host

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// Integration tests against a real systemd. Everything here skips unless explicitly
// enabled, because these tests need a D-Bus system bus and — for the restart cases — a
// polkit rule granting the running user permission.
//
// Enable with:
//
//	CUESEEK_SYSTEMD_TESTS=1 go test ./internal/host/
//	CUESEEK_TEST_UNIT=jellyfin.service CUESEEK_SYSTEMD_TESTS=1 go test ./internal/host/
//
// They are opt-in rather than auto-detected because a restart is a real, visible side
// effect on somebody's machine. A test suite that stops your media server because it
// happened to find a system bus would be a bad neighbour.

func requireSystemd(t *testing.T) *systemdBackend {
	t.Helper()
	if os.Getenv("CUESEEK_SYSTEMD_TESTS") == "" {
		t.Skip("set CUESEEK_SYSTEMD_TESTS=1 to run tests against the real system bus")
	}
	backend, err := newBackend()
	if err != nil {
		t.Skipf("no usable system bus: %v", err)
	}
	t.Cleanup(func() { backend.Close() })
	return backend.(*systemdBackend)
}

// TestSystemdReadsUnitState covers the ADR-0002 claim that unit state is readable over
// the same connection used for control, without elevation and without parsing text.
//
// dbus.service is used because it necessarily exists and is running on any host with a
// system bus — reading it proves the mechanism without depending on the operator's
// particular services.
func TestSystemdReadsUnitState(t *testing.T) {
	backend := requireSystemd(t)

	state, err := backend.UnitState(t.Context(), "dbus.service")
	if err != nil {
		t.Fatalf("UnitState: %v", err)
	}

	t.Logf("dbus.service: load=%s active=%s sub=%s entered=%s",
		state.LoadState, state.ActiveState, state.SubState, state.ActiveEnterTime)

	if !state.Loaded() {
		t.Errorf("LoadState = %q, want loaded", state.LoadState)
	}
	if state.ActiveState == "" || state.SubState == "" {
		t.Error("systemd returned empty state fields")
	}
	// The M0 microsecond finding: a wrong unit conversion puts this in 1970 or the far
	// future, and the error is silent.
	if state.Active() {
		if state.ActiveEnterTime.IsZero() {
			t.Error("active unit has a zero ActiveEnterTime")
		}
		if age := time.Since(state.ActiveEnterTime); age < 0 || age > 50*365*24*time.Hour {
			t.Errorf("ActiveEnterTime %v is implausible (age %v); "+
				"check the microsecond conversion", state.ActiveEnterTime, age)
		}
	}
}

func TestSystemdReportsMissingUnit(t *testing.T) {
	backend := requireSystemd(t)

	_, err := backend.UnitState(t.Context(), "cueseek-definitely-not-real.service")
	if !errors.Is(err, ErrUnitNotFound) {
		t.Errorf("err = %v, want ErrUnitNotFound", err)
	}
}

// TestSystemdRestartUnit is the end-to-end proof of the M0 architecture in production
// form: enqueue a restart over D-Bus, then observe the terminal state via JobRemoved.
//
// Requires the polkit rule to be installed for the running user, and genuinely restarts
// the unit — so the unit must be named explicitly.
func TestSystemdRestartUnit(t *testing.T) {
	backend := requireSystemd(t)

	unit := os.Getenv("CUESEEK_TEST_UNIT")
	if unit == "" {
		t.Skip("set CUESEEK_TEST_UNIT=<unit> to run a real restart (it will actually restart)")
	}

	before, err := backend.UnitState(t.Context(), unit)
	if err != nil {
		t.Fatalf("UnitState before: %v", err)
	}
	t.Logf("before: active=%s entered=%s", before.ActiveState, before.ActiveEnterTime)

	job, err := backend.RestartUnit(t.Context(), unit)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			t.Fatalf("polkit refused the restart — is the rule installed for this user? %v", err)
		}
		t.Fatalf("RestartUnit: %v", err)
	}
	t.Logf("job %d enqueued", job.ID)

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	result, err := job.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	t.Logf("JobRemoved result: %q", result)

	if !result.Succeeded() {
		t.Fatalf("job result = %q, want done", result)
	}

	// M0's lesson: a queued job is not a completed restart. ActiveEnterTime moving
	// forward is the only evidence the unit actually went down and came back.
	after, err := backend.UnitState(t.Context(), unit)
	if err != nil {
		t.Fatalf("UnitState after: %v", err)
	}
	t.Logf("after: active=%s entered=%s", after.ActiveState, after.ActiveEnterTime)

	if !after.ActiveEnterTime.After(before.ActiveEnterTime) {
		t.Errorf("ActiveEnterTime did not advance (%v -> %v); "+
			"the job reported done but the unit may not have restarted",
			before.ActiveEnterTime, after.ActiveEnterTime)
	}
}

// TestSystemdRefusesUnauthorizedUnit checks the error mapping against real polkit.
//
// Requires a unit that exists but is NOT granted by the polkit rule, e.g.
// CUESEEK_UNAUTHORIZED_UNIT=cron.service. Deliberately not defaulted: if the rule were
// wrong and the restart were permitted, defaulting to something like ssh.service could
// drop the session running the test.
func TestSystemdRefusesUnauthorizedUnit(t *testing.T) {
	backend := requireSystemd(t)

	unit := os.Getenv("CUESEEK_UNAUTHORIZED_UNIT")
	if unit == "" {
		t.Skip("set CUESEEK_UNAUTHORIZED_UNIT=<unit> to verify polkit denial mapping")
	}

	job, err := backend.RestartUnit(t.Context(), unit)
	if err == nil {
		// The restart was permitted. Report it loudly: the polkit rule is wider than
		// ADR-0002 describes.
		t.Fatalf("restart of %q was ALLOWED; the polkit rule grants more than the allowlist "+
			"(job %d)", unit, job.ID)
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized — check the classifyError mapping "+
			"against this systemd/polkit version", err)
	}
}
