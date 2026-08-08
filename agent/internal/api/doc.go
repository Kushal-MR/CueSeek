// Package api is the transport layer: HTTP handlers, the SSE stream, token
// authentication and scope enforcement.
//
// It is the only package that knows about HTTP. Everything below it deals in domain
// types, so the same health and adapter logic could be served over a different
// protocol without change.
//
// Handlers are thin. Their job is to authenticate, authorise, translate to and from
// domain types, and hand off. Anything resembling a decision belongs in health,
// adapters or host — if a handler starts branching on service behaviour, the logic
// is in the wrong package.
//
// Server interfaces here are GENERATED from api/openapi.yaml (ADR-0004). Do not
// hand-edit generated files, and do not add an endpoint to this package before it
// exists in the spec — CI enforces that the two agree.
//
// Two constraints this package owns:
//
//   - Scope enforcement is not optional and not the client's job. A request bearing a
//     token without host.power must be rejected here, regardless of what UI produced
//     it. Client-side confirmation prompts are user experience; this is the control.
//
//   - Destructive host actions must acknowledge before they execute. A reboot handler
//     that performs the reboot and then writes a response never writes the response —
//     the machine is gone. Return 202 with an action id, then act after a short delay.
package api
