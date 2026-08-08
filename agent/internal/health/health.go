package health

import (
	"fmt"
	"sort"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// This package depends on nothing but domain and the standard library. No HTTP, no
// adapters, no database. Aggregation is a policy decision, and policy that can only be
// exercised through a web server is policy nobody tests properly.

// ServiceHealth is one service's contribution to the overall picture.
type ServiceHealth struct {
	ServiceID string
	Name      string
	Health    domain.Health
}

// Overall derives host-wide status from the state of every managed service.
//
// The agent computes this rather than each client (ADR-0008). If the phone and the watch
// each decided what counted as degraded, they would eventually disagree about the same
// machine, and the operator would believe whichever screen they were holding.
//
// The rules, in order:
//
//  1. No services configured — unknown. The agent has observed nothing, so it knows
//     nothing. Reporting healthy would be a claim it has no basis for.
//  2. Every service unknown — unknown. Typically the agent has just started and no poll
//     has finished.
//  3. Every service unreachable — unreachable. When nothing at all can be contacted the
//     likely cause is the host or the network, not several services failing at once.
//  4. Anything less than healthy — degraded. One unreachable service on an otherwise
//     working box is a degraded box, not an unreachable one: the machine is fine and one
//     thing on it is not.
//  5. Otherwise — healthy.
//
// Reasons name the services responsible, because "degraded" alone is not actionable and
// "degraded: Jellyfin unreachable" is.
func Overall(services []ServiceHealth, at time.Time) domain.Health {
	at = at.UTC()

	if len(services) == 0 {
		return domain.Health{
			Status:     domain.StatusUnknown,
			Reachable:  false,
			ObservedAt: at,
			Reasons: []domain.HealthReason{{
				Code:    "no_services",
				Message: "No services are configured, so there is nothing to report on.",
			}},
		}
	}

	counts := map[domain.HealthStatus]int{}
	var problems []ServiceHealth
	for _, s := range services {
		status := s.Health.Status
		if !status.Valid() {
			// An unrecognised status is not evidence of health. Treating it as unknown
			// keeps a buggy adapter from colouring the whole host green.
			status = domain.StatusUnknown
		}
		counts[status]++
		if status != domain.StatusHealthy {
			problems = append(problems, s)
		}
	}

	// Deterministic reason order, so identical state produces an identical response and
	// clients do not see spurious changes.
	sort.Slice(problems, func(i, j int) bool { return problems[i].ServiceID < problems[j].ServiceID })

	total := len(services)
	switch {
	case counts[domain.StatusUnknown] == total:
		return domain.Health{
			Status:     domain.StatusUnknown,
			Reachable:  false,
			ObservedAt: at,
			Reasons:    reasonsFor(problems),
		}

	case counts[domain.StatusUnreachable] == total:
		return domain.Health{
			Status:     domain.StatusUnreachable,
			Reachable:  false,
			ObservedAt: at,
			Reasons:    reasonsFor(problems),
		}

	case len(problems) > 0:
		return domain.Health{
			Status: domain.StatusDegraded,
			// Reachable describes the agent's own view of the host, and the agent is
			// answering: at least one service responded.
			Reachable:  counts[domain.StatusHealthy] > 0 || counts[domain.StatusDegraded] > 0,
			ObservedAt: at,
			Reasons:    reasonsFor(problems),
		}

	default:
		return domain.Health{
			Status:     domain.StatusHealthy,
			Reachable:  true,
			ObservedAt: at,
			Reasons:    []domain.HealthReason{},
		}
	}
}

// reasonsFor names each service that is not healthy, and why.
//
// The service's own reason is carried through rather than restated, so the operator sees
// "Jellyfin: Jellyfin rejected the API key" instead of a generic "a service is degraded"
// that requires another click to explain.
func reasonsFor(problems []ServiceHealth) []domain.HealthReason {
	reasons := make([]domain.HealthReason, 0, len(problems))
	for _, s := range problems {
		name := s.Name
		if name == "" {
			name = s.ServiceID
		}

		detail := string(s.Health.Status)
		if len(s.Health.Reasons) > 0 {
			detail = s.Health.Reasons[0].Message
		}
		reasons = append(reasons, domain.HealthReason{
			// Namespaced by service so a client can attribute a reason without parsing
			// the message.
			Code:    fmt.Sprintf("service.%s.%s", s.ServiceID, s.Health.Status),
			Message: fmt.Sprintf("%s: %s", name, detail),
		})
	}
	return reasons
}
