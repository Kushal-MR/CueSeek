package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/config"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
	"github.com/Kushal-MR/CueSeek/agent/internal/store"
)

// A parsed SSE frame: the `event:` name and the decoded envelope from `data:`.
type frame struct {
	name     string
	envelope gen.StreamEnvelope
}

// openStream connects and returns a channel of frames plus a cancel function.
//
// Reads on a goroutine so a test can assert on ordering and timing without blocking, and
// so an abandoned stream is closed by cancelling its context — which is what a client
// disappearing looks like.
func openStream(t *testing.T, env *testEnv, token string) (<-chan frame, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.server.URL+"/v1/stream", nil)
	if err != nil {
		cancel()
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := env.server.Client().Do(req)
	if err != nil {
		cancel()
		t.Fatalf("open stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		resp.Body.Close()
		cancel()
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	frames := make(chan frame, 64)
	go func() {
		defer close(frames)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var name string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				var env gen.StreamEnvelope
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &env); err != nil {
					return
				}
				select {
				case frames <- frame{name: name, envelope: env}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	t.Cleanup(cancel)
	return frames, cancel
}

func nextFrame(t *testing.T, frames <-chan frame, what string) frame {
	t.Helper()
	select {
	case f, ok := <-frames:
		if !ok {
			t.Fatalf("stream closed while waiting for %s", what)
		}
		return f
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return frame{}
	}
}

// ---------------------------------------------------------------- snapshot

// TestStreamSendsSnapshotFirst is the property that removes the need for a replay buffer:
// every connection is told everything, so nothing has to be remembered on its behalf
// (ADR-0004).
func TestStreamSendsSnapshotFirst(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	env.cache.Put("jellyfin", adapters.Observation{Health: domain.Health{
		Status: domain.StatusHealthy, Reachable: true, ObservedAt: time.Now().UTC(),
	}, Actions: []domain.Action{{
		ID:          "restart",
		Label:       "Restart Jellyfin",
		Description: "Restarts the Jellyfin service.",
		Risk:        domain.RiskDisruptive,
	}}})

	frames, _ := openStream(t, env, token)
	first := nextFrame(t, frames, "the snapshot")

	if first.name != string(gen.StreamEventTypeSnapshot) {
		t.Errorf("SSE event name = %q, want snapshot", first.name)
	}
	if first.envelope.Type != gen.StreamEventTypeSnapshot {
		t.Errorf("envelope type = %q", first.envelope.Type)
	}
	if first.envelope.Seq != 0 {
		t.Errorf("seq = %d, want 0 for the snapshot", first.envelope.Seq)
	}
	if first.envelope.SchemaVersion == "" {
		t.Error("schema_version is empty; clients version payloads on it")
	}
	if first.envelope.EmittedAt.IsZero() {
		t.Error("emitted_at is zero")
	}

	snap := first.envelope.Snapshot
	if snap == nil {
		t.Fatal("snapshot event carries no snapshot")
	}
	if snap.System.HostId != "test-host" {
		t.Errorf("system.host_id = %q", snap.System.HostId)
	}
	if len(snap.Services) != 1 || snap.Services[0].Id != "jellyfin" {
		t.Fatalf("services = %+v", snap.Services)
	}
	if snap.Services[0].Health.Status != gen.HealthStatusHealthy {
		t.Errorf("service health = %q, want the cached healthy", snap.Services[0].Health.Status)
	}
	// The snapshot carries the same joined view /v1/services returns — capabilities and
	// actions included — so a client needs one source of truth, not two.
	if len(snap.Services[0].Capabilities) == 0 || len(snap.Services[0].Actions) == 0 {
		t.Error("snapshot service is missing capabilities or actions")
	}
}

// TestStreamRequiresReadScope: the stream exposes everything, so it is authenticated like
// everything else.
func TestStreamRequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, env.server.URL+"/v1/stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// ---------------------------------------------------------------- deltas

// TestStreamSendsServiceDeltas: a poll that changes state must reach connected clients
// without them asking.
func TestStreamSendsServiceDeltas(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	frames, _ := openStream(t, env, token)
	nextFrame(t, frames, "the snapshot")

	env.cache.Put("jellyfin", adapters.Observation{Health: domain.Health{
		Status: domain.StatusUnreachable, ObservedAt: time.Now().UTC(),
		Reasons: []domain.HealthReason{{Code: domain.ReasonUnreachable, Message: "refused"}},
	}, Actions: []domain.Action{{
		ID:          "restart",
		Label:       "Restart Jellyfin",
		Description: "Restarts the Jellyfin service.",
		Risk:        domain.RiskDisruptive,
	}}})

	delta := nextFrame(t, frames, "a service_updated delta")
	if delta.envelope.Type != gen.StreamEventTypeServiceUpdated {
		t.Fatalf("type = %q, want service_updated", delta.envelope.Type)
	}
	if delta.envelope.Seq != 1 {
		t.Errorf("seq = %d, want 1 (monotonic per connection)", delta.envelope.Seq)
	}
	if delta.envelope.Service == nil {
		t.Fatal("service_updated carries no service")
	}
	if delta.envelope.Service.Health.Status != gen.HealthStatusUnreachable {
		t.Errorf("status = %q, want unreachable", delta.envelope.Service.Health.Status)
	}
	if len(delta.envelope.Service.Health.Reasons) == 0 {
		t.Error("delta lost the reasons")
	}
}

// TestStreamSendsActionProgress is the other half of the 202-plus-action-id design: the
// HTTP response says "accepted", and this is how the client learns what happened.
func TestStreamSendsActionProgress(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead, domain.ScopeServiceControl)

	frames, _ := openStream(t, env, token)
	nextFrame(t, frames, "the snapshot")

	resp := env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", token, nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("action status = %d, want 202", resp.StatusCode)
	}
	accepted := decode[wireAction](t, resp)

	// The stream may deliver a service delta first; the action's outcome is what matters.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatal("stream closed before action_progress arrived")
			}
			if f.envelope.Type != gen.StreamEventTypeActionProgress {
				continue
			}
			progress := f.envelope.ActionProgress
			if progress == nil {
				t.Fatal("action_progress carries no payload")
			}
			// The id correlates the stream event with the HTTP response. Without this
			// the client cannot tell which of two restarts finished.
			if progress.ActionId != accepted.ActionId {
				t.Errorf("action_id = %q, want %q", progress.ActionId, accepted.ActionId)
			}
			if progress.ServiceId != "jellyfin" || progress.Action != "restart" {
				t.Errorf("progress = %+v", progress)
			}
			if progress.Status != gen.Succeeded {
				t.Errorf("status = %q, want succeeded", progress.Status)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for action_progress")
		}
	}
}

