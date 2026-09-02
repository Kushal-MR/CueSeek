package systemd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// fakeUnits stands in for the host layer.
//
// A fake rather than a real Controller because these tests must run on the development
// machine, which is not the deployment target. The mapping from systemd's vocabulary to
// CueSeek's four states is the whole of this adapter, and it is pure — there is no reason
// for it to need a system bus to be tested.
type fakeUnits struct {
	state host.UnitState
	err   error

	// invoked records the last lifecycle call, so the tests can prove an action reached
	// the host layer rather than systemd directly.
	invoked string
}

func (f *fakeUnits) UnitState(context.Context, string) (host.UnitState, error) {
	return f.state, f.err
}

func (f *fakeUnits) RestartUnit(_ context.Context, unit string) (*host.Job, error) {
	f.invoked = "restart:" + unit
	return nil, nil
}

func (f *fakeUnits) StartUnit(_ context.Context, unit string) (*host.Job, error) {
	f.invoked = "start:" + unit
	return nil, nil
}

func (f *fakeUnits) StopUnit(_ context.Context, unit string) (*host.Job, error) {
	f.invoked = "stop:" + unit
	return nil, nil
}

func newForTest(t *testing.T, cfg config.Service, units adapters.UnitControl) adapters.Service {
	t.Helper()
	svc, err := New(cfg, adapters.Deps{Units: units})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func baseConfig() config.Service {
	return config.Service{ID: "plex", Name: "Plex", Type: Type, Unit: "plexmediaserver.service"}
}

// ---------------------------------------------------------------- construction

// TestUnitIsRequired: the unit is this adapter's only source of health, so building one
// without a unit would produce a service that can never report anything.
func TestUnitIsRequired(t *testing.T) {
	cfg := baseConfig()
	cfg.Unit = "  "

	_, err := New(cfg, adapters.Deps{Units: &fakeUnits{}})
	if err == nil {
		t.Fatal("built a systemd adapter with no unit")
	}
	// The message has to name the fix: whoever hits this is mid-install.
	if !strings.Contains(err.Error(), "systemctl list-units") {
		t.Errorf("error does not say how to find the unit name: %v", err)
	}
}

func TestHostLayerIsRequired(t *testing.T) {
	if _, err := New(baseConfig(), adapters.Deps{Units: nil}); err == nil {
		t.Fatal("built a systemd adapter with no host layer")
	}
}

// TestServiceAPIFieldsAreRefused: setting base_url or a credential on a generic service
// almost always means the operator expected CueSeek to understand it and picked the wrong
// type. Failing at startup is better than a dashboard that silently never shows what they
// configured a credential for.
func TestServiceAPIFieldsAreRefused(t *testing.T) {
	for field, mutate := range map[string]func(*config.Service){
		"base_url":      func(c *config.Service) { c.BaseURL = "http://127.0.0.1:32400" },
		"api_key":       func(c *config.Service) { c.APIKey = "secret" },
		"api_key_file":  func(c *config.Service) { c.APIKeyFile = "/etc/cueseek/plex.key" },
		"username":      func(c *config.Service) { c.Username = "admin" },
		"password":      func(c *config.Service) { c.Password = "hunter2" },
		"password_file": func(c *config.Service) { c.PasswordFile = "/etc/cueseek/plex.pass" },
	} {
		t.Run(field, func(t *testing.T) {
			cfg := baseConfig()
			mutate(&cfg)

			_, err := New(cfg, adapters.Deps{Units: &fakeUnits{}})
			if err == nil {
				t.Fatalf("accepted %s on a systemd service", field)
			}
			if !strings.Contains(err.Error(), field) {
				t.Errorf("error does not name the offending field %q: %v", field, err)
			}
			// A secret must never reach an error string: startup errors are logged, and
			// journald keeps them.
			for _, secret := range []string{"secret", "hunter2"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("the error leaks a credential value: %v", err)
				}
			}
		})
	}
}

