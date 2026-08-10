package dev.cueseek.core.api

import dev.cueseek.core.api.internal.AuthInterceptor
import dev.cueseek.core.api.internal.CueSeekService
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
     * A client for one agent.
     *
     * Timeouts are short because they can be: the agent polls services on its own schedule
     * and serves cached state, so no request waits on Jellyfin. A slow response here means
     * the network, not the upstream, and on a tailnet that is worth surfacing quickly.
     *
     * @param httpClient supply one to share a connection pool with the stream client, or
     *   to install a logging interceptor in debug builds.
     */
    fun create(
        address: AgentAddress,
        tokens: TokenProvider = TokenProvider.None,
        httpClient: OkHttpClient = defaultHttpClient(),
    ): CueSeekApi {
        val client = httpClient.newBuilder()
            .addInterceptor(AuthInterceptor(tokens))
            .build()

        val retrofit = Retrofit.Builder()
            .baseUrl(address.baseUrl)
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()

        return RetrofitCueSeekApi(retrofit.create(CueSeekService::class.java), json)
    }

    fun defaultHttpClient(): OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(15, TimeUnit.SECONDS)
        .writeTimeout(15, TimeUnit.SECONDS)
        .retryOnConnectionFailure(true)
        .build()
}
