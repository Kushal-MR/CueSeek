// Command cueseekd is the CueSeek host agent.
//
// It runs as an unprivileged system user on the machine being managed, and serves that
// machine's state and control actions to CueSeek clients over a private network.
//
// It deliberately holds no privileges of its own: service restarts and host power actions
// are performed by asking systemd and logind over D-Bus, authorised by a polkit rule that
// enumerates exactly what this user may do. See ADR-0002. That layer lands in M1.4.
//
// Usage:
//
//	cueseekd [-config PATH]         run the agent
//	cueseekd check [flags]          diagnose this install before anything is tapped
//	cueseekd pair [flags]           mint a pairing code for a new device
//	cueseekd host status <unit>     inspect a managed unit
//	cueseekd host restart <unit>    restart a managed unit
//	cueseekd -version               print version and exit
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/adapters/builtin"
	"github.com/Kushal-MR/CueSeek/agent/internal/api"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
	"github.com/Kushal-MR/CueSeek/agent/internal/host/metrics"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// version is overwritten at build time:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always --dirty)"
var version = "0.0.0-dev"

// subcommands is the complete set of words that mean something other than "run the agent".
//
// A map rather than a switch so that classify and the usage text cannot disagree about
// which words exist — the usage is generated from this, and a subcommand added without a
// description will not compile.
var subcommands = map[string]string{
	"check": "diagnose this install; changes nothing",
	"pair":  "mint a pairing code for a new device",
	"host":  "inspect or restart one managed unit",
	"help":  "print this",
}

// errUnknownSubcommand is returned for a first argument that was meant to be a subcommand
// and is not one.
type errUnknownSubcommand struct{ arg string }

func (e errUnknownSubcommand) Error() string {
	return fmt.Sprintf("unknown subcommand %q", e.arg)
}

// classify decides what a command line means, without acting on it.
//
// Separated from main so it can be tested. main calls os.Exit and runServe blocks until a
// signal, so the decision was previously only observable by starting a daemon — which is
// exactly the thing that went wrong.
//
// The rule: a first argument that does not begin with `-` is a subcommand. If it is not a
// known one, that is an error rather than something to ignore.
//
// It was ignored, and the consequence was found on a real host. `flag.Parse` stops at the
// first non-flag argument and reports no error, so `cueseekd check` on a binary predating
// that subcommand fell through to the serve path, and every flag after it — including
// `-config` — was silently dropped. The agent then started against the DEFAULT config, on
// a machine already running one, and failed on `bind: address already in use`.
//
// The near miss is the part worth keeping in mind: on a host where the port happened to be
// free, a mistyped subcommand would have started a second agent against the real
// configuration and the real database rather than failing.
func classify(args []string) (subcommand string, rest []string, err error) {
	if len(args) == 0 {
		return "", nil, nil // serve
	}
	first := args[0]

	// Anything starting with `-` belongs to the serve flag set: `-config`, `-version`,
	// `-log-level`, and flag's own `-h`.
	if strings.HasPrefix(first, "-") {
		return "", args, nil
	}
	if _, known := subcommands[first]; known {
		return first, args[1:], nil
	}
	return "", nil, errUnknownSubcommand{arg: first}
}

func main() {
	subcommand, rest, err := classify(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cueseekd: %v\n\n", err)
		printUsage(os.Stderr)
		os.Exit(2)
	}

	switch subcommand {
	case "help":
		printUsage(os.Stdout)

	case "check":
		// The failure message is deliberately terse: runCheck has already printed a
		// full report, and repeating it here would bury the arrows under a summary.
		if err := runCheck(rest); err != nil {
			os.Exit(1)
		}

	case "pair":
		if err := runPair(rest); err != nil {
			fmt.Fprintf(os.Stderr, "cueseekd pair: %v\n", err)
			os.Exit(1)
		}

	case "host":
		if err := runHost(rest); err != nil {
			fmt.Fprintf(os.Stderr, "cueseekd host: %v\n", err)
			os.Exit(1)
		}

	default:
		if err := runServe(rest); err != nil {
			fmt.Fprintf(os.Stderr, "cueseekd: %v\n", err)
			os.Exit(1)
		}
	}
}

// printUsage lists what this binary can do.
//
// Written to whichever stream suits the caller: stdout for `help`, which somebody asked
// for, and stderr for a mistake, so that piping the output of a script does not swallow
// the explanation.
func printUsage(w *os.File) {
	fmt.Fprintf(w, "cueseekd %s — the CueSeek host agent\n\n", version)
	fmt.Fprintf(w, "  cueseekd [-config PATH] [-log-level LEVEL]   run the agent\n")
	fmt.Fprintf(w, "  cueseekd -version                            print the version\n\n")

	// Sorted, so the list does not reorder between runs of the same binary.
	names := make([]string, 0, len(subcommands))
	for name := range subcommands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, "  cueseekd %-8s                            %s\n", name, subcommands[name])
	}
	fmt.Fprintf(w, "\nEach subcommand takes -h for its own flags.\n")
}

