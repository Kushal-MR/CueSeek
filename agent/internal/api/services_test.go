package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// Shapes for decoding service responses without importing the generated types into a
// test that is meant to check the wire format a client actually sees.

type wireHealth struct {
	Status         string `json:"status"`
	Reachable      bool   `json:"reachable"`
	ReportedStatus string `json:"reported_status"`
	ObservedAt     string `json:"observed_at"`
	Reasons        []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"reasons"`
}

type wireService struct {
	Id           string     `json:"id"`
	Name         string     `json:"name"`
	Health       wireHealth `json:"health"`
	Capabilities []struct {
		Id    string `json:"id"`
		Label string `json:"label"`
	} `json:"capabilities"`
	Actions []struct {
		Id          string  `json:"id"`
		Label       string  `json:"label"`
		Risk        string  `json:"risk"`
		Description *string `json:"description"`
	} `json:"actions"`
	WebUi *struct {
		Scheme string  `json:"scheme"`
		Port   int     `json:"port"`
		Path   *string `json:"path"`
	} `json:"web_ui"`
}

type wireAction struct {
	ActionId   string `json:"action_id"`
	ServiceId  string `json:"service_id"`
	Action     string `json:"action"`
	Status     string `json:"status"`
	AcceptedAt string `json:"accepted_at"`
}

// waitFor polls a condition rather than sleeping a fixed amount.
//
// Actions resolve on a detached goroutine, so their effects — the adapter being invoked,
// the audit row appearing — land after the HTTP response. A fixed sleep would be either
// flaky or slow; this is neither.
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

// ---------------------------------------------------------------- reads

// TestListServicesServesFromCache is the ADR-0003 guarantee: a client request reads what
// the poller last observed and never triggers an upstream call. Here the cache is written
// directly and the response must reflect it exactly.
func TestListServicesServesFromCache(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	observed := time.Now().UTC().Truncate(time.Second)
	env.cache.Put("jellyfin", domain.Health{
		Status:     domain.StatusDegraded,
		Reachable:  true,
		ObservedAt: observed,
		Reasons: []domain.HealthReason{{
			Code: domain.ReasonAuthFailed, Message: "Jellyfin rejected the API key.",
		}},
	}, []domain.Action{{
		ID:          "restart",
		Label:       "Restart Jellyfin",
		Description: "Restarts the Jellyfin service.",
		Risk:        domain.RiskDisruptive,
	}})

	services := decode[[]wireService](t, env.do(t, http.MethodGet, "/v1/services", token, nil))
	if len(services) != 1 {
		t.Fatalf("len(services) = %d, want 1", len(services))
	}

	svc := services[0]
	if svc.Id != "jellyfin" || svc.Name != "Jellyfin" {
		t.Errorf("identity = %q/%q", svc.Id, svc.Name)
	}
	if svc.Health.Status != "degraded" {
		t.Errorf("status = %q, want the cached degraded", svc.Health.Status)
	}
	if !svc.Health.Reachable {
		t.Error("reachable was not carried through from the cache")
	}
	if len(svc.Health.Reasons) != 1 || svc.Health.Reasons[0].Code != domain.ReasonAuthFailed {
		t.Errorf("reasons = %v, want the cached auth_failed", svc.Health.Reasons)
	}
	// The observation time, not the response time — clients render staleness from this.
	if svc.Health.ObservedAt != observed.Format(time.RFC3339) {
		t.Errorf("observed_at = %q, want %q", svc.Health.ObservedAt, observed.Format(time.RFC3339))
	}
}

// TestServiceExposesDiscoveredCapabilitiesAndActions: the client renders per capability
// and gates per risk level, so both must survive the trip to the wire (ADR-0005/0007).
func TestServiceExposesDiscoveredCapabilitiesAndActions(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	// Actions travel with the observation now (ADR-0002 Amendment 1), because which ones
	// apply depends on the unit's state. A service the poller has not reached yet
	// therefore advertises none — the same window in which its health is Unknown, and
	// honest for the same reason: we do not yet know what state the unit is in.
	env.cache.Put("jellyfin", domain.Health{
		Status:     domain.StatusHealthy,
		Reachable:  true,
		ObservedAt: time.Now().UTC(),
	}, []domain.Action{{
		ID:          "restart",
		Label:       "Restart Jellyfin",
		Description: "Restarts the Jellyfin service.",
		Risk:        domain.RiskDisruptive,
	}})

	svc := decode[wireService](t, env.do(t, http.MethodGet, "/v1/services/jellyfin", token, nil))

	var health, control bool
	for _, c := range svc.Capabilities {
		if c.Label == "" {
			t.Errorf("capability %q has no label; a client that predates it renders an empty box", c.Id)
		}
		switch c.Id {
		case domain.CapabilityHealth.ID:
			health = true
		case domain.CapabilityControl.ID:
			control = true
		}
	}
	if !health || !control {
		t.Errorf("capabilities = %v, want health and control", svc.Capabilities)
	}

	if len(svc.Actions) != 1 {
		t.Fatalf("actions = %v, want one", svc.Actions)
	}
	action := svc.Actions[0]
	if action.Id != "restart" || action.Risk != string(domain.RiskDisruptive) {
		t.Errorf("action = %+v", action)
	}
	if action.Description == nil || *action.Description == "" {
		t.Error("action has no description; a confirmation dialog has nothing to show")
	}
}

func TestGetUnknownServiceIs404(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	resp := env.do(t, http.MethodGet, "/v1/services/nope", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestStaleCacheIsReportedAsUnknown: showing stale green while the agent cannot reach a
// service is worse than showing nothing (ADR-0008). The API must inherit that, not just
// the cache.
func TestStaleCacheIsReportedAsUnknown(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	// Observed two hours ago, against a one-minute tolerance.
	env.cache.Put("jellyfin", domain.Health{
		Status:     domain.StatusHealthy,
		Reachable:  true,
		ObservedAt: time.Now().UTC().Add(-2 * time.Hour),
	}, []domain.Action{{
		ID:          "restart",
		Label:       "Restart Jellyfin",
		Description: "Restarts the Jellyfin service.",
		Risk:        domain.RiskDisruptive,
	}})

	svc := decode[wireService](t, env.do(t, http.MethodGet, "/v1/services/jellyfin", token, nil))
	if svc.Health.Status != "unknown" {
		t.Errorf("status = %q, want unknown for a stale observation", svc.Health.Status)
	}
	if len(svc.Health.Reasons) == 0 || svc.Health.Reasons[0].Code != domain.ReasonStale {
		t.Errorf("reasons = %v, want stale", svc.Health.Reasons)
	}
}

// ---------------------------------------------------------------- actions

// TestInvokeActionReturns202WithID is ADR-0004's core shape: the agent has no synchronous
// result to give, because systemd's RestartUnit returns once the job is queued.
func TestInvokeActionReturns202WithID(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeServiceControl)

	resp := env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", token, nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}

	body := decode[wireAction](t, resp)
	if body.ActionId == "" {
		t.Error("no action_id returned; nothing correlates this with a later stream event")
	}
	if body.ServiceId != "jellyfin" || body.Action != "restart" {
		t.Errorf("body = %+v", body)
	}
	if body.Status != "running" && body.Status != "pending" {
		t.Errorf("status = %q, want a non-terminal state", body.Status)
	}
	if body.AcceptedAt == "" {
		t.Error("accepted_at is empty")
	}

	// The action reached the adapter, which is what routes it to the host layer.
	waitFor(t, "the adapter to be invoked", func() bool {
		return len(env.adapter.invocations()) == 1
	})
	if got := env.adapter.invocations(); got[0] != "restart" {
		t.Errorf("adapter invoked with %q", got[0])
	}
}

// TestActionIsAudited: an operations console that can restart services should be able to
// answer "who did that, and when".
func TestActionIsAudited(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeServiceControl)

	env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", token, nil)

	waitFor(t, "the audit entry", func() bool {
		entries, err := env.store.ListAudit(t.Context(), 50)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.Action == "service.action" && e.Target == "jellyfin" && e.DeviceName == "Phone" {
				return true
			}
		}
		return false
	})
}

// TestInvokeRequiresServiceControlScope: the scope check happens in the agent, so it
// holds regardless of what a client's UI offers (ADR-0006).
func TestInvokeRequiresServiceControlScope(t *testing.T) {
	env := newTestEnv(t)
	readOnly := env.pairDevice(t, "Read Only", domain.ScopeRead)

	resp := env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", readOnly, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	// Refused before any business logic ran, so the adapter was never asked.
	if got := env.adapter.invocations(); len(got) != 0 {
		t.Errorf("adapter was invoked despite a missing scope: %v", got)
	}
}

func TestInvokeUnknownServiceOrAction(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeServiceControl)

	for name, path := range map[string]string{
		"unknown service": "/v1/services/nope/actions/restart",
		"unknown action":  "/v1/services/jellyfin/actions/self-destruct",
	} {
		t.Run(name, func(t *testing.T) {
			resp := env.do(t, http.MethodPost, path, token, nil)
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("status = %d, want 404", resp.StatusCode)
			}
		})
	}
	// An action absent from the descriptor list must never reach the adapter, or a client
	// could invoke behaviour that never appeared in the list its UI gates on.
	if got := env.adapter.invocations(); len(got) != 0 {
		t.Errorf("adapter invoked for an unadvertised action: %v", got)
	}
}

// TestDuplicateActionIsRejected is the contract's 409: "not currently possible, e.g.
// already in progress". Queuing a second restart behind the first is never what somebody
// tapping twice wanted.
func TestDuplicateActionIsRejected(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeServiceControl)

	// Hold the first invocation open so it is genuinely still running.
	block := make(chan struct{})
	env.adapter.mu.Lock()
	env.adapter.block = block
	env.adapter.mu.Unlock()

	first := make(chan int, 1)
	go func() {
		req, err := http.NewRequest(http.MethodPost,
			env.server.URL+"/v1/services/jellyfin/actions/restart", nil)
		if err != nil {
			first <- 0
			return
		}
		// Authenticated: an unauthenticated request would be refused at the middleware
		// and never reach the adapter, so nothing would be "in progress" to collide with.
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := env.server.Client().Do(req)
		if err != nil {
			first <- 0
			return
		}
		defer resp.Body.Close()
		first <- resp.StatusCode
	}()

	// Wait until the adapter is actually inside Invoke.
	waitFor(t, "the first action to start", func() bool {
		return len(env.adapter.invocations()) == 1
	})

	resp := env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", token, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("second action: status = %d, want 409", resp.StatusCode)
	}
	problem := decode[map[string]any](t, resp)
	if title, _ := problem["title"].(string); title != "Action already in progress" {
		t.Errorf("title = %q", title)
	}

	close(block)
	<-first
}

// TestHostFailuresMapToActionableResponses: a polkit refusal or an unlisted unit is a
// host misconfiguration the operator can fix. A bare 500 would send them hunting for a
// bug; a 403 would blame the client's token.
func TestHostFailuresMapToActionableResponses(t *testing.T) {
	cases := map[string]error{
		"unlisted unit":        host.ErrUnitNotManaged,
		"polkit refused":       host.ErrUnauthorized,
		"unit absent":          host.ErrUnitNotFound,
		"unsupported platform": host.ErrUnsupportedPlatform,
	}
	for name, hostErr := range cases {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(t)
			token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeServiceControl)
			env.adapter.failWith(hostErr)

			resp := env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", token, nil)
			if resp.StatusCode != http.StatusConflict {
				t.Fatalf("status = %d, want 409", resp.StatusCode)
			}

			problem := decode[map[string]any](t, resp)
			detail, _ := problem["detail"].(string)
			if detail == "" {
				t.Error("no detail; the operator cannot tell what to fix")
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q", ct)
			}
		})
	}
}

// TestFailedActionIsAuditedAsFailed: the audit trail must distinguish an action that was
// accepted and worked from one that was accepted and did not.
func TestFailedActionIsAuditedAsFailed(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeServiceControl)
	env.adapter.failWith(errors.New("boom"))

	env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", token, nil)

	waitFor(t, "a failed audit entry", func() bool {
		entries, _ := env.store.ListAudit(t.Context(), 50)
		for _, e := range entries {
			if e.Action == "service.action" && e.Outcome == domain.OutcomeFailed {
				return true
			}
		}
		return false
	})
}

// ---------------------------------------------------------------- overall health

// TestOverallHealthIsComputedByTheAgent covers ADR-0008's reason for existing: the agent
// decides, so the phone and the watch cannot disagree.
func TestOverallHealthIsComputedByTheAgent(t *testing.T) {
	env := newTestEnv(t)

	// Before any observation.
	if got := env.api.OverallHealth().Status; got != domain.StatusUnknown {
		t.Errorf("status = %q before polling, want unknown", got)
	}

	env.cache.Put("jellyfin", domain.Health{
		Status: domain.StatusHealthy, Reachable: true, ObservedAt: time.Now().UTC(),
	}, []domain.Action{{
		ID:          "restart",
		Label:       "Restart Jellyfin",
		Description: "Restarts the Jellyfin service.",
		Risk:        domain.RiskDisruptive,
	}})
	if got := env.api.OverallHealth().Status; got != domain.StatusHealthy {
		t.Errorf("status = %q with a healthy service, want healthy", got)
	}

	env.cache.Put("jellyfin", domain.Health{
		Status: domain.StatusUnreachable, ObservedAt: time.Now().UTC(),
		Reasons: []domain.HealthReason{{Code: domain.ReasonUnreachable, Message: "refused"}},
	}, []domain.Action{{
		ID:          "restart",
		Label:       "Restart Jellyfin",
		Description: "Restarts the Jellyfin service.",
		Risk:        domain.RiskDisruptive,
	}})
	overall := env.api.OverallHealth()
	if overall.Status != domain.StatusUnreachable {
		t.Errorf("status = %q with the only service unreachable, want unreachable", overall.Status)
	}
	if len(overall.Reasons) == 0 {
		t.Error("no reasons; 'unreachable' alone is not actionable")
	}
}

// TestWebUIIsAdvertisedAsCapabilityAndPayload covers the pairing this contract already
// uses for control/actions: the capability declares that something exists, the sibling
// field carries what a client needs.
//
// A client keys its renderer map on the capability id, so a payload without the capability
// would be invisible, and a capability without the payload would offer to open nothing.
func TestWebUIIsAdvertisedAsCapabilityAndPayload(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	svc := decode[wireService](t, env.do(t, http.MethodGet, "/v1/services/jellyfin", token, nil))

	var advertised bool
	for _, c := range svc.Capabilities {
		if c.Id == domain.CapabilityWebUI.ID {
			advertised = true
			if c.Label == "" {
				t.Error("web_ui capability has no label; a client that predates it renders an empty box")
			}
		}
	}
	if !advertised {
		t.Fatalf("capabilities = %v, want web_ui", svc.Capabilities)
	}
	if svc.WebUi == nil {
		t.Fatal("web_ui capability advertised with no payload to open")
	}
	if svc.WebUi.Port == 0 {
		t.Error("web_ui payload has no port")
	}
	if svc.WebUi.Scheme != "http" && svc.WebUi.Scheme != "https" {
		t.Errorf("web_ui scheme = %q, must be http or https", svc.WebUi.Scheme)
	}
}

// TestWebUIPayloadCarriesNoOrigin is the wire-level half of the security property.
//
// The agent supplies parts and no host. If a host, an authority or a full URL ever
// appeared here, a client composing a URL would be following the agent to an origin the
// operator never configured.
func TestWebUIPayloadCarriesNoOrigin(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	var payload struct {
		WebUI map[string]any `json:"web_ui"`
	}
	resp := env.do(t, http.MethodGet, "/v1/services/jellyfin", token, nil)
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for key := range payload.WebUI {
		switch key {
		case "scheme", "port", "path":
		default:
			t.Errorf("web_ui carries unexpected field %q; the agent must not supply an origin", key)
		}
	}
	if path, ok := payload.WebUI["path"].(string); ok {
		if strings.Contains(path, "://") || strings.HasPrefix(path, "//") {
			t.Errorf("path %q would override the host a client composed", path)
		}
	}
}
