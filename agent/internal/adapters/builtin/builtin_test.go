package builtin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/adapters/jellyfin"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// These tests exercise the real registry with the real adapters, against a real
// configuration — the closest thing to what the agent does at startup without opening a
// socket.

func TestNewRegistryRegistersEveryBuiltinType(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	known := registry.KnownTypes()
	if len(known) != len(factories) {
		t.Errorf("registry knows %d types, builtin declares %d", len(known), len(factories))
	}
	for declared := range factories {
		var found bool
		for _, k := range known {
			if k == declared {
				found = true
			}
		}
		if !found {
			t.Errorf("declared type %q was not registered", declared)
		}
	}
	if !strings.Contains(strings.Join(known, ","), jellyfin.Type) {
		t.Errorf("jellyfin is not registered: %v", known)
	}
}

// TestEveryConfiguredAdapterRegisters is requirement 2's explicit ask, run against the
// production factories rather than fakes. A configuration naming two Jellyfin instances
// must produce two independent adapters — the case that proves `type` selects the
// implementation while `id` names the instance.
func TestEveryConfiguredAdapterRegisters(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	cfg := config.Config{Services: []config.Service{
		{
			ID: "jellyfin", Name: "Jellyfin", Type: jellyfin.Type,
			Unit: "jellyfin.service", BaseURL: "http://127.0.0.1:8096",
			APIKey: "key-one", PollInterval: 30 * time.Second,
		},
		{
			ID: "jellyfin-attic", Name: "Attic Jellyfin", Type: jellyfin.Type,
			BaseURL: "http://127.0.0.1:8097", APIKey: "key-two",
			PollInterval: time.Minute,
		},
	}}

	if err := registry.Build(cfg, adapters.Deps{Units: nil}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(registry.Services()) != len(cfg.Services) {
		t.Fatalf("built %d adapters for %d configured services",
			len(registry.Services()), len(cfg.Services))
	}
	for _, svcCfg := range cfg.Services {
		svc, ok := registry.Service(svcCfg.ID)
		if !ok {
			t.Errorf("configured service %q did not register", svcCfg.ID)
			continue
		}
		if svc.ID() != svcCfg.ID || svc.Name() != svcCfg.Name {
			t.Errorf("%q built as %q/%q", svcCfg.ID, svc.ID(), svc.Name())
		}
		caps, _ := registry.Capabilities(svcCfg.ID)
		if len(caps) == 0 {
			t.Errorf("%q advertises no capabilities", svcCfg.ID)
		}
	}
}

// TestCapabilityDiscoveryReflectsConfiguration: with no host layer wired in, no adapter
// can restart anything, so none may advertise control. The capability is a statement
// about what will actually work.
func TestCapabilityDiscoveryReflectsConfiguration(t *testing.T) {
	registry, _ := NewRegistry()

	cfg := config.Config{Services: []config.Service{{
		ID: "jellyfin", Type: jellyfin.Type, Unit: "jellyfin.service",
		BaseURL: "http://127.0.0.1:8096", APIKey: "k",
	}}}

	if err := registry.Build(cfg, adapters.Deps{Units: nil}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	caps, ok := registry.Capabilities("jellyfin")
	if !ok {
		t.Fatal("no capabilities for jellyfin")
	}
	var health, control bool
	for _, c := range caps {
		switch c.ID {
		case domain.CapabilityHealth.ID:
			health = true
		case domain.CapabilityControl.ID:
			control = true
		}
	}
	if !health {
		t.Error("jellyfin does not advertise health")
	}
	if control {
		t.Error("jellyfin advertises control with no host layer available")
	}
}

// TestBuildRejectsMisconfiguredAdapter: a Jellyfin entry without an API key fails
// startup. The agent refuses to run half-configured rather than showing a permanently
// degraded service whose cause is buried in a log line from thirty minutes ago.
func TestBuildRejectsMisconfiguredAdapter(t *testing.T) {
	registry, _ := NewRegistry()

	err := registry.Build(config.Config{Services: []config.Service{{
		ID: "jellyfin", Type: jellyfin.Type, BaseURL: "http://127.0.0.1:8096",
	}}}, adapters.Deps{})

	if err == nil {
		t.Fatal("a Jellyfin service with no api_key was accepted")
	}
	if !strings.Contains(err.Error(), "jellyfin") || !strings.Contains(err.Error(), "api_key") {
		t.Errorf("error lacks context: %v", err)
	}
}

func TestRegisterRejectsDuplicateRegistration(t *testing.T) {
	registry := adapters.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := Register(registry); err == nil {
		t.Error("registering the built-ins twice was accepted")
	}
}

// TestPollerRunsOverBuiltAdapters ties the whole milestone together: real registry, real
// adapter, real poller. Nothing is listening on the configured port, so the expected
// outcome is a recorded `unreachable` — which is the poller doing its job, not failing.
func TestPollerRunsOverBuiltAdapters(t *testing.T) {
	registry, _ := NewRegistry()

	cfg := config.Config{Services: []config.Service{{
		ID: "jellyfin", Type: jellyfin.Type,
		// Port 1 is reserved and nothing will be listening.
		BaseURL: "http://127.0.0.1:1", APIKey: "k",
		PollInterval: 50 * time.Millisecond, Timeout: 20 * time.Millisecond,
	}}}
	if err := registry.Build(cfg, adapters.Deps{}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	poller := adapters.NewPoller(registry, cfg)

	// Before starting, the service is tracked and unknown — never absent.
	snapshot, ok := poller.Cache().Get("jellyfin")
	if !ok || snapshot.Health.Status != domain.StatusUnknown {
		t.Fatalf("pre-poll snapshot = %+v, ok = %v; want unknown", snapshot, ok)
	}

	ctx, cancel := context.WithCancel(t.Context())
	poller.Start(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s, _ := poller.Cache().Get("jellyfin"); s.Health.Status == domain.StatusUnreachable {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	final, _ := poller.Cache().Get("jellyfin")
	if final.Health.Status != domain.StatusUnreachable {
		t.Errorf("status = %q, want unreachable for a port nothing is listening on",
			final.Health.Status)
	}
	if final.Health.Reachable {
		t.Error("reachable = true for a refused connection")
	}
	if final.Health.ObservedAt.IsZero() {
		t.Error("no observation timestamp recorded")
	}

	cancel()
	poller.Wait()
}
