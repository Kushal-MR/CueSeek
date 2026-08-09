package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// These tests drive the real handler chain over httptest: real router, real
// authentication, real authorization, real database. Nothing is mocked.
//
// The behaviour worth testing here is the interaction between those layers — that an
// unauthenticated request never reaches a handler, that a scope check happens before
// business logic. Unit-testing the pieces separately would assert that each works alone
// and prove nothing about the chain.

type testEnv struct {
	server   *httptest.Server
	store    *store.Store
	api      *Server
	cache    *adapters.Cache
	adapter  *stubAdapter
	registry *adapters.Registry
}

// stubAdapter stands in for a real service. It implements Controllable so action tests
// exercise the real dispatch path, and records what it was asked to do.
type stubAdapter struct {
	id, name string

	mu        sync.Mutex
	invoked   []string
	invokeErr error
	block     chan struct{} // when non-nil, Invoke waits on it before returning
}

func (s *stubAdapter) ID() string   { return s.id }
func (s *stubAdapter) Name() string { return s.name }

func (s *stubAdapter) Health(context.Context) (domain.Health, error) {
	return domain.Health{Status: domain.StatusHealthy, Reachable: true}, nil
}

func (s *stubAdapter) Actions() []domain.Action {
	return []domain.Action{{
		ID: "restart", Label: "Restart Jellyfin",
		Description: "Restarts the service.", Risk: domain.RiskDisruptive,
	}}
}

func (s *stubAdapter) Invoke(_ context.Context, actionID string) (*host.Job, error) {
	s.mu.Lock()
	s.invoked = append(s.invoked, actionID)
	block, err := s.block, s.invokeErr
	s.mu.Unlock()

	if block != nil {
		<-block
	}
	return nil, err
}

func (s *stubAdapter) invocations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.invoked...)
}

func (s *stubAdapter) failWith(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invokeErr = err
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	stub := &stubAdapter{id: "jellyfin", name: "Jellyfin"}

	registry := adapters.NewRegistry()
	if err := registry.RegisterFactory("stub", func(cfg config.Service, _ adapters.Deps) (adapters.Service, error) {
		return stub, nil
	}); err != nil {
		t.Fatalf("RegisterFactory: %v", err)
	}
	cfg := config.Config{Services: []config.Service{
		{ID: "jellyfin", Name: "Jellyfin", Type: "stub", Unit: "jellyfin.service"},
	}}
	if err := registry.Build(cfg, adapters.Deps{}); err != nil {
		t.Fatalf("registry.Build: %v", err)
	}

	// A cache, not a poller: these tests control what the API sees rather than racing a
	// background goroutine to put it there.
	cache := adapters.NewCache()
	cache.Track("jellyfin", time.Minute)

	srv, err := New(Options{
		Store:        st,
		Registry:     registry,
		Cache:        cache,
		AgentVersion: "test",
		HostID:       "test-host",
	})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &testEnv{
		server: ts, store: st, api: srv,
		cache: cache, adapter: stub, registry: registry,
	}
}

// do issues a request. An empty token means no Authorization header at all, which is a
// different case from a present-but-invalid one.
func (e *testEnv) do(t *testing.T, method, path, token string, body any) *http.Response {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(t.Context(), method, e.server.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := e.server.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %T: %v", out, err)
	}
	return out
}

