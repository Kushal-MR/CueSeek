// Package jellyfin adapts a Jellyfin media server to CueSeek's capability model.
//
// Scope discipline (the product rule): CueSeek surfaces Jellyfin's health, activity and
// control. It is not a Jellyfin client. Browsing libraries, editing metadata and playback
// control belong to Jellyfin's own app, and adding them here would grow the adapter
// interface until it stopped being an adapter interface.
package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// Type is the value used in configuration's `type:` field.
const Type = "jellyfin"

// systemInfoPath is the authenticated system endpoint.
//
// Jellyfin also exposes /System/Info/Public, which needs no credentials and would make
// the health check simpler. Using the authenticated one is deliberate: a wrong or expired
// API key would otherwise stay invisible until now_playing shipped in M3 and quietly
// returned nothing. An operations console should report a credential problem as a
// credential problem, at the moment it starts being true.
const systemInfoPath = "/System/Info"

// maxResponseBytes caps how much we will read from the upstream.
//
// /System/Info is a couple of kilobytes. The cap exists because the thing on the other
// end of that URL is only assumed to be Jellyfin — point the config at something that
// streams indefinitely and an unbounded read would consume the agent's memory.
const maxResponseBytes = 1 << 20 // 1 MiB

// systemInfo is the subset of Jellyfin's response this adapter uses.
//
// Only the fields with meaning to CueSeek. Decoding the whole document would couple the
// adapter to Jellyfin's schema far more tightly than health monitoring requires, and
// every added field is one more thing that can change under us.
type systemInfo struct {
	ServerName        string `json:"ServerName"`
	Version           string `json:"Version"`
	ID                string `json:"Id"`
	HasPendingRestart bool   `json:"HasPendingRestart"`
	IsShuttingDown    bool   `json:"IsShuttingDown"`
}

// adapter implements adapters.Service: identity and health, nothing more.
type adapter struct {
	id      string
	name    string
	baseURL string
	apiKey  string
	client  *http.Client
}

// controllable adds the Controllable capability.
//
// A separate type rather than a flag on adapter, because capabilities are discovered by
// type assertion. If one type both implemented Controllable and refused to control
// anything, every Jellyfin service would advertise a "restart" button — including one
// configured with no systemd unit, where pressing it could only ever fail. Deciding the
// concrete type at construction is what makes the advertised capability true (ADR-0005).
type controllable struct {
	*adapter
	units adapters.UnitControl
	unit  string
}

const actionRestart = "restart"

// New builds a Jellyfin adapter from configuration.
//
// Registered as a factory under Type. Validates its own requirements: whether a service
// needs a base_url is a property of its adapter, not something the config package should
// know about each service type.
func New(cfg config.Service, deps adapters.Deps) (adapters.Service, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("base_url is required (e.g. http://127.0.0.1:8096)")
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base_url %q must be an absolute URL", cfg.BaseURL)
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New(
			"api_key (or api_key_file) is required; create one in Jellyfin under " +
				"Dashboard > API Keys")
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

	base := &adapter{
		id:      cfg.ID,
		name:    name,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		client:  client,
	}

	// Control is advertised only when it can actually be performed.
	if cfg.Unit != "" && deps.Units != nil {
		return &controllable{adapter: base, units: deps.Units, unit: cfg.Unit}, nil
	}
	return base, nil
}

func (a *adapter) ID() string   { return a.id }
func (a *adapter) Name() string { return a.name }

// Health observes the server.
//
// Returns an error only when it could not form an opinion at all. "Jellyfin is down" is
// an opinion, and it comes back as an unreachable Health rather than as an error —
// otherwise the poller would have to reconstruct the reason from an error string.
func (a *adapter) Health(ctx context.Context) (domain.Health, error) {
	observedAt := time.Now().UTC()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+systemInfoPath, nil)
	if err != nil {
		return domain.Health{}, fmt.Errorf("build request: %w", err)
	}
	// Jellyfin accepts the key either as this header or inside an Authorization
	// MediaBrowser value. The header is simpler and does not require assembling a
	// client-identity string that Jellyfin does not check anyway.
	req.Header.Set("X-Emby-Token", a.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return a.unreachable(observedAt, err), nil
	}
	defer resp.Body.Close()

	if health, handled := a.healthFromStatus(observedAt, resp.StatusCode); handled {
		return health, nil
	}

	var info systemInfo
	body := io.LimitReader(resp.Body, maxResponseBytes)
	if err := json.NewDecoder(body).Decode(&info); err != nil {
		// Reachable, and answering — just not with anything recognisable. Almost always
		// a base_url pointing at something that is not Jellyfin, so the message says so.
		return domain.Health{
			Status:     domain.StatusDegraded,
			Reachable:  true,
			ObservedAt: observedAt,
			Reasons: []domain.HealthReason{{
				Code: domain.ReasonInvalidResponse,
				Message: "Responded, but not with Jellyfin system information. " +
					"Check that base_url points at a Jellyfin server.",
			}},
		}, nil
	}

	return a.healthFromInfo(observedAt, info), nil
}

