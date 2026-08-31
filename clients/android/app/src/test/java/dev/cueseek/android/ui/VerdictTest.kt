package dev.cueseek.android.ui

import dev.cueseek.android.ui.dashboard.hostConcern
import dev.cueseek.android.ui.dashboard.verdict
import dev.cueseek.core.design.status.Tally
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.Capability
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.Freshness
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.HostId
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.MemoryMetrics
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Scope
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.StorageMetrics
import dev.cueseek.core.model.StreamStatus
import dev.cueseek.core.model.ThermalMetrics
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The headline.
 *
 * The behaviour under test is an ordering, and the ordering is the whole point: a filesystem
 * at 97% is a fact somebody must act on, while `unknown` is the absence of one. Before this,
 * a disk could sit at 97% with its rule drawn red while the headline directly above it read
 * "All good".
 */
class VerdictTest {

    private val host = PairedHost(
        hostId = HostId("h1"),
        address = AgentAddress("100.92.18.125", 7777),
        hostname = "box",
        agentVersion = "m3.6",
        apiVersion = "0.1.0",
        deviceId = DeviceId("d1"),
        deviceName = "Pixel",
        scopes = setOf(Scope.Read),
        pairedAt = Instant.EPOCH,
    )

    private fun service(status: HealthStatus, id: String = "jellyfin") = Service(
        id = id,
        name = id,
        capabilities = listOf(Capability("health", "Health")),
        health = Health(status, reachable = true, reportedStatus = null, reasons = emptyList(), observedAt = Instant.EPOCH),
        actions = emptyList(),
    )

    private fun state(
        services: List<Service> = listOf(service(HealthStatus.Healthy)),
        metrics: HostMetrics? = null,
        stale: Boolean = false,
    ) = AgentState(
        host = host,
        system = null,
        services = services,
        status = StreamStatus.Open(Instant.EPOCH),
        freshness = if (stale) Freshness.Stale(Instant.EPOCH) else Freshness.Fresh,
        hostMetrics = metrics,
    )

    private fun disk(usedFraction: Float) = HostMetrics(
        collectedAt = Instant.EPOCH,
        storage = listOf(
            StorageMetrics(
                mount = "/",
                totalBytes = 1000,
                freeBytes = (1000 * (1 - usedFraction)).toLong(),
            )
        ),
    )

    private fun said(state: AgentState) = verdict(state, Tally.of(state.services))

    @Test
    fun `a healthy fleet on a healthy machine is operational`() {
        assertEquals("Operational", said(state()))
    }

    @Test
    fun `a full disk is not All good`() {
        // The defect this exists to prevent: every service fine, the disk rule drawn red,
        // and the headline above it cheerfully saying nothing was wrong.
        assertEquals("Disk almost full", said(state(metrics = disk(0.97f))))
        assertEquals("Disk filling up", said(state(metrics = disk(0.88f))))
    }

    @Test
    fun `an ordinary disk says nothing`() {
        assertEquals("Operational", said(state(metrics = disk(0.64f))))
        assertNull(hostConcern(disk(0.64f)))
    }

    @Test
    fun `services needing attention outrank the machine`() {
        // A service that is down is more urgent than a disk that is nearly full, and the
        // headline has room for one sentence.
        val degraded = state(
            services = listOf(service(HealthStatus.Unreachable)),
            metrics = disk(0.97f),
        )
        assertEquals("1 needs attention", said(degraded))
    }

    @Test
    fun `a definite problem outranks an unknown one`() {
        // Deliberate: a full disk is a fact, `unknown` is the absence of a fact.
        val unknown = state(
            services = listOf(service(HealthStatus.Unknown)),
            metrics = disk(0.97f),
        )
        assertEquals("Disk almost full", said(unknown))
    }

    @Test
    fun `unknown services still surface when the machine is fine`() {
        val unknown = state(services = listOf(service(HealthStatus.Unknown)), metrics = disk(0.5f))
        assertEquals("1 unknown", said(unknown))
    }

    @Test
    fun `stale outranks everything`() {
        // Nothing on screen can be believed, including the metrics — which the data layer
        // has already dropped by this point.
        assertEquals("Unverified", said(state(metrics = disk(0.97f), stale = true)))
    }

    @Test
    fun `memory and heat are reported too`() {
        val hot = HostMetrics(
            collectedAt = Instant.EPOCH,
            thermal = listOf(ThermalMetrics("coretemp", celsius = 90f, highCelsius = 87f)),
        )
        assertEquals("Running hot", hostConcern(hot))

        val tight = HostMetrics(
            collectedAt = Instant.EPOCH,
            memory = MemoryMetrics(totalBytes = 100, availableBytes = 3, usedBytes = 97),
        )
        assertEquals("Memory almost full", hostConcern(tight))
    }

    @Test
    fun `a busy processor is never a complaint`() {
        // A CPU at 100% is a transcode doing its job. Announcing it would cry wolf every
        // time somebody watched a film.
        val busy = HostMetrics(
            collectedAt = Instant.EPOCH,
            cpu = dev.cueseek.core.model.CpuMetrics(usagePercent = 100f, cores = 4, load1 = 8f),
        )
        assertNull(hostConcern(busy))
    }

    @Test
    fun `no metrics is not a complaint`() {
        assertNull(hostConcern(null))
        assertNull(hostConcern(HostMetrics(collectedAt = Instant.EPOCH)))
    }
}
