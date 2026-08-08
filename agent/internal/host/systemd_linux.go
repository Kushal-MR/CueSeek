//go:build linux

package host

import (
	"context"
	"errors"
	"fmt"
	"time"

	systemd "github.com/coreos/go-systemd/v22/dbus"
	godbus "github.com/godbus/dbus/v5"
)

// systemdBackend controls units through systemd's D-Bus API on the system bus.
//
// It holds no privileges of its own. Every operation is a request to systemd, authorised
// by the polkit rule shipped in deploy/ which names the cueseek user and the exact
// actions it may perform. M0 verified on the target host that this grant survives in a
// session-less daemon context — the case where a rule that works interactively can still
// fail, because polkit evaluates an inactive subject differently from an active one.
//
// Nothing here shells out. `systemctl` is never executed, so a unit name can never become
// part of a command string (ADR-0002).
type systemdBackend struct {
	conn *systemd.Conn
}

// newBackend connects to the system bus.
//
// Called at startup so a broken D-Bus environment is reported when the agent starts
// rather than the first time somebody taps "restart".
func newBackend() (Backend, error) {
	conn, err := systemd.NewSystemConnectionContext(context.Background())
	if err != nil {
		return nil, fmt.Errorf("host: connect to systemd over D-Bus: %w", err)
	}
	return &systemdBackend{conn: conn}, nil
}

func (b *systemdBackend) Platform() string { return "systemd" }

func (b *systemdBackend) Close() error {
	if b.conn != nil {
		b.conn.Close()
	}
	return nil
}

// unitProperties are the fields UnitState needs. Requested by name rather than fetching
// every property, which for a running unit is dozens of values we would discard.
const (
	propLoadState       = "LoadState"
	propActiveState     = "ActiveState"
	propSubState        = "SubState"
	propActiveEnterTime = "ActiveEnterTimestamp"
)

// UnitState reads a unit's current properties.
//
// Property reads are not polkit-gated — M0 confirmed they succeed for an unprivileged
// user with no rule installed at all. That is what lets health monitoring share one
// connection with control instead of needing a second mechanism.
func (b *systemdBackend) UnitState(ctx context.Context, unit string) (UnitState, error) {
	props, err := b.conn.GetUnitPropertiesContext(ctx, unit)
	if err != nil {
		return UnitState{}, classifyError(dbusErrorName(err), err)
	}

	state := UnitState{
		Name:        unit,
		LoadState:   stringProp(props, propLoadState),
		ActiveState: stringProp(props, propActiveState),
		SubState:    stringProp(props, propSubState),
	}

	// systemd reports timestamps in MICROseconds since the epoch. A unit that has never
	// been active reports 0, which must stay the zero time rather than becoming 1970 —
	// otherwise "last active 56 years ago" appears in the UI for a service that has
	// simply never run.
	if micros := uint64Prop(props, propActiveEnterTime); micros > 0 {
		state.ActiveEnterTime = time.UnixMicro(int64(micros)).UTC()
	}

	// systemd answers for units it has never heard of rather than failing, reporting
	// LoadState "not-found". Surfacing that as an error here means callers do not each
	// have to remember to check.
	if state.LoadState == "not-found" {
		return state, fmt.Errorf("%w: %s", ErrUnitNotFound, unit)
	}
	return state, nil
}

// RestartUnit enqueues a restart and returns a Job that resolves when systemd emits
// JobRemoved for it.
//
// Two details in here are load-bearing.
//
// The result channel is BUFFERED. go-systemd delivers the outcome by writing to this
// channel from its signal dispatcher while holding an internal lock, and its
// documentation is explicit that "until the write is unblocked, the Conn object cannot
// handle other jobs". With an unbuffered channel, a caller that never waits — a client
// that fires a restart and disconnects — would stall every subsequent job on this
// connection. A buffer of one is enough because a job produces exactly one result.
//
// "replace" is the job mode: if a conflicting job is already queued for the unit, it is
// replaced. The alternative, "fail", would reject the request when a restart is already
// in progress, which for an operations console is a worse answer than coalescing.
func (b *systemdBackend) RestartUnit(ctx context.Context, unit string) (*Job, error) {
	results := make(chan string, 1)

	id, err := b.conn.RestartUnitContext(ctx, unit, "replace", results)
	if err != nil {
		return nil, classifyError(dbusErrorName(err), err)
	}
	return newJob(id, unit, results), nil
}

// dbusErrorName extracts the D-Bus error name, e.g.
// "org.freedesktop.DBus.Error.InteractiveAuthorizationRequired".
//
// The name is a stable, machine-readable classifier; the human-readable message is not.
// Returns "" for errors that did not originate from D-Bus, in which case classifyError
// falls back to inspecting the message.
func dbusErrorName(err error) string {
	var dbusErr godbus.Error
	if errors.As(err, &dbusErr) {
		return dbusErr.Name
	}
	return ""
}

func stringProp(props map[string]any, key string) string {
	if v, ok := props[key].(string); ok {
		return v
	}
	return ""
}

func uint64Prop(props map[string]any, key string) uint64 {
	switch v := props[key].(type) {
	case uint64:
		return v
	case int64:
		if v > 0 {
			return uint64(v)
		}
	}
	return 0
}
