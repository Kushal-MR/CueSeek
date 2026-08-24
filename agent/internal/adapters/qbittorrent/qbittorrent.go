// Package qbittorrent adapts a qBittorrent client to CueSeek's capability model.
//
// Scope discipline (the product rule): CueSeek surfaces qBittorrent's health and control.
// It is not a torrent manager. Adding, pausing, prioritising and deleting torrents belong
// to qBittorrent's own Web UI, which is exactly what the `web_ui` capability points at —
// CueSeek looks after the *service*, not the transfers.
//
// The per-torrent list lands in M3.5 as the `transfers` capability, and even then it is a
// read-only activity view rather than a management surface.
package qbittorrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// Type is the value used in configuration's `type:` field.
const Type = "qbittorrent"

const (
	// transferInfoPath is the health probe.
	//
	// One authenticated call that answers both questions at once: whether the credentials
	// work, and whether qBittorrent can reach its peers. /api/v2/app/version would prove
	// only the first, and a torrent client that is up but firewalled is precisely the
	// state an operations console exists to surface.
	transferInfoPath = "/api/v2/transfer/info"

	// loginPath exchanges credentials for a session cookie.
	loginPath = "/api/v2/auth/login"

	// maxResponseBytes caps how much will be read from the upstream. The same reasoning
	// as every other adapter: the thing at base_url is only *assumed* to be qBittorrent.
	maxResponseBytes = 1 << 20 // 1 MiB
)

// Connection statuses qBittorrent reports. Kept as strings rather than an enum because
// they cross into `reported_status` verbatim, and a value this build has not seen must
// pass through rather than be flattened into one it has.
const (
	statusConnected    = "connected"
	statusFirewalled   = "firewalled"
	statusDisconnected = "disconnected"
)

// transferInfo is the subset of /api/v2/transfer/info this adapter uses.
//
// Only ConnectionStatus. The endpoint also returns global rates and session totals, which
// belong to `transfers` in M3.5 — decoding them here would put fields in this struct that
// nothing reads and that qBittorrent is free to rename.
type transferInfo struct {
	ConnectionStatus string `json:"connection_status"`
}

// authError marks a failure that came from the login exchange rather than the network.
//
// The distinction is the whole of ADR-0005's complaint about conflating them: "qBittorrent
// rejected the password" and "qBittorrent is not listening" send the operator to entirely
// different places, and a transport error is what a failed login looks like unless
// something says otherwise. Caught by tests before it ever shipped.
type authError struct{ err error }

func (e *authError) Error() string { return e.err.Error() }
func (e *authError) Unwrap() error { return e.err }

// adapter implements adapters.Service: identity and health, nothing more.
type adapter struct {
	id      string
	name    string
	baseURL string
	client  *http.Client

	// username and password are optional. qBittorrent's "bypass authentication for
	// clients on localhost" is the usual setup for a service the agent shares a machine
	// with, and in that case there is nothing to log in with and nothing to log in to.
	username string
	password string

	// sid is the session cookie qBittorrent issues at login, guarded because polls and
	// re-logins can overlap. Sessions expire, so this is a cache rather than a
	// credential — losing it costs one extra request, not an error.
	mu  sync.Mutex
	sid string

	// webUI is what the operator configured, or the zero value when they configured
	// nothing. hasWebUI is what capability discovery keys on.
	webUI    domain.WebUI
	hasWebUI bool
}

// controllable adds the Controllable capability.
//
// A separate type rather than a flag, for the same reason as every other adapter: the
// capability is discovered by type assertion, so a service configured with no systemd
// unit must not be the type that claims it can be restarted (ADR-0005).
type controllable struct {
	*adapter
	units adapters.UnitControl
	unit  string
}

