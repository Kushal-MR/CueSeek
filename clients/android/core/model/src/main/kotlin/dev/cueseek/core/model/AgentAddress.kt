package dev.cueseek.core.model

/**
 * Where an agent is, as the user supplied it.
 *
 * There is no discovery mechanism and no relay: the user types this, and if the tailnet is
 * down the host is simply unreachable (ADR-0001). The port is configuration rather than a
 * constant — `7777` is only the reference deployment's choice — so it must be enterable.
 *
 * This is not the identity of a host. [HostId] is, and it is only knowable after the first
 * successful call. An address changes; the host behind it does not.
 */
data class AgentAddress(
    val host: String,
    val port: Int,
) {
    /** Base URL, trailing slash included, as the HTTP client wants it. */
    val baseUrl: String get() = "http://$host:$port/"

    override fun toString(): String = "$host:$port"

    companion object {
        const val DEFAULT_PORT: Int = 7777
    }
}
