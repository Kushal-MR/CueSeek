package dev.cueseek.core.model

import java.time.Instant

/**
 * Client platform, recorded by the agent for display in the device list.
 *
 * A display label with no security meaning — the agent stores an unrecognised value as
 * `unknown` rather than rejecting it.
 */
enum class Platform(val wire: String) {
    Android("android"),
    WearOs("wearos"),
    Ios("ios"),
    Web("web"),
    Desktop("desktop"),
    Cli("cli"),
    Unknown("unknown"),
    ;

    companion object {
        fun fromWire(value: String): Platform =
            entries.firstOrNull { it.wire == value } ?: Unknown
    }
}

/**
 * An independently grantable permission.
 *
 * Scopes are **not tiers**: a device holding `service.control` does not thereby hold
 * `read`. Enforcement is entirely server-side, so anything this type is used for on the
 * client is user experience — hiding a control the device cannot use — and never a
 * control in itself. Every call still handles a `403`.
 */
enum class Scope(val wire: String) {
    Read("read"),
    ServiceControl("service.control"),
    DevicesManage("devices.manage"),
    HostPower("host.power"),
    ;

    companion object {
        /**
         * Unrecognised scopes are dropped rather than represented.
         *
         * Safe in this direction and only this direction: a scope the client does not
         * know can only cause it to hide UI it might have shown. It can never cause it to
         * offer something the agent would refuse, because the agent is what decides.
         */
        fun fromWire(value: String): Scope? = entries.firstOrNull { it.wire == value }
    }
}

/** A paired device, as listed by `GET /v1/devices`. */
data class Device(
    val id: DeviceId,
    val name: String,
    val platform: Platform,
    val scopes: Set<Scope>,
    val createdAt: Instant,
    /** Absent until the device makes its first authenticated request. */
    val lastSeenAt: Instant?,
)

/**
 * The result of redeeming a pairing code.
 *
 * The token is returned exactly once and is never recoverable — the agent keeps only its
 * hash — so losing it means pairing again.
 */
data class Pairing(
    val device: Device,
    val token: DeviceToken,
)