// New builds a qBittorrent adapter from configuration.
//
// Registered as a factory under Type. Validates its own requirements — whether a service
// needs a base_url, or credentials, is a property of its adapter rather than something
// the config package should know per service type.
func New(cfg config.Service, deps adapters.Deps) (adapters.Service, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("base_url is required (e.g. http://127.0.0.1:8080)")
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base_url %q must be an absolute URL", cfg.BaseURL)
	}

	// Credentials are deliberately not required. Demanding them would break the most
	// common deployment — localhost auth bypass — and would push operators into putting
	// a password in a file for a service that never asks for one.
	username := strings.TrimSpace(cfg.Username)
	password := cfg.Password
	if username == "" && password != "" {
		return nil, errors.New(
			"password is set but username is not; qBittorrent authenticates with both")
	}

	client := deps.HTTPClient
	if client == nil {
		// No Timeout set: deadlines come from the caller's context, so one service's
		// budget can never be spent by another.
		client = &http.Client{}
	}

	name := cfg.Name
	if name == "" {
		name = cfg.ID
	}

	webUI, hasWebUI := cfg.WebUI.Resolved()

	base := &adapter{
		id:       cfg.ID,
		name:     name,
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		client:   client,
		username: username,
		password: password,
		webUI:    webUI,
		hasWebUI: hasWebUI,
	}

	// Control is advertised only when it can actually be performed.
	if cfg.Unit != "" && deps.Units != nil {
		return &controllable{adapter: base, units: deps.Units, unit: cfg.Unit}, nil
	}
	return base, nil
}

func (a *adapter) ID() string   { return a.id }
func (a *adapter) Name() string { return a.name }

// Health observes the client.
//
// Returns an error only when it could not form an opinion at all. "qBittorrent is down" is
// an opinion, and it comes back as an unreachable Health rather than as an error.
func (a *adapter) Health(ctx context.Context) (domain.Health, error) {
	observedAt := time.Now().UTC()

	resp, err := a.get(ctx, transferInfoPath)
	if err != nil {
		return a.classify(observedAt, err), nil
	}

	// A 403 here means the session expired, not that the credentials are wrong — the
	// cookie has a lifetime and the agent may have been idle past it. One silent
	// re-login, then one retry. Reporting an expired session as an auth failure would
	// send the operator to check a password that was never the problem.
	if resp.StatusCode == http.StatusForbidden && a.canLogIn() {
		resp.Body.Close()
		a.forgetSession()

		if err := a.logIn(ctx); err != nil {
			return a.authFailed(observedAt, err), nil
		}
		if resp, err = a.get(ctx, transferInfoPath); err != nil {
			return a.classify(observedAt, err), nil
		}
	}
	defer resp.Body.Close()

	if health, handled := a.healthFromStatus(observedAt, resp.StatusCode); handled {
		return health, nil
	}

	var info transferInfo
	body := io.LimitReader(resp.Body, maxResponseBytes)
	if err := json.NewDecoder(body).Decode(&info); err != nil {
		// Reachable, and answering — just not with anything recognisable. Almost always
		// a base_url pointing at something that is not qBittorrent.
		return domain.Health{
			Status:     domain.StatusDegraded,
			Reachable:  true,
			ObservedAt: observedAt,
			Reasons: []domain.HealthReason{{
				Code: domain.ReasonInvalidResponse,
				Message: "Responded, but not with qBittorrent transfer information. " +
					"Check that base_url points at qBittorrent's Web UI.",
			}},
		}, nil
	}

	return a.healthFromInfo(observedAt, info), nil
}

// healthFromStatus maps HTTP status codes. The second return reports whether the code was
// terminal, i.e. whether the body should still be read.
func (a *adapter) healthFromStatus(observedAt time.Time, code int) (domain.Health, bool) {
	switch {
	case code == http.StatusForbidden, code == http.StatusUnauthorized:
		// Reachable but misconfigured. Emphatically not "unreachable": the fix is a
		// credential or qBittorrent's own auth settings, not a look at the network.
		detail := "qBittorrent rejected the request (HTTP %d). Check username and password."
		if !a.canLogIn() {
			// No credentials configured, so the operator is relying on the localhost
			// bypass — and it is off. Naming the actual setting is worth more than a
			// generic "check your credentials" they cannot act on.
			detail = "qBittorrent refused the request (HTTP %d) and no credentials are " +
				"configured. Either set username and password, or enable " +
				"\"Bypass authentication for clients on localhost\" in qBittorrent's " +
				"Web UI settings."
		}
		return domain.Health{
			Status:     domain.StatusDegraded,
			Reachable:  true,
			ObservedAt: observedAt,
			Reasons: []domain.HealthReason{{
				Code:    domain.ReasonAuthFailed,
				Message: fmt.Sprintf(detail, code),
			}},
		}, true

	case code >= 500:
		return domain.Health{
			Status:     domain.StatusDegraded,
			Reachable:  true,
			ObservedAt: observedAt,
			Reasons: []domain.HealthReason{{
				Code:    domain.ReasonUpstreamError,
				Message: fmt.Sprintf("qBittorrent returned HTTP %d.", code),
			}},
		}, true

	case code < 200 || code >= 300:
		return domain.Health{
			Status:     domain.StatusDegraded,
			Reachable:  true,
			ObservedAt: observedAt,
			Reasons: []domain.HealthReason{{
				Code:    domain.ReasonUpstreamError,
				Message: fmt.Sprintf("Unexpected HTTP %d from qBittorrent.", code),
			}},
		}, true
	}
	return domain.Health{}, false
}

