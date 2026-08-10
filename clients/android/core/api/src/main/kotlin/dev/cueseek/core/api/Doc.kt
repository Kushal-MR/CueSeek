/**
 * Transport against the CueSeek agent: the eight REST operations, the hand-written SSE
 * reader, and the RFC 9457 `application/problem+json` error mapping.
 *
 * Per ADR-0013, wire **types** are generated from `api/openapi.yaml` and the **calls**
 * are hand-written. Generated types stay internal to this module; the surface it exposes
 * is `:core:model` types and a `Result`-style error model. No OpenAPI generator models
 * `text/event-stream`, so `GET /v1/stream` is read directly from OkHttp.
 *
 * Populated in M2 phases P1 and P3.
 */
package dev.cueseek.core.api
