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

/**
 * Acknowledgement of a host power action.
 *
 * Deliberately not [ActionAcceptance], which carries a `serviceId`: a machine is not one of
 * its own services, and a sentinel there would be a small lie told on every power action
 * forever.
 */
data class HostActionAcceptance(
    val actionId: ActionInvocationId,
    val action: String,
    val status: ActionStatus,
    val acceptedAt: Instant,
)

/**
 * The outcome of a host power action, which in practice means **the failure** of one.
 *
 * A power action that worked took the stream, the agent and the machine with it, so nothing
 * can be delivered about it. Receiving this at all is the news: the machine is still here
 * and the button did nothing.
 */
data class HostActionOutcome(
    val actionId: ActionInvocationId,
    val action: String,
    val status: ActionStatus,
    val at: Instant,
    /** Present when [status] is [ActionStatus.Failed]. */
    val error: String?,
)
