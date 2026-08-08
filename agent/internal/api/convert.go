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
