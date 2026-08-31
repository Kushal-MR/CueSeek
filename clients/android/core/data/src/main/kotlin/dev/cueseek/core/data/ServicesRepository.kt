package dev.cueseek.core.data

import dev.cueseek.core.model.ActionAcceptance
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.StreamEvent
import dev.cueseek.core.model.SystemInfo
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope

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
     * Asks the agent to observe every service now.
     *
     * The half of a manual refresh that makes it verification. [snapshot] alone re-reads
     * the agent's cache, which — right after you stopped something — still describes the
     * moment before you stopped it. This makes the agent look again.
     *
     * Returns as soon as the agent accepts. Nothing has been observed yet at that point;
     * the results arrive over the stream, or on the [snapshot] that follows.
     */
    suspend fun requestRefresh(host: PairedHost): ApiResult<Unit> =
        withApi(host) { it.requestRefresh() }

    /**
     * Everything a screen needs, in the same shape the stream delivers it.
     *
     * Returns a [StreamEvent.Snapshot] rather than a pair, and that is the whole point of
     * this method existing. A manual refresh then flows through the identical code path a
     * streamed snapshot does — same application, same clock, same degradation — instead of
     * being a second way for state to arrive that has to be kept in agreement with the
     * first. A capability added later rides along without touching this: it is already
     * inside `services`.
     *
     * The system and services calls must both succeed. A snapshot missing half of itself is
     * not a snapshot, and applying one would replace the roster wholesale with a partial
     * truth. Host metrics are the exception and are allowed to fail: they are one row of
     * the screen rather than the screen, and losing them is no reason to refuse a refresh
     * that otherwise worked.
     *
     * Note what this does *not* do: it does not make the agent poll upstream. Requests are
     * served from the agent's cache by design (ADR-0003), so this returns what the agent
     * already knows. What it proves is that the agent is reachable **now**, which is
     * exactly the thing a frozen stream cannot tell you.
     */
    suspend fun snapshot(host: PairedHost): ApiResult<StreamEvent.Snapshot> = coroutineScope {
        val system = async { system(host) }
        val services = async { services(host) }
        val metrics = async { hostMetrics(host) }

        when (val systemResult = system.await()) {
            is ApiResult.Failure -> ApiResult.Failure(systemResult.error)
            is ApiResult.Success -> when (val servicesResult = services.await()) {
                is ApiResult.Failure -> ApiResult.Failure(servicesResult.error)
                is ApiResult.Success -> ApiResult.Success(
                    StreamEvent.Snapshot(
                        system = systemResult.value,
                        services = servicesResult.value,
                        // Read here rather than left out, because a snapshot replaces
                        // state wholesale: omitting metrics would make every manual
                        // refresh blank the machine's vitals until the next tick, which
                        // would look exactly like a pull having broken something.
                        //
                        // Unlike the other two, a failure here is not fatal. Metrics are
                        // one part of the screen; the services are the screen, and losing
                        // the CPU row is no reason to refuse a refresh that otherwise
                        // worked. The null it falls back to says "not known", which is
                        // the truth in that case.
                        hostMetrics = (metrics.await() as? ApiResult.Success)?.value,
                    )
                )
            }
        }
    }

    /**
     * The host's vitals, or null when the agent has none to report.
     *
     * Null is a successful answer: an agent that started seconds ago, has metrics turned
     * off, or runs on a platform that cannot read them all answer 204. Only a transport or
     * authorisation failure is an [ApiResult.Failure].
     */
    suspend fun hostMetrics(host: PairedHost): ApiResult<HostMetrics?> =
        withApi(host) { it.hostMetrics() }

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
