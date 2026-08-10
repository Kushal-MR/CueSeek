package dev.cueseek.core.model

import java.time.Instant

/**
 * Agent and host identity, from `GET /v1/system`.
 *
 * [apiVersion] is what makes an honest version-skew message possible: a client can say
 * which side is behind rather than silently mis-rendering what it does not understand
 * (ADR-0007).
 */
data class SystemInfo(
    val hostId: HostId,
    val hostname: String,
    val agentVersion: String,
    val apiVersion: String,
    val startedAt: Instant,
)
