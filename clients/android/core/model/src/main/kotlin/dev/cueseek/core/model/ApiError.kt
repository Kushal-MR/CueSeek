package dev.cueseek.core.model

/**
 * Everything that can go wrong talking to an agent.
 *
 * The agent answers every failure with one shape — RFC 9457 `application/problem+json` —
 * and identifies the kind of failure in `type`. **Branch on the case, never on
 * [detail]:** detail strings are human-facing prose and may be reworded without notice.
 *
 * Cases here correspond to problem types under `https://cueseek.dev/problems/`, plus the
 * two failures that never reach the agent at all ([Transport], [Malformed]).
 */
sealed interface ApiError {

    /** The human-facing explanation the agent supplied, if any. Often absent. */
    val detail: String?

    /**
     * No usable credential.
     *
     * The agent distinguishes "no bearer token was presented" from "the token was not
     * accepted", but only in prose, and [detail] is exactly what must not be branched on.
     * [tokenWasSent] carries the same distinction from a source that cannot be reworded:
     * the client knows whether it attached one.
     *
     * `false` is a client bug — an empty variable, a header that never got attached.
     * `true` means the token is genuinely dead and the user must pair again. *Why* it was
     * rejected, unknown or revoked, is deliberately not disclosed by the agent.
     */
    data class Unauthorized(
        val tokenWasSent: Boolean,
        override val detail: String?,
    ) : ApiError

    /** Authenticated, but the device was not granted the scope this operation needs. */
    data class InsufficientScope(override val detail: String?) : ApiError

    /**
     * The pairing code was unknown, expired, or already redeemed.
     *
     * Which of the three is **not** disclosed, on purpose: telling a caller a code
     * "expired" reveals that it was once real. Do not build UI that claims to know.
     */
    data class InvalidPairingCode(override val detail: String?) : ApiError

    /** More than ten pairing attempts from this address in a minute. */
    data class RateLimited(override val detail: String?) : ApiError

    /** No such service, action or device. */
    data class NotFound(override val detail: String?) : ApiError

    /** That action is already running on that service. */
    data class ActionInProgress(override val detail: String?) : ApiError

    /**
     * The host cannot perform the action — the unit is not in the allowlist, polkit
     * refused, the unit is missing, or the platform does not support it.
     *
     * This one deserves its own treatment in the UI. It means the *agent* is not permitted
     * or able, not that the user did anything wrong, and [detail] says which. It is worth
     * surfacing verbatim to an operator: it is usually a polkit rule that does not name
     * the unit, which is fixable in one line on the server.
     */
    data class ActionUnavailable(override val detail: String?) : ApiError

    /** The request was malformed. A client bug. */
    data class BadRequest(override val detail: String?) : ApiError

    /** The agent faulted. Its detail is deliberately generic. */
    data class Internal(override val detail: String?) : ApiError

    /**
     * Currently reachable in exactly one place: the agent was shutting down when a stream
     * request arrived. It is not a permanent property of any endpoint, so it warrants a
     * retry rather than a dead end.
     */
    data class NotImplemented(override val detail: String?) : ApiError

    /**
     * A problem type this build has never heard of.
     *
     * Not a defect — the agent is allowed to grow new ones, and will (ADR-0007). The raw
     * [type] is kept so a bug report can name it.
     */
    data class Unrecognised(
        val type: String,
        val title: String,
        val status: Int,
        override val detail: String?,
    ) : ApiError

    /**
     * The request never got an answer: the host is unreachable, the tailnet is down, or
     * the connection timed out.
     *
     * Overwhelmingly the common failure on a phone, and the one whose message should
     * mention Tailscale — there is no relay and no fallback path, so an unreachable host
     * is unreachable, not slow.
     */
    data class Transport(
        val cause: Throwable,
        override val detail: String? = null,
    ) : ApiError

    /**
     * A response arrived and could not be understood.
     *
     * Usually version skew: an agent newer than this build sent a shape it does not know.
     * Worth pairing with the `api_version` comparison so the user is told which side is
     * behind rather than being shown a parse error.
     */
    data class Malformed(
        val cause: Throwable,
        override val detail: String? = null,
    ) : ApiError
}
