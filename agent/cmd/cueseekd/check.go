package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/adapters/builtin"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/diag"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/host"
)

// `cueseekd check` — answer, before anything is tapped, whether this install will work.
//
// The problem it exists for: ADR-0002 requires the same unit names in the configuration
// and in the polkit rule, deliberately not generated from each other, so that two
// independent things must agree before a unit can be touched. Nothing has ever checked
// that they do. When they disagree the agent starts perfectly, the dashboard looks
// perfectly normal, and the first restart fails with an authorisation error that reads
// like a broken installation. On somebody else's machine, that is where the evening goes.
//
// `cueseekd host restart <unit>` already tells those three failures apart, and it does it
// after the fact, one unit at a time, for somebody who already suspects something. This is
// that diagnosis applied to the whole configuration before anything is wrong.
//
// It verifies; it never repairs. Writing the polkit rule from the configuration would
// collapse the two independent checks into one and remove exactly the defence in depth
// ADR-0002 asks for. Reporting the disagreement costs nothing and keeps both copies.

// defaultPolkitRule is where install.sh puts the rule.
const defaultPolkitRule = "/etc/polkit-1/rules.d/10-cueseek.rules"

func runCheck(args []string) error {
	fs := flag.NewFlagSet("cueseekd check", flag.ExitOnError)
	configPath := fs.String("config", config.DefaultPath, "path to the configuration file")
	polkitPath := fs.String("polkit-rule", defaultPolkitRule, "path to the polkit rule")
	timeout := fs.Duration("timeout", 10*time.Second,
		"budget for probing each service")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var report diag.Report

	// A config that will not load is the end of the check rather than a finding in it:
	// every question after this one is about the contents of that file.
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("\nCueSeek check\n\n")
		printFindingAt(diag.Finding{
			Severity: diag.SeverityFail,
			Subject:  "configuration",
			Detail:   err.Error(),
			Fix:      "fix the file, then run this again",
		}, 14)
		fmt.Printf("\n0 ok, 0 warnings, 1 failure\n\n")
		return errors.New("configuration could not be loaded")
	}
	report.OK("configuration", fmt.Sprintf("%s parsed; %s",
		*configPath, plural(len(cfg.Services), "service", "services")))

	checkBind(&report, cfg)
	checkStorage(&report, cfg)
	checkPolkit(&report, cfg, *polkitPath)

	controller := checkUnits(&report, cfg)
	if controller != nil {
		defer controller.Close()
	}
	checkServices(&report, cfg, controller, *timeout)

	printReport(report)
	if report.HasFailures() {
		return fmt.Errorf("%s", plural(report.Count(diag.SeverityFail), "problem", "problems"))
	}
	return nil
}

// ---------------------------------------------------------------- bind

func checkBind(report *diag.Report, cfg config.Config) {
	const subject = "bind address"

	addr, _, err := net.SplitHostPort(cfg.Bind.Address)
	if err != nil {
		// Validation already rejects this, so reaching it means something changed under us.
		report.Fail(subject, fmt.Sprintf("%q is not host:port", cfg.Bind.Address),
			"set bind.address to something like 100.64.0.5:7777")
		return
	}

	if addr == "" || addr == "0.0.0.0" || addr == "::" {
		report.Warn(subject,
			fmt.Sprintf("%s listens on every interface", cfg.Bind.Address),
			"CueSeek terminates no TLS and can power off this machine (ADR-0001). "+
				"Bind to one address unless you have a specific reason not to")
		return
	}

	ip := net.ParseIP(addr)
	if ip != nil && ip.IsLoopback() {
		report.Warn(subject,
			fmt.Sprintf("%s is loopback, so only this machine can reach the agent",
				cfg.Bind.Address),
			"to pair a phone, set bind.address to your VPN address: `tailscale ip -4`")
		return
	}

	iface, found, err := interfaceFor(addr)
	switch {
	case err != nil:
		report.Warn(subject, fmt.Sprintf("could not enumerate interfaces: %v", err),
			"check by hand with `ip addr`")
	case found:
		report.OK(subject, fmt.Sprintf("%s is assigned to %s", cfg.Bind.Address, iface))
	default:
		// Not a failure. The agent retries bind() for up to 90 seconds precisely because
		// a tailnet address is assigned seconds after the interface appears, so at boot
		// this is the normal state rather than a broken one.
		report.Warn(subject,
			fmt.Sprintf("%s is not assigned to any interface right now", addr),
			"normal at boot — the agent waits up to 90s for it. If it never appears, "+
				"check `tailscale ip -4` and `ip addr`")
	}
}

