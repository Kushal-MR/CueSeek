// Package config loads and validates the agent's configuration file.
//
// Configuration is the agent's only source of host-specific truth: which address to
// listen on, where the database lives, and which services it manages. None of that can
// be compiled in — M0 demonstrated why concretely. The qBittorrent unit on the target
// host is named `qbittorrent.service` despite the unit describing itself as
// "qBittorrent-nox", so a hardcoded guess would have been wrong on the very first
// machine (see docs/m0-findings.md).
//
// Validation is deliberately strict and fails at startup. A daemon that boots with a
// misconfigured allowlist and discovers it only when someone taps "restart" has turned a
// typo into an incident.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultPath is where the packaged systemd unit will look for the config file.
const DefaultPath = "/etc/cueseek/config.yaml"

// Config is the whole configuration file.
type Config struct {
	Bind     Bind      `yaml:"bind"`
	Storage  Storage   `yaml:"storage"`
	Services []Service `yaml:"services"`
}

// Bind controls the network address the API listens on.
type Bind struct {
	// Address is a host:port pair, e.g. "127.0.0.1:7777" or "100.64.0.5:7777".
	Address string `yaml:"address"`

	// AllowUnrestricted permits binding to a wildcard address (0.0.0.0 or ::).
	//
	// Defaults to false, and validation rejects wildcard binds without it. ADR-0001
	// delegated transport security to the VPN and accepted, in exchange, that this API
	// is never exposed broadly — it can power off the machine and has no TLS of its own.
	//
	// "Bind narrowly" as advice gets ignored under deadline pressure; as a startup
	// failure with an explicit escape hatch, widening becomes a decision someone made on
	// purpose and can be found in a config diff.
	AllowUnrestricted bool `yaml:"allow_unrestricted"`
}

// Storage configures persistence.
type Storage struct {
	// Path is the SQLite database file. Its directory must exist and be writable by the
	// cueseek user; the packaged systemd unit provides it via StateDirectory.
	Path string `yaml:"path"`
}

// Service describes one managed service.
//
// `Unit` is the allowlist that ADR-0002 requires to be enforced in two places: here, and
// in the polkit rule shipped in deploy/. Duplication is intentional. The polkit rule is
// the boundary the operator can audit without reading Go; this copy means a
// misconfiguration is refused before it ever reaches D-Bus.
type Service struct {
	// ID is the stable identifier used in API paths, e.g. "jellyfin".
	ID string `yaml:"id"`
	// Name is the display name, e.g. "Jellyfin".
	Name string `yaml:"name"`
	// Type selects the adapter implementation. Distinct from ID so that two instances of
	// the same software can be managed on one host.
	Type string `yaml:"type"`
	// Unit is the exact systemd unit name. Exact: M0 found the real name differs from
	// the one the unit's own description implies.
	Unit string `yaml:"unit"`
	// BaseURL is where the adapter reaches the service's own API.
	BaseURL string `yaml:"base_url"`
	// PollInterval is how often the agent refreshes this service's state. The agent
	// polls on its own schedule and serves cached state; client requests never trigger
	// upstream calls (ADR-0003).
	PollInterval time.Duration `yaml:"poll_interval"`

	// Timeout bounds a single upstream request. Must be shorter than PollInterval, or
	// slow polls pile up on each other.
	Timeout time.Duration `yaml:"timeout"`

	// APIKey authenticates to the service's own API.
	//
	// Putting a secret in the configuration file means the file itself is a secret: the
	// packaged install must ship it 0640, owned by the cueseek user. Prefer APIKeyFile
	// where the deployment has somewhere better to keep it.
	APIKey string `yaml:"api_key"`

	// APIKeyFile reads the key from a separate file, trimmed of whitespace.
	//
	// Exists so the key can live somewhere with its own permissions and its own backup
	// policy — and so a config file can be committed to a private repo or pasted into a
	// support request without leaking a credential. Takes precedence over APIKey.
	APIKeyFile string `yaml:"api_key_file"`
}

// Defaults returns a Config with every optional field populated.
//
// The default bind address is loopback. It is the only choice that is safe when someone
// installs the agent, skips the documentation, and starts it.
func Defaults() Config {
	return Config{
		Bind:    Bind{Address: "127.0.0.1:7777"},
		Storage: Storage{Path: "/var/lib/cueseek/cueseek.db"},
	}
}

const defaultPollInterval = 30 * time.Second

// Load reads, parses and validates the configuration file at path, then resolves any
// secrets held in separate files.
//
// Secret resolution happens here rather than in Parse so that Parse stays a pure function
// of its input — testable without a filesystem, and incapable of reading a file because
// of something a config said.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg, err := Parse(raw)
	if err != nil {
		return Config{}, err
	}
	if err := cfg.resolveSecrets(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) resolveSecrets() error {
	for i, svc := range c.Services {
		if svc.APIKeyFile == "" {
			continue
		}
		raw, err := os.ReadFile(svc.APIKeyFile)
		if err != nil {
			return fmt.Errorf("services[%d] (%q): read api_key_file: %w", i, svc.ID, err)
		}
		// Trimmed because a key file almost always ends with a newline, and a trailing
		// \n in an Authorization header fails as a bafflingly generic 401.
		key := strings.TrimSpace(string(raw))
		if key == "" {
			return fmt.Errorf("services[%d] (%q): api_key_file %s is empty",
				i, svc.ID, svc.APIKeyFile)
		}
		c.Services[i].APIKey = key
	}
	return nil
}

