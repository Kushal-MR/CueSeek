package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// Regression tests from two real debugging sessions on the deployment host, 2026-08-08.
// Both were investigated as suspected authentication defects. Neither was one.
//
// # Incident 1 — an empty credential
//
// A shell pipeline captured an empty token. A pairing code is single-use, so a second
// POST /v1/pair returned 403, the `sed` extracting the token matched nothing, and every
// later request sent `Authorization: Bearer ` with nothing after it.
//
// # Incident 2 — a stale token
//
// A token pasted from terminal scrollback hashed to 7d049c08…, while the single device
// row held 7e03881d…. Traced end to end: token generated once, returned unmodified,
// hashed once, stored — verified by reading the row on a separate SQLite connection and
// computing SHA-256 with the standard library rather than the agent's own helper
// (pairtrace_test.go). Re-run atomically on the same host with nothing copied by hand,
// pairing and authentication agreed and returned 200. The pasted token was simply not
// the one that pairing had issued.
//
// # What was actually wrong, and was fixed
//
// The diagnostics, not the authentication. Two things sent both investigations to the
// wrong layer:
//
//   - "A valid device token is required" was returned identically for a missing
//     credential and a rejected one, so an empty shell variable was indistinguishable
//     from a bad token.
//   - Nothing tied a device to its token in the log, so "wrong token" and "broken
//     lookup" could not be separated without opening the database by hand. The
//     token fingerprint now logged at pairing and at rejection makes that one line.
//
// The security boundary is unchanged. Why a *presented* token failed stays vague, because
// distinguishing unknown from revoked helps an attacker enumerate. Whether a credential
// was presented at all is something the caller already knows about its own request.

// TestMissingAndRejectedCredentialsAreDistinguishable is the test that fails before the
// fix: both cases produced byte-identical bodies.
func TestMissingAndRejectedCredentialsAreDistinguishable(t *testing.T) {
	env := newTestEnv(t)

	noHeader := decode[map[string]any](t,
		env.do(t, http.MethodGet, "/v1/devices", "", nil))
	rejected := decode[map[string]any](t,
		env.do(t, http.MethodGet, "/v1/devices", "csk_definitely-not-a-real-token", nil))

	missingDetail, _ := noHeader["detail"].(string)
	rejectedDetail, _ := rejected["detail"].(string)

	if missingDetail == "" || rejectedDetail == "" {
		t.Fatalf("a 401 carried no detail: missing=%q rejected=%q", missingDetail, rejectedDetail)
	}
	if missingDetail == rejectedDetail {
		t.Errorf("missing and rejected credentials produce the same message (%q); "+
			"an operator cannot tell an empty variable from a bad token", missingDetail)
	}

	// The missing case must say so plainly enough to act on.
	if !strings.Contains(strings.ToLower(missingDetail), "no bearer token") {
		t.Errorf("missing-credential detail does not say nothing was sent: %q", missingDetail)
	}

	// The rejected case must NOT explain why. Unknown, revoked and malformed stay
	// indistinguishable — that is the distinction worth protecting.
	for _, leak := range []string{"revoked", "expired", "unknown", "not found", "malformed"} {
		if strings.Contains(strings.ToLower(rejectedDetail), leak) {
			t.Errorf("rejected-token detail leaks why it failed (%q): %q", leak, rejectedDetail)
		}
	}
}

// TestEmptyBearerCredentialIsReportedAsMissing reproduces the exact wire shape that caused
// the incident: the header is present, the scheme is right, and there is no token.
func TestEmptyBearerCredentialIsReportedAsMissing(t *testing.T) {
	env := newTestEnv(t)

	for name, header := range map[string]string{
		"empty after scheme": "Bearer ",
		"scheme only":        "Bearer",
		"whitespace only":    "Bearer    ",
	} {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
				env.server.URL+"/v1/devices", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Authorization", header)

			resp, err := env.server.Client().Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			problem := decode[map[string]any](t, resp)
			detail, _ := problem["detail"].(string)
			if !strings.Contains(strings.ToLower(detail), "no bearer token") {
				t.Errorf("detail = %q; an empty credential must be reported as missing, "+
					"not as a rejected token", detail)
			}
		})
	}
}

// TestUnauthorizedCarriesWWWAuthenticate: RFC 7235 requires it on a 401, and a generic
// client should be told which scheme to use rather than inferring it from prose.
func TestUnauthorizedCarriesWWWAuthenticate(t *testing.T) {
	env := newTestEnv(t)

	for name, token := range map[string]string{
		"no credentials": "",
		"rejected token": "csk_nope",
	} {
		t.Run(name, func(t *testing.T) {
			resp := env.do(t, http.MethodGet, "/v1/devices", token, nil)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			if got := resp.Header.Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") {
				t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", got)
			}
		})
	}

	// Only on 401. A 403 means we know who you are and the answer is still no, so
	// re-authenticating would be the wrong advice.
	token := env.pairDevice(t, "Read Only", domain.ScopeRead)
	resp := env.do(t, http.MethodPost, "/v1/services/jellyfin/actions/restart", token, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got != "" {
		t.Errorf("403 carries WWW-Authenticate %q; it invites a pointless retry", got)
	}
}

// TestFreshTokenAuthenticatesImmediately pins the property the incident was mistakenly
// attributed to. A token is usable the instant pairing returns it — including on the
// stream, which was the endpoint under suspicion.
func TestFreshTokenAuthenticatesImmediately(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	for _, path := range []string{"/v1/system", "/v1/devices", "/v1/services"} {
		resp := env.do(t, http.MethodGet, path, token, nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s with a freshly minted token: status = %d, want 200",
				path, resp.StatusCode)
		}
	}

	// The stream is opened separately because it does not return.
	frames, cancel := openStream(t, env, token)
	defer cancel()
	nextFrame(t, frames, "the snapshot on a freshly minted token")
}

// TestRedeemedPairingCodeCannotMintASecondToken documents the behaviour that produced the
// empty token: correct, and worth pinning so the incident is not re-diagnosed later as a
// pairing fault.
func TestRedeemedPairingCodeCannotMintASecondToken(t *testing.T) {
	env := newTestEnv(t)

	code, err := env.store.CreatePairingCode(t.Context(),
		[]domain.Scope{domain.ScopeRead}, defaultActionRetention)
	if err != nil {
		t.Fatalf("CreatePairingCode: %v", err)
	}
	body := map[string]string{"code": code, "device_name": "Phone", "platform": "cli"}

	first := env.do(t, http.MethodPost, "/v1/pair", "", body)
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first pair: status = %d, want 201", first.StatusCode)
	}
	issued := decode[struct {
		Token string `json:"token"`
	}](t, first)
	if issued.Token == "" {
		t.Fatal("first pair returned no token")
	}

	second := env.do(t, http.MethodPost, "/v1/pair", "", body)
	if second.StatusCode != http.StatusForbidden {
		t.Errorf("second pair: status = %d, want 403", second.StatusCode)
	}
	// The second response carries no token — which is what an extraction script writes
	// into an empty variable, and then sends as an empty bearer credential.
	reused := decode[map[string]any](t, second)
	if _, hasToken := reused["token"]; hasToken {
		t.Error("a rejected pairing attempt returned a token field")
	}

	// The token from the first, successful call still works.
	if resp := env.do(t, http.MethodGet, "/v1/system", issued.Token, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("the originally issued token stopped working: status = %d", resp.StatusCode)
	}
}
