package config

import (
	"os"
	"path/filepath"
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
		// "service without unit" is deliberately absent: a unit is optional, and such a
		// service is observed but not controlled. See TestUnitIsOptional.
		"unit missing suffix": {
			"services:\n  - id: j\n    type: jellyfin\n    unit: jellyfin\n", "type suffix"},
		// Whitespace is a typo, not a deliberate absence. Omitting `unit` entirely is how
		// you say "not controlled"; this says "controlled, by a unit named nothing".
		"unit is only whitespace": {
			"services:\n  - id: j\n    type: jellyfin\n    unit: \"   \"\n", "type suffix"},
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

// TestUnitIsOptional: a service CueSeek can reach but cannot act on is a real
// configuration, not a mistake. Something installed from a container image has an HTTP API
// and a web interface and no unit to restart, and refusing it outright would refuse a
// configuration the adapters would have served correctly.
func TestUnitIsOptional(t *testing.T) {
	cfg, err := Parse([]byte(`
services:
  - id: jellyfin
    type: jellyfin
    base_url: http://127.0.0.1:8096
`))
	if err != nil {
		t.Fatalf("a service without a unit was refused: %v", err)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("parsed %d services, want 1", len(cfg.Services))
	}
	if cfg.Services[0].Unit != "" {
		t.Errorf("Unit = %q, want empty", cfg.Services[0].Unit)
	}
}

// TestManagedUnitsOmitsServicesWithoutAUnit guards the seam between the two changes above.
//
// host.NewWithBackend refuses an empty unit name outright, and is right to — a blank
// string in a security allowlist is a bug, not a permissive setting. So the moment `unit`
// became optional, an unfiltered ManagedUnits would have failed the agent at startup with
// "managed unit name must not be empty" for a configuration that is perfectly valid. The
// filtering belongs here, where the absence is a known fact.
func TestManagedUnitsOmitsServicesWithoutAUnit(t *testing.T) {
	cfg, err := Parse([]byte(`
services:
  - id: controlled
    type: jellyfin
    unit: jellyfin.service
  - id: observed-only
    type: jellyfin
    base_url: http://127.0.0.1:8096
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	units := cfg.ManagedUnits()
	if len(units) != 1 || units[0] != "jellyfin.service" {
		t.Fatalf("ManagedUnits() = %v, want exactly [jellyfin.service]", units)
	}
}

// TestManagedUnitsFiltersBlankUnits covers the case validation should already have
// stopped.
//
// A whitespace-only unit is refused by Parse — it reads as a typo rather than as a
// deliberate absence, and saying so at startup is more useful than silently treating it as
// "not controlled". This constructs the Config directly to reach ManagedUnits anyway,
// because the filter is a second line of defence and a second line of defence that is only
// exercised through the first is not tested at all.
func TestManagedUnitsFiltersBlankUnits(t *testing.T) {
	cfg := Config{Services: []Service{
		{ID: "real", Unit: "jellyfin.service"},
		{ID: "blank", Unit: ""},
		{ID: "whitespace", Unit: "   "},
	}}

	units := cfg.ManagedUnits()
	if len(units) != 1 || units[0] != "jellyfin.service" {
		t.Fatalf("ManagedUnits() = %#v, want exactly [jellyfin.service]", units)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/does/not/exist/config.yaml"); err == nil {
		t.Error("missing config file did not produce an error")
	}
}

// ---------------------------------------------------------------- adapter fields

func TestServiceAdapterFields(t *testing.T) {
	cfg, err := Parse([]byte(`
services:
  - id: jellyfin
    type: jellyfin
    unit: jellyfin.service
    base_url: http://127.0.0.1:8096
    poll_interval: 30s
    timeout: 5s
    api_key: abc123
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	svc := cfg.Services[0]
	if svc.BaseURL != "http://127.0.0.1:8096" || svc.APIKey != "abc123" {
		t.Errorf("service = %+v", svc)
	}
	if svc.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", svc.Timeout)
	}
}

// TestTimeoutMustBeShorterThanInterval: a request budget at or beyond the poll interval
// lets one slow poll still be running when the next begins, and they accumulate.
func TestTimeoutMustBeShorterThanInterval(t *testing.T) {
	for name, yaml := range map[string]string{
		"equal": "services:\n  - id: j\n    type: jellyfin\n    unit: j.service\n" +
			"    poll_interval: 10s\n    timeout: 10s\n",
		"longer": "services:\n  - id: j\n    type: jellyfin\n    unit: j.service\n" +
			"    poll_interval: 10s\n    timeout: 30s\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(yaml))
			if err == nil {
				t.Fatal("accepted a timeout at or beyond the poll interval")
			}
			if !strings.Contains(err.Error(), "shorter than poll_interval") {
				t.Errorf("error does not explain the constraint: %v", err)
			}
		})
	}
}

