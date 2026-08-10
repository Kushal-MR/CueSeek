package dev.cueseek.core.api

import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.StreamEnvelope
import kotlinx.coroutines.flow.Flow

/**
 * The agent's live event stream.
 *
 * One connection per collection. The flow **fails** rather than reconnecting: deciding
 * when to try again is a policy question — how long to wait, whether to give up, whether
 * the app is even in the foreground — and burying it in the transport would make it
 * untestable and unchangeable. `docs/m2-android-api.md` §8 puts it plainly: reconnect on
 * your own schedule, not when told to.
 *
 * Every connection begins with a full snapshot, so a reconnect needs no replay and the
 * collector needs no merge logic.
 */
interface AgentStream {

    /**
     * Connects and emits envelopes until cancelled or failed.
     *
     * Fails with [StreamFailure] carrying a mapped [ApiError]. A `401` there means the
     * token is dead and retrying is pointless; a `503` means the agent is shutting down
     * and retrying is exactly right.
     */
    fun events(): Flow<StreamEnvelope>
}

/** Terminates [AgentStream.events] with the reason, in the same vocabulary as every call. */
class StreamFailure(val error: ApiError) : Exception(error.toString())
