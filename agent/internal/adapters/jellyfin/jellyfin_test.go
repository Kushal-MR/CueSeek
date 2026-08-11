package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

const testAPIKey = "test-api-key-abc123"

// fakeUnits records what the adapter asked the host layer to do, so control tests assert
// delegation rather than trusting that the adapter did not reach for systemd itself.
type fakeUnits struct {
	restarted []string
	started   []string
	stopped   []string
	err       error

	// state is what UnitState reports. The zero value is an inactive unit, which is what
	// makes the "offers Start when stopped" case the default in these tests.
	state    host.UnitState
	stateErr error
}

func (f *fakeUnits) UnitState(_ context.Context, _ string) (host.UnitState, error) {
	return f.state, f.stateErr
}

func (f *fakeUnits) RestartUnit(_ context.Context, unit string) (*host.Job, error) {
	f.restarted = append(f.restarted, unit)
	return nil, f.err
}

func (f *fakeUnits) StartUnit(_ context.Context, unit string) (*host.Job, error) {
	f.started = append(f.started, unit)
	return nil, f.err
}

func (f *fakeUnits) StopUnit(_ context.Context, unit string) (*host.Job, error) {
	f.stopped = append(f.stopped, unit)
	return nil, f.err
}

// newTestServer stands in for Jellyfin. The handler receives the request so tests can
// assert on headers and path.
func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func newAdapter(t *testing.T, baseURL string, deps adapters.Deps) adapters.Service {
	t.Helper()
	svc, err := New(config.Service{
		ID: "jellyfin", Name: "Jellyfin", Type: Type, BaseURL: baseURL, APIKey: testAPIKey,
	}, deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func health(t *testing.T, svc adapters.Service) domain.Health {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	h, err := svc.Health(ctx)
	if err != nil {
		t.Fatalf("Health returned an error rather than an opinion: %v", err)
	}
	return h
}

func hasReason(h domain.Health, code string) bool {
	for _, r := range h.Reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

func writeSystemInfo(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// ---------------------------------------------------------------- healthy

func TestHealthyServer(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeSystemInfo(w, `{"ServerName":"Mint","Version":"10.9.11","Id":"abc"}`)
	})

	h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

	if h.Status != domain.StatusHealthy {
		t.Errorf("status = %q, want healthy", h.Status)
	}
	if !h.Reachable {
		t.Error("reachable = false for a responding server")
	}
	if h.ObservedAt.IsZero() {
		t.Error("ObservedAt was not set")
	}
	// Jellyfin publishes no self-assessment, so inventing one would fill a field the
	// contract defines as verbatim and unmapped.
	if h.ReportedStatus != "" {
		t.Errorf("ReportedStatus = %q, want empty", h.ReportedStatus)
	}
}

// TestAuthenticationHeaderIsSent covers requirement 3's "handle authentication cleanly",
// and the choice of the authenticated endpoint over /System/Info/Public.
func TestAuthenticationHeaderIsSent(t *testing.T) {
	var gotToken, gotPath string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Emby-Token")
		gotPath = r.URL.Path
		writeSystemInfo(w, `{"ServerName":"Mint"}`)
	})

	health(t, newAdapter(t, server.URL, adapters.Deps{}))

	if gotToken != testAPIKey {
		t.Errorf("X-Emby-Token = %q, want the configured key", gotToken)
	}
	if gotPath != systemInfoPath {
		t.Errorf("path = %q, want %q (the authenticated endpoint, so a bad key is visible)",
			gotPath, systemInfoPath)
	}
}

// TestPendingRestartIsAReasonNotAStatus: reasons are not required to be problems. A
// pending restart is worth telling the operator and not worth colouring the dashboard.
func TestPendingRestartIsAReasonNotAStatus(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeSystemInfo(w, `{"ServerName":"Mint","HasPendingRestart":true}`)
	})

	h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

	if h.Status != domain.StatusHealthy {
		t.Errorf("status = %q, want healthy", h.Status)
	}
	if !hasReason(h, domain.ReasonPendingRestart) {
		t.Errorf("reasons = %v, want pending_restart", h.Reasons)
	}
}

func TestShuttingDownIsDegraded(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeSystemInfo(w, `{"ServerName":"Mint","IsShuttingDown":true}`)
	})

	h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

	if h.Status != domain.StatusDegraded {
		t.Errorf("status = %q, want degraded", h.Status)
	}
	if !h.Reachable {
		t.Error("a server that answered is not reachable")
	}
	if !hasReason(h, domain.ReasonShuttingDown) {
		t.Errorf("reasons = %v, want shutting_down", h.Reasons)
	}
}

