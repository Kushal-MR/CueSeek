// Package host controls and observes the machine itself: systemd unit state and
// restarts, power actions, and host metrics.
//
// # HostController
//
// All of it sits behind a HostController interface. The systemd/logind implementation
// is one backend, chosen because it is the only one that exists today — not because
// the abstraction is speculative. Supporting a non-systemd host later should mean
// writing a second backend, not unpicking D-Bus calls from HTTP handlers (ADR-0002).
//
// # Privilege
//
// The agent runs unprivileged and never elevates. Restarts and power actions are
// requests to system services over D-Bus:
//
//	org.freedesktop.systemd1   RestartUnit, unit property reads
//	org.freedesktop.login1     Reboot, PowerOff
//
// Authorisation lives in a polkit rule shipped in deploy/, which grants those actions
// to the cueseek user and nothing else. That file is the complete statement of what
// CueSeek can do to the machine, and an operator can audit it in a minute.
//
// This is not merely safer than a sudoers allowlist — it is less code. Unit health is
// needed for the dashboard regardless, and D-Bus returns ActiveState, SubState and
// ActiveEnterTimestamp as typed values over the same connection used to restart
// things. The sudoers route needs two mechanisms: shelling out to control, and parsing
// `systemctl show` output to observe.
//
// # Rules
//
//   - Never shell out. No exec of systemctl, ever. Once managed units become
//     user-configurable, a shell would make service names part of a command string,
//     and that is an injection surface this design does not have.
//
//   - Managed units come from an allowlist, and the allowlist is enforced in both
//     places: here, and in the polkit rule. Defence in depth is cheap when one half
//     is a config file.
//
//   - Power actions must be acknowledged to the caller before they are executed. See
//     the note in internal/api.
package host
