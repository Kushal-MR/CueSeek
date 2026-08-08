//go:build !linux

package host

import (
	"context"
	"fmt"
	"runtime"
)

// This file exists so the agent builds, vets and tests on a developer machine that is not
// the deployment target.
//
// ADR-0002 chose systemd and logind over D-Bus, which couples host control to Linux. The
// alternative to a stub is that `go build ./...` simply fails on Windows or macOS the
// moment the real implementation lands — which would mean no local test runs, no IDE
// support, and no CI job that is not a Linux container. That is a high price for a
// package the rest of the agent only reaches through an interface.
//
// The stub fails loudly on every operation rather than pretending to succeed. A no-op
// that silently reported success would be far worse: the API would answer 202, the audit
// log would record an accepted restart, and nothing would have happened.
type unsupportedBackend struct{}

func newBackend() (Backend, error) {
	return unsupportedBackend{}, nil
}

func (unsupportedBackend) Platform() string {
	return "unsupported/" + runtime.GOOS
}

func (unsupportedBackend) Close() error { return nil }

func (unsupportedBackend) UnitState(_ context.Context, unit string) (UnitState, error) {
	return UnitState{}, unsupported("read state of unit %q", unit)
}

func (unsupportedBackend) RestartUnit(_ context.Context, unit string) (*Job, error) {
	return nil, unsupported("restart unit %q", unit)
}

func unsupported(action string, args ...any) error {
	return fmt.Errorf("%w: cannot %s on %s",
		ErrUnsupportedPlatform, fmt.Sprintf(action, args...), runtime.GOOS)
}
