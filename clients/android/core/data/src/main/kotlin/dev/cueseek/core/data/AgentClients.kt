package dev.cueseek.core.data

import dev.cueseek.core.api.AgentStream
import dev.cueseek.core.api.CueSeekApi
import dev.cueseek.core.api.CueSeekApiFactory
import dev.cueseek.core.api.TokenProvider
import dev.cueseek.core.model.PairedHost
import java.util.concurrent.ConcurrentHashMap

/**
 * One API client per paired host, reused across calls.
 *
 * Clients are cached because the underlying HTTP client owns a connection pool and a
 * dispatcher; building one per request would open a new connection every time, which on a
 * phone over a tailnet is the difference between a request and a handshake.
 *
 * A client is rebuilt when its host's address changes, since the base URL is fixed at
 * construction.
 */
class AgentClients(
    private val hosts: HostRepository,
    private val http: CueSeekApiFactory.AgentHttp = CueSeekApiFactory.sharedHttp(),
) {

    private data class Entry(val baseUrl: String, val api: CueSeekApi)

    private val clients = ConcurrentHashMap<String, Entry>()

    /**
     * Returns a client for [host], or `null` if this device holds no usable token for it.
     *
     * Null means the token was never stored or can no longer be decrypted; in the latter
     * case [HostRepository.token] has already dropped the record, so the honest answer to
     * the caller is that there is nothing to talk to the agent with.
     */
    suspend fun apiFor(host: PairedHost): CueSeekApi? {
        // Warms the in-memory cache the interceptor reads, and drops the record if the
        // stored token can no longer be recovered.
        hosts.token(host.hostId) ?: return null

        val key = host.hostId.value
        val baseUrl = host.address.baseUrl
        clients[key]?.takeIf { it.baseUrl == baseUrl }?.let { return it.api }

        val api = CueSeekApiFactory.create(
            address = host.address,
            tokens = TokenProvider { hosts.cachedToken(host.hostId) },
            http = http,
        )
        clients[key] = Entry(baseUrl, api)
        return api
    }

    /**
     * Returns a stream client for [host], or `null` if there is no usable token.
     *
     * Not cached: a stream client is one connection with its own lifetime, created when
     * something starts collecting and discarded when it stops. Caching it would outlive the
     * collector that owns it.
     */
    suspend fun streamFor(host: PairedHost): AgentStream? {
        hosts.token(host.hostId) ?: return null
        return CueSeekApiFactory.createStream(
            address = host.address,
            tokens = TokenProvider { hosts.cachedToken(host.hostId) },
            http = http,
        )
    }

    /** Drops a cached client, for use when a host is forgotten. */
    fun evict(host: PairedHost) {
        clients.remove(host.hostId.value)
    }
}