// TestNameFallsBackToID keeps every service displayable, matching the other adapters.
func TestNameFallsBackToID(t *testing.T) {
	cfg := baseConfig()
	cfg.Name = ""

	svc := newForTest(t, cfg, &fakeUnits{})
	if svc.Name() != "plex" {
		t.Errorf("Name() = %q, want fallback to id", svc.Name())
	}
}

// ---------------------------------------------------------------- capabilities

// TestCapabilities is the ADR-0005 assertion for this adapter: it advertises exactly what
// it can do, and in particular does NOT advertise the activity capabilities. A generic
// adapter claiming now_playing would put an empty section on every phone.
func TestCapabilities(t *testing.T) {
	cfg := baseConfig()
	cfg.WebUI = &config.WebUI{Port: 32400, Path: "/web"}

	svc := newForTest(t, cfg, &fakeUnits{})

	got := map[string]bool{}
	for _, c := range adapters.CapabilitiesOf(svc) {
		got[c.ID] = true
	}

	for _, want := range []string{"health", "control", "web_ui"} {
		if !got[want] {
			t.Errorf("does not advertise %q; has %v", want, got)
		}
	}
	for _, unwanted := range []string{"now_playing", "transfers"} {
		if got[unwanted] {
			t.Errorf("advertises %q, which it cannot provide", unwanted)
		}
	}
}

// TestWebUIIsOptional: a service with no web interface must not advertise one, or a client
// offers to open nothing.
func TestWebUIIsOptional(t *testing.T) {
	svc := newForTest(t, baseConfig(), &fakeUnits{})

	if adapters.HasCapability(svc, domain.CapabilityWebUI.ID) {
		t.Error("advertises web_ui with none configured")
	}
}

// ---------------------------------------------------------------- health mapping

func TestHealthFromUnitState(t *testing.T) {
	cases := map[string]struct {
		state        host.UnitState
		wantStatus   domain.HealthStatus
		wantReach    bool
		wantReason   string
		wantReported string
	}{
		"active": {
			state:        host.UnitState{LoadState: "loaded", ActiveState: "active", SubState: "running"},
			wantStatus:   domain.StatusHealthy,
			wantReach:    true,
			wantReason:   "",
			wantReported: "active (running)",
		},
		// Stopped is unreachable, not degraded. `degraded` means reachable-but-wrong, and
		// a stopped service is not reachable — and Jellyfin's HTTP probe already reports
		// unreachable for exactly this situation. Two adapters watching the same stopped
		// service must not disagree about what colour it is.
		"inactive": {
			state:        host.UnitState{LoadState: "loaded", ActiveState: "inactive", SubState: "dead"},
			wantStatus:   domain.StatusUnreachable,
			wantReach:    false,
			wantReason:   domain.ReasonUnitInactive,
			wantReported: "inactive (dead)",
		},
		"failed": {
			state:        host.UnitState{LoadState: "loaded", ActiveState: "failed", SubState: "failed"},
			wantStatus:   domain.StatusUnreachable,
			wantReach:    false,
			wantReason:   domain.ReasonUnitFailed,
			wantReported: "failed (failed)",
		},
		"activating": {
			state:        host.UnitState{LoadState: "loaded", ActiveState: "activating", SubState: "start"},
			wantStatus:   domain.StatusDegraded,
			wantReach:    false,
			wantReason:   domain.ReasonUnitTransitioning,
			wantReported: "activating (start)",
		},
		"deactivating": {
			state:      host.UnitState{LoadState: "loaded", ActiveState: "deactivating", SubState: "stop"},
			wantStatus: domain.StatusDegraded,
			wantReach:  false,
			wantReason: domain.ReasonUnitTransitioning,
		},
		"reloading": {
			state:      host.UnitState{LoadState: "loaded", ActiveState: "reloading", SubState: "reload"},
			wantStatus: domain.StatusDegraded,
			wantReach:  false,
			wantReason: domain.ReasonUnitTransitioning,
		},
		// Masked is loaded-but-unusable and no amount of restarting will help, so it must
		// not look like an ordinary stop.
		"masked": {
			state:      host.UnitState{LoadState: "masked", ActiveState: "inactive", SubState: "dead"},
			wantStatus: domain.StatusUnreachable,
			wantReach:  false,
			wantReason: domain.ReasonUnitNotLoaded,
		},
		// A state from a systemd newer than this agent. Unknown, never healthy: guessing
		// healthy is the one direction that would be confidently wrong.
		"unrecognised state": {
			state:      host.UnitState{LoadState: "loaded", ActiveState: "quiescing", SubState: "x"},
			wantStatus: domain.StatusUnknown,
			wantReach:  false,
			wantReason: domain.ReasonUnitStateUnknown,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := newForTest(t, baseConfig(), &fakeUnits{state: tc.state})

			got, err := svc.Health(context.Background())
			if err != nil {
				// Health forms an opinion or says it cannot; it never returns an error.
				t.Fatalf("Health returned an error: %v", err)
			}

			if got.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tc.wantStatus)
			}
			if got.Reachable != tc.wantReach {
				t.Errorf("Reachable = %v, want %v", got.Reachable, tc.wantReach)
			}
			if got.ObservedAt.IsZero() {
				t.Error("ObservedAt is zero; a health value with no clock cannot be aged")
			}
			if tc.wantReported != "" && got.ReportedStatus != tc.wantReported {
				t.Errorf("ReportedStatus = %q, want %q", got.ReportedStatus, tc.wantReported)
			}

			if tc.wantReason == "" {
				if len(got.Reasons) != 0 {
					t.Errorf("healthy state carries reasons: %+v", got.Reasons)
				}
				return
			}
			if len(got.Reasons) != 1 {
				t.Fatalf("Reasons = %+v, want exactly one", got.Reasons)
			}
			if got.Reasons[0].Code != tc.wantReason {
				t.Errorf("reason code = %q, want %q", got.Reasons[0].Code, tc.wantReason)
			}
			if strings.TrimSpace(got.Reasons[0].Message) == "" {
				t.Error("reason has no message; a code alone is not actionable")
			}
		})
	}
}

