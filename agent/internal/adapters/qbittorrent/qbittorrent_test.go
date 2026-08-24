package qbittorrent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

const (
	testUsername = "admin"
	testPassword = "correct-horse-battery"
	testSID      = "sid-abc123"
)

// fakeUnits records what the adapter asked the host layer to do, so control tests assert
// delegation rather than trusting that the adapter did not reach for systemd itself.
type fakeUnits struct {
	restarted []string
	started   []string
	stopped   []string
	err       error

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

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// newAdapter builds an adapter with no credentials — the localhost-bypass deployment,
// which is the common one and therefore the default these tests exercise.
func newAdapter(t *testing.T, baseURL string, deps adapters.Deps) adapters.Service {
	t.Helper()
	svc, err := New(config.Service{
		ID: "qbittorrent", Name: "qBittorrent", Type: Type, BaseURL: baseURL,
	}, deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func newAuthenticatedAdapter(t *testing.T, baseURL string) adapters.Service {
	t.Helper()
	svc, err := New(config.Service{
		ID: "qbittorrent", Name: "qBittorrent", Type: Type, BaseURL: baseURL,
		Username: testUsername, Password: testPassword,
	}, adapters.Deps{})
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

func writeTransferInfo(w http.ResponseWriter, status string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"connection_status":%q,"dl_info_speed":1024}`, status)
}

// ---------------------------------------------------------------- connection status

// TestConnectionStatusMapping is the whole of this adapter's health opinion, as a table.
//
// The interesting rows are `firewalled`, which must **not** downgrade the dashboard, and
// the unrecognised value, which must not be guessed at in either direction.
func TestConnectionStatusMapping(t *testing.T) {
	cases := []struct {
		reported   string
		wantStatus domain.HealthStatus
		wantReason string
	}{
		{"connected", domain.StatusHealthy, ""},
		{"firewalled", domain.StatusHealthy, domain.ReasonPeerConnectivity},
		{"disconnected", domain.StatusDegraded, domain.ReasonPeerConnectivity},
		{"", domain.StatusUnknown, domain.ReasonInvalidResponse},
		{"reconfiguring", domain.StatusUnknown, domain.ReasonInvalidResponse},
	}

	for _, tc := range cases {
		t.Run(tc.reported, func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeTransferInfo(w, tc.reported)
			})
			h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

			if h.Status != tc.wantStatus {
				t.Errorf("status = %v, want %v", h.Status, tc.wantStatus)
			}
			if !h.Reachable {
				t.Error("reachable = false, but the service answered")
			}
			// Verbatim and unmapped, whatever it was.
			if h.ReportedStatus != tc.reported {
				t.Errorf("reported_status = %q, want %q", h.ReportedStatus, tc.reported)
			}
			if tc.wantReason != "" && !hasReason(h, tc.wantReason) {
				t.Errorf("missing reason %q, got %+v", tc.wantReason, h.Reasons)
			}
			if tc.wantReason == "" && len(h.Reasons) != 0 {
				t.Errorf("a connected client should carry no reasons, got %+v", h.Reasons)
			}
		})
	}
}

// TestFirewalledIsAReasonNotAStatus states the palette rule in a test.
//
// Chroma means attention. Firewalled is often the operator's deliberate choice and always
// still transfers, so spending a dashboard colour on it would train them to ignore the one
// state that matters.
func TestFirewalledIsAReasonNotAStatus(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeTransferInfo(w, "firewalled")
	})
	h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

	if h.Status != domain.StatusHealthy {
		t.Fatalf("status = %v, want healthy — firewalled is a note, not a downgrade", h.Status)
	}
	if len(h.Reasons) != 1 {
		t.Fatalf("want exactly one reason, got %+v", h.Reasons)
	}
}

// TestReportedStatusCrossesVerbatim guards the field this adapter is the first to use.
func TestReportedStatusCrossesVerbatim(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeTransferInfo(w, "FireWalled")
	})
	h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

	if h.ReportedStatus != "FireWalled" {
		t.Errorf("reported_status = %q, want the service's own casing", h.ReportedStatus)
	}
	// Recognised case-insensitively, but not rewritten on the way through.
	if h.Status != domain.StatusHealthy {
		t.Errorf("status = %v, want healthy", h.Status)
	}
}

// ---------------------------------------------------------------- authentication

// TestNoCredentialsMeansNoLogin covers the common deployment: qBittorrent configured to
// bypass authentication for localhost, which is where the agent always is.
func TestNoCredentialsMeansNoLogin(t *testing.T) {
	var logins atomic.Int32
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "login") {
			logins.Add(1)
		}
		writeTransferInfo(w, "connected")
	})

	if h := health(t, newAdapter(t, server.URL, adapters.Deps{})); h.Status != domain.StatusHealthy {
		t.Fatalf("status = %v, want healthy", h.Status)
	}
	if got := logins.Load(); got != 0 {
		t.Errorf("attempted %d logins with no credentials configured, want 0", got)
	}
}

// TestLogsInBeforeFirstRequest covers the credentialled path end to end.
func TestLogsInBeforeFirstRequest(t *testing.T) {
	var sawLogin, sawSID atomic.Bool
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == loginPath {
			sawLogin.Store(true)
			if err := r.ParseForm(); err != nil {
				t.Errorf("login body was not a form: %v", err)
			}
			if r.Form.Get("username") != testUsername || r.Form.Get("password") != testPassword {
				t.Errorf("login sent %q/%q", r.Form.Get("username"), r.Form.Get("password"))
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: testSID})
			_, _ = w.Write([]byte("Ok."))
			return
		}
		if c, err := r.Cookie("SID"); err == nil && c.Value == testSID {
			sawSID.Store(true)
		}
		writeTransferInfo(w, "connected")
	})

	if h := health(t, newAuthenticatedAdapter(t, server.URL)); h.Status != domain.StatusHealthy {
		t.Fatalf("status = %v, want healthy", h.Status)
	}
	if !sawLogin.Load() {
		t.Error("never logged in despite credentials being configured")
	}
	if !sawSID.Load() {
		t.Error("the session cookie was not sent on the health request")
	}
}

// TestExpiredSessionIsRetriedNotReported is the distinction that matters operationally.
//
// A cookie has a lifetime; an idle agent will outlive one. Reporting that as an auth
// failure would send the operator to check a password that was never wrong.
func TestExpiredSessionIsRetriedNotReported(t *testing.T) {
	var logins atomic.Int32
	var served atomic.Int32
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == loginPath {
			logins.Add(1)
			http.SetCookie(w, &http.Cookie{
				Name: "SID", Value: fmt.Sprintf("sid-%d", logins.Load()),
			})
			_, _ = w.Write([]byte("Ok."))
			return
		}
		// The first authenticated request is rejected as though the session had aged out.
		if served.Add(1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		writeTransferInfo(w, "connected")
	})

	h := health(t, newAuthenticatedAdapter(t, server.URL))

	if h.Status != domain.StatusHealthy {
		t.Fatalf("status = %v, want healthy after a silent re-login: %+v", h.Status, h.Reasons)
	}
	if got := logins.Load(); got != 2 {
		t.Errorf("logged in %d times, want 2 (initial, then after the 403)", got)
	}
}

// TestBadCredentialsAreDegradedNotUnreachable — qBittorrent answers 200 with "Fails." for
// a wrong password, so a login that only checked the status code would report success and
// then loop on every later 403.
func TestBadCredentialsAreDegradedNotUnreachable(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == loginPath {
			_, _ = w.Write([]byte("Fails."))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})

	h := health(t, newAuthenticatedAdapter(t, server.URL))

	if h.Status != domain.StatusDegraded {
		t.Errorf("status = %v, want degraded", h.Status)
	}
	if !h.Reachable {
		t.Error("reachable = false, but qBittorrent answered — the credential is the problem")
	}
	if !hasReason(h, domain.ReasonAuthFailed) {
		t.Errorf("want auth_failed, got %+v", h.Reasons)
	}
}

// TestForbiddenWithoutCredentialsNamesTheSetting — the operator is relying on the
// localhost bypass and it is off. Saying which switch to flip beats "check your
// credentials" when there are none to check.
func TestForbiddenWithoutCredentialsNamesTheSetting(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

	if h.Status != domain.StatusDegraded {
		t.Errorf("status = %v, want degraded", h.Status)
	}
	if !strings.Contains(h.Reasons[0].Message, "Bypass authentication") {
		t.Errorf("message should name the qBittorrent setting, got %q", h.Reasons[0].Message)
	}
}

// TestLoginWithoutCookieFails — a 200 that is not "Fails." but carries no cookie would
// otherwise leave every later request going out unauthenticated.
func TestLoginWithoutCookieFails(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == loginPath {
			_, _ = w.Write([]byte("Ok."))
			return
		}
		writeTransferInfo(w, "connected")
	})

	h := health(t, newAuthenticatedAdapter(t, server.URL))
	if h.Status != domain.StatusDegraded || !hasReason(h, domain.ReasonAuthFailed) {
		t.Errorf("want degraded/auth_failed, got %v %+v", h.Status, h.Reasons)
	}
}

// ---------------------------------------------------------------- transport failures

func TestUpstreamErrorStatuses(t *testing.T) {
	for _, code := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusTeapot} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			})
			h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

			if h.Status != domain.StatusDegraded {
				t.Errorf("status = %v, want degraded", h.Status)
			}
			if !hasReason(h, domain.ReasonUpstreamError) {
				t.Errorf("want upstream_error, got %+v", h.Reasons)
			}
		})
	}
}

func TestMalformedResponseIsDegradedNotHealthy(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("<html>not qBittorrent</html>"))
	})
	h := health(t, newAdapter(t, server.URL, adapters.Deps{}))

	if h.Status != domain.StatusDegraded {
		t.Errorf("status = %v, want degraded", h.Status)
	}
	if !hasReason(h, domain.ReasonInvalidResponse) {
		t.Errorf("want invalid_response, got %+v", h.Reasons)
	}
}

func TestUnreachableHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close() // nothing is listening now

	h := health(t, newAdapter(t, url, adapters.Deps{}))

	if h.Status != domain.StatusUnreachable {
		t.Errorf("status = %v, want unreachable", h.Status)
	}
	if h.Reachable {
		t.Error("reachable = true for a closed port")
	}
	if !hasReason(h, domain.ReasonUnreachable) {
		t.Errorf("want unreachable, got %+v", h.Reasons)
	}
}

func TestTimeoutIsDistinctFromRefused(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	svc := newAdapter(t, server.URL, adapters.Deps{})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	h, err := svc.Health(ctx)
	if err != nil {
		t.Fatalf("Health returned an error rather than an opinion: %v", err)
	}
	if !hasReason(h, domain.ReasonTimeout) {
		t.Errorf("a hung service must read as timeout, not refused: %+v", h.Reasons)
	}
}

// TestErrorsDoNotLeakCredentials — net/http embeds the request URL in transport errors,
// and these messages reach an API response and the log.
func TestErrorsDoNotLeakCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	h := health(t, newAuthenticatedAdapter(t, url))

	for _, r := range h.Reasons {
		if strings.Contains(r.Message, testPassword) {
			t.Errorf("reason leaked the password: %q", r.Message)
		}
	}
}

// ---------------------------------------------------------------- capabilities

// TestControlIsAdvertisedOnlyWhenPerformable is ADR-0005's rule: the advertised capability
// must be true. A service with no unit, or an agent with no host layer, must not be the
// type that claims it can be restarted.
func TestControlIsAdvertisedOnlyWhenPerformable(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeTransferInfo(w, "connected")
	})

	cases := []struct {
		name string
		unit string
		with bool
		want bool
	}{
		{"unit and host layer", "qbittorrent.service", true, true},
		{"no unit", "", true, false},
		{"no host layer", "qbittorrent.service", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := adapters.Deps{}
			if tc.with {
				deps.Units = &fakeUnits{}
			}
			svc, err := New(config.Service{
				ID: "qbittorrent", Type: Type, BaseURL: server.URL, Unit: tc.unit,
			}, deps)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, got := svc.(adapters.Controllable); got != tc.want {
				t.Errorf("Controllable = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLifecycleActionsComeFromTheSharedDescriptors is the M3.4 claim in a test: this
// adapter contributes copy, not mechanism. Start/Stop availability, risk levels and the
// hold-to-confirm classification are all decided in internal/adapters.
func TestLifecycleActionsComeFromTheSharedDescriptors(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeTransferInfo(w, "connected")
	})

	cases := []struct {
		name  string
		state host.UnitState
		want  []string
	}{
		{"running", host.UnitState{ActiveState: "active"}, []string{"restart", "stop"}},
		{"stopped", host.UnitState{ActiveState: "inactive"}, []string{"start"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			units := &fakeUnits{state: tc.state}
			svc, err := New(config.Service{
				ID: "qbittorrent", Type: Type, BaseURL: server.URL,
				Unit: "qbittorrent.service",
			}, adapters.Deps{Units: units})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			actions := svc.(adapters.Controllable).Actions(t.Context())
			ids := make([]string, 0, len(actions))
			for _, a := range actions {
				ids = append(ids, a.ID)
			}
			slices.Sort(ids)
			slices.Sort(tc.want)
			if !slices.Equal(ids, tc.want) {
				t.Errorf("actions = %v, want %v", ids, tc.want)
			}

			for _, a := range actions {
				if a.ID == "stop" {
					if a.Risk != domain.RiskDestructive {
						t.Errorf("stop risk = %v, want destructive", a.Risk)
					}
					if !strings.Contains(a.Description, "qBittorrent") {
						t.Errorf("stop description should name the service: %q", a.Description)
					}
					if !strings.Contains(a.Description, "paused") {
						t.Errorf("stop description should state the consequence: %q", a.Description)
					}
				}
			}
		})
	}
}

func TestInvokeDelegatesToHostLayer(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeTransferInfo(w, "connected")
	})
	units := &fakeUnits{state: host.UnitState{ActiveState: "active"}}
	svc, err := New(config.Service{
		ID: "qbittorrent", Type: Type, BaseURL: server.URL, Unit: "qbittorrent.service",
	}, adapters.Deps{Units: units})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := svc.(adapters.Controllable).Invoke(t.Context(), "restart"); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !slices.Equal(units.restarted, []string{"qbittorrent.service"}) {
		t.Errorf("restarted = %v; the adapter must go through the host layer", units.restarted)
	}
}

func TestInvokeRejectsUnknownAction(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeTransferInfo(w, "connected")
	})
	svc, err := New(config.Service{
		ID: "qbittorrent", Type: Type, BaseURL: server.URL, Unit: "qbittorrent.service",
	}, adapters.Deps{Units: &fakeUnits{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := svc.(adapters.Controllable).Invoke(t.Context(), "delete-everything"); err == nil {
		t.Fatal("an unknown action must be refused before it reaches the host layer")
	}
}

// TestWebUIIsAdvertisedOnlyWhenConfigured — the capability declares that something exists,
// so advertising it without a destination would make a client offer to open nothing.
func TestWebUIIsAdvertisedOnlyWhenConfigured(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeTransferInfo(w, "connected")
	})

	bare := newAdapter(t, server.URL, adapters.Deps{})
	if _, configured := bare.(adapters.WebUIProvider).WebUI(); configured {
		t.Error("advertised a web UI with none configured")
	}

	svc, err := New(config.Service{
		ID: "qbittorrent", Type: Type, BaseURL: server.URL,
		WebUI: &config.WebUI{Port: 8080},
	}, adapters.Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ui, configured := svc.(adapters.WebUIProvider).WebUI()
	if !configured {
		t.Fatal("a configured web_ui was not advertised")
	}
	// Defaults applied by config, not invented here.
	if ui.Scheme != "http" || ui.Port != 8080 || ui.Path != "/" {
		t.Errorf("web_ui = %+v, want http/8080//", ui)
	}
}

// ---------------------------------------------------------------- construction

func TestFactoryValidatesItsOwnRequirements(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Service
	}{
		{"no base_url", config.Service{ID: "qbittorrent", Type: Type}},
		{"relative base_url", config.Service{ID: "qbittorrent", Type: Type, BaseURL: "127.0.0.1:8080"}},
		{
			"password without username",
			config.Service{ID: "qbittorrent", Type: Type, BaseURL: "http://127.0.0.1:8080", Password: "x"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg, adapters.Deps{}); err == nil {
				t.Fatal("expected the factory to refuse this configuration")
			}
		})
	}
}

// TestCredentialsAreOptional — the localhost-bypass deployment must construct cleanly.
func TestCredentialsAreOptional(t *testing.T) {
	if _, err := New(config.Service{
		ID: "qbittorrent", Type: Type, BaseURL: "http://127.0.0.1:8080",
	}, adapters.Deps{}); err != nil {
		t.Fatalf("credentials must be optional: %v", err)
	}
}

func TestNameFallsBackToID(t *testing.T) {
	svc, err := New(config.Service{
		ID: "qbittorrent", Type: Type, BaseURL: "http://127.0.0.1:8080",
	}, adapters.Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.Name() != "qbittorrent" {
		t.Errorf("Name() = %q, want the id", svc.Name())
	}
}

func TestTrailingSlashInBaseURL(t *testing.T) {
	var path string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		writeTransferInfo(w, "connected")
	})

	svc, err := New(config.Service{
		ID: "qbittorrent", Type: Type, BaseURL: server.URL + "/",
	}, adapters.Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	health(t, svc)

	if path != transferInfoPath {
		t.Errorf("path = %q, want %q — a trailing slash must not double up", path, transferInfoPath)
	}
}

func TestIdentity(t *testing.T) {
	svc, err := New(config.Service{
		ID: "qb-attic", Name: "Attic qBittorrent", Type: Type,
		BaseURL: "http://127.0.0.1:8080",
	}, adapters.Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.ID() != "qb-attic" || svc.Name() != "Attic qBittorrent" {
		t.Errorf("identity = %q/%q", svc.ID(), svc.Name())
	}
}

// TestHealthNeverReturnsAnError — "I could not connect" is information, not a failure, and
// the poller depends on that distinction rather than parsing error strings.
func TestHealthNeverReturnsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	if _, err := newAdapter(t, url, adapters.Deps{}).Health(t.Context()); err != nil {
		var target *host.Job
		_ = target
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Health returned an error for an unreachable service: %v", err)
		}
	}
}