// pairDevice mints a code and redeems it, returning the device token.
func (e *testEnv) pairDevice(t *testing.T, name string, scopes ...domain.Scope) string {
	t.Helper()
	if len(scopes) == 0 {
		scopes = []domain.Scope{domain.ScopeRead, domain.ScopeServiceControl}
	}
	code, err := e.store.CreatePairingCode(t.Context(), scopes, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}

	resp := e.do(t, http.MethodPost, "/v1/pair", "", map[string]string{
		"code": code, "device_name": name, "platform": "cli",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("pair %s: status = %d, want 201", name, resp.StatusCode)
	}
	return decode[struct {
		Token string `json:"token"`
	}](t, resp).Token
}

// ---------------------------------------------------------------- authentication

// TestUnauthenticatedRequestsAreRejected is the core promise of ADR-0001. Transport
// security is delegated to the VPN, so this check is the only thing between a tailnet and
// an API that will be able to power off the machine.
func TestUnauthenticatedRequestsAreRejected(t *testing.T) {
	env := newTestEnv(t)

	protected := []struct{ method, path string }{
		{http.MethodGet, "/v1/system"},
		{http.MethodGet, "/v1/devices"},
		{http.MethodDelete, "/v1/devices/anything"},
		{http.MethodGet, "/v1/services"},
		{http.MethodGet, "/v1/services/jellyfin"},
		{http.MethodPost, "/v1/services/jellyfin/actions/restart"},
		{http.MethodGet, "/v1/stream"},
	}
	for _, tc := range protected {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := env.do(t, tc.method, tc.path, "", nil)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", ct)
			}
		})
	}
}

func TestMalformedCredentialsRejected(t *testing.T) {
	env := newTestEnv(t)
	valid := env.pairDevice(t, "Phone")

	cases := map[string]string{
		"garbage token":  "not-a-real-token",
		"truncated":      valid[:len(valid)-1],
		"extra char":     valid + "x",
		"revoked-shaped": "csk_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if resp := env.do(t, http.MethodGet, "/v1/system", token, nil); resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}

// TestAuthorizationSchemeIsCaseInsensitive: RFC 7235 says the scheme is case-insensitive,
// and clients in the wild send all three spellings.
func TestAuthorizationSchemeIsCaseInsensitive(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone")

	for _, scheme := range []string{"Bearer", "bearer", "BEARER"} {
		t.Run(scheme, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
				env.server.URL+"/v1/system", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Authorization", scheme+" "+token)
			resp, err := env.server.Client().Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200", resp.StatusCode)
			}
		})
	}
}

// ---------------------------------------------------------------- authorization

// TestScopeEnforcement is the guarantee ADR-0006 is sold on: a watch can restart a
// service but is structurally incapable of powering off the machine. Enforcement is in
// the agent, so it holds no matter what the client's UI offers.
func TestScopeEnforcement(t *testing.T) {
	env := newTestEnv(t)
	readOnly := env.pairDevice(t, "Read Only", domain.ScopeRead)

	// read is enough to look.
	if resp := env.do(t, http.MethodGet, "/v1/services", readOnly, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /v1/services with read scope: status = %d, want 200", resp.StatusCode)
	}

	// service.control is not held, so the action is refused before any business logic.
	resp := env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", readOnly, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}

	problem := decode[map[string]any](t, resp)
	if problem["title"] != "Insufficient scope" {
		t.Errorf("title = %v", problem["title"])
	}
	detail, _ := problem["detail"].(string)
	if detail == "" {
		t.Error("403 carries no detail; a client cannot tell the user what is missing")
	}
}

// TestDeniedAttemptsAreAudited: refused attempts are the most interesting rows in the
// log, because they are how an operator learns a device is being used beyond its remit.
func TestDeniedAttemptsAreAudited(t *testing.T) {
	env := newTestEnv(t)
	readOnly := env.pairDevice(t, "Read Only", domain.ScopeRead)

	env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", readOnly, nil)

	entries, err := env.store.ListAudit(t.Context(), 50)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	var found bool
	for _, e := range entries {
		if e.Outcome == domain.OutcomeDenied && e.DeviceName == "Read Only" {
			found = true
		}
	}
	if !found {
		t.Error("denied attempt was not recorded in the audit log")
	}
}

