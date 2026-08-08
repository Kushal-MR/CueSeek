//go:build !linux

package host

import (
	"errors"
	"strings"
	"testing"
)

// The stub must fail loudly on every operation.
//
// The dangerous alternative is a no-op that reports success: the API would answer 202,
// the audit log would record an accepted restart, and nothing would have happened. A
// developer would then "verify" a feature on their laptop that cannot work at all.

func TestUnsupportedBackendRefusesEverything(t *testing.T) {
	backend, err := newBackend()
	if err != nil {
		t.Fatalf("newBackend: %v", err)
	}
	t.Cleanup(func() { backend.Close() })

	if _, err := backend.UnitState(t.Context(), "jellyfin.service"); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("UnitState err = %v, want ErrUnsupportedPlatform", err)
	}
	if _, err := backend.RestartUnit(t.Context(), "jellyfin.service"); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("RestartUnit err = %v, want ErrUnsupportedPlatform", err)
	}
	if !strings.HasPrefix(backend.Platform(), "unsupported/") {
		t.Errorf("Platform() = %q, want an unsupported/<goos> marker", backend.Platform())
	}
}

// TestUnsupportedStillEnforcesAllowlist: the policy layer runs identically on every
// platform, so an unmanaged unit is refused for being unmanaged rather than for being on
// the wrong operating system. Ordering matters — the allowlist is the security rule.
func TestUnsupportedStillEnforcesAllowlist(t *testing.T) {
	c, err := New([]string{"jellyfin.service"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	if _, err := c.RestartUnit(t.Context(), "ssh.service"); !errors.Is(err, ErrUnitNotManaged) {
		t.Errorf("err = %v, want ErrUnitNotManaged (allowlist is checked first)", err)
	}
	if _, err := c.RestartUnit(t.Context(), "jellyfin.service"); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("err = %v, want ErrUnsupportedPlatform for a managed unit", err)
	}
}
