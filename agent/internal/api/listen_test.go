package api

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// Regression tests for a real deployment failure.
//
// On the target host the agent binds to a Tailscale address. At boot, tailscaled creates
// the interface immediately but assigns 100.92.18.125/32 only after authenticating, tens
// of seconds later — so an unconditional bind fails with EADDRNOTAVAIL.
//
// Ordering the unit after tailscaled.service does not fix it: that waits for the daemon,
// not the address. An attempt to wait on a `tailscale0.device` unit was worse — systemd
// resolved that name to a device path `/tailscale0` that never appears, and the boot
// stalled for the full 90-second DefaultTimeoutStartSec before starting the agent.
//
// The listen retry replaces both. These tests drive it through an injected listen
// function, so they are deterministic and need no interface to appear on the test
// machine — and they run identically on Linux and Windows.

func newListenTestServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv, err := New(Options{Store: st, HostID: "test-host"})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	return srv
}

// addrNotAvail is shaped like a real failure from the net package: a *net.OpError
// wrapping a *os.SyscallError wrapping the errno. Testing against a bare errno would not
// prove that the classifier unwraps what the standard library actually returns.
func addrNotAvail() error {
	return &net.OpError{
		Op: "listen", Net: "tcp",
		Err: &net.AddrError{Err: "bind", Addr: "100.64.0.5:7777"},
	}
}

func realAddrNotAvail() error {
	return &net.OpError{Op: "listen", Net: "tcp", Err: syscall.EADDRNOTAVAIL}
}

// TestListenWaitsForAddressToAppear is the fix for the boot race: the address shows up
// after a few seconds and the agent binds without ever reporting a failure.
func TestListenWaitsForAddressToAppear(t *testing.T) {
	srv := newListenTestServer(t)

	var attempts atomic.Int32
	srv.listen = func(ctx context.Context, network, address string) (net.Listener, error) {
		if attempts.Add(1) < 3 {
			return nil, realAddrNotAvail()
		}
		return (&net.ListenConfig{}).Listen(ctx, network, "127.0.0.1:0")
	}

	started := time.Now()
	listener, err := srv.Listen(t.Context(), config.Bind{Address: "100.64.0.5:7777"})
	if err != nil {
		t.Fatalf("Listen gave up on an address that appeared: %v", err)
	}
	defer listener.Close()

	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
	// Two waits of listenRetryInterval, and nothing like the 90-second window.
	if elapsed := time.Since(started); elapsed > listenRetryWindow/2 {
		t.Errorf("took %v; it should have bound as soon as the address appeared", elapsed)
	}
}

// TestListenSucceedsFirstTryWithoutWaiting: the common case must not pay for the retry.
func TestListenSucceedsFirstTryWithoutWaiting(t *testing.T) {
	srv := newListenTestServer(t)

	var attempts atomic.Int32
	srv.listen = func(ctx context.Context, network, address string) (net.Listener, error) {
		attempts.Add(1)
		return (&net.ListenConfig{}).Listen(ctx, network, "127.0.0.1:0")
	}

	started := time.Now()
	listener, err := srv.Listen(t.Context(), config.Bind{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()

	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("took %v for an address that was already present", elapsed)
	}
}

// TestListenDoesNotRetryUnrecoverableErrors is the most important test here.
//
// A port already in use will never resolve on its own. Retrying it would turn an
// immediate, legible startup error into a silent 90-second hang followed by a confusing
// message about a missing address.
func TestListenDoesNotRetryUnrecoverableErrors(t *testing.T) {
	cases := map[string]error{
		"address in use":    &net.OpError{Op: "listen", Err: syscall.EADDRINUSE},
		"permission denied": &net.OpError{Op: "listen", Err: syscall.EACCES},
		"malformed address": addrNotAvail(),
		"unclassified":      errors.New("something else entirely"),
	}

	for name, listenErr := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newListenTestServer(t)

			var attempts atomic.Int32
			srv.listen = func(context.Context, string, string) (net.Listener, error) {
				attempts.Add(1)
				return nil, listenErr
			}

			started := time.Now()
			if _, err := srv.Listen(t.Context(), config.Bind{Address: "127.0.0.1:7777"}); err == nil {
				t.Fatal("Listen succeeded on an unrecoverable error")
			}

			if got := attempts.Load(); got != 1 {
				t.Errorf("attempts = %d, want 1 — this error will never resolve", got)
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Errorf("waited %v before failing; unrecoverable errors must be immediate", elapsed)
			}
		})
	}
}