// TestEveryOperationHasARequirement re-checks at runtime what the spec test checks at
// build time. Cheap, and it covers the case where the embedded spec and the generated
// router disagree — which would otherwise mean an endpoint routed but unauthorised.
func TestEveryOperationHasARequirement(t *testing.T) {
	reqs, err := loadRequirements()
	if err != nil {
		t.Fatalf("loadRequirements: %v", err)
	}
	if len(reqs) < 6 {
		t.Fatalf("only %d requirements loaded; the contract defines more", len(reqs))
	}

	// The generated middleware passes Go method names; the contract declares camelCase.
	// This is the mismatch that would silently break authorization, so assert the
	// lookup tolerates it.
	for _, operationID := range []string{"ListDevices", "PairDevice", "InvokeServiceAction"} {
		if _, ok := reqs.lookup(operationID); !ok {
			t.Errorf("no requirement found for generated operation id %q", operationID)
		}
	}

	pair, ok := reqs.lookup("PairDevice")
	if !ok || !pair.public {
		t.Error("PairDevice must be public; nothing else can pair a first device")
	}
	action, ok := reqs.lookup("InvokeServiceAction")
	if !ok || action.scope != domain.ScopeServiceControl {
		t.Errorf("InvokeServiceAction scope = %q, want service.control", action.scope)
	}

	// Pinned deliberately. Revocation is the most destructive operation in the API, and
	// the whole point of devices.manage is that it cannot be reached by a device holding
	// only the scopes a watch routinely carries.
	revoke, ok := reqs.lookup("RevokeDevice")
	if !ok || revoke.scope != domain.ScopeDevicesManage {
		t.Errorf("RevokeDevice scope = %q, want devices.manage", revoke.scope)
	}
	list, ok := reqs.lookup("ListDevices")
	if !ok || list.scope != domain.ScopeRead {
		t.Errorf("ListDevices scope = %q, want read", list.scope)
	}
}

// ---------------------------------------------------------------- pairing