// healthFromInfo turns qBittorrent's connection status into CueSeek's.
//
// This is the first adapter to populate ReportedStatus. Jellyfin publishes no
// self-assessment, so it leaves the field empty; qBittorrent publishes exactly one, and it
// crosses **verbatim and unmapped** — which is what the field is for. A client showing
// "firewalled" is showing qBittorrent's word, not the agent's paraphrase of it.
func (a *adapter) healthFromInfo(observedAt time.Time, info transferInfo) domain.Health {
	reported := strings.TrimSpace(info.ConnectionStatus)

	health := domain.Health{
		Status:         domain.StatusHealthy,
		Reachable:      true,
		ObservedAt:     observedAt,
		ReportedStatus: reported,
		Reasons:        []domain.HealthReason{},
	}

	switch strings.ToLower(reported) {
	case statusConnected:
		// Nothing to add. The service is doing what it exists to do.

	case statusFirewalled:
		// A reason without a status change, the same shape as Jellyfin's pending restart.
		// qBittorrent is running, authenticated, and transferring — it simply cannot
		// accept incoming connections. Colouring the dashboard for this would spend
		// attention on something that is often the operator's deliberate choice, and
		// this palette treats chroma as attention.
		health.Reasons = append(health.Reasons, domain.HealthReason{
			Code: domain.ReasonPeerConnectivity,
			Message: "qBittorrent is firewalled: transfers work, but peers cannot " +
				"connect inbound. Forward its listening port to improve speeds.",
		})

	case statusDisconnected:
		health.Status = domain.StatusDegraded
		health.Reasons = append(health.Reasons, domain.HealthReason{
			Code: domain.ReasonPeerConnectivity,
			Message: "qBittorrent is running but not connected to any peers. " +
				"Check the host's network and qBittorrent's connection settings.",
		})

	case "":
		// Answered, decoded, but said nothing about its connection. Treated as unknown
		// rather than assumed healthy: a missing field is not a green light.
		health.Status = domain.StatusUnknown
		health.Reasons = append(health.Reasons, domain.HealthReason{
			Code:    domain.ReasonInvalidResponse,
			Message: "qBittorrent did not report a connection status.",
		})

	default:
		// A status this build has never heard of. Reported verbatim above and treated as
		// unknown here — the same forward-compatibility stance the client takes toward
		// capability ids it does not recognise (ADR-0007). Guessing which of healthy or
		// degraded a new value means is the one thing that would be worse than saying so.
		health.Status = domain.StatusUnknown
		health.Reasons = append(health.Reasons, domain.HealthReason{
			Code: domain.ReasonInvalidResponse,
			Message: fmt.Sprintf(
				"qBittorrent reported a connection status this version of CueSeek does "+
					"not recognise (%q).", reported),
		})
	}

	return health
}

// classify decides whether a failure was the credential's fault or the network's.
func (a *adapter) classify(observedAt time.Time, err error) domain.Health {
	var authErr *authError
	if errors.As(err, &authErr) {
		return a.authFailed(observedAt, authErr.err)
	}
	return a.unreachable(observedAt, err)
}

func (a *adapter) unreachable(observedAt time.Time, err error) domain.Health {
	reason := domain.HealthReason{
		Code:    domain.ReasonUnreachable,
		Message: "Could not reach qBittorrent: " + summarise(err),
	}
	// A timeout and a refused connection look identical in a status dot and have
	// different causes: one is a service that is up but wedged, the other is a service
	// that is not listening.
	if errors.Is(err, context.DeadlineExceeded) {
		reason = domain.HealthReason{
			Code:    domain.ReasonTimeout,
			Message: "qBittorrent did not respond before the request deadline.",
		}
	}
	return domain.Health{
		Status:     domain.StatusUnreachable,
		Reachable:  false,
		ObservedAt: observedAt,
		Reasons:    []domain.HealthReason{reason},
	}
}