// ---------------------------------------------------------------- serve

func runServe(args []string) error {
	fs := flag.NewFlagSet("cueseekd", flag.ExitOnError)
	configPath := fs.String("config", config.DefaultPath, "path to the configuration file")
	showVersion := fs.Bool("version", false, "print version and exit")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Defence in depth behind classify, and the direct cause of the failure that produced
	// this check. flag.Parse stops at the first non-flag argument and reports no error, so
	// without this a stray word both starts the daemon and silently discards every flag
	// after it — `cueseekd oops -config /tmp/x.yaml` ran against /etc/cueseek/config.yaml.
	//
	// Refusing here as well as in classify is cheap, and it means the serve path cannot be
	// entered with arguments it did not understand no matter how it was reached.
	if fs.NArg() > 0 {
		return fmt.Errorf(
			"unexpected argument %q; the agent takes flags only (try `cueseekd help`)",
			fs.Arg(0))
	}

	if *showVersion {
		fmt.Printf("cueseekd %s (%s/%s, %s)\n",
			version, runtime.GOOS, runtime.GOARCH, runtime.Version())
		return nil
	}

	setupLogging(*logLevel)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	st, err := store.Open(cfg.Storage.Path)
	if err != nil {
		return err
	}
	defer st.Close()

	// Startup housekeeping. Expiry is enforced at redemption regardless, so this is
	// tidiness rather than correctness.
	if n, err := st.PurgeExpiredPairingCodes(context.Background()); err != nil {
		slog.Warn("could not purge expired pairing codes", "error", err)
	} else if n > 0 {
		slog.Info("purged expired pairing codes", "count", n)
	}

	hostID, err := st.HostID(context.Background())
	if err != nil {
		return fmt.Errorf("resolve host id: %w", err)
	}

	// The host layer. Built even when no service is controllable, because its
	// construction is what reports a broken D-Bus environment at startup rather than the
	// first time somebody taps "restart".
	hostController, err := host.New(cfg.ManagedUnits())
	if err != nil {
		return fmt.Errorf("host layer: %w", err)
	}
	defer hostController.Close()
	slog.Info("host control ready",
		"platform", hostController.Platform(), "managed_units", hostController.ManagedUnits())

	// Adapters. Registry construction validates every service's configuration, so a
	// missing API key or an unknown type fails startup instead of surfacing as a
	// permanently degraded service half an hour later.
	registry, err := builtin.NewRegistry()
	if err != nil {
		return err
	}
	if err := registry.Build(cfg, adapters.Deps{
		HTTPClient: newUpstreamClient(),
		Units:      hostController,
	}); err != nil {
		return err
	}

	poller := adapters.NewPoller(registry, cfg)

	server, err := api.New(api.Options{
		Store:     st,
		Registry:  registry,
		Cache:     poller.Cache(),
		Refresher: poller,
		// The same controller that restarts units also powers the machine down. Passed
		// as an interface, so an agent on a platform without logind advertises no power
		// actions rather than offering buttons that fail (ADR-0002 Amendment 2).
		HostPower:    hostController,
		AgentVersion: version,
		HostID:       hostID,
	})
	if err != nil {
		return err
	}

	// SIGINT and SIGTERM both mean stop. systemd sends SIGTERM, so honouring it is what
	// makes `systemctl stop cueseekd` a graceful shutdown rather than a kill.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Started before the listener so the first client request is answered from real
	// state rather than from `unknown`.
	poller.Start(ctx)

	var hostMetrics sync.WaitGroup
	startHostMetrics(ctx, cfg, server, &hostMetrics)

	err = server.ListenAndServe(ctx, cfg.Bind)

	// Polling goroutines observe the same cancelled context; waiting for them here means
	// none is midway through a cache write when the process exits.
	poller.Wait()
	hostMetrics.Wait()

	if err != nil {
		return err
	}
	slog.Info("stopped")
	return nil
}

// startHostMetrics launches the host vitals collector, or explains why it did not.
//
// Both silences are worth breaking. A platform that cannot read /proc and a config that
// switched collection off produce exactly the same thing on screen — no metrics — and
// without a line in the journal the operator has no way to tell which happened.
func startHostMetrics(
	ctx context.Context, cfg config.Config, server *api.Server, wg *sync.WaitGroup,
) {
	if !metrics.Supported {
		slog.Info("host metrics unavailable on this platform", "platform", runtime.GOOS)
		return
	}
	if !cfg.Host.Metrics.IsEnabled() {
		slog.Info("host metrics disabled by configuration")
		return
	}

	collector := metrics.New(cfg.Host.Metrics.StorageMounts)
	interval := cfg.Host.Metrics.EffectiveInterval()

	wg.Add(1)
	go func() {
		defer wg.Done()
		collector.Run(ctx, interval, server.PublishHostMetrics)
	}()

	mounts := cfg.Host.Metrics.StorageMounts
	if len(mounts) == 0 {
		mounts = metrics.DefaultMounts
	}
	slog.Info("host metrics started", "interval", interval, "mounts", mounts)
}

