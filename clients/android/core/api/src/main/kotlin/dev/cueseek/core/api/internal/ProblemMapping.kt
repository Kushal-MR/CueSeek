package dev.cueseek.core.api.internal

import dev.cueseek.core.api.wire.Problem
import dev.cueseek.core.model.ApiError
import kotlinx.serialization.json.Json
import retrofit2.Response

/** Every problem `type` the agent emits is this prefix plus a slug. */
private const val PROBLEM_PREFIX = "https://cueseek.dev/problems/"

/**
 * Maps a non-2xx response to an [ApiError].
 *
 * Branching is on the problem `type` alone. `detail` is carried through for display but is
 * never inspected: it is human-facing prose, and the agent is free to reword it.
 *
 * The one place that needs more than `type` is `401`, where a missing credential and a
 * rejected one share a type and differ only in prose. The discriminator used instead is
 * whether the request that produced this response actually carried an `Authorization`
 * header — a fact about what this client did, which no amount of server rewording can
 * change.
 */
internal fun Response<*>.toApiError(json: Json): ApiError {
    val problem = readProblem(json)
    val detail = problem?.detail
    val tokenWasSent = raw().request.header("Authorization") != null

    val slug = problem?.type?.removePrefix(PROBLEM_PREFIX)

    return when (slug) {
        "unauthorized" -> ApiError.Unauthorized(tokenWasSent = tokenWasSent, detail = detail)
        "insufficient-scope" -> ApiError.InsufficientScope(detail)
        "invalid-pairing-code" -> ApiError.InvalidPairingCode(detail)
        "rate-limited" -> ApiError.RateLimited(detail)
        "not-found" -> ApiError.NotFound(detail)
        "action-in-progress" -> ApiError.ActionInProgress(detail)
        "action-unavailable" -> ApiError.ActionUnavailable(detail)
        "bad-request" -> ApiError.BadRequest(detail)
        "internal" -> ApiError.Internal(detail)
        "not-implemented" -> ApiError.NotImplemented(detail)

        // A problem type this build predates, or a failure from something between the
        // phone and the agent that never produced a problem document at all. The status is
        // kept either way, because "502" is still more useful to a user than "something
        // went wrong".
        else -> ApiError.Unrecognised(
            type = problem?.type.orEmpty(),
            title = problem?.title ?: "HTTP ${code()}",
            status = problem?.status ?: code(),
            detail = detail,
        )
    }
}

private fun Response<*>.readProblem(json: Json): Problem? {
    val body = errorBody()?.string().orEmpty()
    if (body.isBlank()) return null
    return runCatching { json.decodeFromString<Problem>(body) }.getOrNull()
}
