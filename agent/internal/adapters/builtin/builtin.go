// Package builtin wires the compiled-in adapters into a registry.
//
// It exists to break an import cycle, and the cycle is a consequence of the design being
// right: each adapter imports internal/adapters for the Service and capability
// interfaces, so internal/adapters cannot import the adapters back. Registration
// therefore lives one level up, in a package that imports both.
//
// The practical effect is the measurement ADR-0011 sets for M6 — "adding a third adapter,
// how many files changed outside its own package?" The intended answer is two: this file,
// and the config that names the new service. If a future adapter needs more than that,
// the capability model has a gap worth finding before a fourth arrives.
//
// No init() registration. Import-for-side-effect would make the set of available adapters
// depend on which packages happen to be linked in, which is invisible at the call site
// and awkward to vary in tests. An explicit list is greppable.
package builtin

import (
	"fmt"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/adapters/jellyfin"
	"github.com/Kushal-MR/CueSeek/agent/internal/adapters/qbittorrent"
	"github.com/Kushal-MR/CueSeek/agent/internal/adapters/systemd"
)

// factories is the complete set of adapter types this build understands.
//
// One line per adapter. This is the file a new adapter touches.
//
// Two tiers, and the difference is what the adapter observes rather than how it is wired.
// jellyfin and qbittorrent ask the service about itself and can therefore report what it
// is doing; systemd asks the init system about the process and can only report whether it
// is running. Both arrive here identically, which is the point.
var factories = map[string]adapters.Factory{
	jellyfin.Type:    jellyfin.New,
	qbittorrent.Type: qbittorrent.New,
	systemd.Type:     systemd.New,
}

// Types lists the adapter types available in this build.
func Types() []string {
	out := make([]string, 0, len(factories))
	for t := range factories {
		out = append(out, t)
	}
	return out
}

// Register adds every built-in adapter type to a registry.
func Register(registry *adapters.Registry) error {
	for adapterType, factory := range factories {
		if err := registry.RegisterFactory(adapterType, factory); err != nil {
			return fmt.Errorf("builtin: %w", err)
		}
	}
	return nil
}

// NewRegistry returns a registry with every built-in adapter type registered.
func NewRegistry() (*adapters.Registry, error) {
	registry := adapters.NewRegistry()
	if err := Register(registry); err != nil {
		return nil, err
	}
	return registry, nil
}