// newUpstreamClient builds the HTTP client every adapter shares.
//
// Shared rather than one per adapter, so connections to a service are reused across
// polls. Opening a fresh TCP connection — and a TLS handshake, once anything is behind
// HTTPS — every thirty seconds for the life of the process is pure waste.
//
// Client.Timeout is deliberately NOT set. It would cap the whole request including the
// body read, and would silently override the per-service deadline the poller derives from
// configuration. Deadlines belong in the context, where one service's budget cannot be
// spent by another.
func newUpstreamClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	// The agent talks to a handful of services on one machine. The stdlib default of 2
	// idle connections per host is tuned for a browser talking to the wider internet and
	// would close and reopen connections between polls.
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 4
	transport.IdleConnTimeout = 90 * time.Second

	// Bounds the TCP handshake specifically, so an unreachable host fails fast instead of
	// consuming the whole per-request budget waiting for a connection that will not open.
	transport.DialContext = (&net.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ExpectContinueTimeout = 1 * time.Second

	return &http.Client{Transport: transport}
}

// ---------------------------------------------------------------- pair

func runPair(args []string) error {
	fs := flag.NewFlagSet("cueseekd pair", flag.ExitOnError)
	configPath := fs.String("config", config.DefaultPath, "path to the configuration file")
	// Default is least privilege that is still useful: look and control services, but
	// neither power off the machine nor lock other devices out. Both of those must be
	// asked for by name.
	scopeList := fs.String("scopes", "read,service.control",
		"comma-separated scopes to grant: read, service.control, devices.manage, host.power")
	ttl := fs.Duration("ttl", store.DefaultPairingTTL, "how long the code remains valid")
	if err := fs.Parse(args); err != nil {
		return err
	}

	scopes, err := parseScopeList(*scopeList)
	if err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	// Opens the same database the running agent uses. Safe: SQLite in WAL mode
	// coordinates across processes with file locking, which is exactly why pairing codes
	// live in the database rather than in the daemon's memory — this command is a
	// separate process and could not otherwise hand a code to the server.
	st, err := store.Open(cfg.Storage.Path)
	if err != nil {
		return err
	}
	defer st.Close()

	code, err := st.CreatePairingCode(context.Background(), scopes, *ttl)
	if err != nil {
		return err
	}

	fmt.Printf("\n  Pairing code:  %s\n", code)
	fmt.Printf("  Scopes:        %s\n", *scopeList)
	fmt.Printf("  Valid for:     %s\n\n", *ttl)
	fmt.Printf("  Single use. Enter it in the CueSeek app, or:\n\n")
	fmt.Printf("    curl -sX POST http://%s/v1/pair \\\n", cfg.Bind.Address)
	fmt.Printf("      -H 'Content-Type: application/json' \\\n")
	fmt.Printf("      -d '{\"code\":\"%s\",\"device_name\":\"My Phone\",\"platform\":\"cli\"}'\n\n", code)

	if hasScope(scopes, domain.ScopeHostPower) {
		fmt.Fprintf(os.Stderr,
			"  WARNING: this code grants host.power — the paired device will be able to\n"+
				"           reboot and shut down this machine.\n\n")
	}
	if hasScope(scopes, domain.ScopeDevicesManage) {
		fmt.Fprintf(os.Stderr,
			"  WARNING: this code grants devices.manage — the paired device will be able\n"+
				"           to revoke any other device, including this one.\n\n")
	}
	return nil
}

func parseScopeList(list string) ([]domain.Scope, error) {
	var scopes []domain.Scope
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		scope, err := domain.ParseScope(part)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	if len(scopes) == 0 {
		return nil, errors.New("at least one scope is required")
	}
	return scopes, nil
}

func hasScope(scopes []domain.Scope, want domain.Scope) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- host