// Parse validates configuration from raw YAML bytes.
//
// Separate from Load so tests exercise the real parser against real bytes rather than
// constructing structs directly — which would test nothing about the YAML mapping.
func Parse(raw []byte) (Config, error) {
	cfg := Defaults()

	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	// Reject unknown fields. A typo like `adress:` would otherwise be silently ignored
	// and the agent would bind to the default, which is the kind of failure that costs
	// an hour because everything reports success.
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.normalise(); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) normalise() error {
	for i := range c.Services {
		if c.Services[i].PollInterval == 0 {
			c.Services[i].PollInterval = defaultPollInterval
		}
		if c.Services[i].Name == "" {
			c.Services[i].Name = c.Services[i].ID
		}
	}
	return nil
}

// Validate reports every problem it finds, not just the first.
//
// A config file with three mistakes should take one round trip to fix, not three.
func (c Config) Validate() error {
	var errs []error

	errs = append(errs, c.Bind.validate()...)

	if c.Storage.Path == "" {
		errs = append(errs, errors.New("storage.path must not be empty"))
	} else if !isAbsPath(c.Storage.Path) {
		// A relative path resolves against the working directory, which for a systemd
		// unit is not where anyone expects.
		errs = append(errs, fmt.Errorf("storage.path %q must be absolute", c.Storage.Path))
	}

	seen := make(map[string]bool, len(c.Services))
	for i, s := range c.Services {
		errs = append(errs, s.validate(i)...)
		if s.ID != "" {
			if seen[s.ID] {
				errs = append(errs, fmt.Errorf("services[%d]: duplicate id %q", i, s.ID))
			}
			seen[s.ID] = true
		}
	}

	return errors.Join(errs...)
}

// isAbsPath reports whether p is absolute, accepting Unix form on every platform.
//
// filepath.IsAbs is host-relative: on Windows it rejects "/var/lib/cueseek/cueseek.db"
// because Windows absolute paths start with a drive letter. But this config describes a
// Linux deployment and is routinely parsed on a Windows development machine — by tests,
// and by anyone validating a config before shipping it. Judging a Linux path by Windows
// rules would make every valid production config look broken.
//
// The host's own convention is still accepted so that a Windows path works if the agent
// is ever run there.
func isAbsPath(p string) bool {
	return strings.HasPrefix(p, "/") || filepath.IsAbs(p)
}

func (b Bind) validate() []error {
	if b.Address == "" {
		return []error{errors.New("bind.address must not be empty")}
	}

	host, port, err := net.SplitHostPort(b.Address)
	if err != nil {
		return []error{fmt.Errorf("bind.address %q is not host:port: %w", b.Address, err)}
	}
	if port == "" {
		return []error{fmt.Errorf("bind.address %q has no port", b.Address)}
	}

	// An empty host means "all interfaces" just as surely as 0.0.0.0 does.
	if host == "" || host == "0.0.0.0" || host == "::" {
		if !b.AllowUnrestricted {
			return []error{fmt.Errorf(
				"bind.address %q listens on all interfaces; CueSeek can power off this "+
					"machine and has no TLS of its own, so this is refused by default "+
					"(ADR-0001). Bind to a specific address — loopback or the tailnet "+
					"interface — or set bind.allow_unrestricted: true deliberately",
				b.Address)}
		}
	}
	return nil
}

func (s Service) validate(i int) []error {
	var errs []error
	require := func(field, value string) {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, fmt.Errorf("services[%d]: %s must not be empty", i, field))
		}
	}
	require("id", s.ID)
	require("type", s.Type)
	require("unit", s.Unit)

	// systemd units are always suffixed. Catching "jellyfin" here turns a confusing
	// D-Bus error at action time into a startup message naming the file to fix.
	if s.Unit != "" && !strings.Contains(s.Unit, ".") {
		errs = append(errs, fmt.Errorf(
			"services[%d]: unit %q has no type suffix (did you mean %q?)",
			i, s.Unit, s.Unit+".service"))
	}
	if s.PollInterval < time.Second {
		errs = append(errs, fmt.Errorf(
			"services[%d]: poll_interval %s is too aggressive; minimum is 1s",
			i, s.PollInterval))
	}
	if s.Timeout < 0 {
		errs = append(errs, fmt.Errorf("services[%d]: timeout must not be negative", i))
	}
	// A request budget at or beyond the poll interval lets one slow poll still be running
	// when the next begins, and they accumulate.
	if s.Timeout > 0 && s.PollInterval > 0 && s.Timeout >= s.PollInterval {
		errs = append(errs, fmt.Errorf(
			"services[%d]: timeout %s must be shorter than poll_interval %s",
			i, s.Timeout, s.PollInterval))
	}
	if s.APIKey != "" && s.APIKeyFile != "" {
		errs = append(errs, fmt.Errorf(
			"services[%d]: set api_key or api_key_file, not both", i))
	}
	// base_url is not required here: whether a service needs one is a property of its
	// adapter, not of configuration. The factory validates that (see internal/adapters).
	if s.BaseURL != "" {
		if u, err := url.Parse(s.BaseURL); err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf(
				"services[%d]: base_url %q must be an absolute URL like http://127.0.0.1:8096",
				i, s.BaseURL))
		}
	}
	return errs
}

// ManagedUnits returns the allowlist of systemd units, for the host layer to enforce
// before any D-Bus call is attempted.
func (c Config) ManagedUnits() []string {
	units := make([]string, 0, len(c.Services))
	for _, s := range c.Services {
		units = append(units, s.Unit)
	}
	return units
}