// interfaceFor reports which interface carries an address, if any.
func interfaceFor(addr string) (string, bool, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", false, err
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue // one unreadable interface must not hide the others
		}
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil {
				continue
			}
			if ip.String() == addr {
				return iface.Name, true, nil
			}
		}
	}
	return "", false, nil
}

// ---------------------------------------------------------------- storage

func checkStorage(report *diag.Report, cfg config.Config) {
	const subject = "state directory"
	dir := parentDir(cfg.Storage.Path)

	info, err := os.Stat(dir)
	if err != nil {
		report.Fail(subject, fmt.Sprintf("%s: %v", dir, err),
			"systemd's StateDirectory= creates it when the unit starts; "+
				"install.sh also creates it. Check the unit is installed")
		return
	}
	if !info.IsDir() {
		report.Fail(subject, fmt.Sprintf("%s is not a directory", dir),
			"remove whatever is there and let the unit recreate it")
		return
	}

	// Permissions are not inferred from the mode bits: the answer depends on uid, gid and
	// supplementary groups, and getting that arithmetic subtly wrong is how a check
	// reports a problem the agent does not have. Writing a file is the only honest test.
	probe, err := os.CreateTemp(dir, ".cueseek-check-*")
	if err != nil {
		report.Fail(subject, fmt.Sprintf("%s is not writable: %v", dir, err),
			"the agent runs as the cueseek user; run this the same way: "+
				"`sudo -u cueseek cueseekd check`")
		return
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)

	detail := fmt.Sprintf("%s is writable", dir)
	if os.Geteuid() == 0 {
		// Root can write anywhere, so a pass here says nothing about the user that
		// actually runs the daemon. Better to say so than to be quietly reassuring.
		detail += " by root — this does not prove the cueseek user can"
	}
	report.OK(subject, detail)
}

// parentDir is filepath.Dir for a path written the way a config file writes it.
//
// The same reasoning as config.isAbsPath: this configuration describes a Linux deployment
// and is routinely read on a Windows development machine. filepath.Dir there turns
// "/var/lib/cueseek/cueseek.db" into "\var\lib\cueseek", which then cannot be found — so
// the check reports a missing state directory for a configuration that is perfectly fine.
// Judging a Linux path by Windows rules makes every valid production config look broken.
func parentDir(p string) string {
	if strings.HasPrefix(p, "/") {
		return path.Dir(p)
	}
	return filepath.Dir(p)
}

// ---------------------------------------------------------------- polkit

func checkPolkit(report *diag.Report, cfg config.Config, path string) {
	const subject = "polkit rule"
	configured := cfg.ManagedUnits()

	raw, err := os.ReadFile(path)
	if err != nil {
		if len(configured) == 0 {
			report.Warn(subject, fmt.Sprintf("%s is not readable: %v", path, err),
				"nothing needs it yet — no configured service names a unit — "+
					"but service control will need it")
			return
		}
		report.Fail(subject, fmt.Sprintf("%s is not readable: %v", path, err),
			"without it polkit refuses every start, stop and restart. "+
				"Reinstall it from deploy/10-cueseek.rules")
		return
	}

	granted, err := diag.PolkitUnits(raw)
	if err != nil {
		// Deliberately not a failure. The rule may be perfectly correct and simply
		// written in a way this cannot read, and claiming it is broken would send an
		// operator to fix a file that is fine.
		report.Warn(subject, fmt.Sprintf("could not read the allowlist from %s: %v", path, err),
			"check by hand that allowedUnits names "+diag.Quote(configured))
	} else {
		reportUnitAllowlist(report, path, configured, granted)
	}

	checkPolkitPower(report, raw, path)
}

func reportUnitAllowlist(report *diag.Report, path string, configured, granted []string) {
	const subject = "polkit allowlist"
	cmp := diag.CompareUnits(configured, granted)

	if len(cmp.MissingFromRule) > 0 {
		report.Fail(subject,
			fmt.Sprintf("%s is configured but not granted by the rule",
				diag.Quote(cmp.MissingFromRule)),
			fmt.Sprintf("add %s to allowedUnits in %s, then `sudo systemctl restart polkit`. "+
				"Until then polkit refuses every start, stop and restart of it",
				diag.Quote(cmp.MissingFromRule), path))
	}

	if len(cmp.MissingFromConfig) > 0 {
		// Nothing is broken. But this file's whole job is to state the ceiling, and a
		// ceiling higher than the room needs is worth mentioning once.
		report.Warn(subject,
			fmt.Sprintf("%s is granted by the rule but not configured",
				diag.Quote(cmp.MissingFromConfig)),
			fmt.Sprintf("harmless, but the rule grants more than this agent will ever "+
				"ask for. Remove it from allowedUnits in %s if it is left over", path))
	}

	if cmp.Agrees() {
		if len(configured) == 0 {
			report.OK(subject, "no services name a unit, and the rule grants none")
			return
		}
		report.OK(subject, fmt.Sprintf("agrees with the configuration on %s",
			plural(len(configured), "unit", "units")))
	}
}

