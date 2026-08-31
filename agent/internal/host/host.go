package host

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// Errors returned by this package. Callers should compare with errors.Is rather than
// matching strings — the wrapped detail varies by platform and systemd version.
var (
	// ErrUnitNotManaged means the unit is not in the configured allowlist. Returned
	// before any call to the host, so an unlisted unit is never named to systemd.
	ErrUnitNotManaged = errors.New("unit is not managed by this agent")

	// ErrUnauthorized means polkit refused the operation.
	//
	// M0 established that this arrives as "Interactive authentication required" rather
	// than "Access denied": polkit is asking for a password that a daemon can never
	// supply. Treating it as an unexpected internal failure — which the M0 spike's own
	// probe script did — turns a clean authorisation result into a confusing 500.
	ErrUnauthorized = errors.New("not authorized to perform this operation on the host")

	// ErrUnitNotFound means systemd has no such unit loaded.
	ErrUnitNotFound = errors.New("unit not found")

	// ErrUnsupportedPlatform is returned by every operation on non-Linux builds.
	ErrUnsupportedPlatform = errors.New("host control requires a systemd-based Linux host")

	// ErrUnknownPowerAction means the id is not one this agent offers. Distinct from an
	// authorisation failure: the caller may be perfectly entitled to power the machine
	// off and simply have asked for something that does not exist.
	ErrUnknownPowerAction = errors.New("unknown host power action")
)

// UnitState is a point-in-time view of a systemd unit.
//
// These are the properties M0 confirmed are readable over D-Bus without elevation, from
// the same connection used to control the unit. That is the concrete reason ADR-0002
// chose D-Bus over sudo plus `systemctl show`: observation and control share one
// mechanism instead of needing two.
type UnitState struct {
	Name        string
	LoadState   string // loaded, not-found, masked, error
	ActiveState string // active, reloading, inactive, failed, activating, deactivating
	SubState    string // running, dead, exited, ... (unit-type specific)

	// ActiveEnterTime is when the unit last entered the active state, or the zero time
	// if it never has.
	//
	// systemd reports this as microseconds since the epoch (M0 finding). Seconds and
	// nanoseconds are the far more common conventions, so the unit is converted here
	// once rather than at each call site, where getting it wrong is silent.
	ActiveEnterTime time.Time
}

// Loaded reports whether systemd knows about the unit at all.
func (u UnitState) Loaded() bool { return u.LoadState == "loaded" }

// Active reports whether the unit is currently running.
func (u UnitState) Active() bool { return u.ActiveState == "active" }

// Failed reports whether the unit is in the failed state.
func (u UnitState) Failed() bool { return u.ActiveState == "failed" }

// JobResult is systemd's verdict on a job, delivered via the JobRemoved signal.
type JobResult string

const (
	JobDone       JobResult = "done"
	JobCanceled   JobResult = "canceled"
	JobTimeout    JobResult = "timeout"
	JobFailed     JobResult = "failed"
	JobDependency JobResult = "dependency"
	JobSkipped    JobResult = "skipped"
)

// Succeeded reports whether the job completed successfully.
func (r JobResult) Succeeded() bool { return r == JobDone }

// Job is a systemd job that has been enqueued but may not have finished.
//
// This type exists because of a fact M0 established empirically: RestartUnit returns as
// soon as the job is *queued*, not when the restart completes. During the spike a
// successful-looking return was only confirmed to have actually restarted Jellyfin by
// observing ActiveEnterTimestamp move forward afterwards.
//
// That is also why the API returns 202 with an action id rather than a synchronous
// result (ADR-0004): the agent has no synchronous result to give, because systemd does
// not have one either.
type Job struct {
	ID   int
	Unit string

	// results carries exactly one value, from systemd's JobRemoved signal.
	results <-chan string
}

func newJob(id int, unit string, results <-chan string) *Job {
	return &Job{ID: id, Unit: unit, results: results}
}

// Wait blocks until systemd reports the job's outcome, or ctx is cancelled.
//
// Not calling Wait is safe: the underlying channel is buffered, so the result is simply
// discarded when the Job is collected. This matters more than it looks — see the note in
// the Linux backend about why an unread channel would otherwise stall every subsequent
// job on the connection.
func (j *Job) Wait(ctx context.Context) (JobResult, error) {
	if j == nil || j.results == nil {
		return "", errors.New("host: job has no result channel")
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result, ok := <-j.results:
		if !ok {
			return "", errors.New("host: job result channel closed without a result")
		}
		return JobResult(result), nil
	}
}