// runHost exercises the host control layer directly, without going through the API.
//
// Two audiences. During development it verifies the systemd/polkit path on a real machine
// before the API is wired to it. In production it is the first thing to reach for when a
// restart fails: it distinguishes "the allowlist does not contain this unit" from "polkit
// refused" from "systemd could not restart it" — three problems with identical symptoms
// through the API.
//
//	cueseekd host status  <unit>
//	cueseekd host restart <unit>
func runHost(args []string) error {
	if len(args) == 0 {
		return errors.New(
			"usage: cueseekd host <status|restart> [-config PATH] [-timeout D] <unit>\n" +
				"       flags must precede the unit name")
	}
	action := args[0]

	fs := flag.NewFlagSet("cueseekd host", flag.ExitOnError)
	configPath := fs.String("config", config.DefaultPath, "path to the configuration file")
	timeout := fs.Duration("timeout", 60*time.Second, "how long to wait for a restart to finish")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf(
			"exactly one unit name is required, got %d (%v)\n"+
				"  usage: cueseekd host %s [-config PATH] [-timeout D] <unit>\n"+
				"  note:  flags must come BEFORE the unit name",
			fs.NArg(), fs.Args(), action)
	}
	unit := fs.Arg(0)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	// The allowlist comes from configuration, never from a compiled-in list (ADR-0002).
	controller, err := host.New(cfg.ManagedUnits())
	if err != nil {
		return err
	}
	defer controller.Close()

	fmt.Printf("platform: %s\n", controller.Platform())
	fmt.Printf("managed:  %s\n\n", strings.Join(controller.ManagedUnits(), ", "))

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	switch action {
	case "status":
		return hostStatus(ctx, controller, unit)
	case "restart":
		return hostRestart(ctx, controller, unit)
	default:
		return fmt.Errorf("unknown action %q (want status or restart)", action)
	}
}

func hostStatus(ctx context.Context, controller *host.Controller, unit string) error {
	state, err := controller.UnitState(ctx, unit)
	if err != nil {
		return describeHostError(unit, err)
	}
	fmt.Printf("unit:          %s\n", state.Name)
	fmt.Printf("load state:    %s\n", state.LoadState)
	fmt.Printf("active state:  %s\n", state.ActiveState)
	fmt.Printf("sub state:     %s\n", state.SubState)
	if state.ActiveEnterTime.IsZero() {
		fmt.Printf("active since:  never\n")
	} else {
		fmt.Printf("active since:  %s (%s ago)\n",
			state.ActiveEnterTime.Format(time.RFC3339),
			time.Since(state.ActiveEnterTime).Round(time.Second))
	}
	return nil
}

func hostRestart(ctx context.Context, controller *host.Controller, unit string) error {
	before, err := controller.UnitState(ctx, unit)
	if err != nil {
		return describeHostError(unit, err)
	}

	job, err := controller.RestartUnit(ctx, unit)
	if err != nil {
		return describeHostError(unit, err)
	}
	// This is the moment the API returns 202: the job exists, its outcome does not yet.
	fmt.Printf("job %d enqueued for %s — waiting for JobRemoved...\n", job.ID, unit)

	result, err := job.Wait(ctx)
	if err != nil {
		return fmt.Errorf("waiting for job %d: %w", job.ID, err)
	}
	fmt.Printf("job result:    %s\n", result)

	after, err := controller.UnitState(ctx, unit)
	if err != nil {
		return describeHostError(unit, err)
	}

	// A completed job is not proof the unit restarted. M0 established that RestartUnit
	// returns once the job is queued, and confirmed the restart only by watching this
	// timestamp advance.
	fmt.Printf("active since:  %s -> %s\n",
		before.ActiveEnterTime.Format(time.RFC3339),
		after.ActiveEnterTime.Format(time.RFC3339))
	if after.ActiveEnterTime.After(before.ActiveEnterTime) {
		fmt.Printf("verified:      the unit genuinely restarted\n")
	} else {
		fmt.Printf("WARNING:       the job reported %s but the unit's start time did not move\n", result)
	}

	if !result.Succeeded() {
		return fmt.Errorf("systemd reported %q", result)
	}
	return nil
}

// describeHostError turns the package's error vocabulary into advice, because these three
// failures look identical from the outside and have completely different fixes.
func describeHostError(unit string, err error) error {
	switch {
	case errors.Is(err, host.ErrUnitNotManaged):
		return fmt.Errorf("%w\n  -> add it to `services:` in the config file", err)
	case errors.Is(err, host.ErrUnauthorized):
		return fmt.Errorf("%w\n  -> polkit refused. Check that the rule in deploy/ is installed "+
			"and names this user and unit", err)
	case errors.Is(err, host.ErrUnitNotFound):
		return fmt.Errorf("%w\n  -> systemd has no unit %q. Check the exact name with "+
			"`systemctl list-units --type=service`", err, unit)
	case errors.Is(err, host.ErrUnsupportedPlatform):
		return fmt.Errorf("%w\n  -> host control requires a systemd Linux host", err)
	default:
		return err
	}
}

// ---------------------------------------------------------------- logging

// setupLogging configures structured logging to stderr.
//
// Text rather than JSON, because the agent's logs are read through `journalctl` by a
// person diagnosing their own server, not shipped to a log aggregator. Timestamps are
// omitted: journald records its own, and duplicating them wastes half the line width.
func setupLogging(level string) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: lvl,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
}