// TestListenRealPortConflictFailsFast covers the same property against the real network
// stack rather than a synthetic error, on whatever platform the tests run.
func TestListenRealPortConflictFailsFast(t *testing.T) {
	srv := newListenTestServer(t)

	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold a port: %v", err)
	}
	defer held.Close()

	started := time.Now()
	if _, err := srv.Listen(t.Context(), config.Bind{Address: held.Addr().String()}); err == nil {
		t.Fatal("Listen succeeded on an already-bound port")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("a port conflict took %v to report", elapsed)
	}
}

// TestListenGivesUpAfterTheWindow: a genuinely wrong bind.address must eventually surface
// as a startup failure rather than a process that waits forever. systemd's
// Restart=on-failure then retries the whole cycle.
//
// The window is shortened to milliseconds rather than waiting the real ninety seconds.
func TestListenGivesUpAfterTheWindow(t *testing.T) {
	srv := newListenTestServer(t)
	srv.listen = func(context.Context, string, string) (net.Listener, error) {
		return nil, realAddrNotAvail()
	}
	srv.listenWindow = 300 * time.Millisecond
	srv.listenInterval = 50 * time.Millisecond

	started := time.Now()
	_, err := srv.Listen(t.Context(), config.Bind{Address: "100.64.0.5:7777"})
	if err == nil {
		t.Fatal("Listen never gave up")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("took %v to give up on a %v window", elapsed, srv.listenWindow)
	}

	// Whoever reads this is looking at a service that will not start, so it must name
	// the address, say how long it waited, and suggest where to look.
	for _, want := range []string{"100.64.0.5:7777", "not present", "VPN"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error is missing %q: %v", want, err)
		}
	}
	// The underlying cause survives wrapping, so callers can still classify it.
	if !isAddressUnavailable(err) {
		t.Errorf("the wrapped error lost EADDRNOTAVAIL: %v", err)
	}
}

// TestListenCancellationErrorNamesTheAddress: returning a bare "context canceled" gives
// an operator nothing to act on. Found by an earlier version of this test.
func TestListenCancellationErrorNamesTheAddress(t *testing.T) {
	srv := newListenTestServer(t)
	srv.listen = func(context.Context, string, string) (net.Listener, error) {
		return nil, realAddrNotAvail()
	}
	srv.listenInterval = 20 * time.Millisecond

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	_, err := srv.Listen(ctx, config.Bind{Address: "100.64.0.5:7777"})
	if err == nil {
		t.Fatal("Listen ignored the deadline")
	}
	if !strings.Contains(err.Error(), "100.64.0.5:7777") {
		t.Errorf("error does not name the address: %v", err)
	}
	// Wrapping must not break classification by callers.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
}

// TestListenStopsOnContextCancellation: SIGTERM during the wait must be honoured.
//
// Without this, a shutdown issued while the agent is waiting for a VPN address would be
// ignored until the window expired, and systemd would escalate to SIGKILL.
func TestListenStopsOnContextCancellation(t *testing.T) {
	srv := newListenTestServer(t)
	srv.listen = func(context.Context, string, string) (net.Listener, error) {
		return nil, realAddrNotAvail()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := srv.Listen(ctx, config.Bind{Address: "100.64.0.5:7777"})
		done <- err
	}()

	// Let it enter the retry loop, then interrupt.
	time.Sleep(200 * time.Millisecond)
	started := time.Now()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		// Must return promptly, not at the end of the current retry interval plus the
		// remaining window.
		if elapsed := time.Since(started); elapsed > 2*time.Second {
			t.Errorf("took %v to honour cancellation", elapsed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Listen ignored context cancellation")
	}
}

// TestIsAddressUnavailableClassification pins the classifier itself, including the
// wrapping the net package actually applies.
func TestIsAddressUnavailableClassification(t *testing.T) {
	retryable := map[string]error{
		"bare errno":  syscall.EADDRNOTAVAIL,
		"in OpError":  &net.OpError{Op: "listen", Err: syscall.EADDRNOTAVAIL},
		"double wrap": &net.OpError{Op: "listen", Err: &net.OpError{Err: syscall.EADDRNOTAVAIL}},
	}
	for name, err := range retryable {
		if !isAddressUnavailable(err) {
			t.Errorf("%s: should be retryable, is not", name)
		}
	}

	notRetryable := map[string]error{
		"nil":               nil,
		"address in use":    &net.OpError{Op: "listen", Err: syscall.EADDRINUSE},
		"permission denied": &net.OpError{Op: "listen", Err: syscall.EACCES},
		"plain error":       errors.New("boom"),
	}
	for name, err := range notRetryable {
		if isAddressUnavailable(err) {
			t.Errorf("%s: should not be retryable, is", name)
		}
	}
}
