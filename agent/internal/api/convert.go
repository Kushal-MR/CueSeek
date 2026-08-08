package api

import (
	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// This file is the only place domain types meet generated types.
//
// ADR-0009 requires generated code to be wrapped rather than consumed directly, and the
// package comment in internal/api/gen states that nothing outside internal/api may import
// it. Concentrating the translation here means a change of generator touches one file
// instead of every handler — and it keeps generated idioms, like pointers for every
// optional field, from spreading inward.

func toGenDevice(d domain.Device) gen.Device {
	return gen.Device{
		Id:         d.ID,
		Name:       d.Name,
		Platform:   gen.Platform(d.Platform),
		Scopes:     toGenScopes(d.Scopes),
		CreatedAt:  d.CreatedAt,
		LastSeenAt: d.LastSeenAt,
	}
}

func toGenDevices(devices []domain.Device) []gen.Device {
	// Non-nil even when empty: a nil slice marshals to JSON `null`, an empty one to
	// `[]`. Clients should not have to handle both for "no devices".
	out := make([]gen.Device, 0, len(devices))
	for _, d := range devices {
		out = append(out, toGenDevice(d))
	}
	return out
}

func toGenScopes(scopes []domain.Scope) []gen.Scope {
	out := make([]gen.Scope, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, gen.Scope(s))
	}
	return out
}

// ---------------------------------------------------------------- services

func toGenHealth(h domain.Health) gen.Health {
	out := gen.Health{
		Status:     gen.HealthStatus(h.Status),
		Reachable:  h.Reachable,
		ObservedAt: h.ObservedAt,
		Reasons:    toGenReasons(h.Reasons),
	}
	// Absent rather than empty when the service reports nothing about itself. The
	// contract defines this field as verbatim and unmapped, and an empty string would
	// read as "the service said nothing" rather than "the service says nothing".
	if h.ReportedStatus != "" {
		reported := h.ReportedStatus
		out.ReportedStatus = &reported
	}
	return out
}

func toGenReasons(reasons []domain.HealthReason) []gen.HealthReason {
	out := make([]gen.HealthReason, 0, len(reasons))
	for _, r := range reasons {
		out = append(out, gen.HealthReason{Code: r.Code, Message: r.Message})
	}
	return out
}

func toGenCapabilities(caps []domain.Capability) []gen.Capability {
	out := make([]gen.Capability, 0, len(caps))
	for _, c := range caps {
		out = append(out, gen.Capability{Id: c.ID, Label: c.Label})
	}
	return out
}

func toGenActions(actions []domain.Action) []gen.Action {
	out := make([]gen.Action, 0, len(actions))
	for _, a := range actions {
		action := gen.Action{
			Id:    a.ID,
			Label: a.Label,
			Risk:  gen.ActionRisk(a.Risk),
		}
		if a.Description != "" {
			description := a.Description
			action.Description = &description
		}
		out = append(out, action)
	}
	return out
}
