package dev.cueseek.core.model

import java.time.Instant

/**
 * Where an invocation has got to.
 *
 * The agent publishes only [Succeeded] and [Failed] in progress events; [Pending] and
 * [Running] exist in the contract and [Running] is what every acceptance reports, but no
 * progress event carries them today.
 */
enum class ActionStatus(val wire: String) {
    Pending("pending"),
    Running("running"),
    Succeeded("succeeded"),
    Failed("failed"),
    Unrecognised(""),
    ;

    val isTerminal: Boolean
        get() = this == Succeeded || this == Failed

    companion object {
        fun fromWire(value: String): ActionStatus =
            entries.firstOrNull { it != Unrecognised && it.wire == value } ?: Unrecognised
    }
}

/**
 * The agent's acknowledgement that an action was accepted.
 *
 * Not its outcome. `RestartUnit` returns once systemd has *queued* the job, so there is no
 * synchronous result to report even in principle. The outcome arrives later as a single
 * `action_progress` event carrying the same [actionId] — and there is no endpoint to ask
 * for it, so a client that is not streaming when it arrives has lost it.
 */
data class ActionAcceptance(
    val actionId: ActionInvocationId,
    val serviceId: String,
    val action: String,
    val status: ActionStatus,
    val acceptedAt: Instant,
)

/** The outcome of an invocation, delivered over the stream. */
data class ActionProgress(
    val actionId: ActionInvocationId,
    val serviceId: String,
    val action: String,
    val status: ActionStatus,
    val at: Instant,
    /** Present when [status] is [ActionStatus.Failed]. */
    val error: String?,
)
