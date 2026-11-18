package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

// The limit, per-key independence, rollover and pruning are covered in api_test.go.
// These pin the two edges it leaves open: the exact end of a window, and what counts as
// one caller.

func TestRateLimiterWindowEndsAfterItsDuration(t *testing.T) {
	now := time.Now()
	rl := newRateLimiter(1, time.Minute)
	rl.now = func() time.Time { return now }

	rl.Allow("a")
	now = now.Add(time.Minute)
	if rl.Allow("a") {
		t.Fatal("allowed at the exact end of the window; it should reset only after it")
	}
	now = now.Add(time.Nanosecond)
	if !rl.Allow("a") {
		t.Fatal("still refused once the window has passed")
	}
}

// TestClientKeyUsesTheConnectionAddressOnly: the forwarding headers are whatever the
// caller sent. Honouring them would give every guess a fresh limit.
func TestClientKeyUsesTheConnectionAddressOnly(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/pair", nil)
	r.RemoteAddr = "100.64.0.7:51234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	r.Header.Set("X-Real-IP", "203.0.113.10")

	if got := clientKey(r); got != "100.64.0.7" {
		t.Fatalf("clientKey = %q, want the connection's host %q", got, "100.64.0.7")
	}

	// A new source port is the same caller.
	r2 := httptest.NewRequest("POST", "/v1/pair", nil)
	r2.RemoteAddr = "100.64.0.7:60000"
	if clientKey(r2) != clientKey(r) {
		t.Fatal("a new source port gave the same host a separate limit")
	}
}

func TestClientKeyFallsBackToTheWholeAddress(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/pair", nil)
	r.RemoteAddr = "not-host-port"
	if got := clientKey(r); got != "not-host-port" {
		t.Fatalf("clientKey = %q, want the address used whole rather than an empty key", got)
	}
}
