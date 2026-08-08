package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a fixed-window counter, used to make pairing codes unguessable in
// practice rather than only in theory.
//
// A pairing code carries 40 bits of entropy — far short of a token's 256. That is safe
// only because a code is single-use, short-lived, and cannot be guessed at speed. This
// type provides the third property; without it the other two would not be enough.
//
// A fixed window is coarse: a caller can burst at the end of one window and the start of
// the next, getting 2×limit in quick succession. For slowing down guesses against a
// five-minute code that is entirely adequate, and a sliding window or token bucket would
// be more machinery than the problem warrants.
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	counts map[string]*window

	// now is injectable so tests can advance time instead of sleeping through it.
	now func() time.Time
}

type window struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(limit int, per time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:  limit,
		window: per,
		counts: make(map[string]*window),
		now:    time.Now,
	}
}

// Allow records an attempt for key and reports whether it is within the limit.
func (rl *rateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()

	// Prune expired entries on the way past. Without this the map grows once per
	// distinct source address forever — a slow memory leak an attacker can drive by
	// varying source ports, which costs them nothing.
	for k, w := range rl.counts {
		if now.After(w.resetAt) {
			delete(rl.counts, k)
		}
	}

	w, ok := rl.counts[key]
	if !ok || now.After(w.resetAt) {
		rl.counts[key] = &window{count: 1, resetAt: now.Add(rl.window)}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}

// clientKey identifies the caller for rate-limiting purposes.
//
// Uses the transport-level remote address only. X-Forwarded-For and X-Real-IP are
// deliberately ignored: ADR-0001 puts the agent behind a VPN with no reverse proxy, so
// those headers can only have come from the caller — and honouring them would let an
// attacker mint a fresh identity per attempt simply by changing a header, defeating the
// limit entirely.
//
// If a deployment ever does put a trusted proxy in front, this is the function to change,
// and it must be accompanied by a list of trusted proxy addresses.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// Not host:port — use it whole rather than failing open.
		return r.RemoteAddr
	}
	return host
}
