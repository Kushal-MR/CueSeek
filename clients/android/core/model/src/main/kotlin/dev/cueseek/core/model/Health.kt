package dev.cueseek.core.model

import java.time.Instant

/**
 * A service's health, as a closed set.
 *
 * [Unknown] is a real state and not an error: before the first poll, when cached state has
 * aged past tolerance, and — the case that matters most on a phone — when the event stream
 * has gone silent while still claiming to be connected. It renders as "I don't know", never
 * as healthy. Showing stale green is worse than showing nothing, because it is confidently
 * wrong (ADR-0008).
 *
 * An unrecognised value from a newer agent also maps here, which is the honest answer:
 * the client genuinely does not know what the agent meant.
 */
enum class HealthStatus(val wire: String) {
    Healthy("healthy"),
    Degraded("degraded"),
    Unreachable("unreachable"),
    Unknown("unknown"),
    ;

    companion object {
        fun fromWire(value: String): HealthStatus =
            entries.firstOrNull { it.wire == value } ?: Unknown
    }
}

/**
 * Why a service is in its current state.
 *
 * [code] is stable and safe to branch on; [message] is prose and is not. Codes emitted
 * today: `not_polled`, `stale`, `unreachable`, `timeout`, `auth_failed`, `upstream_error`,
 * `invalid_response`, `shutting_down`, `pending_restart`.
 *
 * A reason is not necessarily a problem — a healthy service can carry `pending_restart`.
 */
data class HealthReason(
    val code: String,
    val message: String,
)

/**
 * Health as observed by the agent.
 *
 * [status] and [reachable] are separate facts and conflating them sends the user to the
 * wrong place. `Degraded` with `reachable = true` means the service answered and something
 * is wrong — a rejected API key, say. `Unreachable` means no contact at all.
 */
data class Health(
    val status: HealthStatus,
    val reachable: Boolean,
    /** What the service says about itself, verbatim. Absent for services that say nothing. */
    val reportedStatus: String?,
    val reasons: List<HealthReason>,
    /**
     * When this was **observed**, not when it was served. Staleness is rendered from this
     * rather than from arrival time — the agent serves cached state by design, so arrival
     * time says nothing about how current it is.
     */
    val observedAt: Instant,
)
