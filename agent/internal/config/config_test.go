package config

import (
	"strings"
	"testing"
	"time"
)

// Tests exercise Parse against real YAML bytes rather than constructing Config structs.
// Building the struct directly would skip the YAML field mapping entirely — which is
// where a mistyped tag silently costs an afternoon.

const validYAML = `
bind:
  address: "127.0.0.1:7777"
storage:
  path: /var/lib/cueseek/cueseek.db
services:
  - id: jellyfin
    name: Jellyfin
    type: jellyfin
    unit: jellyfin.service
    base_url: http://127.0.0.1:8096
    poll_interval: 15s
`

func TestParseValid(t *testing.T) {
	cfg, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Bind.Address != "127.0.0.1:7777" {
		t.Errorf("bind.address = %q", cfg.Bind.Address)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("len(services) = %d, want 1", len(cfg.Services))
	}
	svc := cfg.Services[0]
	if svc.Unit != "jellyfin.service" {
		t.Errorf("unit = %q", svc.Unit)
	}
	if svc.PollInterval != 15*time.Second {
		t.Errorf("poll_interval = %v, want 15s", svc.PollInterval)
	}
}

func TestDefaultsApplied(t *testing.T) {
	cfg, err := Parse([]byte(`
services:
  - id: jellyfin
    type: jellyfin
    unit: jellyfin.service
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Bind.Address != "127.0.0.1:7777" {
		t.Errorf("default bind = %q, want loopback", cfg.Bind.Address)
	}
	if cfg.Storage.Path == "" {
		t.Error("storage.path was not defaulted")
	}
	if cfg.Services[0].PollInterval != defaultPollInterval {
		t.Errorf("poll_interval = %v, want %v", cfg.Services[0].PollInterval, defaultPollInterval)
	}
	// Name falls back to id so that every service has something displayable.
	if cfg.Services[0].Name != "jellyfin" {
		t.Errorf("name = %q, want fallback to id", cfg.Services[0].Name)
	}
}

// TestWildcardBindRejected covers ADR-0001's only enforced consequence in this package.
// The agent can power off the machine and terminates no TLS; listening on every
// interface must be a deliberate act, not a default or an oversight.
func TestWildcardBindRejected(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:7777", ":7777", "[::]:7777"} {
		t.Run(addr, func(t *testing.T) {
			_, err := Parse([]byte("bind:\n  address: \"" + addr + "\"\n"))
			if err == nil {
				t.Fatalf("%q was accepted", addr)
			}
			// The message has to explain itself: whoever hits this is mid-install and
			// needs to know why, not just that.
			if !strings.Contains(err.Error(), "ADR-0001") {
				t.Errorf("error does not cite the reason: %v", err)
			}
		})
	}
}

func TestWildcardBindAllowedWhenExplicit(t *testing.T) {
	cfg, err := Parse([]byte(`
bind:
  address: "0.0.0.0:7777"
  allow_unrestricted: true
`))
	if err != nil {
		t.Fatalf("explicit opt-out was still rejected: %v", err)
	}
	if cfg.Bind.Address != "0.0.0.0:7777" {
		t.Errorf("address = %q", cfg.Bind.Address)
	}
}

// TestUnknownFieldRejected: a typo like `adress:` would otherwise be ignored, the agent
// would bind to the default, and everything would report success.
func TestUnknownFieldRejected(t *testing.T) {
	_, err := Parse([]byte("bind:\n  adress: \"127.0.0.1:7777\"\n"))
	if err == nil {
		t.Fatal("misspelled field was silently ignored")
	}
}

func TestInvalidConfigs(t *testing.T) {
	cases := map[string]struct{ yaml, wantSubstring string }{
		"no port":            {"bind:\n  address: \"127.0.0.1\"\n", "host:port"},
		"relative db path":   {"storage:\n  path: cueseek.db\n", "absolute"},
		"empty db path":      {"storage:\n  path: \"\"\n", "must not be empty"},
		"service without id": {"services:\n  - type: jellyfin\n    unit: a.service\n", "id must not be empty"},
		"service without unit": {
			"services:\n  - id: jellyfin\n    type: jellyfin\n", "unit must not be empty"},
		"unit missing suffix": {
			"services:\n  - id: j\n    type: jellyfin\n    unit: jellyfin\n", "type suffix"},
		"poll interval too low": {
			"services:\n  - id: j\n    type: jellyfin\n    unit: j.service\n    poll_interval: 100ms\n",
			"too aggressive"},
		"duplicate service id": {
			"services:\n" +
				"  - id: jellyfin\n    type: jellyfin\n    unit: a.service\n" +
				"  - id: jellyfin\n    type: jellyfin\n    unit: b.service\n",
			"duplicate id"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatalf("accepted invalid config")
			}
			if !strings.Contains(err.Error(), tc.wantSubstring) {
				t.Errorf("error %q does not mention %q", err, tc.wantSubstring)
			}
		})
	}
}

// TestValidationReportsEveryError: a config with three mistakes should take one round
// trip to fix, not three.
func TestValidationReportsEveryError(t *testing.T) {
	_, err := Parse([]byte(`
storage:
  path: relative.db
services:
  - id: ""
    type: jellyfin
    unit: nosuffix
`))
	if err == nil {
		t.Fatal("expected errors")
	}
	msg := err.Error()
	for _, want := range []string{"absolute", "id must not be empty", "type suffix"} {
		if !strings.Contains(msg, want) {
			t.Errorf("combined error is missing %q:\n%s", want, msg)
		}
	}
}

// TestManagedUnits: the host layer uses this to refuse an unlisted unit before any D-Bus
// call is made. ADR-0002 requires the allowlist in two places — here and in the polkit
// rule — so a misconfiguration is caught before it reaches the system bus.
func TestManagedUnits(t *testing.T) {
	cfg, err := Parse([]byte(`
services:
  - id: jellyfin
    type: jellyfin
    unit: jellyfin.service
  - id: qbittorrent
    type: qbittorrent
    unit: qbittorrent.service
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	units := cfg.ManagedUnits()
	if len(units) != 2 || units[0] != "jellyfin.service" || units[1] != "qbittorrent.service" {
		t.Errorf("ManagedUnits() = %v", units)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/does/not/exist/config.yaml"); err == nil {
		t.Error("missing config file did not produce an error")
	}
}
