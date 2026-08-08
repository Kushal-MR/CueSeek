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

	var none Device
	for _, s := range AllScopes {
		if none.HasScope(s) {
			t.Errorf("device with no scopes reports %q", s)
		}
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