// TestHealthWhenTheUnitDoesNotExist covers the single most likely misconfiguration on a
// new install — M0 established that a unit's real name routinely differs from what the
// software calls itself.
func TestHealthWhenTheUnitDoesNotExist(t *testing.T) {
	units := &fakeUnits{err: fmt.Errorf("%w: plexmediaserver.service", host.ErrUnitNotFound)}
	svc := newForTest(t, baseConfig(), units)

	got, err := svc.Health(context.Background())
	if err != nil {
		t.Fatalf("Health returned an error: %v", err)
	}
	if got.Status != domain.StatusUnreachable {
		t.Errorf("Status = %q, want unreachable", got.Status)
	}
	if len(got.Reasons) != 1 || got.Reasons[0].Code != domain.ReasonUnitNotLoaded {
		t.Fatalf("Reasons = %+v", got.Reasons)
	}
	// The message must name the command that settles it, not merely state the problem.
	if !strings.Contains(got.Reasons[0].Message, "systemctl list-units") {
		t.Errorf("message does not say how to find the right name: %q", got.Reasons[0].Message)
	}
}

// TestHealthWhenTheStateCannotBeRead: unknown, never unreachable. The service may be
// running perfectly well and the agent simply could not look; reporting it as down would
// be confidently wrong, which is the state ADR-0008's `unknown` exists to prevent.
func TestHealthWhenTheStateCannotBeRead(t *testing.T) {
	for name, readErr := range map[string]error{
		"polkit refused":       host.ErrUnauthorized,
		"unsupported platform": host.ErrUnsupportedPlatform,
		"not in the allowlist": host.ErrUnitNotManaged,
		"bus failure":          errors.New("dial unix /run/dbus/system_bus_socket: no such file"),
	} {
		t.Run(name, func(t *testing.T) {
			svc := newForTest(t, baseConfig(), &fakeUnits{err: readErr})

			got, err := svc.Health(context.Background())
			if err != nil {
				t.Fatalf("Health returned an error: %v", err)
			}
			if got.Status != domain.StatusUnknown {
				t.Errorf("Status = %q, want unknown", got.Status)
			}
			if got.Reachable {
				t.Error("Reachable is true for a state that could not be read")
			}
			if len(got.Reasons) != 1 || got.Reasons[0].Code != domain.ReasonUnitStateUnknown {
				t.Errorf("Reasons = %+v", got.Reasons)
			}
		})
	}
}

