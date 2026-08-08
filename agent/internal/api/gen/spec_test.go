package gen

import (
	"strings"
	"testing"
)

// The contract is validated in Go rather than by an external linter so that CI needs no
// Node toolchain, and — more importantly — so it is checked by the same parser the
// generator uses. A spec that lints clean under one parser and fails under another is
// not actually validated.

func TestSpecIsValid(t *testing.T) {
	spec, err := GetSwagger()
	if err != nil {
		t.Fatalf("embedded spec failed to load: %v", err)
	}
	if err := spec.Validate(t.Context()); err != nil {
		t.Fatalf("spec is not valid OpenAPI: %v", err)
	}
}

// TestSpecIsOpenAPI30 guards a deliberate deviation from ADR-0004, which specifies
// OpenAPI 3.1.
//
// oapi-codegen v2.4.1 does not support 3.1. Pointed at a 3.1 document it prints a
// warning and continues with reduced functionality rather than failing — so a future
// version bump would degrade generation quietly, and the first symptom would be missing
// types rather than a build error. This test converts that silent degradation into a
// loud one.
//
// Remove it, and update ADR-0004, when the generator supports 3.1:
// https://github.com/oapi-codegen/oapi-codegen/issues/373
func TestSpecIsOpenAPI30(t *testing.T) {
	spec, err := GetSwagger()
	if err != nil {
		t.Fatalf("embedded spec failed to load: %v", err)
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.0") {
		t.Fatalf("spec declares OpenAPI %q; oapi-codegen v2.x only supports 3.0.x. "+
			"Bumping this silently degrades codegen — see docs/adr/0004-contract-openapi-sse.md",
			spec.OpenAPI)
	}
}

// TestEveryOperationDeclaresScope enforces ADR-0006 at build time.
//
// Scopes are the agent's actual authorisation control — ADR-0001 delegated transport
// security to the VPN, so nothing else stands between a tailnet and this API. An
// operation that forgets to declare its required scope is not a documentation gap; it is
// an endpoint whose authorisation nobody decided. Catching that in CI is far cheaper
// than catching it in review.
//
// Unauthenticated operations opt out explicitly by declaring empty security, which makes
// the exemption visible in the contract rather than implied by omission.
func TestEveryOperationDeclaresScope(t *testing.T) {
	spec, err := GetSwagger()
	if err != nil {
		t.Fatalf("embedded spec failed to load: %v", err)
	}

	checked := 0
	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			// Explicitly unauthenticated (e.g. POST /v1/pair): security is present and
			// empty, overriding the document-level requirement.
			if op.Security != nil && len(*op.Security) == 0 {
				continue
			}
			checked++
			if _, ok := op.Extensions["x-required-scope"]; !ok {
				t.Errorf("%s %s (operationId %q) declares no x-required-scope",
					method, path, op.OperationID)
			}
		}
	}

	// Without this, the test passes trivially if the spec fails to load its paths, or if
	// a future kin-openapi changes how extensions are exposed. A guard that can pass
	// while inspecting nothing is worse than no guard, because it reads as coverage.
	if checked < 6 {
		t.Fatalf("only %d authenticated operations inspected; the spec defines more, "+
			"so this test is not actually checking anything", checked)
	}
}

// TestEveryOperationHasOperationID fails fast on a missing operationId, which would
// otherwise produce a generated method name derived from the path — stable-looking but
// prone to changing under refactors, and therefore a source of silent breaking changes
// in every generated client.
func TestEveryOperationHasOperationID(t *testing.T) {
	spec, err := GetSwagger()
	if err != nil {
		t.Fatalf("embedded spec failed to load: %v", err)
	}

	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if op.OperationID == "" {
				t.Errorf("%s %s has no operationId", method, path)
			}
		}
	}
}
