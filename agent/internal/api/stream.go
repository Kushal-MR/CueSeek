package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/adapters"
	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
)

// Stream tuning. Every one of these traces back to a measurement in A7
// (docs/m0-findings.md), not to a guess.
const (
	// streamSchemaVersion versions the event payload schema independently of the URL
	// path, because a Wear build will routinely be older than the agent it talks to.
	streamSchemaVersion = "1"

	// streamHeartbeat is how often an idle stream emits a heartbeat.
	//
	// Its purpose is to make silence unambiguous. A7 found that a backgrounded phone
	// holds a connection open and reports it as healthy while receiving nothing for
	// minutes; without a regular pulse, a quiet system and a frozen one look identical.
	// The contract tells clients to treat ~2× this interval as stale, so 15s puts the
	// detection window at roughly 30 seconds.
	streamHeartbeat = 15 * time.Second

	// streamWriteTimeout bounds a single write.
	//
	// The A7 finding made concrete: a frozen client backpressures the write, and without
	// a deadline the goroutine parks until TCP gives up on its own schedule — minutes.
	//
	// It also solves a problem this package would otherwise have. http.Server sets a
	// global WriteTimeout of 60s, which would kill every stream after exactly one
	// minute. Setting a per-write deadline through ResponseController overrides that, so
	// the connection may live indefinitely while each individual write stays bounded.
	streamWriteTimeout = 10 * time.Second

	// streamBuffer is how many events a subscriber may fall behind by.
	//
	// Small on purpose. A client that cannot keep up with sixteen events is not going to
	// recover by being given a larger queue; it needs to reconnect and be told the truth
	// from scratch.
	streamBuffer = 16
)

// streamEvent is one thing worth telling every connected client about.
//
// Carries no sequence number: seq is per-connection and monotonic, so each connection
// stamps its own. emittedAt is set once at publish time, so every client agrees on when
// the event happened rather than when it was delivered to them.
type streamEvent struct {
	typ         gen.StreamEventType
	emittedAt   time.Time
	service     *gen.Service
	hostMetrics *gen.HostMetrics
	action      *gen.ActionProgress

	// hostActionResult carries the failure of a power action. There is no success case:
	// one that worked took this stream with it.
	hostActionResult *gen.HostActionProgress
}

// subscriber is one connected client's mailbox.
type subscriber struct {
	events chan streamEvent

	// done is closed to tell the connection to stop: either it fell too far behind, or
	// the agent is shutting down.
	done      chan struct{}
	closeOnce sync.Once
}

func (s *subscriber) stop() { s.closeOnce.Do(func() { close(s.done) }) }

// hub fans events out to every connected client.
//
// The one rule that matters: publishing never blocks. A send that would wait is dropped
// and that subscriber is disconnected instead. This is what stops one frozen phone from
// delaying every other client — the failure A7 showed is not hypothetical, it is what a
// locked Android screen does for minutes at a time.
type hub struct {
	mu          sync.RWMutex
	subscribers map[*subscriber]struct{}
	closed      bool
}

func newHub() *hub {
	return &hub{subscribers: make(map[*subscriber]struct{})}
}

// subscribe adds a client. Returns nil once the hub is closed, so a request arriving
// during shutdown is refused rather than hanging on a stream nobody will feed.
func (h *hub) subscribe() *subscriber {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	sub := &subscriber{
		events: make(chan streamEvent, streamBuffer),
		done:   make(chan struct{}),
	}
	h.subscribers[sub] = struct{}{}
	return sub
}

func (h *hub) unsubscribe(sub *subscriber) {
	h.mu.Lock()
	delete(h.subscribers, sub)
	h.mu.Unlock()
	sub.stop()
}

// publish delivers to every subscriber without ever waiting.
func (h *hub) publish(event streamEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for sub := range h.subscribers {
		select {
		case sub.events <- event:
		default:
			// Mailbox full. Disconnect rather than dropping the event silently: a client
			// that quietly misses updates keeps rendering state it believes is current,
			// which is the exact failure ADR-0008 calls "confidently wrong". Reconnecting
			// costs 3–4 seconds (A7) and delivers a fresh snapshot, so the correct state
			// is cheap to recover and the incorrect one is not.
			slog.Warn("stream subscriber fell behind; disconnecting it")
			sub.stop()
		}
	}
}

