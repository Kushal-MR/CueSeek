package dev.cueseek.core.model

import java.time.Instant

/**
 * One event from `GET /v1/stream`.
 *
 * The agent never sends an SSE `id:` field, deliberately: an id would invite clients to
 * send `Last-Event-ID` on reconnect, implying a replay buffer that does not exist.
 * Reconnecting yields a fresh [Snapshot] instead.
 */
sealed interface StreamEvent {

    /**
     * Always the first event on every connection, at `seq` 0.
     *
     * Replace local state wholesale with this. Merging it into what was there before
     * assumes the two describe the same moment, and after a reconnect they do not.
     */
    data class Snapshot(
        val system: SystemInfo,
        val services: List<Service>,
        /**
         * The host's vitals as of the agent's last collection.
         *
         * Null in the first seconds after an agent restart, before anything has been
         * measured, and for the whole session on a platform that cannot read them. There
         * is no replay buffer, so whatever the snapshot omits is simply absent until the
         * next [HostUpdated] arrives.
         */
        val hostMetrics: HostMetrics? = null,
        /**
         * Power actions the agent offers for the machine.
         *
         * Static for the life of the agent process, so it rides the snapshot rather than
         * needing an event of its own. Empty on a platform that cannot perform them, which
         * a client renders as no buttons rather than buttons that fail.
         */
        val hostActions: List<Action> = emptyList(),
    ) : StreamEvent

    /**
     * One complete service, not a diff.
     *
     * Emitted after **every** poll, whether or not anything changed, so consecutive
     * identical events differing only in `observed_at` are normal. Replace by id.
     */
    data class ServiceUpdated(val service: Service) : StreamEvent

    /**
     * A fresh measurement of the machine itself. Replace wholesale.
     *
     * Emitted on the agent's own faster ticker rather than with service updates, because
     * a CPU figure averaged over a service poll interval hides exactly the spike worth
     * seeing.
     */
    data class HostUpdated(val metrics: HostMetrics) : StreamEvent

    /** The outcome of an invocation, correlated by [ActionProgress.actionId]. */
    data class ActionOutcome(val progress: ActionProgress) : StreamEvent

    /**
     * A host power action failed, which means the machine is still running.
     *
     * There is no success counterpart and there cannot be: a reboot that worked ends the
     * stream carrying the news. Silence after a power action is the good outcome.
     */
    data class HostActionFailed(val outcome: HostActionOutcome) : StreamEvent

    /**
     * Carries no payload; its existence is the message.
     *
     * Emitted on a fixed 15-second ticker that is *not* reset by other events, so
     * heartbeats interleave with updates rather than appearing only during silence. This
     * is what makes silence unambiguous, and silence is the only reliable signal a client
     * has — see [dev.cueseek.core.model.Freshness].
     */
    data object Heartbeat : StreamEvent

    /**
     * An event type this build predates.
     *
     * Kept rather than dropped so that version skew is observable instead of looking like
     * a quiet stream (ADR-0007).
     */
    data class Unrecognised(val type: String) : StreamEvent
}

/**
 * A stream event with its transport metadata.
 *
 * [seq] is monotonic **per connection**, starting at 0, and is not a resume token — it
 * detects a gap within one connection and resets to 0 on every reconnect.
 */
data class StreamEnvelope(
    val seq: Long,
    val emittedAt: Instant,
    val schemaVersion: String,
    val event: StreamEvent,
)

/**
 * Whether what is on screen can still be believed.
 *
 * Derived from a clock, never from the transport. This is the single most important
 * requirement in `docs/m2-android-api.md`, and it exists because of what A7 measured on
 * real hardware: with the screen off, the stream does not disconnect — it **freezes
 * silently** for up to 168 seconds while continuing to report itself as connected. A
 * console that trusted the connection would show a green dot for a service that died three
 * minutes earlier, which is exactly the "confidently wrong" state ADR-0008 exists to
 * prevent.
 */
sealed interface Freshness {

    data object Fresh : Freshness

    /**
     * Nothing has arrived for longer than the tolerance, whatever the transport claims.
     *
     * @param lastEventAt when anything was last received, or null if nothing ever was.
     */
    data class Stale(val lastEventAt: Instant?) : Freshness

    val isStale: Boolean get() = this is Stale
}

/**
 * What the transport is doing.
 *
 * Useful for telling the user that a reconnect is in progress. **Not** usable as evidence
 * that the data is current: [Open] is precisely the state a frozen stream reports.
 */
sealed interface StreamStatus {

    data object Idle : StreamStatus

    data object Connecting : StreamStatus

    data class Open(val since: Instant) : StreamStatus

    /** Disconnected, with a retry already scheduled. */
    data class Retrying(val error: ApiError, val attempt: Int) : StreamStatus

    /**
     * Given up, because retrying cannot help.
     *
     * Reached when the credential is rejected: the token is dead and no amount of
     * reconnecting will revive it. The user must pair the device again.
     */
    data class Stopped(val error: ApiError) : StreamStatus
}
