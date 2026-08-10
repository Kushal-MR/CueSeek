package dev.cueseek.core.api.internal

import dev.cueseek.core.api.TokenProvider
import okhttp3.Interceptor
import okhttp3.Response

/**
 * Attaches `Authorization: Bearer <token>` to everything except pairing.
 *
 * Pairing is marked by a header the Retrofit declaration carries and this interceptor
 * removes, rather than by matching on the path. A path match would silently start sending
 * credentials the day someone renames the endpoint, and pairing is the one request that
 * must not carry one — there is nothing to carry yet.
 *
 * The header this leaves behind is also the evidence used to tell the agent's two 401s
 * apart. See [toApiError].
 */
internal class AuthInterceptor(
    private val tokens: TokenProvider,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()

        if (request.header(NO_AUTH_HEADER) != null) {
            return chain.proceed(request.newBuilder().removeHeader(NO_AUTH_HEADER).build())
        }

        val token = tokens.current() ?: return chain.proceed(request)

        return chain.proceed(
            request.newBuilder()
                .header("Authorization", "Bearer ${token.value}")
                .build()
        )
    }

    companion object {
        const val NO_AUTH_HEADER: String = "X-CueSeek-No-Auth"
    }
}