// ---------------------------------------------------------------- authentication

// TestAuthFailureIsDegradedNotUnreachable is the ADR-0005 distinction that matters most
// here. A rejected key means the server is up and the configuration is wrong; reporting
// it as unreachable sends the operator to look at the network instead of the key.
func TestAuthFailureIsDegradedNotUnreachable(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			})

			h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

			if h.Status != domain.StatusDegraded {
				t.Errorf("status = %q, want degraded", h.Status)
			}
			if !h.Reachable {
				t.Error("reachable = false; the server answered, it just refused us")
			}
			if !hasReason(h, domain.ReasonAuthFailed) {
				t.Errorf("reasons = %v, want auth_failed", h.Reasons)
			}
		})
	}
}

// ---------------------------------------------------------------- upstream errors

func TestUpstreamErrorStatuses(t *testing.T) {
	for _, code := range []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusNotFound,
		http.StatusTeapot,
	} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			})

			h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

			if h.Status != domain.StatusDegraded {
				t.Errorf("status = %q, want degraded", h.Status)
			}
			if !h.Reachable {
				t.Error("reachable = false for a server that returned a status code")
			}
			if !hasReason(h, domain.ReasonUpstreamError) {
				t.Errorf("reasons = %v, want upstream_error", h.Reasons)
			}
		})
	}
}

// TestMalformedResponses: reachable, answering, but not with Jellyfin. Almost always a
// base_url pointing somewhere else, so the message says so rather than reporting a
// generic parse failure.
func TestMalformedResponses(t *testing.T) {
	cases := map[string]string{
		"html":           "<html><body>nginx</body></html>",
		"truncated json": `{"ServerName":`,
		"empty body":     "",
		"json array":     `[1,2,3]`,
		"plain text":     "OK",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				writeSystemInfo(w, body)
			})

			h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

			if h.Status != domain.StatusDegraded {
				t.Errorf("status = %q, want degraded", h.Status)
			}
			if !h.Reachable {
				t.Error("reachable = false for a server that sent bytes")
			}
			if !hasReason(h, domain.ReasonInvalidResponse) {
				t.Errorf("reasons = %v, want invalid_response", h.Reasons)
			}
			if !strings.Contains(h.Reasons[0].Message, "base_url") {
				t.Errorf("message does not point at the likely cause: %q", h.Reasons[0].Message)
			}
		})
	}
}

// ---------------------------------------------------------------- unreachable

func TestUnreachableHost(t *testing.T) {
	// A server that is closed immediately, so the port is almost certainly refusing.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	h := health(t, newAdapter(t, url, adapters.Deps{}))

	if h.Status != domain.StatusUnreachable {
		t.Errorf("status = %q, want unreachable", h.Status)
	}
	if h.Reachable {
		t.Error("reachable = true for a refused connection")
	}
	if !hasReason(h, domain.ReasonUnreachable) {
		t.Errorf("reasons = %v, want unreachable", h.Reasons)
	}
}

// TestTimeoutIsDistinctFromRefused: a wedged service and an absent one look the same in a
// status dot and have different causes, so the reason code separates them.
func TestTimeoutIsDistinctFromRefused(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})

	svc := newAdapter(t, server.URL, adapters.Deps{})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	started := time.Now()
	h, err := svc.Health(ctx)
	if err != nil {
		t.Fatalf("Health returned an error rather than an opinion: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Errorf("Health took %v; the context deadline was not honoured", elapsed)
	}

	if h.Status != domain.StatusUnreachable {
		t.Errorf("status = %q, want unreachable", h.Status)
	}
	if !hasReason(h, domain.ReasonTimeout) {
		t.Errorf("reasons = %v, want timeout specifically", h.Reasons)
	}
}

// TestErrorsDoNotLeakTheAPIKey: net/http embeds the request URL in transport errors, and
// these messages reach an API response and the log.
func TestErrorsDoNotLeakTheAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	h := health(t, newAdapter(t, url, adapters.Deps{}))

	for _, reason := range h.Reasons {
		if strings.Contains(reason.Message, testAPIKey) {
			t.Errorf("reason leaks the API key: %q", reason.Message)
		}
	}
}

