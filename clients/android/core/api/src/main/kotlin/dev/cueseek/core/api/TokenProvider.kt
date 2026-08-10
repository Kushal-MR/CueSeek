package dev.cueseek.core.api

import dev.cueseek.core.model.DeviceToken

/**
 * Supplies the token for the agent this client talks to, or `null` before pairing.
 *
 * Non-suspending on purpose. It is consulted from an OkHttp interceptor, on OkHttp's
 * thread, for every request; the alternative is blocking there, which is exactly the kind
 * of thing that works until the disk is slow. The data layer decrypts once and keeps the
 * token in memory, so this is a field read.
 */
fun interface TokenProvider {
    fun current(): DeviceToken?

    companion object {
        /** For pairing, and for tests that do not care. */
        val None: TokenProvider = TokenProvider { null }
    }
}