// healthFromStatus maps HTTP status codes. The second return reports whether the code was
// terminal, i.e. whether the body should still be read.
func (a *adapter) healthFromStatus(observedAt time.Time, code int) (domain.Health, bool) {
	switch {
	case code == http.StatusUnauthorized, code == http.StatusForbidden:
		// Reachable but misconfigured. Emphatically not "unreachable": the fix is a new
		// API key, not a look at the network, and conflating the two sends the operator
		// to the wrong place (ADR-0005).
		return domain.Health{
			Status:     domain.StatusDegraded,
			Reachable:  true,
			ObservedAt: observedAt,
			Reasons: []domain.HealthReason{{
				Code: domain.ReasonAuthFailed,
				Message: fmt.Sprintf(
					"Jellyfin rejected the API key (HTTP %d). Reissue it under Dashboard > API Keys.",
					code),
			}},
		}, true

	case code >= 500:
		return domain.Health{
			Status:     domain.StatusDegraded,
			Reachable:  true,
			ObservedAt: observedAt,
			Reasons: []domain.HealthReason{{
				Code:    domain.ReasonUpstreamError,
				Message: fmt.Sprintf("Jellyfin returned HTTP %d.", code),
			}},
		}, true

	case code < 200 || code >= 300:
		return domain.Health{
			Status:     domain.StatusDegraded,
			Reachable:  true,
			ObservedAt: observedAt,
			Reasons: []domain.HealthReason{{
				Code:    domain.ReasonUpstreamError,
				Message: fmt.Sprintf("Unexpected HTTP %d from Jellyfin.", code),
			}},
		}, true
	}
	return domain.Health{}, false
}

func (a *adapter) healthFromInfo(observedAt time.Time, info systemInfo) domain.Health {
	health := domain.Health{
		Status:     domain.StatusHealthy,
		Reachable:  true,
		ObservedAt: observedAt,
		Reasons:    []domain.HealthReason{},
		// ReportedStatus stays empty on purpose. Jellyfin publishes no self-assessment —
		// no "ok"/"degraded" string — and inventing one from the version or server name
		// would put a value in a field the contract defines as verbatim and unmapped.
	}

	if info.IsShuttingDown {
		health.Status = domain.StatusDegraded
		health.Reasons = append(health.Reasons, domain.HealthReason{
			Code:    domain.ReasonShuttingDown,
			Message: "Jellyfin reports that it is shutting down.",
		})
	}
	if info.HasPendingRestart {
		// A reason without a status change: worth telling the operator, not worth
		// colouring the dashboard amber. Reasons are not required to be problems.
		health.Reasons = append(health.Reasons, domain.HealthReason{
			Code:    domain.ReasonPendingRestart,
			Message: "Jellyfin has applied changes that need a restart to take effect.",
		})
	}
	return health
}

func (a *adapter) unreachable(observedAt time.Time, err error) domain.Health {
	reason := domain.HealthReason{
		Code:    domain.ReasonUnreachable,
		Message: "Could not reach Jellyfin: " + summarise(err),
	}
	// A timeout and a refused connection look identical in a status dot and have
	// different causes: one is a service that is up but wedged, the other is a service
	// that is not listening.
	if errors.Is(err, context.DeadlineExceeded) {
		reason = domain.HealthReason{
			Code:    domain.ReasonTimeout,
			Message: "Jellyfin did not respond before the request deadline.",
		}
	}
	return domain.Health{
		Status:     domain.StatusUnreachable,
		Reachable:  false,
		ObservedAt: observedAt,
		Reasons:    []domain.HealthReason{reason},
	}
}

// summarise strips the URL out of a transport error.
//
// net/http errors embed the full request URL, which for some services carries the API key
// in a query string. This message ends up in an API response and in the log, so the URL
// does not travel with it.
func summarise(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}

// ---------------------------------------------------------------- Controllable

// lifecycleCopy is Jellyfin's contribution to the shared action descriptors: the
// mechanism is identical for every unit-backed service, only the consequence differs.
var lifecycleCopy = adapters.LifecycleCopy{
	DisplayName:  "Jellyfin",
	Interruption: "Anyone currently watching will be interrupted and will need to resume playback.",
}

// Actions describes what can be done to this service right now.
//
// State-dependent since ADR-0002 Amendment 1: Start is offered only when the unit is
// inactive, Stop only when it is active.
func (c *controllable) Actions(ctx context.Context) []domain.Action {
	return adapters.AvailableLifecycleActions(ctx, c.units, c.unit, lifecycleCopy)
}

// Invoke performs an action.
//
// The restart goes through the host layer, never through Jellyfin's own /System/Restart
// endpoint and never through systemd directly. Two reasons: a Jellyfin that is wedged
// enough to need restarting is often too wedged to honour its own restart API, and going
// via the host layer means the request passes the configured unit allowlist and then
// polkit (ADR-0002). An adapter reaching for go-systemd itself would bypass both.
func (c *controllable) Invoke(ctx context.Context, actionID string) (*host.Job, error) {
	job, err := adapters.InvokeLifecycle(ctx, c.units, c.unit, actionID)
	if err != nil {
		return nil, fmt.Errorf("jellyfin: %w", err)
	}
	return job, nil
}
