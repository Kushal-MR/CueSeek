package dev.cueseek.core.model

/**
 * The address a phone hands to a watch, and the shape it travels in.
 *
 * # Why this exists
 *
 * Typing `100.92.18.125` on a 233dp round screen is the step where somebody stops setting
 * the app up. The code is eight characters, case-insensitive, and says so immediately when
 * wrong; the address is fifteen characters, unguessable, and fails as a timeout. So the
 * phone supplies the address and the watch still asks for the code
 * ([ADR-0014](../../../../../../../../docs/adr/0014-watch-pairing-address-handoff.md)).
 *
 * # Why it lives in `:core:model`
 *
 * Both halves of the protocol must agree, and neither half is where agreement belongs. This
 * module is plain Kotlin/JVM, so the format is defined once, shared by construction rather
 * than by copy-paste, and tested without a device on either end.
 *
 * # The format
 *
 * ```
 * cueseek://pair?host=100.92.18.125&port=7777
 * ```
 *
 * Deliberately the URI ADR-0006 Amendment 3 already recorded for a future QR flow, minus
 * `code`. One canonical format for "where the agent is" means a QR producer, if it is ever
 * built, is the same string with one more parameter rather than a second dialect.
 */
object AgentHandoff {

    /**
     * The Data Layer path the address is published at.
     *
     * A stable data item rather than a message: the watch reads it whenever it opens, which
     * may be hours after the phone last changed anything. A message would require both
     * devices awake at the same instant, which is the one thing a watch cannot promise.
     */
    const val PATH: String = "/cueseek/agent-address"

    /** The single key inside the data item. The value is [encode]'s output. */
    const val KEY_URI: String = "uri"

    private const val SCHEME = "cueseek"
    private const val AUTHORITY = "pair"

    fun encode(address: AgentAddress): String =
        "$SCHEME://$AUTHORITY?host=${address.host}&port=${address.port}"

    /**
     * Parses [uri], or returns null if it is not something this understands.
     *
     * Null rather than an exception: the input arrives from another device, and a
     * malformed one is a thing to ignore rather than a programming error to crash on.
     *
     * **A URI carrying a `code` is rejected outright**, and that is the interesting rule
     * here. ADR-0014 turns on the token never travelling between devices — two devices
     * sharing one credential would mean revoking either revokes both, and the audit log
     * attributing a watch's action to a phone. A pairing code is one redemption away from
     * being that credential. So the constraint is enforced by the parser rather than left
     * as a sentence in a record: if a future change ever sends one, the watch refuses it.
     */
    fun decode(uri: String): AgentAddress? {
        val trimmed = uri.trim()
        val prefix = "$SCHEME://$AUTHORITY?"
        if (!trimmed.startsWith(prefix)) return null

        val params = trimmed.removePrefix(prefix)
            .split('&')
            .mapNotNull { pair ->
                val i = pair.indexOf('=')
                if (i <= 0) null else pair.substring(0, i) to pair.substring(i + 1)
            }
            .toMap()

        if (params.containsKey("code")) return null

        val host = params["host"]?.takeIf { it.isNotBlank() } ?: return null
        val port = params["port"]?.toIntOrNull() ?: return null
        if (port !in 1..65535) return null

        return AgentAddress(host, port)
    }
}