// ---------------------------------------------------------------- heartbeat

// TestStreamSendsHeartbeats covers the A7 requirement directly. A backgrounded phone holds
// a connection open and reports it as healthy while receiving nothing; without a regular
// pulse, a quiet system and a frozen one are indistinguishable.
func TestStreamSendsHeartbeats(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	// Shortened for the test rather than waiting 15 seconds. Per-server, so parallel
	// tests cannot interfere with each other.
	env.api.heartbeat = 50 * time.Millisecond

	frames, _ := openStream(t, env, token)
	nextFrame(t, frames, "the snapshot")

	beat := nextFrame(t, frames, "a heartbeat")
	if beat.envelope.Type != gen.StreamEventTypeHeartbeat {
		t.Fatalf("type = %q, want heartbeat", beat.envelope.Type)
	}
	if beat.name != string(gen.StreamEventTypeHeartbeat) {
		t.Errorf("SSE event name = %q", beat.name)
	}
	// A heartbeat carries no payload — its existence is the message.
	if beat.envelope.Snapshot != nil || beat.envelope.Service != nil || beat.envelope.ActionProgress != nil {
		t.Error("heartbeat carries a payload it should not")
	}

	second := nextFrame(t, frames, "a second heartbeat")
	if second.envelope.Seq <= beat.envelope.Seq {
		t.Errorf("seq did not advance: %d then %d", beat.envelope.Seq, second.envelope.Seq)
	}
}

// ---------------------------------------------------------------- fan-out