// Backend is the platform-specific half of host control.
//
// It performs raw operations and applies no policy: the allowlist, name validation and
// error classification all live in Controller, above it. That split is deliberate —
// putting the allowlist inside the Linux backend would make the single most
// security-relevant rule in this package untestable anywhere except a systemd host.
type Backend interface {
	// UnitState reads a unit's current properties.
	UnitState(ctx context.Context, unit string) (UnitState, error)

	// RestartUnit enqueues a restart and returns a Job whose result arrives later.
	RestartUnit(ctx context.Context, unit string) (*Job, error)

	// StartUnit and StopUnit enqueue the corresponding job.
	//
	// Stop is the one verb here that is not self-healing: a restart leaves the service
	// running whatever happens, a stop does not. See ADR-0002 Amendment 1 for the
	// widened ceiling and what bounds it.
	StartUnit(ctx context.Context, unit string) (*Job, error)
	StopUnit(ctx context.Context, unit string) (*Job, error)

	// Reboot and PowerOff act on the machine itself, through logind.
	//
	// Neither returns a job, and neither has a meaningful success value: the only
	// answer they can deliver is a failure, because a call that worked takes the process
	// with it before it can return. Callers must treat a nil error as "accepted", not as
	// "done" (ADR-0002 Amendment 2).
	//
	// PowerOff is the first grant in this package that is not undoable from a phone. A
	// reboot comes back on its own; recovering from a power-off needs somebody in the
	// same room as the machine.
	Reboot(ctx context.Context) error
	PowerOff(ctx context.Context) error

	// SupportsPower reports whether this platform can do the two above at all.
	//
	// Asked rather than inferred from an error, so an agent on an unsupported platform
	// advertises no power actions instead of offering buttons that fail when pressed.
	SupportsPower() bool

	// Platform names the implementation, for logs and diagnostics.
	Platform() string

	// Close releases any connection held.
	Close() error
}

// Controller is the agent's entry point for host control.
//
// It owns policy; Backend owns mechanism.
type Controller struct {
	backend Backend
	managed []string // preserves configuration order for display
	allowed map[string]struct{}
}

// New builds a Controller for this platform, managing exactly the given units.
//
// The unit list comes from configuration (ADR-0002, and M1.2's config package), never
// from a compiled-in constant. M0 demonstrated why concretely: the qBittorrent unit on
// the target host is named `qbittorrent.service` even though the unit describes itself
// as "qBittorrent-nox", so a hardcoded guess would have been wrong on the first machine.
func New(managedUnits []string) (*Controller, error) {
	backend, err := newBackend()
	if err != nil {
		return nil, err
	}
	controller, err := NewWithBackend(backend, managedUnits)
	if err != nil {
		backend.Close()
		return nil, err
	}
	return controller, nil
}

// NewWithBackend builds a Controller over an explicit Backend.
//
// Exists so tests can drive the policy layer with a fake backend on any operating
// system, and so a future non-systemd backend is a substitution rather than a rewrite.
func NewWithBackend(backend Backend, managedUnits []string) (*Controller, error) {
	if backend == nil {
		return nil, errors.New("host: backend must not be nil")
	}

	allowed := make(map[string]struct{}, len(managedUnits))
	managed := make([]string, 0, len(managedUnits))

	for _, unit := range managedUnits {
		unit = strings.TrimSpace(unit)
		if unit == "" {
			return nil, errors.New("host: managed unit name must not be empty")
		}
		// systemd units always carry a type suffix. Rejecting "jellyfin" here turns an
		// opaque D-Bus error at action time into a startup message naming the problem.
		if !strings.Contains(unit, ".") {
			return nil, fmt.Errorf(
				"host: managed unit %q has no type suffix (did you mean %q?)", unit, unit+".service")
		}
		if _, dup := allowed[unit]; dup {
			continue
		}
		allowed[unit] = struct{}{}
		managed = append(managed, unit)
	}

	return &Controller{backend: backend, managed: managed, allowed: allowed}, nil
}

// ManagedUnits returns the allowlist in configuration order.
func (c *Controller) ManagedUnits() []string { return slices.Clone(c.managed) }

// IsManaged reports whether the unit is in the allowlist.
func (c *Controller) IsManaged(unit string) bool {
	_, ok := c.allowed[unit]
	return ok
}

// Platform names the active backend.
func (c *Controller) Platform() string { return c.backend.Platform() }

// UnitState reads a managed unit's current state.
func (c *Controller) UnitState(ctx context.Context, unit string) (UnitState, error) {
	if err := c.checkManaged(unit); err != nil {
		return UnitState{}, err
	}
	return c.backend.UnitState(ctx, unit)
}

// RestartUnit enqueues a restart of a managed unit.
//
// The allowlist is checked first and the backend is not called at all for an unlisted
// unit. ADR-0002 requires the allowlist to be enforced in two places — here and in the
// shipped polkit rule — so that a misconfiguration is refused by the agent before it
// ever reaches the system bus, and a compromised agent is still bounded by polkit.
func (c *Controller) RestartUnit(ctx context.Context, unit string) (*Job, error) {
	if err := c.checkManaged(unit); err != nil {
		return nil, err
	}
	return c.backend.RestartUnit(ctx, unit)
}

