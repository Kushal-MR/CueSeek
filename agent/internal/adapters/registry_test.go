package adapters

import (
	"errors"
	"strings"
	"testing"

	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

func fakeFactory(cfg config.Service, _ Deps) (Service, error) {
	return &fakeService{id: cfg.ID, name: cfg.Name}, nil
}

func controllableFactory(cfg config.Service, _ Deps) (Service, error) {
	return &fakeControllable{fakeService: &fakeService{id: cfg.ID, name: cfg.Name}}, nil
}

func testConfig(services ...config.Service) config.Config {
	return config.Config{Services: services}
}

func TestRegistryBuildsConfiguredServices(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterFactory("fake", fakeFactory); err != nil {
		t.Fatalf("RegisterFactory: %v", err)
	}
	if err := r.RegisterFactory("controllable", controllableFactory); err != nil {
		t.Fatalf("RegisterFactory: %v", err)
	}

	cfg := testConfig(
		config.Service{ID: "jellyfin", Name: "Jellyfin", Type: "controllable"},
		config.Service{ID: "qbittorrent", Name: "qBittorrent", Type: "fake"},
	)
	if err := r.Build(cfg, Deps{}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Configuration order is preserved so the API and the config file agree.
	if got := r.IDs(); len(got) != 2 || got[0] != "jellyfin" || got[1] != "qbittorrent" {
		t.Errorf("IDs() = %v, want [jellyfin qbittorrent]", got)
	}
	if svc, ok := r.Service("jellyfin"); !ok || svc.Name() != "Jellyfin" {
		t.Errorf("Service(jellyfin) = %v, %v", svc, ok)
	}
	if _, ok := r.Service("nope"); ok {
		t.Error("Service returned an unconfigured id")
	}
}

// TestRegistryExposesDiscoveredCapabilities is requirement 2's "expose discovered
// capabilities": the registry reports what each built adapter can actually do, derived
// from the concrete type rather than from anything the configuration claimed.
func TestRegistryExposesDiscoveredCapabilities(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterFactory("fake", fakeFactory)
	_ = r.RegisterFactory("controllable", controllableFactory)

	cfg := testConfig(
		config.Service{ID: "with-control", Type: "controllable"},
		config.Service{ID: "without-control", Type: "fake"},
	)
	if err := r.Build(cfg, Deps{}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	withControl, ok := r.Capabilities("with-control")
	if !ok {
		t.Fatal("no capabilities for with-control")
	}
	if len(withControl) != 2 {
		t.Errorf("capabilities = %v, want health and control", withControl)
	}

	withoutControl, _ := r.Capabilities("without-control")
	for _, c := range withoutControl {
		if c.ID == domain.CapabilityControl.ID {
			t.Error("a service without Controllable advertises control")
		}
	}

	if _, ok := r.Capabilities("nope"); ok {
		t.Error("Capabilities returned true for an unknown service")
	}
}

// TestEveryConfiguredServiceRegisters is requirement 2's explicit ask. The loop asserts
// each configured id resolves to a built adapter — the failure it guards against is a
// factory that silently skips an entry, leaving the agent running without the service its
// operator asked for.
func TestEveryConfiguredServiceRegisters(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterFactory("fake", fakeFactory)

	cfg := testConfig(
		config.Service{ID: "one", Type: "fake"},
		config.Service{ID: "two", Type: "fake"},
		config.Service{ID: "three", Type: "fake"},
	)
	if err := r.Build(cfg, Deps{}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(r.Services()) != len(cfg.Services) {
		t.Fatalf("built %d services, configured %d", len(r.Services()), len(cfg.Services))
	}
	for _, svcCfg := range cfg.Services {
		svc, ok := r.Service(svcCfg.ID)
		if !ok {
			t.Errorf("configured service %q did not register", svcCfg.ID)
			continue
		}
		if svc.ID() != svcCfg.ID {
			t.Errorf("service %q built with id %q", svcCfg.ID, svc.ID())
		}
		if caps, ok := r.Capabilities(svcCfg.ID); !ok || len(caps) == 0 {
			t.Errorf("service %q advertises no capabilities", svcCfg.ID)
		}
	}
}

// TestBuildRejectsUnknownType: an unrecognised type fails startup rather than being
// skipped. A console that silently omits the service you are trying to watch is worse
// than one that refuses to start and says which line is wrong.
func TestBuildRejectsUnknownType(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterFactory("fake", fakeFactory)

	err := r.Build(testConfig(config.Service{ID: "x", Type: "plex"}), Deps{})
	if err == nil {
		t.Fatal("unknown adapter type was accepted")
	}
	// The message must list what is available, or the operator is left guessing at the
	// spelling of a type they cannot see.
	if !strings.Contains(err.Error(), "fake") {
		t.Errorf("error does not list known types: %v", err)
	}
}

func TestBuildRejectsDuplicateServiceIDs(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterFactory("fake", fakeFactory)

	err := r.Build(testConfig(
		config.Service{ID: "same", Type: "fake"},
		config.Service{ID: "same", Type: "fake"},
	), Deps{})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("err = %v, want a duplicate-id error", err)
	}
}

// TestBuildPropagatesFactoryErrors: an adapter that cannot be configured must stop
// startup, not be quietly absent.
func TestBuildPropagatesFactoryErrors(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterFactory("broken", func(config.Service, Deps) (Service, error) {
		return nil, errors.New("base_url is required")
	})

	err := r.Build(testConfig(config.Service{ID: "x", Type: "broken"}), Deps{})
	if err == nil {
		t.Fatal("factory error did not fail the build")
	}
	// The service id must appear, or a multi-service config gives no clue which entry
	// is at fault.
	if !strings.Contains(err.Error(), "x") || !strings.Contains(err.Error(), "base_url") {
		t.Errorf("error lacks context: %v", err)
	}
}

func TestBuildRejectsNilServiceFromFactory(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterFactory("nil", func(config.Service, Deps) (Service, error) {
		return nil, nil
	})
	if err := r.Build(testConfig(config.Service{ID: "x", Type: "nil"}), Deps{}); err == nil {
		t.Error("a nil service with no error was accepted; it would panic on first poll")
	}
}

// TestBuildRejectsIDMismatch: a factory that ignores the configured id would leave the
// service unreachable through the API by the name its operator gave it.
func TestBuildRejectsIDMismatch(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterFactory("wrong-id", func(config.Service, Deps) (Service, error) {
		return &fakeService{id: "something-else"}, nil
	})
	if err := r.Build(testConfig(config.Service{ID: "configured", Type: "wrong-id"}), Deps{}); err == nil {
		t.Error("factory returning a mismatched id was accepted")
	}
}

func TestRegisterFactoryValidation(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterFactory("", fakeFactory); err == nil {
		t.Error("empty adapter type was accepted")
	}
	if err := r.RegisterFactory("x", nil); err == nil {
		t.Error("nil factory was accepted")
	}

	// Duplicate registration is rejected rather than silently overwritten: letting the
	// last one win would make which adapter you got depend on import order.
	if err := r.RegisterFactory("dup", fakeFactory); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if err := r.RegisterFactory("dup", fakeFactory); err == nil {
		t.Error("duplicate adapter type was accepted")
	}
}

func TestEmptyConfigBuildsNothing(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterFactory("fake", fakeFactory)

	if err := r.Build(testConfig(), Deps{}); err != nil {
		t.Fatalf("Build with no services: %v", err)
	}
	if len(r.Services()) != 0 {
		t.Errorf("built %d services from an empty config", len(r.Services()))
	}
	if r.IDs() == nil {
		t.Log("IDs() is nil for an empty registry, which callers must tolerate")
	}
}

func TestKnownTypesIsSorted(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"zebra", "alpha", "middle"} {
		_ = r.RegisterFactory(name, fakeFactory)
	}
	got := r.KnownTypes()
	if len(got) != 3 || got[0] != "alpha" || got[1] != "middle" || got[2] != "zebra" {
		t.Errorf("KnownTypes() = %v, want sorted", got)
	}
}
