package adapters

import (
	"fmt"
	"slices"
	"sort"

	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// Factory builds one adapter instance from its configuration.
//
// Registered against a config `type` value. The type is what selects an implementation;
// the id names the instance — which is why two Jellyfin servers on one host are two
// entries of type "jellyfin" with different ids, not a special case anywhere in the code.
type Factory func(cfg config.Service, deps Deps) (Service, error)

// Registry maps configured services to adapter implementations.
//
// The alternative — a switch on cfg.Type inside a build function — works fine at two
// adapters and rots predictably: every new service edits a growing switch in a file that
// has nothing to do with it, and the switch becomes a merge-conflict magnet. A map keyed
// by type means adding a service is one registration line (ADR-0005).
type Registry struct {
	factories map[string]Factory

	services map[string]Service
	order    []string // configuration order, for stable output
}

// NewRegistry returns an empty registry with no adapter types known to it.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
		services:  make(map[string]Service),
	}
}

// RegisterFactory makes an adapter type available for use in configuration.
//
// Rejects duplicates rather than overwriting. Two packages claiming the same type name is
// a build-time mistake, and silently letting the last registration win would make which
// adapter you got depend on import order.
func (r *Registry) RegisterFactory(adapterType string, factory Factory) error {
	if adapterType == "" {
		return fmt.Errorf("adapters: adapter type must not be empty")
	}
	if factory == nil {
		return fmt.Errorf("adapters: factory for %q must not be nil", adapterType)
	}
	if _, exists := r.factories[adapterType]; exists {
		return fmt.Errorf("adapters: adapter type %q is already registered", adapterType)
	}
	r.factories[adapterType] = factory
	return nil
}

// KnownTypes lists registered adapter types, sorted.
func (r *Registry) KnownTypes() []string {
	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// Build instantiates every service in the configuration.
//
// All-or-nothing: one bad service entry fails startup rather than leaving the agent
// running with a silently missing service. An operations console that quietly omits the
// thing you are trying to watch is worse than one that refuses to start and says why.
func (r *Registry) Build(cfg config.Config, deps Deps) error {
	for i, svcCfg := range cfg.Services {
		factory, known := r.factories[svcCfg.Type]
		if !known {
			return fmt.Errorf(
				"adapters: services[%d] (%q) has unknown type %q; known types: %v",
				i, svcCfg.ID, svcCfg.Type, r.KnownTypes())
		}
		if _, duplicate := r.services[svcCfg.ID]; duplicate {
			return fmt.Errorf("adapters: services[%d]: duplicate service id %q", i, svcCfg.ID)
		}

		service, err := factory(svcCfg, deps)
		if err != nil {
			return fmt.Errorf("adapters: services[%d] (%q): %w", i, svcCfg.ID, err)
		}
		if service == nil {
			return fmt.Errorf("adapters: services[%d] (%q): factory returned nil", i, svcCfg.ID)
		}
		// The factory is trusted to honour the configured id. Checking is cheap, and a
		// mismatch would make the service unreachable through the API by the id its
		// operator configured.
		if service.ID() != svcCfg.ID {
			return fmt.Errorf("adapters: services[%d]: factory built id %q, configured id is %q",
				i, service.ID(), svcCfg.ID)
		}

		r.services[svcCfg.ID] = service
		r.order = append(r.order, svcCfg.ID)
	}
	return nil
}

// Service returns one built adapter.
func (r *Registry) Service(id string) (Service, bool) {
	s, ok := r.services[id]
	return s, ok
}

// Services returns every built adapter in configuration order, so the API and the config
// file list them the same way.
func (r *Registry) Services() []Service {
	out := make([]Service, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.services[id])
	}
	return out
}

// IDs returns built service ids in configuration order.
func (r *Registry) IDs() []string { return slices.Clone(r.order) }

// Capabilities returns what a built service supports.
func (r *Registry) Capabilities(id string) ([]domain.Capability, bool) {
	service, ok := r.services[id]
	if !ok {
		return nil, false
	}
	return CapabilitiesOf(service), true
}
