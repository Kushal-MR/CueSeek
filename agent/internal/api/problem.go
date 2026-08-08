package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

// Error is an API failure that knows how to describe itself as an RFC 9457 problem
// document.
//
// One error shape across the whole API means each client writes one error mapper instead
// of one per endpoint. The contract declares application/problem+json for every failure
// response, so this type is what satisfies it.
type Error struct {
	Status int
	Type   string
	Title  string
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%d %s", e.Status, e.Title)
	}
	return fmt.Sprintf("%d %s: %s", e.Status, e.Title, e.Detail)
}

const problemBase = "https://cueseek.dev/problems/"

// The error vocabulary. Detail is deliberately vague on the authentication and pairing
// paths: a caller who learns that a token "expired" rather than "was never valid" has
// learned something about the token space, and a caller who learns that a pairing code
// was "already used" has learned that it was once real.
var (
	errUnauthorized = &Error{
		Status: http.StatusUnauthorized,
		Type:   problemBase + "unauthorized",
		Title:  "Unauthorized",
		Detail: "A valid device token is required.",
	}
	errForbidden = &Error{
		Status: http.StatusForbidden,
		Type:   problemBase + "insufficient-scope",
		Title:  "Insufficient scope",
	}
	errInvalidPairingCode = &Error{
		Status: http.StatusForbidden,
		Type:   problemBase + "invalid-pairing-code",
		Title:  "Invalid pairing code",
		Detail: "The pairing code is not valid.",
	}
	errRateLimited = &Error{
		Status: http.StatusTooManyRequests,
		Type:   problemBase + "rate-limited",
		Title:  "Too many requests",
		Detail: "Too many pairing attempts. Wait and try again.",
	}
	errNotFound = &Error{
		Status: http.StatusNotFound,
		Type:   problemBase + "not-found",
		Title:  "Not found",
	}
	// errActionInProgress is the contract's 409 for invokeServiceAction: "not currently
	// possible, e.g. already in progress". Queuing a second restart behind the first is
	// never what somebody tapping twice wanted.
	errActionInProgress = &Error{
		Status: http.StatusConflict,
		Type:   problemBase + "action-in-progress",
		Title:  "Action already in progress",
	}
	// errActionUnavailable covers a host that cannot perform the action — an unlisted
	// unit, a missing polkit grant, an unsupported platform.
	//
	// 409 rather than 500, because every one of those is a configuration problem an
	// operator can fix, and a bare 500 would send them looking for a bug instead. Not
	// 403: the caller's token was fine, the agent's own permissions were not.
	errActionUnavailable = &Error{
		Status: http.StatusConflict,
		Type:   problemBase + "action-unavailable",
		Title:  "Action not possible on this host",
	}
	errInternal = &Error{
		Status: http.StatusInternalServerError,
		Type:   problemBase + "internal",
		Title:  "Internal error",
		Detail: "The agent failed to complete the request.",
	}
	// errNotImplemented covers endpoints declared in the contract but not yet built.
	//
	// 503 rather than 501 because the contract does not declare 501 for any operation,
	// and adding one would bake a temporary implementation state into a permanent
	// contract that four client platforms must handle forever.
	errNotImplemented = &Error{
		Status: http.StatusServiceUnavailable,
		Type:   problemBase + "not-implemented",
		Title:  "Not implemented",
		Detail: "This endpoint is declared in the contract but not yet implemented.",
	}
)

// withDetail returns a copy carrying a specific detail, leaving the shared value alone.
// The vars above are package-level pointers; mutating one would corrupt every future
// response that used it.
func (e *Error) withDetail(format string, args ...any) *Error {
	clone := *e
	clone.Detail = fmt.Sprintf(format, args...)
	return &clone
}

// writeProblem renders err as an RFC 9457 document.
//
// Anything that is not an *Error becomes a generic 500 and its real text goes to the log
// instead of the response. Internal failures — a SQL error, a nil dereference — routinely
// name tables, paths and query structure, and a client is the wrong audience for that.
func writeProblem(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		slog.ErrorContext(r.Context(), "unhandled error",
			"method", r.Method, "path", r.URL.Path, "error", err)
		apiErr = errInternal
	}

	body := map[string]any{
		"type":     apiErr.Type,
		"title":    apiErr.Title,
		"status":   apiErr.Status,
		"instance": r.URL.Path,
	}
	if apiErr.Detail != "" {
		body["detail"] = apiErr.Detail
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(apiErr.Status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.ErrorContext(r.Context(), "write problem response", "error", err)
	}
}

// writeBadRequest renders a malformed-request failure.
//
// Wired to the generated RequestErrorHandlerFunc, which fires when path or query
// parameters fail to parse — before any handler runs. Its default writes plain text,
// which would be the one response in the API not shaped like every other.
func writeBadRequest(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, r, &Error{
		Status: http.StatusBadRequest,
		Type:   problemBase + "bad-request",
		Title:  "Bad request",
		Detail: err.Error(),
	})
}
