package dev.cueseek.core.data

import dev.cueseek.core.model.ActionAcceptance
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.SystemInfo

/**
 * Reads and acts on a host's services.
 *
 * Cold-start reads only. Live updates arrive over the stream in P3, and this stays as the
 * path used before a stream is open and whenever one cannot be held — which on Android is
 * more often than it sounds, since a held connection does not survive Doze (ADR-0004
 * Amendment 2).
 *
 * Requests never wait on an upstream service: the agent polls on its own schedule and
 * serves cached state, so a wedged Jellyfin cannot hang this call.
 */
class ServicesRepository(
    private val clients: AgentClients,
) {

    suspend fun services(host: PairedHost): ApiResult<List<Service>> =
        withApi(host) { it.services() }

    suspend fun service(host: PairedHost, serviceId: String): ApiResult<Service> =
        withApi(host) { it.service(serviceId) }

    suspend fun system(host: PairedHost): ApiResult<SystemInfo> =
        withApi(host) { it.system() }

    /**
     * Invokes an action, returning the agent's acknowledgement.
     *
     * The result says the invocation was accepted, not that it happened. Keep the returned
     * [dev.cueseek.core.model.ActionInvocationId]: the outcome arrives only as a stream
     * event carrying it, and the agent exposes no endpoint to ask again.
     */
    suspend fun invokeAction(
        host: PairedHost,
        serviceId: String,
        actionId: String,
    ): ApiResult<ActionAcceptance> = withApi(host) { it.invokeAction(serviceId, actionId) }

    /**
     * Runs [block] against a client for [host], or fails if there is no usable credential.
     *
     * The missing-credential case is reported as an [ApiError.Unauthorized] with
     * `tokenWasSent = false`, which is literally true — nothing was sent, because there was
     * nothing to send — and routes to the same "pair this device" affordance as a 401 from
     * the agent would.
     */
    private suspend fun <T> withApi(
        host: PairedHost,
        block: suspend (dev.cueseek.core.api.CueSeekApi) -> ApiResult<T>,
    ): ApiResult<T> {
        val api = clients.apiFor(host) ?: return ApiResult.Failure(
            ApiError.Unauthorized(
                tokenWasSent = false,
                detail = "This device holds no usable token for ${host.hostname}. Pair it again.",
            )
        )
        return block(api)
    }
}
