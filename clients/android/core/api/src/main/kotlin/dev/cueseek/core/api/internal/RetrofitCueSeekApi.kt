package dev.cueseek.core.api.internal

import dev.cueseek.core.api.CueSeekApi
import dev.cueseek.core.api.wire.PairRequest
import dev.cueseek.core.model.ActionAcceptance
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.Device
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.Pairing
import dev.cueseek.core.model.Platform
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.SystemInfo
import java.io.IOException
import java.time.DateTimeException
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import retrofit2.Response

/**
 * [CueSeekApi] over Retrofit.
 *
 * All the interesting behaviour is in [call]: every failure that can reach a caller is
 * turned into an [ApiError] there, so no operation below has any error handling of its own
 * and none of them can forget to.
 */
internal class RetrofitCueSeekApi(
    private val service: CueSeekService,
    private val json: Json,
) : CueSeekApi {

    override suspend fun system(): ApiResult<SystemInfo> =
        call({ service.system() }) { it.toDomain() }

    override suspend fun pair(
        code: String,
        deviceName: String,
        platform: Platform,
    ): ApiResult<Pairing> = call(
        {
            service.pair(
                PairRequest(
                    code = code,
                    deviceName = deviceName,
                    platform = platform.wire,
                )
            )
        }
    ) { it.toDomain() }

    override suspend fun devices(): ApiResult<List<Device>> =
        call({ service.devices() }) { devices -> devices.map { it.toDomain() } }

    override suspend fun revokeDevice(id: DeviceId): ApiResult<Unit> =
        callEmpty { service.revokeDevice(id.value) }

    override suspend fun requestRefresh(): ApiResult<Unit> =
        callEmpty { service.requestRefresh() }

    override suspend fun services(): ApiResult<List<Service>> =
        call({ service.services() }) { services -> services.map { it.toDomain() } }

    override suspend fun service(id: String): ApiResult<Service> =
        call({ service.service(id) }) { it.toDomain() }

    override suspend fun invokeAction(
        serviceId: String,
        actionId: String,
    ): ApiResult<ActionAcceptance> =
        call({ service.invokeAction(serviceId, actionId) }) { it.toDomain() }

    private suspend fun <W : Any, D> call(
        request: suspend () -> Response<W>,
        map: (W) -> D,
    ): ApiResult<D> = guard {
        val response = request()
        when {
            !response.isSuccessful -> ApiResult.Failure(response.toApiError(json))

            else -> {
                val body = response.body()
                if (body == null) {
                    // A 2xx that the contract says carries a body, without one. Nothing
                    // sensible can be rendered from it, and pretending otherwise would
                    // show empty state as though it were real state.
                    ApiResult.Failure(
                        ApiError.Malformed(
                            IllegalStateException("empty body from ${response.raw().request.url}")
                        )
                    )
                } else {
                    ApiResult.Success(map(body))
                }
            }
        }
    }

    /** For `202` and `204`, where a body's absence is the success condition. */
    private suspend fun callEmpty(request: suspend () -> Response<Unit>): ApiResult<Unit> =
        guard {
            val response = request()
            if (response.isSuccessful) {
                ApiResult.Success(Unit)
            } else {
                ApiResult.Failure(response.toApiError(json))
            }
        }

    /**
     * Turns the three ways a call can fail outside HTTP into values.
     *
     * [IOException] is the common one on a phone and means the request never got an
     * answer — the tailnet is down, or the host is off. It is not "slow": there is no
     * relay and no fallback path, so it will not resolve itself by waiting.
     *
     * The other two are version skew wearing different hats: a field shaped unexpectedly
     * ([SerializationException]) or a timestamp that will not parse ([DateTimeException]).
     * Both mean the agent said something this build does not understand.
     */
    private inline fun <T> guard(block: () -> ApiResult<T>): ApiResult<T> = try {
        block()
    } catch (e: IOException) {
        ApiResult.Failure(ApiError.Transport(e))
    } catch (e: SerializationException) {
        ApiResult.Failure(ApiError.Malformed(e))
    } catch (e: DateTimeException) {
        ApiResult.Failure(ApiError.Malformed(e))
    }
}
