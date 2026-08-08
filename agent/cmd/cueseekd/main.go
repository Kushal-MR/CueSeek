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
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/api"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// version is overwritten at build time:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always --dirty)"
var version = "0.0.0-dev"

func main() {
	// Subcommand dispatch before flag parsing: subcommands have their own flags.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "pair":
			if err := runPair(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "cueseekd pair: %v\n", err)
				os.Exit(1)
			}
			return
		case "host":
			if err := runHost(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "cueseekd host: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
	if err := runServe(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "cueseekd: %v\n", err)
		os.Exit(1)
	}
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

	server, err := api.New(api.Options{
		Store:        st,
		Config:       cfg,
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

	if err := server.ListenAndServe(ctx, cfg.Bind); err != nil {
		return err
	}
	slog.Info("stopped")
	return nil
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