// ---------------------------------------------------------------- actions

// TestActionsFollowUnitState: Start on a stopped unit, Restart and Stop on a running one.
// Shared with every other unit-backed adapter, so this asserts the wiring rather than
// re-testing LifecycleActions.
func TestActionsFollowUnitState(t *testing.T) {
	cases := map[string]struct {
		state host.UnitState
		want  []string
	}{
		"running": {
			state: host.UnitState{LoadState: "loaded", ActiveState: "active", SubState: "running"},
			want:  []string{adapters.ActionRestart, adapters.ActionStop},
		},
		"stopped": {
			state: host.UnitState{LoadState: "loaded", ActiveState: "inactive", SubState: "dead"},
			want:  []string{adapters.ActionStart},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := newForTest(t, baseConfig(), &fakeUnits{state: tc.state})

			controllable, ok := svc.(adapters.Controllable)
			if !ok {
				t.Fatal("systemd adapter does not implement Controllable")
			}

			var got []string
			for _, a := range controllable.Actions(context.Background()) {
				got = append(got, a.ID)
				if a.Label == "" || !a.Risk.Valid() {
					t.Errorf("action %q is not renderable: %+v", a.ID, a)
				}
				// The label must name the service, not the unit: a phone shows
				// "Restart Plex", not "Restart plexmediaserver.service".
				if !strings.Contains(a.Label, "Plex") {
					t.Errorf("label %q does not name the service", a.Label)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("actions = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestInvokeGoesThroughTheHostLayer is a security assertion, not a behavioural one.
//
// The host layer enforces the configured unit allowlist before any D-Bus call, and polkit
// enforces it again behind that (ADR-0002). An adapter that reached for go-systemd itself
// would bypass the first of those entirely.
func TestInvokeGoesThroughTheHostLayer(t *testing.T) {
	units := &fakeUnits{state: host.UnitState{LoadState: "loaded", ActiveState: "active"}}
	svc := newForTest(t, baseConfig(), units)

	controllable := svc.(adapters.Controllable)
	if _, err := controllable.Invoke(context.Background(), adapters.ActionRestart); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if units.invoked != "restart:plexmediaserver.service" {
		t.Errorf("host layer saw %q, want restart:plexmediaserver.service", units.invoked)
	}
}

func TestInvokeRejectsUnknownActions(t *testing.T) {
	units := &fakeUnits{}
	svc := newForTest(t, baseConfig(), units)

	if _, err := svc.(adapters.Controllable).Invoke(context.Background(), "enable"); err == nil {
		t.Fatal("accepted an action outside the lifecycle vocabulary")
	}
	if units.invoked != "" {
		t.Errorf("an unknown action still reached the host layer as %q", units.invoked)
	}
}

// ---------------------------------------------------------------- reported status

func TestReportedStatus(t *testing.T) {
	cases := map[string]struct {
		state host.UnitState
		want  string
	}{
		"both":        {host.UnitState{ActiveState: "active", SubState: "running"}, "active (running)"},
		"active only": {host.UnitState{ActiveState: "active"}, "active"},
		"sub only":    {host.UnitState{SubState: "running"}, "running"},
		"neither":     {host.UnitState{}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := reportedStatus(tc.state); got != tc.want {
				t.Errorf("reportedStatus = %q, want %q", got, tc.want)
			}
		})
	}
}