// ---------------------------------------------------------------- capabilities

// TestControlIsAdvertisedOnlyWhenPerformable is the payoff of returning a different
// concrete type from the factory. A Jellyfin with no configured unit cannot be restarted,
// so it must not offer a restart button that could only ever fail.
func TestControlIsAdvertisedOnlyWhenPerformable(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeSystemInfo(w, `{"ServerName":"Mint"}`)
	})

	cases := map[string]struct {
		unit      string
		units     adapters.UnitControl
		wantsCtrl bool
	}{
		"unit and host layer": {"jellyfin.service", &fakeUnits{}, true},
		"unit but no host":    {"jellyfin.service", nil, false},
		"host but no unit":    {"", &fakeUnits{}, false},
		"neither":             {"", nil, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc, err := New(config.Service{
				ID: "jellyfin", Type: Type, BaseURL: server.URL,
				APIKey: testAPIKey, Unit: tc.unit,
			}, adapters.Deps{Units: tc.units})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			got := adapters.HasCapability(svc, domain.CapabilityControl.ID)
			if got != tc.wantsCtrl {
				t.Errorf("advertises control = %v, want %v", got, tc.wantsCtrl)
			}
			// Health is unconditional either way.
			if !adapters.HasCapability(svc, domain.CapabilityHealth.ID) {
				t.Error("does not advertise health")
			}
		})
	}
}

