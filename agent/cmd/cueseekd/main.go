// Command cueseekd is the CueSeek host agent.
//
// It runs as an unprivileged system user on the machine being managed, polls the
// services it has adapters for, and serves their state and control actions to
// CueSeek clients over a private network.
//
// It deliberately holds no privileges of its own: service restarts and host power
// actions are performed by asking systemd and logind over D-Bus, authorised by a
// polkit rule that enumerates exactly what this user may do. See ADR-0002.
//
// This file is currently a stub. Wiring lands with the agent milestone.
package main

import (
	"fmt"
	"os"
	"runtime"
)

// version is overwritten at build time:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always --dirty)"
var version = "0.0.0-dev"

func main() {
	fmt.Printf("cueseekd %s (%s/%s, %s)\n",
		version, runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Fprintln(os.Stderr, "not implemented: this is a skeleton build")
}