// closeAll disconnects every client. Wired to http.Server.RegisterOnShutdown.
//
// Without this, graceful shutdown would wait out its entire grace period on every
// restart: Shutdown drains in-flight requests, and a stream is an in-flight request that
// never finishes on its own.
func (h *hub) closeAll() {
	h.mu.Lock()
	h.closed = true
	subs := make([]*subscriber, 0, len(h.subscribers))
	for sub := range h.subscribers {
		subs = append(subs, sub)
	}
	h.subscribers = make(map[*subscriber]struct{})
	h.mu.Unlock()

	for _, sub := range subs {
		sub.stop()
	}
	if len(subs) > 0 {
		slog.Info("closed event streams for shutdown", "connections", len(subs))
	}
}

func (h *hub) count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

// ---------------------------------------------------------------- publishing

// onServiceUpdate is registered with the adapter cache and fires after every poll.
//
// Deltas therefore leave the agent the moment state changes, rather than the stream
// re-reading the cache on a timer of its own and adding latency for no benefit.
func (s *Server) onServiceUpdate(snapshot adapters.Snapshot) {
	svc, ok := s.registry.Service(snapshot.ServiceID)
	if !ok {
		return
	}
	view := s.describeService(svc)
	s.hub.publish(streamEvent{
		typ:       gen.StreamEventTypeServiceUpdated,
		emittedAt: s.now(),
		service:   &view,
	})
}

// publishActionProgress emits an action's terminal state.
//
// This is the other half of the 202-plus-action-id design: the HTTP response says
// "accepted", and this is how the client eventually learns what happened (ADR-0004).
func (s *Server) publishActionProgress(actionID, serviceID, action string, state actionState, failure error) {
	progress := gen.ActionProgress{
		ActionId:  actionID,
		ServiceId: serviceID,
		Action:    action,
		Status:    gen.ActionStatus(state),
		At:        s.now(),
	}
	if failure != nil {
		message := failure.Error()
		progress.Error = &message
	}
	s.hub.publish(streamEvent{
		typ:       gen.StreamEventTypeActionProgress,
		emittedAt: progress.At,
		action:    &progress,
	})
}

// ---------------------------------------------------------------- the endpoint

// StreamEvents opens an event stream.
//
// Returns a response object rather than writing directly, because the generated strict
// server owns the response. The generated text/event-stream type cannot be used: it takes
// an io.Reader and does io.Copy, which never flushes, so every event would sit in a buffer
// until the connection ended. Implementing the generated interface ourselves keeps this
// inside the contract without touching generated code.
func (s *Server) StreamEvents(ctx context.Context, _ gen.StreamEventsRequestObject) (gen.StreamEventsResponseObject, error) {
	sub := s.hub.subscribe()
	if sub == nil {
		return nil, errNotImplemented.withDetail("the agent is shutting down")
	}
	return &sseStream{server: s, ctx: ctx, sub: sub}, nil
}

// sseStream implements gen.StreamEventsResponseObject.
type sseStream struct {
	server *Server
	ctx    context.Context
	sub    *subscriber
}