func TestBaseURLMustBeAbsolute(t *testing.T) {
	for name, value := range map[string]string{
		"relative":  "/jellyfin",
		"no scheme": "127.0.0.1:8096",
		"bare host": "jellyfin",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(
				"services:\n  - id: j\n    type: jellyfin\n    unit: j.service\n" +
					"    base_url: \"" + value + "\"\n"))
			if err == nil || !strings.Contains(err.Error(), "base_url") {
				t.Errorf("err = %v, want a base_url error", err)
			}
		})
	}

	// base_url is optional at the config layer: whether a service needs one is a property
	// of its adapter, and the factory enforces that.
	if _, err := Parse([]byte(
		"services:\n  - id: j\n    type: jellyfin\n    unit: j.service\n")); err != nil {
		t.Errorf("a service without base_url was rejected by config: %v", err)
	}
}

func TestAPIKeyAndAPIKeyFileAreMutuallyExclusive(t *testing.T) {
	_, err := Parse([]byte(`
services:
  - id: j
    type: jellyfin
    unit: j.service
    api_key: abc
    api_key_file: /etc/cueseek/jellyfin.key
`))
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("err = %v, want a mutual-exclusion error", err)
	}
}

// TestLoadResolvesAPIKeyFile covers the whole point of api_key_file: the key lives
// somewhere with its own permissions, and the config file stays safe to share.
func TestLoadResolvesAPIKeyFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "jellyfin.key")
	// Trailing newline included on purpose: almost every key file has one, and an
	// untrimmed \n in an Authorization header fails as a bafflingly generic 401.
	if err := os.WriteFile(keyPath, []byte("secret-key-value\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	body := "storage:\n  path: " + filepath.ToSlash(dir) + "/c.db\n" +
		"services:\n  - id: jellyfin\n    type: jellyfin\n    unit: jellyfin.service\n" +
		"    base_url: http://127.0.0.1:8096\n" +
		"    api_key_file: " + filepath.ToSlash(keyPath) + "\n"
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Services[0].APIKey; got != "secret-key-value" {
		t.Errorf("APIKey = %q, want the trimmed file contents", got)
	}
}

func TestLoadRejectsUnreadableOrEmptyKeyFile(t *testing.T) {
	dir := t.TempDir()

	writeConfig := func(t *testing.T, keyPath string) string {
		t.Helper()
		configPath := filepath.Join(dir, "config-"+filepath.Base(keyPath)+".yaml")
		body := "storage:\n  path: " + filepath.ToSlash(dir) + "/c.db\n" +
			"services:\n  - id: jellyfin\n    type: jellyfin\n    unit: jellyfin.service\n" +
			"    api_key_file: " + filepath.ToSlash(keyPath) + "\n"
		if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return configPath
	}

	t.Run("missing", func(t *testing.T) {
		path := writeConfig(t, filepath.Join(dir, "absent.key"))
		if _, err := Load(path); err == nil {
			t.Error("a missing api_key_file was accepted")
		}
	})

	t.Run("empty", func(t *testing.T) {
		empty := filepath.Join(dir, "empty.key")
		if err := os.WriteFile(empty, []byte("   \n"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		path := writeConfig(t, empty)
		// An empty key file would otherwise become an empty header and a confusing 401.
		if _, err := Load(path); err == nil {
			t.Error("an empty api_key_file was accepted")
		}
	})
}

// TestParseDoesNotTouchTheFilesystem: Parse is a pure function of its input, so a config
// cannot cause a file read merely by being parsed.
func TestParseDoesNotTouchTheFilesystem(t *testing.T) {
	cfg, err := Parse([]byte(
		"services:\n  - id: j\n    type: jellyfin\n    unit: j.service\n" +
			"    api_key_file: /does/not/exist.key\n"))
	if err != nil {
		t.Fatalf("Parse tried to read the file: %v", err)
	}
	if cfg.Services[0].APIKey != "" {
		t.Error("Parse resolved a secret; that belongs to Load")
	}
}

// TestShippedExampleConfigIsValid parses deploy/config.example.yaml.
//
// A shipped example that fails validation breaks every fresh install, and the
// failure surfaces on someone else's machine rather than in CI. Parse rather than
// Load: api_key_file names a path that exists only on a deployed host.
func TestShippedExampleConfigIsValid(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", "config.example.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	cfg, err := Parse(raw)
	if err != nil {
		t.Fatalf("the shipped example config does not validate: %v", err)
	}

	// The defaults it ships with are the ones the documentation promises.
	if cfg.Bind.Address != "127.0.0.1:7777" {
		t.Errorf("example binds to %q; the safe default is loopback", cfg.Bind.Address)
	}
	if cfg.Bind.AllowUnrestricted {
		t.Error("the example ships with allow_unrestricted enabled")
	}
	if cfg.Storage.Path != "/var/lib/cueseek/cueseek.db" {
		t.Errorf("storage.path = %q, want the unit's StateDirectory", cfg.Storage.Path)
	}

	// The shipped example manages NOTHING, and that is the property under test.
	//
	// install.sh copies this file to /etc/cueseek/config.yaml on a fresh host. When it
	// shipped with Jellyfin active and an api_key_file that does not exist yet, the first
	// `systemctl start cueseekd` failed — on every machine, but informatively only on one
	// that runs Jellyfin. For anyone else the first thing CueSeek ever did was refuse to
	// start, citing somebody else's media server.
	//
	// An empty roster is a working install: the host's own vitals need no configuration
	// and no privilege, so there is something real to see immediately.
	if len(cfg.Services) != 0 {
		t.Fatalf("the shipped example activates %d service(s); it must activate none, "+
			"so that a fresh install starts on a machine that runs neither of them",
			len(cfg.Services))
	}

	// Absent is not the same as unhelpful. The examples must still be present, commented,
	// or a new operator has nothing to copy.
	text := string(raw)
	for _, want := range []string{
		"type: jellyfin",
		"type: qbittorrent",
		"api_key_file:",
		"web_ui:",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the example config no longer shows %q anywhere, "+
				"so there is nothing for a new operator to uncomment", want)
		}
	}

	// No secret may ever be committed here. Parse() would have surfaced an inline key on
	// an active service; this catches one left in a commented example, which Parse never
	// sees and review reliably misses.
	for _, forbidden := range []string{"api_key: ", "password: "} {
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
			if !strings.HasPrefix(trimmed, forbidden) {
				continue
			}
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, forbidden))
			// Placeholders are the point of an example; real-looking values are not.
			if value != "YOUR_KEY_HERE" && value != "YOUR_PASSWORD" {
				t.Errorf("the example config may contain a real secret: %q", line)
			}
		}
	}
}

