package dev.cueseek.core.data

import dev.cueseek.core.data.internal.HostRecords
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.HostId
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Scope
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The persisted host record, on the JVM.
 *
 * Everything here is deliberately free of Android so it runs without a device: the record
 * format is the part most likely to be changed carelessly, and it is worth being able to
 * check it in a second rather than in a minute.
 */
class HostRecordsTest {

    private val host = PairedHost(
        hostId = HostId("664917f8b739290c57d971481accef0e"),
        address = AgentAddress("100.92.18.125", 7777),
        hostname = "kushal-HP-paviliong6",
        agentVersion = "m1.8-listenretry",
        apiVersion = "0.1.0",
        deviceId = DeviceId("217f2f3dbf991996"),
        deviceName = "Pixel 8",
        scopes = setOf(Scope.Read, Scope.ServiceControl),
        pairedAt = Instant.parse("2026-08-10T05:38:14Z"),
    )

    @Test
    fun `a host survives a round trip unchanged`() {
        val decoded = HostRecords.decode(HostRecords.encode(listOf(host)))

        assertEquals(listOf(host), decoded)
    }

    @Test
    fun `scopes are stored by their wire spelling`() {
        val encoded = HostRecords.encode(listOf(host))

        // "service.control", not "ServiceControl". The wire spelling is the stable one;
        // an enum constant can be renamed by a refactor that has no idea it is persisted.
        assertTrue(encoded, encoded.contains("\"service.control\""))
    }

    @Test
    fun `several hosts coexist`() {
        val second = host.copy(
            hostId = HostId("aaaa1111bbbb2222"),
            address = AgentAddress("100.64.0.9", 8080),
            hostname = "nas",
        )

        val decoded = HostRecords.decode(HostRecords.encode(listOf(host, second)))

        // ADR-0008: the interface shows one host in M2, but nothing in the data layer
        // assumes that. Adding the second one is inserting a record.
        assertEquals(2, decoded.size)
        assertEquals(setOf(host.hostId, second.hostId), decoded.map { it.hostId }.toSet())
    }

    @Test
    fun `a scope this build predates is dropped, not fatal`() {
        val raw = """
            [{
              "hostId":"h1","host":"10.0.0.2","port":7777,"hostname":"box",
              "agentVersion":"1","apiVersion":"0.1.0",
              "deviceId":"d1","deviceName":"Pixel","scopes":["read","host.thermal"],
              "pairedAtEpochMillis":1000
            }]
        """.trimIndent()

        val decoded = HostRecords.decode(raw)

        assertEquals(setOf(Scope.Read), decoded.single().scopes)
    }

    @Test
    fun `a field added by a later version does not break older records`() {
        val raw = """
            [{
              "hostId":"h1","host":"10.0.0.2","port":7777,"hostname":"box",
              "agentVersion":"1","apiVersion":"0.1.0",
              "deviceId":"d1","deviceName":"Pixel","scopes":["read"],
              "pairedAtEpochMillis":1000,
              "favouriteColour":"blue"
            }]
        """.trimIndent()

        assertEquals(1, HostRecords.decode(raw).size)
    }

    @Test
    fun `an unreadable document yields no hosts rather than a crash`() {
        // Losing the host list costs one re-pairing. Refusing to start because one record
        // is malformed bricks the app until app data is cleared, which is worse.
        assertEquals(emptyList<PairedHost>(), HostRecords.decode("{not json"))
        assertEquals(emptyList<PairedHost>(), HostRecords.decode(""))
        assertEquals(emptyList<PairedHost>(), HostRecords.decode(null))
    }

    @Test
    fun `scope helpers reflect what the agent granted`() {
        assertTrue(host.canRead())
        assertTrue(host.canControlServices())
        // The CLI's default grant is read,service.control - so a typical device cannot
        // revoke anything, including itself.
        assertTrue(!host.canManageDevices())
    }
}
