package health

import (
	"strings"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

func service(id string, status domain.HealthStatus, reason string) ServiceHealth {
	h := domain.Health{Status: status, Reachable: status == domain.StatusHealthy ||
		status == domain.StatusDegraded}
	if reason != "" {
		h.Reasons = []domain.HealthReason{{Code: "test", Message: reason}}
	}
	return ServiceHealth{ServiceID: id, Name: strings.ToUpper(id), Health: h}
}

func TestOverallAggregation(t *testing.T) {
	now := time.Now().UTC()

	cases := map[string]struct {
		services []ServiceHealth
		want     domain.HealthStatus
		why      string
	}{
		"nothing configured": {
			nil, domain.StatusUnknown,
			"the agent has observed nothing, so it knows nothing",
		},
		"all healthy": {
			[]ServiceHealth{
				service("a", domain.StatusHealthy, ""),
				service("b", domain.StatusHealthy, ""),
			},
			domain.StatusHealthy, "",
		},
		"all unknown": {
			[]ServiceHealth{
				service("a", domain.StatusUnknown, ""),
				service("b", domain.StatusUnknown, ""),
			},
			domain.StatusUnknown,
			"typically the agent has just started and no poll has finished",
		},
		"all unreachable": {
			[]ServiceHealth{
				service("a", domain.StatusUnreachable, ""),
				service("b", domain.StatusUnreachable, ""),
			},
			domain.StatusUnreachable,
			"when nothing can be contacted the cause is likely the host, not every service at once",
		},
		"one unreachable among healthy": {
			[]ServiceHealth{
				service("a", domain.StatusHealthy, ""),
				service("b", domain.StatusUnreachable, ""),
			},
			domain.StatusDegraded,
			"the machine is fine and one thing on it is not",
		},
		"one degraded": {
			[]ServiceHealth{
				service("a", domain.StatusHealthy, ""),
				service("b", domain.StatusDegraded, ""),
			},
			domain.StatusDegraded, "",
		},
		"one unknown among healthy": {
			[]ServiceHealth{
				service("a", domain.StatusHealthy, ""),
				service("b", domain.StatusUnknown, ""),
			},
			domain.StatusDegraded,
			"not knowing about one service means the overall picture is not fully healthy",
		},
		"mixed unknown and unreachable": {
			[]ServiceHealth{
				service("a", domain.StatusUnknown, ""),
				service("b", domain.StatusUnreachable, ""),
			},
			domain.StatusDegraded,
			"neither 'all unknown' nor 'all unreachable' applies",
		},
		"single healthy": {
			[]ServiceHealth{service("a", domain.StatusHealthy, "")},
			domain.StatusHealthy, "",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := Overall(tc.services, now)
			if got.Status != tc.want {
				t.Errorf("status = %q, want %q%s", got.Status, tc.want, hint(tc.why))
			}
			if !got.Status.Valid() {
				t.Errorf("status %q is outside the closed set", got.Status)
			}
			if !got.ObservedAt.Equal(now) {
				t.Errorf("ObservedAt = %v, want %v", got.ObservedAt, now)
			}
		})
	}
}

func hint(why string) string {
	if why == "" {
		return ""
	}
	return " — " + why
}

// TestReasonsNameTheResponsibleServices: "degraded" alone is not actionable; "degraded:
// Jellyfin rejected the API key" is (ADR-0008).
func TestReasonsNameTheResponsibleServices(t *testing.T) {
	overall := Overall([]ServiceHealth{
		service("jellyfin", domain.StatusHealthy, ""),
		service("qbittorrent", domain.StatusUnreachable, "connection refused"),
	}, time.Now())

	if len(overall.Reasons) != 1 {
		t.Fatalf("reasons = %v, want exactly the unhealthy service", overall.Reasons)
	}
	reason := overall.Reasons[0]

	// The service's own explanation is carried through rather than restated, so the
	// operator sees the cause without another request.
	if !strings.Contains(reason.Message, "connection refused") {
		t.Errorf("message loses the underlying reason: %q", reason.Message)
	}
	if !strings.Contains(reason.Message, "QBITTORRENT") {
		t.Errorf("message does not name the service: %q", reason.Message)
	}
	// Namespaced so a client can attribute it without parsing prose.
	if !strings.Contains(reason.Code, "qbittorrent") {
		t.Errorf("code does not identify the service: %q", reason.Code)
	}
	// The healthy service contributes no noise.
	if strings.Contains(reason.Code, "jellyfin") {
		t.Errorf("a healthy service produced a reason: %q", reason.Code)
	}
}