func (st *sseStream) VisitStreamEventsResponse(w http.ResponseWriter) error {
	s := st.server
	defer s.hub.unsubscribe(st.sub)

	rc := http.NewResponseController(w)

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// Tells nginx and friends not to buffer. An intermediary that buffers turns a stream
	// into a batch, and that failure looks exactly like a network stall.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if err := rc.Flush(); err != nil {
		return err
	}

	start := time.Now()
	var seq int64

	// The snapshot is unconditional and first. It is what removes the need for a replay
	// buffer: a reconnecting client is told everything, so nothing has to be remembered
	// on its behalf (ADR-0004).
	if err := st.send(w, rc, seq, gen.StreamEventTypeSnapshot, s.buildSnapshot(), s.now()); err != nil {
		return st.finish(start, seq, err)
	}

	interval := s.heartbeat
	if interval <= 0 {
		interval = streamHeartbeat
	}
	heartbeat := time.NewTicker(interval)
	defer heartbeat.Stop()

	for {
		select {
		case <-st.ctx.Done():
			// The client went away, or the server is closing connections after its
			// drain deadline expired.
			return st.finish(start, seq, nil)

		case <-st.sub.done:
			// Either this client fell too far behind, or shutdown began.
			return st.finish(start, seq, nil)

		case event := <-st.sub.events:
			seq++
			if err := st.send(w, rc, seq, event.typ, event, event.emittedAt); err != nil {
				return st.finish(start, seq, err)
			}

		case <-heartbeat.C:
			seq++
			if err := st.send(w, rc, seq, gen.StreamEventTypeHeartbeat, nil, s.now()); err != nil {
				return st.finish(start, seq, err)
			}
		}
	}
}

// send writes one envelope and flushes it.
//
// The write deadline is reset before every write rather than once per connection, so it
// bounds each individual write instead of the connection's lifetime.
func (st *sseStream) send(
	w http.ResponseWriter, rc *http.ResponseController,
	seq int64, typ gen.StreamEventType, payload any, emittedAt time.Time,
) error {
	if err := rc.SetWriteDeadline(time.Now().Add(streamWriteTimeout)); err != nil {
		// Not fatal: some ResponseWriter wrappers do not support deadlines. The stream
		// still works, it simply loses this protection, so it is worth saying so.
		slog.Warn("stream write deadlines unavailable on this connection", "error", err)
	}

	envelope := gen.StreamEnvelope{
		Type:          typ,
		Seq:           seq,
		EmittedAt:     emittedAt,
		SchemaVersion: streamSchemaVersion,
	}
	switch p := payload.(type) {
	case *gen.Snapshot:
		envelope.Snapshot = p
	case streamEvent:
		envelope.Service = p.service
		envelope.HostMetrics = p.hostMetrics
		envelope.ActionProgress = p.action
		envelope.HostActionProgress = p.hostActionResult
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal stream envelope: %w", err)
	}

	// The SSE `event:` field mirrors the envelope's type so a browser EventSource can
	// addEventListener by name. No `id:` field is emitted, deliberately: an id would make
	// clients send Last-Event-ID on reconnect, implying a replay buffer this design does
	// not have and does not want.
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ, body); err != nil {
		return err
	}
	return rc.Flush()
}

// finish logs the connection's lifetime.
//
// A write error here is normal, not exceptional: it is what a client disappearing looks
// like, and on a mobile network that happens constantly (A7). Logged at debug so a
// commuting phone does not fill the journal.
func (st *sseStream) finish(start time.Time, seq int64, err error) error {
	slog.Debug("stream closed",
		"duration", time.Since(start).Round(time.Second), "events", seq, "error", err)
	// Returning nil: the response has already been written, and handing the error back
	// to the strict handler would make it try to write a problem document into a
	// half-sent event stream.
	return nil
}

// buildSnapshot assembles complete current state.
func (s *Server) buildSnapshot() *gen.Snapshot {
	services := s.registry.Services()
	out := make([]gen.Service, 0, len(services))
	for _, svc := range services {
		out = append(out, s.describeService(svc))
	}
	return &gen.Snapshot{
		System: gen.System{
			HostId:       s.hostID,
			Hostname:     s.hostname,
			AgentVersion: s.agentVersion,
			ApiVersion:   s.apiVersion,
			StartedAt:    s.startedAt,
		},
		Services: out,
		// Absent until the first collection has run, and absent for the whole session on
		// a platform that cannot read them. A client is told nothing rather than told
		// zero, which would describe an idle machine that was never measured.
		HostMetrics: s.currentHostMetrics(),
		// Static for the life of the process, so it rides the snapshot rather than
		// needing an event of its own. A client that never calls the REST endpoint still
		// knows what this machine can be asked to do.
		HostActions: ptrTo(s.hostActions()),
	}
}

// ptrTo is the small adapter the generator's optional-array shape needs.
func ptrTo[T any](v T) *T { return &v }