func TestPairingFlow(t *testing.T) {
	env := newTestEnv(t)
	code, err := env.store.CreatePairingCode(t.Context(),
		[]domain.Scope{domain.ScopeRead}, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}

	resp := env.do(t, http.MethodPost, "/v1/pair", "", map[string]string{
		"code": code, "device_name": "Pixel 8", "platform": "android",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	body := decode[struct {
		Token  string `json:"token"`
		Device struct {
			Id       string   `json:"id"`
			Name     string   `json:"name"`
			Platform string   `json:"platform"`
			Scopes   []string `json:"scopes"`
		} `json:"device"`
	}](t, resp)

	if body.Token == "" {
		t.Fatal("no token returned")
	}
	if body.Device.Name != "Pixel 8" || body.Device.Platform != "android" {
		t.Errorf("device = %+v", body.Device)
	}
	if len(body.Device.Scopes) != 1 || body.Device.Scopes[0] != "read" {
		t.Errorf("scopes = %v, want [read]", body.Device.Scopes)
	}

	// The token works immediately.
	if resp := env.do(t, http.MethodGet, "/v1/system", body.Token, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("issued token rejected: status = %d", resp.StatusCode)
	}
}

func TestPairingRejectsBadCodes(t *testing.T) {
	env := newTestEnv(t)
	for name, code := range map[string]string{
		"unknown": "AAAA-AAAA",
		"empty":   "",
		"garbage": "!!!!",
	} {
		t.Run(name, func(t *testing.T) {
			resp := env.do(t, http.MethodPost, "/v1/pair", "", map[string]string{
				"code": code, "device_name": "Phone", "platform": "cli",
			})
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("status = %d, want 403", resp.StatusCode)
			}
		})
	}
}

// TestPairingCodeCannotBeReused: the code is visible on a screen, so it must be spent the
// moment it is used.
func TestPairingCodeCannotBeReused(t *testing.T) {
	env := newTestEnv(t)
	code, err := env.store.CreatePairingCode(t.Context(),
		[]domain.Scope{domain.ScopeRead}, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	body := map[string]string{"code": code, "device_name": "Phone", "platform": "cli"}

	if resp := env.do(t, http.MethodPost, "/v1/pair", "", body); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first pair: status = %d, want 201", resp.StatusCode)
	}
	if resp := env.do(t, http.MethodPost, "/v1/pair", "", body); resp.StatusCode != http.StatusForbidden {
		t.Errorf("second pair: status = %d, want 403", resp.StatusCode)
	}
}

// TestPairingIsRateLimited: a pairing code carries 40 bits, which is only safe because
// guessing is slow. This is the mechanism that makes it slow.
func TestPairingIsRateLimited(t *testing.T) {
	env := newTestEnv(t)
	body := map[string]string{"code": "AAAA-AAAA", "device_name": "Attacker", "platform": "cli"}

	var sawRateLimit bool
	for i := range pairAttemptLimit + 5 {
		resp := env.do(t, http.MethodPost, "/v1/pair", "", body)
		if resp.StatusCode == http.StatusTooManyRequests {
			sawRateLimit = true
			break
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("attempt %d: status = %d, want 403 or 429", i, resp.StatusCode)
		}
	}
	if !sawRateLimit {
		t.Errorf("no 429 after %d attempts; guessing is not being slowed", pairAttemptLimit+5)
	}
}

func TestPairingRejectsEmptyDeviceName(t *testing.T) {
	env := newTestEnv(t)
	code, err := env.store.CreatePairingCode(t.Context(),
		[]domain.Scope{domain.ScopeRead}, time.Minute)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	resp := env.do(t, http.MethodPost, "/v1/pair", "", map[string]string{
		"code": code, "device_name": "", "platform": "cli",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// ---------------------------------------------------------------- devices

func TestListAndRevokeDevices(t *testing.T) {
	env := newTestEnv(t)
	admin := env.pairDevice(t, "Admin", domain.ScopeRead, domain.ScopeDevicesManage)
	victimToken := env.pairDevice(t, "Old Phone")

	devices := decode[[]struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}](t, env.do(t, http.MethodGet, "/v1/devices", admin, nil))
	if len(devices) != 2 {
		t.Fatalf("len(devices) = %d, want 2", len(devices))
	}

	var victimID string
	for _, d := range devices {
		if d.Name == "Old Phone" {
			victimID = d.Id
		}
	}
	if victimID == "" {
		t.Fatal("could not find the device to revoke")
	}

	resp := env.do(t, http.MethodDelete, "/v1/devices/"+victimID, admin, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: status = %d, want 204", resp.StatusCode)
	}

	// The revoked token stops working immediately; the revoking device is unaffected.
	if resp := env.do(t, http.MethodGet, "/v1/system", victimToken, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("revoked token still works: status = %d", resp.StatusCode)
	}
	if resp := env.do(t, http.MethodGet, "/v1/system", admin, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("revoking device was affected: status = %d", resp.StatusCode)
	}
}

func TestRevokeUnknownDevice(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeDevicesManage)
	resp := env.do(t, http.MethodDelete, "/v1/devices/nope", token, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestRevokeRequiresDevicesManage is the reason devices.manage exists as its own scope.
//
// A watch is routinely paired with read + service.control so it can restart a service
// from your wrist. It must not be able to revoke the phone — revocation can lock every
// remaining device out of the agent, including the one you would use to fix it. Had
// revocation reused service.control, this test would fail and the watch would hold that
// power by default.
func TestRevokeRequiresDevicesManage(t *testing.T) {
	env := newTestEnv(t)

	manager := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeDevicesManage)
	watch := env.pairDevice(t, "Watch", domain.ScopeRead, domain.ScopeServiceControl)

	devices := decode[[]struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}](t, env.do(t, http.MethodGet, "/v1/devices", watch, nil))

	var phoneID string
	for _, d := range devices {
		if d.Name == "Phone" {
			phoneID = d.Id
		}
	}
	if phoneID == "" {
		t.Fatal("could not find the target device")
	}

	// The watch can see the device list — that is `read` — but cannot revoke.
	resp := env.do(t, http.MethodDelete, "/v1/devices/"+phoneID, watch, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("watch revoked a device: status = %d, want 403", resp.StatusCode)
	}
	problem := decode[map[string]any](t, resp)
	if detail, _ := problem["detail"].(string); !strings.Contains(detail, "devices.manage") {
		t.Errorf("403 detail does not name the missing scope: %q", detail)
	}

	// The phone was not affected by the failed attempt.
	if resp := env.do(t, http.MethodGet, "/v1/system", manager, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("target device was affected by a denied revocation: %d", resp.StatusCode)
	}

	// A device holding the scope succeeds.
	watchID := ""
	for _, d := range devices {
		if d.Name == "Watch" {
			watchID = d.Id
		}
	}
	if resp := env.do(t, http.MethodDelete, "/v1/devices/"+watchID, manager, nil); resp.StatusCode != http.StatusNoContent {
		t.Errorf("devices.manage holder could not revoke: status = %d", resp.StatusCode)
	}
}

// TestSelfRevocationIsPermitted: a client logging itself out is the ordinary case, and
// still requires the scope.
func TestSelfRevocationIsPermitted(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeDevicesManage)

	devices := decode[[]struct {
		Id string `json:"id"`
	}](t, env.do(t, http.MethodGet, "/v1/devices", token, nil))
	if len(devices) != 1 {
		t.Fatalf("len = %d, want 1", len(devices))
	}

	if resp := env.do(t, http.MethodDelete, "/v1/devices/"+devices[0].Id, token, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("self-revocation: status = %d, want 204", resp.StatusCode)
	}
	if resp := env.do(t, http.MethodGet, "/v1/system", token, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("token still works after self-revocation: %d", resp.StatusCode)
	}
}

// ---------------------------------------------------------------- system & services

func TestGetSystem(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone")

	sys := decode[struct {
		HostId       string `json:"host_id"`
		Hostname     string `json:"hostname"`
		AgentVersion string `json:"agent_version"`
		ApiVersion   string `json:"api_version"`
	}](t, env.do(t, http.MethodGet, "/v1/system", token, nil))

	if sys.HostId != "test-host" {
		t.Errorf("host_id = %q", sys.HostId)
	}
	if sys.AgentVersion != "test" || sys.ApiVersion != APIVersion {
		t.Errorf("versions = %q / %q", sys.AgentVersion, sys.ApiVersion)
	}
	if sys.Hostname == "" {
		t.Error("hostname is empty")
	}
}

// TestServicesReportUnknownBeforePolling: the service is registered and tracked, but the
// poller has not observed it yet. ADR-0008 makes `unknown` a first-class state precisely
// so the agent never invents a status it has not observed.
func TestServicesReportUnknownBeforePolling(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone")

	services := decode[[]struct {
		Id     string `json:"id"`
		Name   string `json:"name"`
		Health struct {
			Status    string `json:"status"`
			Reachable bool   `json:"reachable"`
			Reasons   []struct {
				Code string `json:"code"`
			} `json:"reasons"`
		} `json:"health"`
		Capabilities []any `json:"capabilities"`
		Actions      []any `json:"actions"`
	}](t, env.do(t, http.MethodGet, "/v1/services", token, nil))

	if len(services) != 1 {
		t.Fatalf("len(services) = %d, want 1", len(services))
	}
	svc := services[0]
	if svc.Id != "jellyfin" {
		t.Errorf("id = %q", svc.Id)
	}
	if svc.Health.Status != "unknown" {
		t.Errorf("status = %q, want unknown before the first poll", svc.Health.Status)
	}
	if svc.Health.Reachable {
		t.Error("reachable is true, but nothing has been reached")
	}
	if len(svc.Health.Reasons) == 0 {
		t.Error("no reason given for the unknown status")
	}
	// Empty arrays, not null: clients should not have to handle both.
	if svc.Capabilities == nil || svc.Actions == nil {
		t.Error("capabilities/actions serialised as null rather than []")
	}
}

func TestGetUnknownService(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone")
	if resp := env.do(t, http.MethodGet, "/v1/services/nope", token, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// The stream's own behaviour is covered in stream_test.go. A7 closed, so it is no longer
// a declared-but-unimplemented endpoint.

// ---------------------------------------------------------------- errors

// TestProblemResponsesAreWellFormed: the contract declares application/problem+json for
// every failure, so a client writes one error mapper. A single endpoint returning plain
// text would break that promise.
func TestProblemResponsesAreWellFormed(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	cases := map[string]struct {
		method, path, token string
		wantStatus          int
	}{
		"401 no token":   {http.MethodGet, "/v1/devices", "", 401},
		"403 no scope":   {http.MethodPost, "/v1/services/jellyfin/actions/restart", token, 403},
		"404 no service": {http.MethodGet, "/v1/services/nope", token, 404},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resp := env.do(t, tc.method, tc.path, tc.token, nil)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("Content-Type = %q", ct)
			}

			problem := decode[map[string]any](t, resp)
			for _, field := range []string{"type", "title", "status", "instance"} {
				if _, ok := problem[field]; !ok {
					t.Errorf("problem document is missing %q: %v", field, problem)
				}
			}
			if got := problem["status"].(float64); int(got) != tc.wantStatus {
				t.Errorf("problem.status = %v, want %d", got, tc.wantStatus)
			}
		})
	}
}