// TestReasonOrderIsDeterministic: identical state must produce an identical response, or
// clients see changes that did not happen.
func TestReasonOrderIsDeterministic(t *testing.T) {
	services := []ServiceHealth{
		service("zebra", domain.StatusUnreachable, "down"),
		service("alpha", domain.StatusDegraded, "slow"),
		service("middle", domain.StatusUnknown, "no data"),
	}

	first := Overall(services, time.Now())
	second := Overall(services, time.Now())

	if len(first.Reasons) != 3 {
		t.Fatalf("reasons = %v, want 3", first.Reasons)
	}
	for i := range first.Reasons {
		if first.Reasons[i].Code != second.Reasons[i].Code {
			t.Fatalf("order is not stable: %v vs %v", first.Reasons, second.Reasons)
		}
	}
	if !strings.Contains(first.Reasons[0].Code, "alpha") {
		t.Errorf("reasons are not sorted by service id: %v", first.Reasons)
	}
}

// TestInvalidStatusIsTreatedAsUnknown: an adapter bug must not colour the whole host
// green.
func TestInvalidStatusIsTreatedAsUnknown(t *testing.T) {
	overall := Overall([]ServiceHealth{
		{ServiceID: "broken", Health: domain.Health{Status: "totally-fine"}},
	}, time.Now())

	if overall.Status != domain.StatusUnknown {
		t.Errorf("status = %q, want unknown for an unrecognised value", overall.Status)
	}
}

func TestHealthyHasNoReasons(t *testing.T) {
	overall := Overall([]ServiceHealth{service("a", domain.StatusHealthy, "")}, time.Now())

	if len(overall.Reasons) != 0 {
		t.Errorf("reasons = %v, want none when everything is healthy", overall.Reasons)
	}
	// Non-nil so it serialises as [] rather than null.
	if overall.Reasons == nil {
		t.Error("Reasons is nil; it would marshal to null rather than []")
	}
	if !overall.Reachable {
		t.Error("reachable = false when every service is healthy")
	}
}

// TestServiceIDUsedWhenNameIsEmpty: a service with no display name must still be
// identifiable in a reason.
func TestServiceIDUsedWhenNameIsEmpty(t *testing.T) {
	overall := Overall([]ServiceHealth{{
		ServiceID: "jellyfin",
		Health: domain.Health{Status: domain.StatusUnreachable,
			Reasons: []domain.HealthReason{{Code: "x", Message: "refused"}}},
	}}, time.Now())

	if !strings.Contains(overall.Reasons[0].Message, "jellyfin") {
		t.Errorf("message = %q, want the service id as a fallback", overall.Reasons[0].Message)
	}
}

// TestStatusWithoutReasonStillExplains: an adapter that returns a bare status must not
// produce an empty explanation.
func TestStatusWithoutReasonStillExplains(t *testing.T) {
	overall := Overall([]ServiceHealth{{
		ServiceID: "x", Name: "X",
		Health: domain.Health{Status: domain.StatusDegraded},
	}}, time.Now())

	if len(overall.Reasons) != 1 || overall.Reasons[0].Message == "" {
		t.Fatalf("reasons = %v", overall.Reasons)
	}
	if !strings.Contains(overall.Reasons[0].Message, "degraded") {
		t.Errorf("message = %q, want it to fall back to the status", overall.Reasons[0].Message)
	}
}
