// Package systemd adapts any systemd unit to CueSeek's capability model.
//
// The other adapters ask a service about itself over HTTP. This one asks systemd about
// the process, and that is the whole difference between the two tiers of support CueSeek
// offers:
//
//	jellyfin, qbittorrent   the service answered its own API, and here is what it is doing
//	systemd                 the process is running
//
// The gap between "running" and "answering" is real and this package does not pretend
// otherwise — a wedged service that has not exited is `active` here and healthy on the
// dashboard. Closing that gap needs an HTTP probe, which is a different adapter.
//
// # Why it exists
//
// Until M4.4 a host running anything but Jellyfin or qBittorrent got nothing from CueSeek
// at all: not a health dot, not a restart button, not a link to a web interface. Two
// supported services is a dashboard for one machine; every systemd unit is a dashboard for
// anybody's. Plex, Sonarr, Immich, Syncthing, Vaultwarden, Samba, Postgres, a compose
// stack behind a unit — all of them become configurable here without a line of new Go.
//
// # What it deliberately does not do
//
// No `now_playing`, no `transfers`, no `reported_status` from the service itself. Those
// require translating a service's own domain into a shared vocabulary, which is the work an
// adapter exists to do and cannot be derived from a unit name (ADR-0005). An operator who
// wants those moves from `type: systemd` to a service-specific type in one line, keeping
// the same `id`, and gains them without any client changing — which is the property M3.4
// established when qBittorrent reached the phone with no Android release.
package systemd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// Type is the value used in configuration's `type:` field.
const Type = "systemd"

// adapter observes and controls one systemd unit.
//
// A single type, unlike jellyfin's split into adapter and controllable. There the split
// exists because control depends on a unit that may be absent; here the unit *is* the
// adapter's only source of health, so it is required by the factory and control is
// therefore always available. A systemd adapter that could not control anything would be a
// systemd adapter that could not observe anything either.
type adapter struct {
	id   string
	name string
	unit string

	units adapters.UnitControl

	// webUI is what the operator configured, or the zero value when they configured
	// nothing. hasWebUI is what capability discovery keys on.
	webUI    domain.WebUI
	hasWebUI bool
}

// Compile-time proof that the capabilities this package claims are the ones it has.
//
// ADR-0005 accepts type assertions as the discovery mechanism and names the cost: a
// capability implemented with a subtly wrong signature silently fails to register, and the
// failure is a missing button rather than a build error. These three lines convert that
// class of mistake back into a compile error for this adapter.
var (
	_ adapters.Service       = (*adapter)(nil)
	_ adapters.Controllable  = (*adapter)(nil)
	_ adapters.WebUIProvider = (*adapter)(nil)
)

// New builds a systemd adapter from configuration.
//
// Registered as a factory under Type. Validates its own requirements, because whether a
// service needs a unit is a property of its adapter rather than something the config
// package should know per type — the same rule that leaves base_url to each factory.
func New(cfg config.Service, deps adapters.Deps) (adapters.Service, error) {
	unit := strings.TrimSpace(cfg.Unit)
	if unit == "" {
		return nil, errors.New(
			"unit is required for type: systemd — it is the only thing this adapter " +
				"observes (e.g. unit: plexmediaserver.service). Find the exact name with " +
				"`systemctl list-units --type=service`")
	}
	if deps.Units == nil {
		// Reached only in tests and on a platform with no host layer at all. The agent
		// always supplies one; the unsupported backend answers every call with an error,
		// which Health degrades into `unknown` rather than crashing.
		return nil, errors.New("type: systemd requires a host layer to read unit state")
	}

	// Fields that only make sense when something talks to the service's own API. Setting
	// one here is almost never a harmless extra — it means the operator expected CueSeek
	// to understand this service and picked the generic type by mistake. Failing at
	// startup with that sentence is far better than a dashboard that silently never shows
	// what they configured a credential for.
	var misplaced []string
	for _, f := range []struct {
		name  string
		value string
	}{
		{"base_url", cfg.BaseURL},
		{"api_key", cfg.APIKey},
		{"api_key_file", cfg.APIKeyFile},
		{"username", cfg.Username},
		{"password", cfg.Password},
		{"password_file", cfg.PasswordFile},
	} {
		if strings.TrimSpace(f.value) != "" {
			misplaced = append(misplaced, f.name)
		}
	}
	if len(misplaced) > 0 {
		return nil, fmt.Errorf(
			"type: systemd never contacts the service, so %s %s ignored. "+
				"It reports whether the unit is running, nothing more. If you want health "+
				"from this service's own API, use a service-specific type "+
				"(known types are listed at startup); if you want the generic behaviour, "+
				"remove %s",
			strings.Join(misplaced, ", "),
			map[bool]string{true: "is", false: "are"}[len(misplaced) == 1],
			map[bool]string{true: "it", false: "them"}[len(misplaced) == 1])
	}

	name := cfg.Name
	if name == "" {
		name = cfg.ID
	}

	webUI, hasWebUI := cfg.WebUI.Resolved()

	return &adapter{
		id:       cfg.ID,
		name:     name,
		unit:     unit,
		units:    deps.Units,
		webUI:    webUI,
		hasWebUI: hasWebUI,
	}, nil
}