// TestMultipleClientsAllReceiveEvents: several devices watch the same host, and every one
// must see every change.
func TestMultipleClientsAllReceiveEvents(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	const clients = 4
	streams := make([]<-chan frame, clients)
	for i := range clients {
		streams[i], _ = openStream(t, env, token)
		nextFrame(t, streams[i], "the snapshot")
	}

	waitFor(t, "all clients to be registered", func() bool { return env.api.hub.count() == clients })

	env.cache.Put("jellyfin", adapters.Observation{Health: domain.Health{
		Status: domain.StatusDegraded, Reachable: true, ObservedAt: time.Now().UTC(),
	}, Actions: []domain.Action{{
		ID:          "restart",
		Label:       "Restart Jellyfin",
		Description: "Restarts the Jellyfin service.",
		Risk:        domain.RiskDisruptive,
	}}})

	for i, s := range streams {
		delta := nextFrame(t, s, "a delta on client")
		if delta.envelope.Type != gen.StreamEventTypeServiceUpdated {
			t.Errorf("client %d: type = %q", i, delta.envelope.Type)
		}
		if delta.envelope.Service == nil ||
			delta.envelope.Service.Health.Status != gen.HealthStatusDegraded {
			t.Errorf("client %d did not receive the update", i)
		}
	}
}

// TestSlowClientDoesNotStallOthers is the concurrency claim this milestone rests on.
//
// One subscriber never reads its mailbox — the frozen-phone case A7 measured. The hub
// must keep serving everyone else at full speed, and eventually disconnect the laggard
// rather than waiting on it.
func TestSlowClientDoesNotStallOthers(t *testing.T) {
	env := newTestEnv(t)

	// A subscriber taken directly from the hub, so nothing ever drains it.
	frozen := env.api.hub.subscribe()
	if frozen == nil {
		t.Fatal("subscribe returned nil")
	}

	token := env.pairDevice(t, "Phone", domain.ScopeRead)
	healthy, _ := openStream(t, env, token)
	nextFrame(t, healthy, "the snapshot")

	// Enough to overflow the frozen client's mailbox, paced so a working connection has
	// time to drain its own. Publishing instantaneously would overflow every mailbox
	// including healthy ones — which cannot happen in practice, where events arrive at
	// one per poll interval, but would make this test assert the wrong thing.
	for i := range streamBuffer + 5 {
		status := domain.StatusHealthy
		if i%2 == 0 {
			status = domain.StatusDegraded
		}
		env.cache.Put("jellyfin", adapters.Observation{Health: domain.Health{
			Status: status, Reachable: true, ObservedAt: time.Now().UTC(),
		}, Actions: []domain.Action{{
			ID:          "restart",
			Label:       "Restart Jellyfin",
			Description: "Restarts the Jellyfin service.",
			Risk:        domain.RiskDisruptive,
		}}})
		time.Sleep(2 * time.Millisecond)
	}

	// The frozen one is disconnected rather than being waited on.
	select {
	case <-frozen.done:
	case <-time.After(5 * time.Second):
		t.Fatal("a subscriber that never reads was not disconnected")
	}

	// And the working client is unaffected: it is still connected and still receiving
	// events published after the laggard was cut loose.
	env.cache.Put("jellyfin", adapters.Observation{Health: domain.Health{
		Status: domain.StatusUnreachable, ObservedAt: time.Now().UTC(),
	}, Actions: []domain.Action{{
		ID:          "restart",
		Label:       "Restart Jellyfin",
		Description: "Restarts the Jellyfin service.",
		Risk:        domain.RiskDisruptive,
	}}})

	deadline := time.After(10 * time.Second)
	for {
		select {
		case f, ok := <-healthy:
			if !ok {
				t.Fatal("the working client was disconnected along with the frozen one")
			}
			if f.envelope.Type == gen.StreamEventTypeServiceUpdated &&
				f.envelope.Service != nil &&
				f.envelope.Service.Health.Status == gen.HealthStatusUnreachable {
				return
			}
		case <-deadline:
			t.Fatal("the working client stopped receiving events")
		}
	}
}

// ---------------------------------------------------------------- lifecycle

