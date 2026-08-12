package dev.cueseek.core.model

/**
 * Where a service's own interface lives — as parts, never as a URL.
 *
 * The agent supplies no host, and this client must never accept one. The agent reaches
 * Jellyfin at `127.0.0.1` because they share a machine; that address means nothing here.
 * What this client does hold is an address that demonstrably works: the one it paired
 * with. So it composes the URL itself from [urlFor].
 *
 * The security property falls out of the same fact. Because the agent never supplies an
 * origin, a wrong or compromised agent cannot send this client to a host the operator
 * never configured — there is no field in which to put one.
 */
data class WebUi(
    val scheme: String,
    val port: Int,
    val path: String,
)

/**
 * Composes an absolute URL from the address this client paired with.
 *
 * Returns `null` rather than a best guess when anything is wrong. A malformed value here
 * would become an `ACTION_VIEW` intent, so the honest response to input this client cannot
 * vouch for is to offer nothing — the row falls back to opening the detail sheet, and the
 * user sees a working app rather than a browser opening something unexpected.
 *
 * The agent validates all of this too. Repeating it is deliberate, and the same reasoning
 * as ADR-0002's duplicated unit allowlist: the check that protects the user should not
 * live only on the far side of the network.
 */
fun WebUi.urlFor(address: AgentAddress): String? {
    val scheme = scheme.lowercase()
    if (scheme != "http" && scheme != "https") return null
    if (port !in 1..65535) return null

    val path = path.ifEmpty { "/" }
    if (!path.startsWith("/")) return null

    // "//evil.example/x" is protocol-relative: appended to a scheme it silently replaces
    // the host, which would defeat the entire point of composing the origin locally.
    if (path.startsWith("//")) return null
    if (path.contains("://")) return null

    // A host containing either would let the composed string carry credentials or a second
    // authority. The paired address is trusted, but "trusted" is not "unvalidated".
    val host = address.host
    if (host.isBlank() || host.contains("/") || host.contains("@")) return null

    // Host from the pairing, port from the service. The agent's own port is where CueSeek
    // listens, not where Jellyfin does; taking both from one side would be wrong in
    // whichever direction it leaned.
    return "$scheme://$host:$port$path"
}