func (a *adapter) ID() string   { return a.id }
func (a *adapter) Name() string { return a.name }

// WebUI reports the configured interface, if any.
//
// Returning false is normal rather than a failure: whether a service has a web interface
// is operator configuration, not a property of this code.
func (a *adapter) WebUI() (domain.WebUI, bool) { return a.webUI, a.hasWebUI }

// Actions returns the lifecycle actions that currently apply.
//
// Shared with every other unit-backed adapter, so `restart` means the same thing and is
// spelled the same way everywhere. The state read happens here, on the poll path, never on
// a request path (ADR-0003).
func (a *adapter) Actions(ctx context.Context) []domain.Action {
	return adapters.AvailableLifecycleActions(ctx, a.units, a.unit, adapters.LifecycleCopy{
		DisplayName: a.name,
		// No Interruption text. The shared default — "Anything using it will be
		// interrupted." — is exactly right here and nothing more specific is knowable:
		// this adapter does not know what the service is for, which is the entire
		// premise of the generic tier.
	})
}

// Invoke dispatches an action through the host layer, never through systemd directly, so
// the request passes the configured unit allowlist and then polkit (ADR-0002).
func (a *adapter) Invoke(ctx context.Context, actionID string) (*host.Job, error) {
	return adapters.InvokeLifecycle(ctx, a.units, a.unit, actionID)
}

// Health observes the unit now.
//
// Returns an error only never: every outcome here is an opinion about the service, and
// `Service.Health` documents that "I could not connect" is information rather than a
// failure. A D-Bus read that fails becomes `unknown`, which is the honest answer and the
// one the freshness rules already know how to render.
//
// # What Reachable means for a unit
//
// For an HTTP adapter, Reachable is literally "the agent talked to the service". This
// adapter never talks to the service; it asks systemd about the process. The closest
// honest reading is therefore **the unit is active** — if the process is not running there
// is nothing to reach, and if it is, a client's "the agent reached it at that moment" is
// true of the only observation this adapter makes.
//
// The alternative, reporting Reachable whenever systemd answered, would be true of a
// stopped service too, and would put "the agent reached it" beside a red dot.
//
// # Why a stopped unit is `unreachable` rather than `degraded`
//
// `degraded` is documented as reachable-but-wrong. A stopped unit is not reachable at all.
// It also has to agree with the adapters already shipping: stop Jellyfin today and its HTTP
// probe fails, which reports `unreachable`. Two adapters watching the same stopped service
// must not disagree about what colour it is.
func (a *adapter) Health(ctx context.Context) (domain.Health, error) {
	observedAt := time.Now().UTC()

	state, err := a.units.UnitState(ctx, a.unit)
	if err != nil {
		return a.healthFromError(observedAt, err), nil
	}
	return a.healthFromState(observedAt, state), nil
}

// healthFromError turns a failed state read into an observation.
//
// The three cases have completely different fixes, which is why they are not one message:
// a name that does not exist is a typo in the config, an authorisation failure is a polkit
// problem, and anything else is the agent's own connection to the bus.
func (a *adapter) healthFromError(at time.Time, err error) domain.Health {
	switch {
	case errors.Is(err, host.ErrUnitNotFound):
		// The single most likely misconfiguration on a new install, and M0 proved why: a
		// unit's real name routinely differs from what the software calls itself. The
		// message names the command that settles it.
		return domain.Health{
			Status:     domain.StatusUnreachable,
			Reachable:  false,
			ObservedAt: at,
			Reasons: []domain.HealthReason{{
				Code: domain.ReasonUnitNotLoaded,
				Message: fmt.Sprintf(
					"systemd has no unit called %q. Check the exact name with "+
						"`systemctl list-units --type=service`.", a.unit),
			}},
		}

	case errors.Is(err, host.ErrUnitNotManaged):
		// Should be unreachable in practice: the allowlist is built from the same config
		// that named this unit. It becomes possible the moment anything else constructs a
		// Controller, so it is answered rather than assumed away.
		return domain.UnknownHealth(at, domain.HealthReason{
			Code: domain.ReasonUnitStateUnknown,
			Message: fmt.Sprintf(
				"%q is not in this agent's managed-unit allowlist.", a.unit),
		})

	default:
		// Includes ErrUnauthorized and ErrUnsupportedPlatform. Unknown rather than
		// unreachable: the service may be running perfectly well and the agent simply
		// could not look, and claiming it is down would be confidently wrong (ADR-0008).
		return domain.UnknownHealth(at, domain.HealthReason{
			Code:    domain.ReasonUnitStateUnknown,
			Message: fmt.Sprintf("Could not read the state of %q: %v", a.unit, err),
		})
	}
}

