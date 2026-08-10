package dev.cueseek.core.data.internal

import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.HostId
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Scope
import java.time.Instant
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * The on-disk shape of a paired host.
 *
 * Deliberately not [PairedHost] itself. A domain type is free to gain a computed property
 * or change a field's type; a persisted type cannot, without a migration. Keeping them
 * apart means the day the UI wants `PairedHost.displayName` is not the day existing
 * installs stop reading their own records.
 *
 * Contains no secret. The token is sealed separately by [TokenCipher], so this record can
 * be read, logged or diffed without anyone needing to know which field is dangerous.
 */
@Serializable
internal data class StoredHost(
    val hostId: String,
    val host: String,
    val port: Int,
    val hostname: String,
    val agentVersion: String,
    val apiVersion: String,
    val deviceId: String,
    val deviceName: String,
    /** Wire values, not enum names: the wire is the stable spelling (`service.control`). */
    val scopes: List<String>,
    val pairedAtEpochMillis: Long,
) {
    fun toDomain(): PairedHost = PairedHost(
        hostId = HostId(hostId),
        address = AgentAddress(host, port),
        hostname = hostname,
        agentVersion = agentVersion,
        apiVersion = apiVersion,
        deviceId = DeviceId(deviceId),
        deviceName = deviceName,
        // Unknown scopes are dropped, exactly as they are on the wire: a scope this build
        // does not know can only cause it to hide UI, never to offer something the agent
        // would refuse.
        scopes = scopes.mapNotNull(Scope::fromWire).toSet(),
        pairedAt = Instant.ofEpochMilli(pairedAtEpochMillis),
    )

    companion object {
        fun from(host: PairedHost): StoredHost = StoredHost(
            hostId = host.hostId.value,
            host = host.address.host,
            port = host.address.port,
            hostname = host.hostname,
            agentVersion = host.agentVersion,
            apiVersion = host.apiVersion,
            deviceId = host.deviceId.value,
            deviceName = host.deviceName,
            scopes = host.scopes.map { it.wire },
            pairedAtEpochMillis = host.pairedAt.toEpochMilli(),
        )
    }
}

/**
 * Reads and writes the whole host list as one JSON document.
 *
 * A list rather than a row per host because there will be a handful at most, and one
 * document means adding a host is one atomic write rather than a set of writes that can be
 * half-applied.
 */
internal object HostRecords {

    private val json = Json {
        ignoreUnknownKeys = true
        encodeDefaults = true
    }

    fun encode(hosts: List<PairedHost>): String =
        json.encodeToString(hosts.map(StoredHost::from))

    /**
     * Decodes, returning an empty list if the document is unreadable.
     *
     * Losing the host list means re-entering an address and pairing again — annoying.
     * Refusing to start because one record is malformed means the app is bricked until
     * app data is cleared, which is worse and harder to explain.
     */
    fun decode(raw: String?): List<PairedHost> {
        if (raw.isNullOrBlank()) return emptyList()
        return try {
            json.decodeFromString<List<StoredHost>>(raw).map { it.toDomain() }
        } catch (_: Exception) {
            emptyList()
        }
    }
}
