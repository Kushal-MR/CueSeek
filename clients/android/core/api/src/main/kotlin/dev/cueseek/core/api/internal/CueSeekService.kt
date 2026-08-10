package dev.cueseek.core.api.internal

import dev.cueseek.core.api.wire.ActionAccepted
import dev.cueseek.core.api.wire.Device
import dev.cueseek.core.api.wire.PairRequest
import dev.cueseek.core.api.wire.PairResponse
import dev.cueseek.core.api.wire.Service
import dev.cueseek.core.api.wire.System
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.Headers
import retrofit2.http.POST
import retrofit2.http.Path

/**
 * The eight REST operations, hand-written against the generated wire types.
 *
 * Every method returns `Response<T>` rather than `T`: a non-2xx response carries a problem
 * document that has to be read, and Retrofit's alternative is an exception that discards
 * the body's meaning on the way out.
 *
 * `GET /v1/stream` is absent — no generator and no Retrofit adapter models
 * `text/event-stream`. It arrives in P3, read from OkHttp directly.
 */
internal interface CueSeekService {

    @GET("v1/system")
    suspend fun system(): Response<System>

    @Headers("${AuthInterceptor.NO_AUTH_HEADER}: 1")
    @POST("v1/pair")
    suspend fun pair(@Body request: PairRequest): Response<PairResponse>

    @GET("v1/devices")
    suspend fun devices(): Response<List<Device>>

    @DELETE("v1/devices/{deviceId}")
    suspend fun revokeDevice(@Path("deviceId") deviceId: String): Response<Unit>

    @GET("v1/services")
    suspend fun services(): Response<List<Service>>

    @GET("v1/services/{serviceId}")
    suspend fun service(@Path("serviceId") serviceId: String): Response<Service>

    @POST("v1/services/{serviceId}/actions/{actionId}")
    suspend fun invokeAction(
        @Path("serviceId") serviceId: String,
        @Path("actionId") actionId: String,
    ): Response<ActionAccepted>
}
