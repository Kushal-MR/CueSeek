/**
 * The domain vocabulary the UI actually speaks: hosts, services, capabilities, health,
 * actions and stream events, as sealed Kotlin types.
 *
 * Nothing generated from `api/openapi.yaml` appears here, and nothing here imports the
 * Android framework — this is a plain Kotlin/JVM module so that both constraints are
 * enforced by the compiler rather than by review. `:core:api` maps generated wire types
 * onto these; no ViewModel ever sees a generated type (ADR-0009).
 *
 * Populated in M2 phase P1.
 */
package dev.cueseek.core.model
