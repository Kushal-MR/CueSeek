package dev.cueseek.core.api

import dev.cueseek.core.api.internal.AuthInterceptor
import dev.cueseek.core.api.internal.CueSeekService
import dev.cueseek.core.api.internal.OkHttpAgentStream
import dev.cueseek.core.api.internal.RetrofitCueSeekApi
import dev.cueseek.core.model.AgentAddress
import java.util.concurrent.TimeUnit
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory

/**
 * How `:core:api` is constructed. The only entry point; everything else is internal.
 */
object CueSeekApiFactory {

    /**
     * `ignoreUnknownKeys` is load-bearing rather than lazy configuration.
     *
     * A client will meet responses from agents newer than itself for the whole life of the
     * project (ADR-0007). Failing on a field that was added after this build shipped would
     * turn every agent update into a client outage.
     */
    val json: Json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
    }

    /**
     * A connection pool, shared between clients, without naming what it is made of.
     *
     * Opaque on purpose. OkHttp is an `implementation` dependency of this module so that
     * transport types cannot reach a ViewModel (ADR-0009); a factory signature taking an
     * `OkHttpClient` would have quietly undone that by forcing every caller to put OkHttp
     * on its own classpath. Callers that want pooling pass this around and never open it.
     */
    class AgentHttp internal constructor(internal val client: OkHttpClient)

    /** One pool for the whole app. Cheap to hold, expensive to duplicate. */
    fun sharedHttp(): AgentHttp = AgentHttp(defaultHttpClient())

    /**
     * A client for one agent.
     *
     * Timeouts are short because they can be: the agent polls services on its own schedule
     * and serves cached state, so no request waits on Jellyfin. A slow response here means
     * the network, not the upstream, and on a tailnet that is worth surfacing quickly.
     *
     * @param http pass a shared instance so every host reuses one connection pool, and so
     *   the stream client can join it in P3.
     */
    fun create(
        address: AgentAddress,
        tokens: TokenProvider = TokenProvider.None,
        http: AgentHttp = sharedHttp(),
    ): CueSeekApi {
        val client = http.client.newBuilder()
            .addInterceptor(AuthInterceptor(tokens))
            .build()

        val retrofit = Retrofit.Builder()
            .baseUrl(address.baseUrl)
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()

        return RetrofitCueSeekApi(retrofit.create(CueSeekService::class.java), json)
    }

    /**
     * A stream client for one agent.
     *
     * Built from the same pool as [create], but with **no read timeout**: a stream is
     * meant to be silent between events, and a 15-second read timeout would sever it every
     * time the agent had nothing to say. Silence is detected by the freshness watchdog on
     * a clock, which is the only signal that survives Doze — `docs/m2-android-api.md` §8.
     */
    fun createStream(
        address: AgentAddress,
        tokens: TokenProvider = TokenProvider.None,
        http: AgentHttp = sharedHttp(),
    ): AgentStream {
        val client = http.client.newBuilder()
            .addInterceptor(AuthInterceptor(tokens))
            .readTimeout(java.time.Duration.ZERO)
            // A frozen connection must not be resurrected by OkHttp behind our back: the
            // reconnect decision belongs to the collector, which is the only thing that
            // knows whether the app is even in the foreground.
            .retryOnConnectionFailure(false)
            .build()

        return OkHttpAgentStream(address, client, json)
    }

    private fun defaultHttpClient(): OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(15, TimeUnit.SECONDS)
        .writeTimeout(15, TimeUnit.SECONDS)
        .retryOnConnectionFailure(true)
        .build()
}