// ---------------------------------------------------------------- credentials

// TestLoadResolvesPasswordFile — a password read differently from an API key is a password
// that fails in a way nobody reproduces, so both go through the same loader.
func TestLoadResolvesPasswordFile(t *testing.T) {
	dir := t.TempDir()
	pwPath := filepath.Join(dir, "qbittorrent.pass")
	if err := os.WriteFile(pwPath, []byte("hunter2\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}

	configPath := filepath.Join(dir, "config.yaml")
	body := "storage:\n  path: " + filepath.ToSlash(dir) + "/c.db\n" +
		"services:\n  - id: qbittorrent\n    type: qbittorrent\n" +
		"    unit: qbittorrent.service\n" +
		"    base_url: http://127.0.0.1:8080\n" +
		"    username: admin\n" +
		"    password_file: " + filepath.ToSlash(pwPath) + "\n"
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Services[0].Password; got != "hunter2" {
		t.Errorf("Password = %q, want the trimmed file contents", got)
	}
	if got := cfg.Services[0].Username; got != "admin" {
		t.Errorf("Username = %q", got)
	}
}

func TestPasswordAndPasswordFileAreMutuallyExclusive(t *testing.T) {
	_, err := Parse([]byte(
		"services:\n  - id: qbittorrent\n    type: qbittorrent\n" +
			"    base_url: http://127.0.0.1:8080\n" +
			"    username: admin\n    password: a\n    password_file: /tmp/b\n"))
	if err == nil {
		t.Fatal("setting both password and password_file must be refused")
	}
}

// TestUsernameWithoutASecretIsRefused — almost always a half-finished edit, and it would
// otherwise surface as a login rejection from the service rather than as a config error.
func TestUsernameWithoutASecretIsRefused(t *testing.T) {
	_, err := Parse([]byte(
		"services:\n  - id: qbittorrent\n    type: qbittorrent\n" +
			"    base_url: http://127.0.0.1:8080\n    username: admin\n"))
	if err == nil {
		t.Fatal("a username with no password must be refused")
	}
	if !strings.Contains(err.Error(), "password") {
		t.Errorf("the error should name what is missing, got %v", err)
	}
}

// TestCredentialsAreOptional — qBittorrent's localhost auth bypass is the common setup,
// and a config that omits credentials entirely must remain valid.
func TestCredentialsAreOptional(t *testing.T) {
	cfg, err := Parse([]byte(
		"services:\n  - id: qbittorrent\n    type: qbittorrent\n" +
			"    unit: qbittorrent.service\n    base_url: http://127.0.0.1:8080\n"))
	if err != nil {
		t.Fatalf("credentials must be optional: %v", err)
	}
	if cfg.Services[0].Username != "" || cfg.Services[0].Password != "" {
		t.Error("nothing should have been invented")
	}
}

// ---------------------------------------------------------------- host metrics

func TestHostMetricsDefaults(t *testing.T) {
	// A config that never mentions the host must still measure it. Metrics are the kind
	// of thing an operator expects to work out of the box, and requiring a block to turn
	// them on would mean most installs silently have none.
	cfg, err := Parse([]byte("services: []\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.Host.Metrics.IsEnabled() {
		t.Error("host metrics are off by default")
	}
	if got := cfg.Host.Metrics.EffectiveInterval(); got != DefaultHostMetricsInterval {
		t.Errorf("interval = %v, want %v", got, DefaultHostMetricsInterval)
	}
}

func TestHostMetricsExplicitlyDisabled(t *testing.T) {
	// The reason Enabled is a pointer. With a plain bool this config would be
	// indistinguishable from one that says nothing, and the right default for the two is
	// opposite.
	cfg, err := Parse([]byte("host:\n  metrics:\n    enabled: false\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Host.Metrics.IsEnabled() {
		t.Error("host metrics stayed on after being turned off")
	}
}

func TestHostMetricsConfigured(t *testing.T) {
	cfg, err := Parse([]byte(
		"host:\n  metrics:\n    interval: 5s\n    storage_mounts: [\"/\", \"/mnt/media\"]\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.Host.Metrics.EffectiveInterval(); got != 5*time.Second {
		t.Errorf("interval = %v, want 5s", got)
	}
	if len(cfg.Host.Metrics.StorageMounts) != 2 {
		t.Errorf("storage_mounts = %v", cfg.Host.Metrics.StorageMounts)
	}
}

func TestHostMetricsRejectsRelativeMounts(t *testing.T) {
	// A relative mount is measured against the agent's working directory, which for a
	// systemd unit is nowhere the operator meant.
	_, err := Parse([]byte("host:\n  metrics:\n    storage_mounts: [\"media\"]\n"))
	if err == nil {
		t.Fatal("a relative storage mount was accepted")
	}
	if !strings.Contains(err.Error(), "storage_mounts") {
		t.Errorf("error does not name the field: %v", err)
	}
}