// StartUnit enqueues a start of a managed unit.
//
// Same two-place enforcement as RestartUnit: the allowlist is checked here before the
// system bus is touched, and the polkit rule checks it again behind that.
func (c *Controller) StartUnit(ctx context.Context, unit string) (*Job, error) {
	if err := c.checkManaged(unit); err != nil {
		return nil, err
	}
	return c.backend.StartUnit(ctx, unit)
}

// StopUnit enqueues a stop of a managed unit.
//
// The unit remains enabled, so it returns on the next boot. `enable`, `disable` and
// `mask` are deliberately absent from the agent and from the polkit rule, which is what
// bounds a stop to a single boot rather than making it persistent (ADR-0002 Amendment 1).
func (c *Controller) StopUnit(ctx context.Context, unit string) (*Job, error) {
	if err := c.checkManaged(unit); err != nil {
		return nil, err
	}
	return c.backend.StopUnit(ctx, unit)
}

// ---------------------------------------------------------------- power

// Power action ids. Public API: a client keys its confirmation copy on them, so renaming
// one silently changes what an older build shows before it powers a machine off.
const (
	ActionReboot   = "reboot"
	ActionPowerOff = "power-off"
)

// PowerActions is what this agent offers for the machine itself.
//
// Empty rather than absent on a platform that cannot perform them, so a client renders
// nothing instead of buttons that fail when held. Both are `destructive`, which routes
// them to the client's press-and-hold confirmation.
//
// Both carry the *consequence* in the description rather than a restatement of the verb.
// "Reboot the host" tells a reader nothing they did not get from the label; what they
// need at the moment of deciding is what it costs and whether they can undo it.
func (c *Controller) PowerActions() []domain.Action {
	if !c.backend.SupportsPower() {
		return nil
	}
	return []domain.Action{
		{
			ID:          ActionReboot,
			Label:       "Restart machine",
			Description: "Everything on this machine stops and comes back in a minute or two.",
			Risk:        domain.RiskDestructive,
		},
		{
			ID:    ActionPowerOff,
			Label: "Shut down machine",
			// The only action in CueSeek that cannot be undone from the phone, and the
			// description says so plainly rather than leaving the user to work it out.
			Description: "The machine turns off and stays off. " +
				"Turning it back on needs someone physically there.",
			Risk: domain.RiskDestructive,
		},
	}
}

// SupportsPower reports whether power actions exist on this platform at all.
func (c *Controller) SupportsPower() bool { return c.backend.SupportsPower() }

// InvokePower performs a power action by id.
//
// A nil error means logind accepted the request, **not** that the machine has rebooted.
// There is no way to observe the latter from inside the process about to be ended, which
// is why callers acknowledge before they call this rather than after.
func (c *Controller) InvokePower(ctx context.Context, action string) error {
	switch action {
	case ActionReboot:
		return c.backend.Reboot(ctx)
	case ActionPowerOff:
		return c.backend.PowerOff(ctx)
	default:
		return fmt.Errorf("%w: %q", ErrUnknownPowerAction, action)
	}
}

// Close releases the backend's resources.
func (c *Controller) Close() error { return c.backend.Close() }

func (c *Controller) checkManaged(unit string) error {
	if c.IsManaged(unit) {
		return nil
	}
	// The rejected name is echoed back, but the allowlist is not: this error reaches an
	// API client, and enumerating which units exist on the host is not something an
	// unauthorised caller should be able to do by guessing.
	return fmt.Errorf("%w: %q", ErrUnitNotManaged, unit)
}

// classifyError maps a raw backend failure onto this package's error vocabulary.
//
// Kept platform-neutral, and taking the D-Bus error name as a plain string rather than a
// dbus.Error, so it can be unit-tested on any operating system. The classification rules
// are the security-relevant part; extracting the name from a D-Bus error is not.
//
// Both the error name and the message are considered. The name is the reliable signal,
// but M0 observed the human-readable form in practice, and older or proxied paths
// sometimes surface only the message.
func classifyError(dbusErrorName string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()

	switch {
	case dbusErrorName == "org.freedesktop.DBus.Error.InteractiveAuthorizationRequired",
		dbusErrorName == "org.freedesktop.DBus.Error.AccessDenied",
		strings.Contains(message, "Interactive authentication required"),
		strings.Contains(message, "InteractiveAuthorizationRequired"),
		strings.Contains(message, "Access denied"),
		strings.Contains(message, "AccessDenied"):
		return fmt.Errorf("%w: %v", ErrUnauthorized, err)

	case dbusErrorName == "org.freedesktop.systemd1.NoSuchUnit",
		strings.Contains(message, "not loaded"),
		strings.Contains(message, "NoSuchUnit"):
		return fmt.Errorf("%w: %v", ErrUnitNotFound, err)
	}

	return err
}