func (a *adapter) authFailed(observedAt time.Time, err error) domain.Health {
	return domain.Health{
		Status:     domain.StatusDegraded,
		Reachable:  true,
		ObservedAt: observedAt,
		Reasons: []domain.HealthReason{{
			Code:    domain.ReasonAuthFailed,
			Message: "qBittorrent rejected the configured credentials: " + summarise(err),
		}},
	}
}

// ---------------------------------------------------------------- transport

// get issues an authenticated GET, logging in first if there is no session yet.
func (a *adapter) get(ctx context.Context, path string) (*http.Response, error) {
	if a.canLogIn() && a.session() == "" {
		if err := a.logIn(ctx); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	// qBittorrent rejects cross-site requests by default, and an absent Referer counts as
	// one on some builds. Sending our own base URL is what a browser on the same origin
	// would send.
	req.Header.Set("Referer", a.baseURL)
	if sid := a.session(); sid != "" {
		req.AddCookie(&http.Cookie{Name: "SID", Value: sid})
	}

	return a.client.Do(req)
}

// logIn exchanges credentials for a session cookie.
//
// qBittorrent answers 200 with the body "Fails." for bad credentials rather than a 4xx,
// which is why the body is checked and not only the status. A login that reports success
// while having failed would turn every later 403 into an infinite re-login loop.
func (a *adapter) logIn(ctx context.Context) error {
	form := url.Values{}
	form.Set("username", a.username)
	form.Set("password", a.password)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, a.baseURL+loginPath, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", a.baseURL)

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The service answered, so this is its verdict rather than a network problem.
		return &authError{fmt.Errorf("login returned HTTP %d", resp.StatusCode)}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}
	if strings.Contains(strings.ToLower(string(body)), "fail") {
		return &authError{errors.New("qBittorrent rejected the username or password")}
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "SID" && cookie.Value != "" {
			a.setSession(cookie.Value)
			return nil
		}
	}
	// 200, not "Fails.", and no cookie. Nothing usable came back, and pretending
	// otherwise would mean every subsequent request silently going out unauthenticated.
	return &authError{errors.New("login succeeded but returned no session cookie")}
}

func (a *adapter) canLogIn() bool { return a.username != "" }

func (a *adapter) session() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sid
}

func (a *adapter) setSession(sid string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sid = sid
}

func (a *adapter) forgetSession() { a.setSession("") }

// summarise strips the URL out of a transport error.
//
// net/http errors embed the full request URL, which for some deployments carries
// credentials. This message ends up in an API response and in the log, so the URL does not
// travel with it.
func summarise(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}

// ---------------------------------------------------------------- capabilities

// lifecycleCopy is qBittorrent's contribution to the shared action descriptors: the
// mechanism is identical for every unit-backed service, only the consequence differs.
//
// That this is the entire lifecycle implementation is the point of M3.4. Start, Stop and
// Restart, their risk levels, their confirmation copy and their state-dependence all come
// from adapters.AvailableLifecycleActions — an adapter contributes the two sentences only
// it can know.
var lifecycleCopy = adapters.LifecycleCopy{
	DisplayName:  "qBittorrent",
	Interruption: "Active torrents will be paused and will resume when it starts again.",
}

// WebUI reports where qBittorrent's own interface lives, if the operator configured it.
//
// A pass-through by design, and the capability that carries most of this service's value:
// managing torrents is qBittorrent's job, and CueSeek's job is to get you there.
func (a *adapter) WebUI() (domain.WebUI, bool) { return a.webUI, a.hasWebUI }

// Actions describes what can be done to this service right now.
//
// State-dependent per ADR-0002 Amendment 1: Start is offered only when the unit is
// inactive, Stop only when it is active.
func (c *controllable) Actions(ctx context.Context) []domain.Action {
	return adapters.AvailableLifecycleActions(ctx, c.units, c.unit, lifecycleCopy)
}

// Invoke performs an action.
//
// Through the host layer, never through qBittorrent's own API and never through systemd
// directly. Going via the host layer means the request passes the configured unit
// allowlist and then polkit (ADR-0002); an adapter reaching for go-systemd itself would
// bypass both.
func (c *controllable) Invoke(ctx context.Context, actionID string) (*host.Job, error) {
	job, err := adapters.InvokeLifecycle(ctx, c.units, c.unit, actionID)
	if err != nil {
		return nil, fmt.Errorf("qbittorrent: %w", err)
	}
	return job, nil
}
