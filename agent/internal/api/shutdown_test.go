package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// newServeEnv starts a real Serve loop on an ephemeral port and returns its address plus
// a cancel function standing in for SIGTERM.
func newServeEnv(t *testing.T) (addr string, sigterm context.CancelFunc, serveErr <-chan error) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv, err := New(Options{Store: st, AgentVersion: "test", HostID: "test-host"})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}

	listener, err := srv.Listen(t.Context(), config.Bind{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx, listener) }()

	return listener.Addr().String(), cancel, errCh
}

// TestGracefulShutdownCompletesInFlightRequest is a regression test for a real bug.
//
// http.Server.BaseContext was originally the signal context, making it the parent of
// every request context. SIGTERM therefore cancelled every in-flight request at the exact
// moment Shutdown began waiting for those requests to finish — so the drain was a formality
// and any request already running failed with context.Canceled while the logs still
// reported a clean shutdown.
//
// The request here is deliberately slow: its body arrives through a pipe, so the handler
// is blocked reading when shutdown starts. Under the old code the body read is cancelled
// and the client sees a transport error or a 400. Under the fix the request completes
// normally and returns the 403 an invalid pairing code deserves.
func TestGracefulShutdownCompletesInFlightRequest(t *testing.T) {
	addr, sigterm, serveErr := newServeEnv(t)

	bodyReader, bodyWriter := io.Pipe()
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/pair", bodyReader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	type result struct {
		resp *http.Response
		err  error
	}
	results := make(chan result, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		results <- result{resp, err}
	}()

	// Send half the body so the server has read the headers and entered the handler.
	if _, err := bodyWriter.Write([]byte(`{"code":"AAAA-AAAA",`)); err != nil {
		t.Fatalf("write first half: %v", err)
	}
	time.Sleep(250 * time.Millisecond)

	sigterm() // the request is now in flight
	time.Sleep(250 * time.Millisecond)

	// Finish the request during the drain window.
	if _, err := bodyWriter.Write([]byte(`"device_name":"Phone","platform":"cli"}`)); err != nil {
		t.Fatalf("write second half: %v", err)
	}
	bodyWriter.Close()

	select {
	case r := <-results:
		if r.err != nil {
			t.Fatalf("in-flight request failed during shutdown: %v", r.err)
		}
		defer r.resp.Body.Close()
		// 403 = the pairing code was rejected, which means the request was fully read,
		// routed, and handled. Any other status means shutdown interfered.
		if r.resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want 403; the request did not complete normally",
				r.resp.StatusCode)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("in-flight request never completed")
	}

	select {
	case err := <-serveErr:
		if err != nil {
			t.Errorf("Serve returned %v, want nil after a clean shutdown", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Serve did not return after shutdown")
	}
}

// TestShutdownReleasesListener: if the port were still held, systemd restarting the unit
// would fail with "address already in use" — a confusing symptom for a shutdown bug.
func TestShutdownReleasesListener(t *testing.T) {
	addr, sigterm, serveErr := newServeEnv(t)

	// Confirm it is actually serving first, so a failure to rebind later cannot be
	// explained by it never having bound.
	resp, err := http.Get("http://" + addr + "/v1/system")
	if err != nil {
		t.Fatalf("pre-shutdown request: %v", err)
	}
	resp.Body.Close()

	sigterm()
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Serve did not return")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port %s still held after shutdown: %v", addr, err)
	}
	listener.Close()
}

// TestShutdownStopsAcceptingNewConnections: the drain window is for requests already in
// flight, not an invitation for new ones.
func TestShutdownStopsAcceptingNewConnections(t *testing.T) {
	addr, sigterm, serveErr := newServeEnv(t)

	sigterm()
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Serve did not return")
	}

	// Fresh client so no keep-alive connection is reused.
	client := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
		Timeout:   3 * time.Second,
	}
	if resp, err := client.Get("http://" + addr + "/v1/system"); err == nil {
		resp.Body.Close()
		t.Error("server accepted a new connection after shutdown")
	}
}

// TestServeReturnsListenerErrors: if Serve stops for a reason other than shutdown, that
// must surface as an error rather than looking like a clean stop.
func TestServeReturnsListenerErrors(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv, err := New(Options{Store: st, HostID: "test-host"})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	listener, err := srv.Listen(t.Context(), config.Bind{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(context.Background(), listener) }()

	time.Sleep(200 * time.Millisecond)
	listener.Close() // simulate the listener failing underneath us

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Serve returned nil after the listener broke; a crash would look clean")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return")
	}
}

// TestListenRejectsUnavailablePort: binding failures must be reported before anything
// else starts, so the operator sees the real cause.
func TestListenRejectsUnavailablePort(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	srv, err := New(Options{Store: st, HostID: "test-host"})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}

	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold a port: %v", err)
	}
	defer held.Close()

	if _, err := srv.Listen(t.Context(), config.Bind{Address: held.Addr().String()}); err == nil {
		t.Error("Listen succeeded on an already-bound port")
	}
}