func TestActionDescriptors(t *testing.T) {
	// Actions are state-dependent since ADR-0002 Amendment 1. Offering Start on a running
	// service is noise; offering Stop on a stopped one is a lie.
	cases := []struct {
		name      string
		units     *fakeUnits
		wantIDs   []string
		wantRisks map[string]domain.RiskLevel
	}{
		{
			name:    "running: restart and stop, no start",
			units:   &fakeUnits{state: host.UnitState{ActiveState: "active"}},
			wantIDs: []string{adapters.ActionRestart, adapters.ActionStop},
			wantRisks: map[string]domain.RiskLevel{
				adapters.ActionRestart: domain.RiskDisruptive,
				// Destructive, not disruptive: a stop does not undo itself, so it must
				// route to a client's press-and-hold rather than a single tap.
				adapters.ActionStop: domain.RiskDestructive,
			},
		},
		{
			name:    "stopped: start only",
			units:   &fakeUnits{state: host.UnitState{ActiveState: "inactive"}},
			wantIDs: []string{adapters.ActionStart},
			wantRisks: map[string]domain.RiskLevel{
				// Nothing is interrupted and nothing is lost: it is not running.
				adapters.ActionStart: domain.RiskSafe,
			},
		},
		{
			name: "state unreadable: restart only",
			// Restart is the one verb correct whichever state we failed to observe:
			// systemd's RestartUnit starts an inactive unit.
			units:     &fakeUnits{stateErr: errors.New("dbus unavailable")},
			wantIDs:   []string{adapters.ActionRestart},
			wantRisks: map[string]domain.RiskLevel{adapters.ActionRestart: domain.RiskDisruptive},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := buildControllable(t, tc.units)
			controllable, ok := svc.(adapters.Controllable)
			if !ok {
				t.Fatal("service does not implement Controllable")
			}

			actions := controllable.Actions(context.Background())

			var gotIDs []string
			for _, a := range actions {
				gotIDs = append(gotIDs, a.ID)
				if a.Label == "" || a.Description == "" {
					t.Errorf("action descriptor is incomplete: %+v", a)
				}
				// Clients gate on risk without knowing what the action does, so an
				// invalid level would make a destructive action look safe.
				if !a.Risk.Valid() {
					t.Errorf("risk %q is not a recognised level", a.Risk)
				}
				if want, ok := tc.wantRisks[a.ID]; ok && a.Risk != want {
					t.Errorf("%s risk = %q, want %q", a.ID, a.Risk, want)
				}
			}
			if !slices.Equal(gotIDs, tc.wantIDs) {
				t.Errorf("actions = %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

// TestStopDescriptionWarnsItStaysStopped is the copy the operator reads before confirming.
// systemd leaves a stopped unit enabled, so it returns on the next boot — and nobody
// should have to know that to predict what their tap does.
func TestStopDescriptionWarnsItStaysStopped(t *testing.T) {
	svc := buildControllable(t, &fakeUnits{state: host.UnitState{ActiveState: "active"}})
	controllable := svc.(adapters.Controllable)

	for _, a := range controllable.Actions(context.Background()) {
		if a.ID != adapters.ActionStop {
			continue
		}
		if !strings.Contains(a.Description, "reboot") ||
			!strings.Contains(a.Description, "stays stopped") {
			t.Errorf("stop description does not say it persists: %q", a.Description)
		}
		return
	}
	t.Fatal("a running service advertised no stop action")
}

// TestInvokeDelegatesToHostLayer is requirement 5: the adapter never touches systemd. It
// asks the host layer, which enforces the unit allowlist and is bounded by polkit.
func TestInvokeDelegatesToHostLayer(t *testing.T) {
	units := &fakeUnits{}
	svc := buildControllable(t, units)

	controllable := svc.(adapters.Controllable)
	if _, err := controllable.Invoke(t.Context(), actionRestart); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if len(units.restarted) != 1 || units.restarted[0] != "jellyfin.service" {
		t.Errorf("host layer received %v, want one restart of jellyfin.service", units.restarted)
	}
}

func TestInvokeRejectsUnknownAction(t *testing.T) {
	units := &fakeUnits{}
	svc := buildControllable(t, units)

	_, err := svc.(adapters.Controllable).Invoke(t.Context(), "self-destruct")
	if err == nil {
		t.Fatal("unknown action was accepted")
	}
	if len(units.restarted) != 0 {
		t.Errorf("host layer was called for an unknown action: %v", units.restarted)
	}
}

func buildControllable(t *testing.T, units adapters.UnitControl) adapters.Service {
	t.Helper()
	svc, err := New(config.Service{
		ID: "jellyfin", Type: Type, BaseURL: "http://127.0.0.1:8096",
		APIKey: testAPIKey, Unit: "jellyfin.service",
	}, adapters.Deps{Units: units})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

// ---------------------------------------------------------------- construction

// TestFactoryValidatesItsOwnRequirements: whether a service needs a base_url is a
// property of its adapter, not something the config package should know per service type.
func TestFactoryValidatesItsOwnRequirements(t *testing.T) {
	cases := map[string]struct {
		cfg  config.Service
		want string
	}{
		"no base_url":       {config.Service{ID: "j", APIKey: "k"}, "base_url"},
		"blank base_url":    {config.Service{ID: "j", BaseURL: "   ", APIKey: "k"}, "base_url"},
		"relative base_url": {config.Service{ID: "j", BaseURL: "/jellyfin", APIKey: "k"}, "absolute"},
		"no scheme":         {config.Service{ID: "j", BaseURL: "127.0.0.1:8096", APIKey: "k"}, "absolute"},
		"no api key":        {config.Service{ID: "j", BaseURL: "http://x:8096"}, "api_key"},
		"blank api key":     {config.Service{ID: "j", BaseURL: "http://x:8096", APIKey: " "}, "api_key"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := New(tc.cfg, adapters.Deps{})
			if err == nil {
				t.Fatalf("accepted invalid config %+v", tc.cfg)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestNameFallsBackToID(t *testing.T) {
	svc, err := New(config.Service{
		ID: "jellyfin-basement", Type: Type, BaseURL: "http://x:8096", APIKey: "k",
	}, adapters.Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.Name() != "jellyfin-basement" {
		t.Errorf("Name() = %q, want the id as a fallback", svc.Name())
	}
}

// TestTrailingSlashInBaseURL: an operator writing http://host:8096/ must not produce
// //System/Info, which some reverse proxies reject.
func TestTrailingSlashInBaseURL(t *testing.T) {
	var gotPath string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeSystemInfo(w, `{"ServerName":"Mint"}`)
	})

	svc, err := New(config.Service{
		ID: "jellyfin", Type: Type, BaseURL: server.URL + "/", APIKey: testAPIKey,
	}, adapters.Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	health(t, svc)

	if gotPath != systemInfoPath {
		t.Errorf("path = %q, want %q", gotPath, systemInfoPath)
	}
}

func TestIdentity(t *testing.T) {
	svc, err := New(config.Service{
		ID: "jellyfin", Name: "Living Room", Type: Type,
		BaseURL: "http://x:8096", APIKey: "k",
	}, adapters.Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.ID() != "jellyfin" || svc.Name() != "Living Room" {
		t.Errorf("identity = %q/%q", svc.ID(), svc.Name())
	}
}
