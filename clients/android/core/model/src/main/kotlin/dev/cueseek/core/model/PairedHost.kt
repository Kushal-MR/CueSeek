package dev.cueseek.core.model

import java.time.Instant

/**
 * An agent this app has paired with.
 *
 * Keyed by [hostId], not by [address], from the first commit (ADR-0008). The address is
 * how the user reached the host once; the id is what the host *is*, and it survives
 * restarts, hostname changes and IP changes. Keying on the address instead would make a
 * DHCP lease change look like a new server.
 *
 * The token is deliberately not a field here. This record is written to disk in clear
 * text; the token lives in [dev.cueseek.core.model.DeviceToken] form behind the Keystore,
 * and keeping them in separate places means a record can be read, logged or diffed without
 * anyone having to remember which field is a secret.
 */
data class PairedHost(
    val hostId: HostId,
    /** Where this host was last reachable. Updated whenever the user edits it. */
    val address: AgentAddress,
    /** Display-only, and may change under us. */
    val hostname: String,
    val agentVersion: String,
    /** The contract version the agent implements, for honest skew reporting (ADR-0007). */
    val apiVersion: String,
    val deviceId: DeviceId,
    val deviceName: String,
    /**
     * The scopes this device was granted at pairing time.
     *
     * Used to hide controls the device cannot use. That is user experience, not a control:
     * enforcement is entirely server-side, and every call handles a `403` regardless.
     */
    val scopes: Set<Scope>,
    val pairedAt: Instant,
) {
    fun canControlServices(): Boolean = Scope.ServiceControl in scopes

    fun canManageDevices(): Boolean = Scope.DevicesManage in scopes

    fun canRead(): Boolean = Scope.Read in scopes
}
