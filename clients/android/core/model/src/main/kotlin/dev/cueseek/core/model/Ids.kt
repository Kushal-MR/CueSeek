package dev.cueseek.core.model

/**
 * The agent's stable identifier for a host.
 *
 * Distinct from `hostname`, which is display-only and may change, and from the address the
 * user typed, which changes whenever the network does. This is the key every repository is
 * keyed by (ADR-0008), so it is worth being unable to pass a hostname where one belongs.
 */
@JvmInline
value class HostId(val value: String)

/** A paired device's identifier, as issued by the agent. */
@JvmInline
value class DeviceId(val value: String)

/**
 * The identifier returned by an action invocation, correlating it with the single
 * `action_progress` event that reports its outcome.
 *
 * Deliberately not a `String`: the action *name* ("restart") and the invocation id
 * ("2944a731d4a8af63") are both strings, appear next to each other in every action payload,
 * and mean entirely different things.
 */
@JvmInline
value class ActionInvocationId(val value: String)

/**
 * A device token.
 *
 * Long-lived, unrecoverable once lost, and sufficient to restart services on a real
 * machine. [toString] is redacted so it cannot reach a log through a data class that
 * happens to hold one.
 */
@JvmInline
value class DeviceToken(val value: String) {
    override fun toString(): String = "DeviceToken(redacted)"
}