// TestInternalErrorsDoNotLeak: a database failure must not put SQL, table names or file
// paths in a client-visible response.
func TestInternalErrorsDoNotLeak(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone")

	// Closing the store makes every subsequent query fail.
	env.store.Close()

	resp := env.do(t, http.MethodGet, "/v1/devices", token, nil)
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 401 or 500", resp.StatusCode)
	}
	problem := decode[map[string]any](t, resp)
	detail, _ := problem["detail"].(string)
	for _, leak := range []string{"SELECT", "sqlite", "devices", ".db"} {
		if bytes.Contains([]byte(detail), []byte(leak)) {
			t.Errorf("response leaks internals (%q): %s", leak, detail)
		}
	}
}

// ---------------------------------------------------------------- rate limiter

func TestRateLimiter(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter(3, time.Minute)
	rl.now = func() time.Time { return now }

	for i := range 3 {
		if !rl.Allow("a") {
			t.Fatalf("attempt %d denied within limit", i)
		}
	}
	if rl.Allow("a") {
		t.Error("fourth attempt allowed; limit is 3")
	}

	// Limits are per key: one noisy client must not lock out another.
	if !rl.Allow("b") {
		t.Error("unrelated key was affected")
	}

	// The window rolls over.
	now = now.Add(2 * time.Minute)
	if !rl.Allow("a") {
		t.Error("attempt denied after the window expired")
	}
}

