// Package domain holds the vocabulary shared by every other package in the agent:
// scopes, devices, audit entries.
//
// It depends on nothing but the standard library, and nothing here knows about HTTP,
// SQL, D-Bus or JSON wire formats. That is the point. ADR-0009 requires generated code
// to be wrapped rather than consumed directly, so the types that cross package
// boundaries cannot be the generated ones — internal/api translates between this
// vocabulary and the generated types at the edge, and everything inward speaks only
// this.
//
// If this package ever needs an import beyond the standard library, something has
// leaked into it that belongs further out.
package domain

import (
	"fmt"
	"slices"
	"time"
)

// Scope is an independently grantable permission carried by a device token.
//
// Scopes are separate grants, not tiers: a device may hold ServiceControl without Read,
// nonsensical though that would be. Modelling them as a set rather than a level is what
// allows a watch to restart Jellyfin while being structurally incapable of powering off
// the machine (ADR-0006).
type Scope string

const (
	// ScopeRead permits reading state: system, devices, services, the stream.
	ScopeRead Scope = "read"
	// ScopeServiceControl permits invoking service actions, e.g. restarting Jellyfin.
	ScopeServiceControl Scope = "service.control"
	// ScopeDevicesManage permits revoking paired devices.
	//
	// Separate from ScopeServiceControl on purpose. Revocation is the most destructive
	// operation in the API — it can lock every remaining device out of the agent,
	// including the one an operator would reach for to undo the mistake. Bundling it
	// with the scope a watch routinely carries would mean any device able to restart a
	// service could also lock out the phone.
	ScopeDevicesManage Scope = "devices.manage"
	// ScopeHostPower permits rebooting or shutting down the machine.
	ScopeHostPower Scope = "host.power"
)

// AllScopes is the closed set of valid scopes. Adding one is an API change: it appears
// in the contract's Scope enum and in every generated client.
var AllScopes = []Scope{
	ScopeRead,
	ScopeServiceControl,
	ScopeDevicesManage,
	ScopeHostPower,
}

// Valid reports whether s is a recognised scope.
func (s Scope) Valid() bool { return slices.Contains(AllScopes, s) }

// ParseScope validates a scope string.
//
// Unknown scopes are rejected rather than ignored. Silently dropping an unrecognised
// scope from a config file would grant a device less than its operator intended, and
// they would find out only when something failed at an inconvenient moment.
func ParseScope(s string) (Scope, error) {
	sc := Scope(s)
	if !sc.Valid() {
		return "", fmt.Errorf("unknown scope %q (valid: %v)", s, AllScopes)
	}
	return sc, nil
}

// Platform records what kind of client a device is, for display in the device list.
type Platform string

const (
	PlatformAndroid Platform = "android"
	PlatformWearOS  Platform = "wearos"
	PlatformIOS     Platform = "ios"
	PlatformWeb     Platform = "web"
	PlatformDesktop Platform = "desktop"
	PlatformCLI     Platform = "cli"
	PlatformUnknown Platform = "unknown"
)

var allPlatforms = []Platform{
	PlatformAndroid, PlatformWearOS, PlatformIOS,
	PlatformWeb, PlatformDesktop, PlatformCLI, PlatformUnknown,
}

// Valid reports whether p is a recognised platform.
func (p Platform) Valid() bool { return slices.Contains(allPlatforms, p) }

// ParsePlatform maps a string to a Platform, falling back to PlatformUnknown.
//
// Unlike scopes, an unrecognised platform is tolerated. It is a display label with no
// security meaning, and rejecting a pairing request because a future client reported a
// platform this agent has not heard of would be a poor trade.
func ParsePlatform(s string) Platform {
	p := Platform(s)
	if !p.Valid() {
		return PlatformUnknown
	}
	return p
}

// Device is a paired client, identified independently of every other client so that it
// can be revoked on its own (ADR-0006).
//
// It deliberately carries no token or token hash. Those live only in the store, and are
// never returned from it — a Device is safe to log, serialise and hand to a client.
type Device struct {
	ID         string
	Name       string
	Platform   Platform
	Scopes     []Scope
	CreatedAt  time.Time
	LastSeenAt *time.Time // nil until the device makes its first authenticated request
}

// HasScope reports whether the device holds the given scope.
//
// This is the check that actually protects the machine. ADR-0001 delegated transport
// security to the VPN, so nothing upstream of here distinguishes a phone from a
// compromised laptop on the same tailnet. A client-side confirmation dialog is user
// experience; this is the control.
func (d Device) HasScope(s Scope) bool { return slices.Contains(d.Scopes, s) }

// Outcome is the result recorded for an audited action.
type Outcome string

const (
	// OutcomeAccepted records that an action was authorised and started. Actions are
	// asynchronous, so acceptance and completion are separate events (ADR-0004).
	OutcomeAccepted  Outcome = "accepted"
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	// OutcomeDenied records an attempt refused for lack of scope. These are the most
	// interesting rows in the table.
	OutcomeDenied Outcome = "denied"
)

// AuditEntry is one line in the append-only record of what was done to this host.
//
// An operations console that can reboot a machine should be able to answer "who rebooted
// it, from which device, when". That question is usually asked after something has gone
// wrong, which is exactly when the answer is hardest to reconstruct from logs.
type AuditEntry struct {
	ID       int64
	At       time.Time
	DeviceID string // empty for actions taken by the agent itself
	// DeviceName is a snapshot, not a lookup.
	//
	// Denormalised on purpose: revoking a device must not erase the record of what it
	// did. A foreign key to devices would either block revocation or leave dangling
	// rows, and the audit trail would decay exactly when a device is removed — which is
	// often precisely the incident being investigated.
	DeviceName string
	Action     string // e.g. "service.restart", "device.revoke"
	Target     string // e.g. "jellyfin", or the revoked device id
	Outcome    Outcome
	Detail     string // free text; error message on failure
}