// TestStreamGoroutinesAreReleasedOnDisconnect: streams are long-lived, so a leak here
// accumulates for the life of a process expected to run for months.
func TestStreamGoroutinesAreReleasedOnDisconnect(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	settle := func() {
		for range 20 {
			runtime.GC()
			time.Sleep(20 * time.Millisecond)
		}
	}
	settle()
	before := runtime.NumGoroutine()

	for range 5 {
		frames, cancel := openStream(t, env, token)
		nextFrame(t, frames, "the snapshot")
		cancel()
	}

	waitFor(t, "the hub to empty", func() bool { return env.api.hub.count() == 0 })
	settle()

	after := runtime.NumGoroutine()
	// A small allowance: the http client and test server keep their own pools, and the
	// exact count is not deterministic. A leak of one goroutine per stream would show up
	// as five or more.
	if after > before+3 {
		t.Errorf("goroutines: %d before, %d after five streams — likely a leak", before, after)
	}
}

// TestHubUnsubscribeIsIdempotent: unsubscribe runs on the connection's defer and stop() may
// already have been called by the hub or by shutdown. A double close would panic.
func TestHubUnsubscribeIsIdempotent(t *testing.T) {
	h := newHub()
	sub := h.subscribe()

	h.unsubscribe(sub)
	h.unsubscribe(sub)
	sub.stop()

	if h.count() != 0 {
		t.Errorf("count = %d, want 0", h.count())
	}
}

// TestHubCloseAllDisconnectsEveryone is what RegisterOnShutdown calls. Without it,
// Shutdown waits out its whole grace period on every restart.
func TestHubCloseAllDisconnectsEveryone(t *testing.T) {
	h := newHub()
	subs := []*subscriber{h.subscribe(), h.subscribe(), h.subscribe()}

	h.closeAll()

	for i, sub := range subs {
		select {
		case <-sub.done:
		case <-time.After(2 * time.Second):
			t.Errorf("subscriber %d was not disconnected", i)
		}
	}
	if h.count() != 0 {
		t.Errorf("count = %d after closeAll", h.count())
	}
	// A connection arriving mid-shutdown is refused rather than left hanging on a stream
	// nothing will feed.
	if h.subscribe() != nil {
		t.Error("subscribe succeeded after closeAll")
	}
}

// TestShutdownClosesLiveStreams exercises the real path against a real Serve loop: a
// stream is open, SIGTERM arrives, and shutdown must complete promptly.
//
// Without RegisterOnShutdown this test hangs until the drain deadline expires, because a
// stream is an in-flight request that never ends on its own — exactly the stall it exists
// to prevent.
func TestShutdownClosesLiveStreams(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	// A device created directly, so the test does not depend on the pairing flow.
	_, token, err := st.CreateDevice(t.Context(), "Phone", domain.PlatformCLI,
		[]domain.Scope{domain.ScopeRead})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}

	srv, err := New(Options{Store: st, AgentVersion: "test", HostID: "test-host"})
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	listener, err := srv.Listen(t.Context(), config.Bind{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}

	ctx, sigterm := context.WithCancel(context.Background())
	t.Cleanup(sigterm)
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ctx, listener) }()

	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()

	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet,
		"http://"+listener.Addr().String()+"/v1/stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", resp.StatusCode)
	}

	// Read the snapshot so the stream is genuinely established, not merely accepted.
	buf := make([]byte, 1)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("read stream: %v", err)
	}

	waitFor(t, "the stream to register", func() bool { return srv.hub.count() == 1 })

	started := time.Now()
	sigterm()

	select {
	case err := <-serveErr:
		if err != nil {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Serve did not return; shutdown waited on the open stream")
	}

	// Well inside the 10s drain deadline. If RegisterOnShutdown were missing this would
	// take the full grace period.
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("shutdown took %v; the stream was not closed promptly", elapsed)
	}
	if srv.hub.count() != 0 {
		t.Errorf("hub still holds %d subscribers after shutdown", srv.hub.count())
	}
}

// TestPublishToEmptyHubIsSafe: the poller publishes on every observation whether or not
// anyone is listening, which is the common case.
func TestPublishToEmptyHubIsSafe(t *testing.T) {
	h := newHub()
	for range 100 {
		h.publish(streamEvent{typ: gen.StreamEventTypeHeartbeat, emittedAt: time.Now()})
	}
	if h.count() != 0 {
		t.Errorf("count = %d", h.count())
	}
}
