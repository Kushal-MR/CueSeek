package domain

import (
	"testing"
	"time"
)

func TestScopeValidity(t *testing.T) {
	for _, s := range AllScopes {
		if !s.Valid() {
			t.Errorf("%q is in AllScopes but reports invalid", s)
		}
	}
	for _, s := range []Scope{"", "root", "admin", "READ", "read ", "host.power.all"} {
		if s.Valid() {
			t.Errorf("%q reports valid", s)
		}
	}
}

// TestParseScopeRejectsUnknown: silently dropping an unrecognised scope would grant a
// device less than its operator intended, and they would find out at the worst moment.
func TestParseScopeRejectsUnknown(t *testing.T) {
	got, err := ParseScope("service.control")
	if err != nil || got != ScopeServiceControl {
		t.Errorf("ParseScope(service.control) = %q, %v", got, err)
	}
	if _, err := ParseScope("root"); err == nil {
		t.Error("unknown scope was accepted")
	}
}

// TestParsePlatformFallsBack: unlike scopes, an unknown platform is tolerated. It is a
// display label with no security meaning, and refusing to pair a future client because
// it reported an unfamiliar platform would be a poor trade.
func TestParsePlatformFallsBack(t *testing.T) {
	if got := ParsePlatform("android"); got != PlatformAndroid {
		t.Errorf("= %q, want android", got)
	}
	for _, in := range []string{"", "fridge", "ANDROID"} {
		if got := ParsePlatform(in); got != PlatformUnknown {
			t.Errorf("ParsePlatform(%q) = %q, want unknown", in, got)
		}
	}
}

// TestHasScope exercises the check that actually protects the machine. The watch case is
// the one ADR-0006 is sold on: it can restart a service but is structurally incapable of
// powering off the host.
func TestHasScope(t *testing.T) {
	watch := Device{
		Name:     "Watch",
		Platform: PlatformWearOS,
		Scopes:   []Scope{ScopeRead, ScopeServiceControl},
	}
	if !watch.HasScope(ScopeRead) || !watch.HasScope(ScopeServiceControl) {
		t.Error("granted scopes not reported")
	}
	if watch.HasScope(ScopeHostPower) {
		t.Error("watch reports host.power, which it was never granted")
	}
	// devices.manage exists as a separate scope precisely so this holds. Had revocation
	// reused service.control, the watch would be able to revoke the phone.
	if watch.HasScope(ScopeDevicesManage) {
		t.Error("watch reports devices.manage, which it was never granted")
	}

	var none Device
	for _, s := range AllScopes {
		if none.HasScope(s) {
			t.Errorf("device with no scopes reports %q", s)
		}
	}
}

// TestHealthStatusIsAClosedSetOfFour: ADR-0008 makes a fifth status a contract change
// shared by the API, both clients and the design system. If this fails, that decision is
// being made here by accident.
func TestHealthStatusIsAClosedSetOfFour(t *testing.T) {
	if len(AllHealthStatuses) != 4 {
		t.Fatalf("AllHealthStatuses has %d entries, want 4 (ADR-0008)", len(AllHealthStatuses))
	}
	for _, s := range AllHealthStatuses {
		if !s.Valid() {
			t.Errorf("%q is in AllHealthStatuses but reports invalid", s)
		}
	}
	for _, s := range []HealthStatus{"", "ok", "down", "Healthy", "healthy "} {
		if s.Valid() {
			t.Errorf("%q reports valid", s)
		}
	}
}

func TestRiskLevelValidity(t *testing.T) {
	for _, r := range []RiskLevel{RiskSafe, RiskDisruptive, RiskDestructive} {
		if !r.Valid() {
			t.Errorf("%q reports invalid", r)
		}
	}
	// An unrecognised risk must not pass: clients gate confirmation on this value.
	for _, r := range []RiskLevel{"", "critical", "SAFE"} {
		if r.Valid() {
			t.Errorf("%q reports valid", r)
		}
	}
}

// TestUnknownHealthIsTheHonestDefault: the zero Health has an empty status, which is not
// one of the four. UnknownHealth exists so "I don't know" is the easy thing to write.
func TestUnknownHealthIsTheHonestDefault(t *testing.T) {
	var zero Health
	if zero.Status.Valid() {
		t.Fatal("the zero Health reports a valid status; UnknownHealth would have no reason to exist")
	}

	at := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	reason := HealthReason{Code: ReasonNotPolled, Message: "Not checked yet."}
	h := UnknownHealth(at, reason)

	if h.Status != StatusUnknown || !h.Status.Valid() {
		t.Errorf("status = %q, want %q", h.Status, StatusUnknown)
	}
	if h.Reachable {
		t.Error("an unknown observation claims the service was reachable")
	}
	if len(h.Reasons) != 1 || h.Reasons[0] != reason {
		t.Errorf("reasons = %v, want exactly %v", h.Reasons, reason)
	}
	if !h.ObservedAt.Equal(at) {
		t.Errorf("observed at %v, want %v", h.ObservedAt, at)
	}
}

// TestDeviceCarriesNoSecret is a structural guard. A Device is passed to clients, logged
// and serialised; ADR-0006 requires that a token exists in exactly one place. If someone
// later adds a Token or TokenHash field for convenience, this fails.
func TestDeviceCarriesNoSecret(t *testing.T) {
	now := time.Now()

	// An UNKEYED composite literal must list every field in order, so adding a field to
	// Device breaks this line and forces a deliberate decision about whether it belongs
	// on a type that gets logged and sent to clients. A keyed literal would compile
	// happily and guard nothing.
	_ = Device{
		"id",               // ID
		"name",             // Name
		PlatformCLI,        // Platform
		[]Scope{ScopeRead}, // Scopes
		now,                // CreatedAt
		&now,               // LastSeenAt
	}
}