func checkPolkitPower(report *diag.Report, raw []byte, path string) {
	const subject = "power actions"

	granted, missing, err := diag.PolkitPowerActions(raw)
	if err != nil {
		report.Warn(subject, fmt.Sprintf("could not read powerActions from %s: %v", path, err),
			"reboot and shut down from the phone need all four logind actions granted")
		return
	}

	switch {
	case len(granted) == 0:
		report.Warn(subject, "the rule grants no logind actions",
			"reboot and shut down from the phone will be refused. That is a valid, "+
				"more locked-down choice; enable the powerActions block to allow them")

	case len(missing) > 0:
		// The trap ADR-0002 Amendment 2 was written about: logind consults the
		// -multiple-sessions form when another user is logged in, so a partial grant works
		// perfectly for whoever tests it alone and fails the first time somebody is at the
		// console — late, and looking like a permissions bug.
		report.Fail(subject,
			fmt.Sprintf("only some logind actions are granted; %s missing",
				diag.Quote(missing)),
			fmt.Sprintf("grant all four or none in %s. logind consults the "+
				"-multiple-sessions variants whenever another user is logged in, so a "+
				"partial grant works alone and fails with somebody at the console", path))

	default:
		report.OK(subject, "all four logind actions granted")
	}
}

// ---------------------------------------------------------------- units

// checkUnits resolves every configured unit against systemd.
//
// Returns the controller so the service probe can reuse it — building a second D-Bus
// connection to ask the same daemon the same questions would be wasteful and would make
// the two halves of this command able to disagree.
func checkUnits(report *diag.Report, cfg config.Config) *host.Controller {
	units := cfg.ManagedUnits()

	controller, err := host.New(units)
	if err != nil {
		report.Fail("host layer", fmt.Sprintf("could not start: %v", err),
			"the agent needs D-Bus. Check `systemctl status dbus`")
		return nil
	}

	if len(units) == 0 {
		report.OK("managed units", "none configured; no service offers lifecycle actions")
		return controller
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, unit := range units {
		subject := "unit " + unit
		state, err := controller.UnitState(ctx, unit)

		// One finding, not one per unit. The platform does not become more unsupported
		// with each service, and a report that repeats itself teaches the reader to skim.
		if errors.Is(err, host.ErrUnsupportedPlatform) {
			report.Warn("managed units",
				fmt.Sprintf("%s cannot be checked on %s",
					plural(len(units), "unit", "units"), controller.Platform()),
				"run this on the Linux host that runs the agent")
			return controller
		}

		switch {
		case errors.Is(err, host.ErrUnitNotFound):
			// The most likely misconfiguration on a new install: M0 established that a
			// unit's real name routinely differs from what the software calls itself.
			report.Fail(subject, "systemd has no unit by that name",
				fmt.Sprintf("find the real name with `systemctl list-units --type=service "+
					"| grep -i %s`, then correct both the config and the polkit rule",
					firstWord(unit)))

		case err != nil:
			report.Warn(subject, fmt.Sprintf("state could not be read: %v", err),
				"property reads are not polkit-gated, so this is usually D-Bus itself")

		case !state.Loaded():
			report.Fail(subject, fmt.Sprintf("systemd reports it as %s", state.LoadState),
				fmt.Sprintf("a masked unit cannot be started at all: `sudo systemctl "+
					"unmask %s`", unit))

		case state.Failed():
			report.Warn(subject, "loaded, but in the failed state",
				fmt.Sprintf("`journalctl -u %s -n 50` says why", unit))

		default:
			// A stopped unit is reported as OK on purpose. Stopping a service is a
			// supported action, including from this app a moment ago, and colouring
			// somebody's own decision as a problem is how a diagnostic teaches people to
			// ignore it.
			report.OK(subject, describeUnit(state.ActiveState, state.SubState))
		}
	}
	return controller
}

func describeUnit(active, sub string) string {
	if sub == "" {
		return active
	}
	return fmt.Sprintf("%s (%s)", active, sub)
}

// firstWord trims a unit name down to something worth grepping for.
func firstWord(unit string) string {
	if i := strings.IndexAny(unit, ".@"); i > 0 {
		return unit[:i]
	}
	return unit
}

// ---------------------------------------------------------------- services

// checkServices builds the adapters and asks each one for its health.
//
// Deliberately the real registry and the real Health call rather than a bare HTTP GET.
// Whether a service is reachable is an adapter's question — Jellyfin needs its API key,
// qBittorrent may need a session — and re-implementing that here would produce a check
// that can disagree with the agent about the very thing it is checking.
func checkServices(
	report *diag.Report, cfg config.Config, units adapters.UnitControl, timeout time.Duration,
) {
	if len(cfg.Services) == 0 {
		report.OK("services", "none configured; the agent reports host vitals only")
		return
	}

	registry, err := builtin.NewRegistry()
	if err != nil {
		report.Fail("adapters", err.Error(), "this is a bug; please report it")
		return
	}
	if err := registry.Build(cfg, adapters.Deps{
		HTTPClient: newUpstreamClient(),
		Units:      units,
	}); err != nil {
		// The agent refuses to start on this too, all-or-nothing, rather than running
		// with a silently missing service. Saying so here means the operator learns it
		// from a command they ran on purpose instead of from a failed boot.
		report.Fail("adapters", err.Error(),
			"the agent will refuse to start until this is fixed")
		return
	}

	for _, svc := range registry.Services() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		health, err := svc.Health(ctx)
		cancel()

		subject := svc.ID()
		if err != nil {
			report.Warn(subject, fmt.Sprintf("could not be observed: %v", err),
				"the adapter could not form an opinion; check the journal")
			continue
		}

		detail := string(health.Status)
		if health.ReportedStatus != "" {
			detail += fmt.Sprintf(" — it reports %q", health.ReportedStatus)
		}

		if health.Status == domain.StatusHealthy {
			report.OK(subject, detail)
			continue
		}

		// The adapters already write these well, and they are the same words the phone
		// shows. Restating them here in different language would give the operator two
		// descriptions of one problem.
		fix := "see the reason above"
		if len(health.Reasons) > 0 {
			detail += " — " + health.Reasons[0].Message
			fix = "check the service itself; the agent is reporting what it observed"
		}
		report.Warn(subject, detail, fix)
	}
}