// healthFromState maps systemd's vocabulary onto CueSeek's four states.
func (a *adapter) healthFromState(at time.Time, state host.UnitState) domain.Health {
	reported := reportedStatus(state)

	// Loaded but not usable: masked, or a load error. Distinct from not-found, which
	// arrives as an error above, and worth its own message — a masked unit is a
	// deliberate act by somebody and no amount of restarting will help.
	if !state.Loaded() {
		return domain.Health{
			Status:         domain.StatusUnreachable,
			Reachable:      false,
			ReportedStatus: reported,
			ObservedAt:     at,
			Reasons: []domain.HealthReason{{
				Code: domain.ReasonUnitNotLoaded,
				Message: fmt.Sprintf(
					"systemd reports %q as %s, so it cannot be started.",
					a.unit, orUnknown(state.LoadState)),
			}},
		}
	}

	switch state.ActiveState {
	case "active":
		return domain.Health{
			Status:         domain.StatusHealthy,
			Reachable:      true,
			ReportedStatus: reported,
			ObservedAt:     at,
			// No reasons. Nothing is wrong, and inventing a reason to fill the field
			// would put text under a green dot for a reader to interpret.
			Reasons: []domain.HealthReason{},
		}

	case "failed":
		return domain.Health{
			Status:         domain.StatusUnreachable,
			Reachable:      false,
			ReportedStatus: reported,
			ObservedAt:     at,
			Reasons: []domain.HealthReason{{
				Code: domain.ReasonUnitFailed,
				Message: fmt.Sprintf(
					"systemd reports %q as failed. `journalctl -u %s` says why.",
					a.unit, a.unit),
			}},
		}

	case "inactive":
		return domain.Health{
			Status:         domain.StatusUnreachable,
			Reachable:      false,
			ReportedStatus: reported,
			ObservedAt:     at,
			Reasons: []domain.HealthReason{{
				Code: domain.ReasonUnitInactive,
				// Deliberately not called an error. A stopped service is very often
				// somebody's decision, including a decision made from this app.
				Message: fmt.Sprintf("%q is stopped.", a.unit),
			}},
		}

	case "activating", "deactivating", "reloading":
		return domain.Health{
			Status: domain.StatusDegraded,
			// In flux. Not reachable yet, or not for much longer.
			Reachable:      false,
			ReportedStatus: reported,
			ObservedAt:     at,
			Reasons: []domain.HealthReason{{
				Code: domain.ReasonUnitTransitioning,
				Message: fmt.Sprintf("%q is %s.", a.unit,
					map[string]string{
						"activating":   "starting",
						"deactivating": "stopping",
						"reloading":    "reloading",
					}[state.ActiveState]),
			}},
		}

	default:
		// A state this agent has not heard of, from a systemd newer than it. Unknown is
		// the honest answer: the state was read successfully and simply cannot be
		// interpreted, and guessing healthy would be the one wrong direction to guess in.
		return domain.Health{
			Status:         domain.StatusUnknown,
			Reachable:      false,
			ReportedStatus: reported,
			ObservedAt:     at,
			Reasons: []domain.HealthReason{{
				Code: domain.ReasonUnitStateUnknown,
				Message: fmt.Sprintf(
					"systemd reports %q as %q, which this version of CueSeek does not "+
						"recognise.", a.unit, orUnknown(state.ActiveState)),
			}},
		}
	}
}

// reportedStatus renders systemd's own words for the unit, e.g. "active (running)".
//
// This field is specified as "what the service says about itself, verbatim and unmapped".
// Systemd is not the service — but for this adapter it is the only observer there is, and
// its words are passed through exactly as the field requires rather than being translated.
// "It reports: failed (exited)" tells an operator something real; leaving the field empty
// would discard the most useful detail this adapter holds.
func reportedStatus(state host.UnitState) string {
	active := strings.TrimSpace(state.ActiveState)
	sub := strings.TrimSpace(state.SubState)
	switch {
	case active == "" && sub == "":
		return ""
	case sub == "":
		return active
	case active == "":
		return sub
	default:
		return active + " (" + sub + ")"
	}
}

// orUnknown keeps an empty property out of a sentence that would read as a bug.
func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
}
