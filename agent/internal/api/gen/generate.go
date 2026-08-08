// Package gen contains code generated from api/openapi.yaml.
//
// DO NOT EDIT anything in this package except this file. The contents of gen.go are
// regenerated from the contract, and CI fails if the committed output differs from a
// fresh run (ADR-0009). Hand-edits are silently reverted by the next regeneration.
//
// Regenerate:
//
//	cd agent && go generate ./...
//
// Nothing outside internal/api should import this package directly. Generated types
// carry the generator's idioms — pointers for optional fields, its own error handling —
// and letting those reach the rest of the agent would make replacing the generator a
// rewrite instead of a swap.
package gen

//go:generate go tool oapi-codegen -config cfg.yaml ../../../../api/openapi.yaml
