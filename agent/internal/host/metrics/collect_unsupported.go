//go:build !linux

package metrics

import (
	"fmt"
	"runtime"
)

// This file exists so the agent builds, vets and tests on a developer machine that is not
// the deployment target — the same reason the parent package carries a stub backend.
//
// The parsers are not stubbed, and that is the point of keeping them in an untagged file.
// Every judgement call this package makes — counter differencing, which memory number is
// honest, how a sensor is labelled — is exercised by the test suite on any platform. What
// is missing here is one syscall and a filesystem layout, not the logic.

// Supported reports whether this build can read host metrics at all. See the Linux file.
const Supported = false

func diskUsage(path string) (total, free int64, err error) {
	return 0, 0, fmt.Errorf("cannot measure %q on %s", path, runtime.GOOS)
}