// ---------------------------------------------------------------- output

func printReport(report diag.Report) {
	fmt.Printf("\nCueSeek check\n\n")
	width := subjectWidth(report.Findings)
	for _, f := range report.Findings {
		printFindingAt(f, width)
	}

	fmt.Printf("\n%d ok, %s, %s\n",
		report.Count(diag.SeverityOK),
		plural(report.Count(diag.SeverityWarn), "warning", "warnings"),
		plural(report.Count(diag.SeverityFail), "failure", "failures"))

	if report.HasFailures() {
		fmt.Printf("\nSomething the configuration asks for cannot happen. See the arrows above.\n\n")
		return
	}
	if report.Count(diag.SeverityWarn) > 0 {
		fmt.Printf("\nNothing is broken. The warnings are things worth knowing.\n\n")
		return
	}
	fmt.Printf("\n")
}

// subjectWidth sizes the subject column to the findings actually present.
//
// A fixed width was wrong the first time it met a real unit name: `unit
// plexmediaserver.service` overran it, the detail ran on unaligned, and the arrow beneath
// pointed at nothing. Capped so that one long subject cannot push every other line off the
// right of an 80-column terminal — anything past the cap simply overruns, alone.
func subjectWidth(findings []diag.Finding) int {
	const (
		min = 14
		max = 30
	)
	width := min
	for _, f := range findings {
		if n := len(f.Subject); n > width {
			width = n
		}
	}
	if width > max {
		return max
	}
	return width
}

// printFindingAt renders one finding over one or two lines.
//
// No colour. This output is read over SSH, pasted into issues and quoted in journals, and
// escape codes survive none of those well — the same reasoning that keeps the agent's own
// logs plain text rather than JSON.
func printFindingAt(f diag.Finding, width int) {
	label := map[diag.Severity]string{
		diag.SeverityOK:   "ok  ",
		diag.SeverityWarn: "WARN",
		diag.SeverityFail: "FAIL",
	}[f.Severity]

	fmt.Printf("  %s  %-*s  %s\n", label, width, f.Subject, f.Detail)
	if f.Fix != "" {
		// Indented to the detail column so the arrow sits under what it is about:
		// 2 spaces + 4 label + 2 + width + 2.
		fmt.Printf("%s-> %s\n", strings.Repeat(" ", 10+width), f.Fix)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