// TestRateLimiterPrunesExpiredKeys: without pruning, the map grows once per distinct
// source address forever — a slow leak an attacker drives for free by varying source
// ports.
func TestRateLimiterPrunesExpiredKeys(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter(5, time.Minute)
	rl.now = func() time.Time { return now }

	for i := range 100 {
		rl.Allow(fmt.Sprintf("client-%d", i))
	}
	if len(rl.counts) != 100 {
		t.Fatalf("len = %d, want 100", len(rl.counts))
	}

	now = now.Add(2 * time.Minute)
	rl.Allow("trigger-prune")

	if len(rl.counts) != 1 {
		t.Errorf("len = %d after expiry, want 1; stale keys are not pruned", len(rl.counts))
	}
}

func TestBearerTokenParsing(t *testing.T) {
	cases := map[string]struct {
		header    string
		want      string
		wantFound bool
	}{
		"standard":      {"Bearer abc123", "abc123", true},
		"lowercase":     {"bearer abc123", "abc123", true},
		"extra spaces":  {"Bearer   abc123  ", "abc123", true},
		"missing":       {"", "", false},
		"wrong scheme":  {"Basic abc123", "", false},
		"scheme only":   {"Bearer", "", false},
		"empty creds":   {"Bearer ", "", false},
		"no separator":  {"Bearerabc123", "", false},
		"token missing": {"Bearer  ", "", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				r.Header.Set("Authorization", tc.header)
			}
			got, found := bearerToken(r)
			if found != tc.wantFound || got != tc.want {
				t.Errorf("= %q, %v; want %q, %v", got, found, tc.want, tc.wantFound)
			}
		})
	}
}
