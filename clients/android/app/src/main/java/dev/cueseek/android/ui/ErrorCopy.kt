package dev.cueseek.android.ui

import dev.cueseek.core.model.ApiError

/**
 * What the user reads when something fails.
 *
 * @param title one line, naming what happened in the user's terms.
 * @param body what to do about it, or why nothing can be done.
 * @param detail the agent's own words, shown verbatim where they help an operator more
 *   than ours would.
 */
data class ErrorCopy(
    val title: String,
    val body: String,
    val detail: String? = null,
)

/**
 * Every failure, in plain language.
 *
 * Named `explain` rather than `copy` because every [ApiError] case is a data class, and a
 * `copy()` extension is silently shadowed by the generated member on every concrete
 * subtype — it would resolve correctly only when the receiver happened to be typed as the
 * sealed interface. That is the kind of bug that compiles.
 *
 * A total `when` over [ApiError] — a new case stops the build rather than falling through
 * to "Something went wrong", which is the message that teaches users an app cannot be
 * trusted to know what happened.
 *
 * Two rules borrowed from the contract:
 *
 *  - **Never claim to know why a pairing code was rejected.** Unknown, expired and
 *    already-used are merged by the agent on purpose, because saying "expired" reveals the
 *    code was once real.
 *  - **`action-unavailable` shows the agent's own words.** It means the agent is not
 *    permitted or able — usually a polkit rule that does not name the unit — and our
 *    paraphrase would be strictly less useful than its detail.
 */
fun ApiError.explain(): ErrorCopy = when (this) {
    is ApiError.Unauthorized ->
        if (tokenWasSent) {
            ErrorCopy(
                "This device is no longer paired",
                "The agent rejected its credentials. Pair the device again to continue.",
            )
        } else {
            // No credential was sent at all, so re-pairing is not the fix and saying so
            // would send the user in a circle.
            ErrorCopy(
                "No credentials were sent",
                "This is a bug in the app rather than something you can fix. " +
                    "Pairing again is the safest way out.",
            )
        }

    is ApiError.InsufficientScope -> ErrorCopy(
        "This device is not allowed to do that",
        "It was paired without the scope this needs. Pair it again with wider scopes.",
        detail,
    )

    is ApiError.InvalidPairingCode -> ErrorCopy(
        "That code was not accepted",
        "Codes work once and expire quickly. Run cueseekd pair on the host for a new one.",
    )

    is ApiError.RateLimited -> ErrorCopy(
        "Too many attempts",
        "The agent is refusing further pairing attempts for a minute. Try again shortly.",
    )

    is ApiError.NotFound -> ErrorCopy(
        "The agent does not have that",
        "The service or action may have been removed from its configuration.",
        detail,
    )

    is ApiError.ActionInProgress -> ErrorCopy(
        "That is already running",
        "Wait for the current one to finish before starting another.",
        detail,
    )

    is ApiError.ActionUnavailable -> ErrorCopy(
        "The agent cannot do that",
        "This is a permission or configuration problem on the host, not a mistake here. " +
            "The agent explains:",
        detail,
    )

    is ApiError.BadRequest -> ErrorCopy(
        "The agent rejected the request",
        "This is a bug in the app.",
        detail,
    )

    is ApiError.Internal -> ErrorCopy(
        "The agent hit an error",
        "Its logs on the host will say more than it is willing to say here.",
    )

    is ApiError.NotImplemented -> ErrorCopy(
        "The agent is restarting",
        "It is shutting down or coming back up. This should clear on its own.",
    )

    is ApiError.Unrecognised -> ErrorCopy(
        "Unexpected response",
        "The agent returned something this version of CueSeek does not recognise " +
            "(HTTP $status). It may be newer than this app.",
        detail,
    )

    is ApiError.Transport -> ErrorCopy(
        "Cannot reach the agent",
        "Check that the address is right and that the VPN is connected. There is no " +
            "fallback route — if the tailnet is down, the host is simply unreachable.",
    )

    is ApiError.Malformed -> ErrorCopy(
        "Could not read the agent's reply",
        "This usually means the agent is newer than this app. Updating CueSeek should fix it.",
    )
}
