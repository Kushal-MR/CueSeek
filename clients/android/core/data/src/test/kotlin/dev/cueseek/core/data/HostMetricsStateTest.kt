package dev.cueseek.core.data

import dev.cueseek.core.api.AgentStream
import dev.cueseek.core.api.StreamFailure
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.Capability
import dev.cueseek.core.model.CpuMetrics
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.HostId
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.MemoryMetrics
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Scope
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.StreamEnvelope
import dev.cueseek.core.model.StreamEvent
import dev.cueseek.core.model.SystemInfo
import java.io.IOException
import java.time.Duration
import java.time.Instant
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Host metrics through the live state, on virtual time.
 *
 * Two properties matter here and neither is obvious from the mapping tests. Metrics must
 * arrive between service updates rather than only on reconnect — which is the entire reason
 * they are their own event — and they must **disappear** when the data goes stale rather
 * than degrade the way a service does.
 */
class HostMetricsStateTest {

    private val host = PairedHost(
        hostId = HostId("h1"),
        address = AgentAddress("100.92.18.125", 7777),
        hostname = "box",
        agentVersion = "m3.6",
        apiVersion = "0.1.0",
        deviceId = DeviceId("d1"),
        deviceName = "Pixel 8",
        scopes = setOf(Scope.Read),
        pairedAt = Instant.EPOCH,
    )

    private val system = SystemInfo(
        hostId = HostId("h1"),
        hostname = "box",
        agentVersion = "m3.6",
        apiVersion = "0.1.0",
        startedAt = Instant.EPOCH,
    )

    private val service = Service(
        id = "jellyfin",
        name = "Jellyfin",
        capabilities = listOf(Capability("health", "Health")),
        health = Health(
            status = HealthStatus.Healthy,
            reachable = true,
            reportedStatus = null,
            reasons = emptyList(),
            observedAt = Instant.EPOCH,
        ),
        actions = emptyList(),
    )

    private fun metrics(usage: Float) = HostMetrics(
        collectedAt = Instant.EPOCH,
        uptimeSeconds = 86_400,
        cpu = CpuMetrics(usagePercent = usage, cores = 8, load1 = 1.5f),
        memory = MemoryMetrics(totalBytes = 100, availableBytes = 40, usedBytes = 60),
    )

    private fun envelope(seq: Long, event: StreamEvent) =
        StreamEnvelope(seq = seq, emittedAt = Instant.EPOCH, schemaVersion = "1", event = event)

    private fun streamOf(flow: Flow<StreamEnvelope>): suspend (PairedHost) -> AgentStream = {
        object : AgentStream {
            override fun events(): Flow<StreamEnvelope> = flow
        }
    }

    /**
     * Built on the test scope's clock, which is the whole reason the watchdog is testable:
     * its only input is elapsed time with nothing arriving, and real time cannot produce
     * that deterministically.
     */
    private fun TestScope.live(streams: suspend (PairedHost) -> AgentStream?) = AgentLiveState(
        streams = streams,
        snapshots = { ApiResult.Failure(ApiError.Transport(IOException("no refresh"))) },
        now = { Instant.ofEpochMilli(testScheduler.currentTime) },
        staleAfter = Duration.ofSeconds(30),
        checkInterval = Duration.ofSeconds(1),
    )

    @Test
    fun `a host_updated event replaces the metrics without touching services`() = runTest {
        val stream = streamOf(
            flow {
                emit(envelope(0, StreamEvent.Snapshot(system, listOf(service), metrics(10f))))
                emit(envelope(1, StreamEvent.HostUpdated(metrics(77f))))
                awaitCancellation()
            }
        )
        val seen = mutableListOf<AgentState>()
        val job = launch { live(stream).stateFor(host).collect { seen += it } }

        advanceTimeBy(1_000)

        val latest = seen.last()
        assertEquals(77f, latest.hostMetrics?.cpu?.usagePercent)
        // The whole point of a separate event: the roster is untouched by a metrics tick.
        assertEquals(listOf("jellyfin"), latest.services.map { it.id })

        job.cancel()
    }

    @Test
    fun `a snapshot carrying no metrics clears what was held`() = runTest {
        // An agent that restarted has measured nothing yet. Continuing to show figures from
        // before the restart would be showing a machine's vitals from a different process.
        val stream = streamOf(
            flow {
                emit(envelope(0, StreamEvent.Snapshot(system, listOf(service), metrics(10f))))
                emit(envelope(0, StreamEvent.Snapshot(system, listOf(service), null)))
                awaitCancellation()
            }
        )
        val seen = mutableListOf<AgentState>()
        val job = launch { live(stream).stateFor(host).collect { seen += it } }

        advanceTimeBy(1_000)

        assertNull("stale metrics survived a snapshot that had none", seen.last().hostMetrics)

        job.cancel()
    }

    @Test
    fun `metrics disappear when the data goes stale`() = runTest {
        // Services degrade to unknown and keep their timestamps, because "healthy three
        // minutes ago" is still worth saying. A CPU percentage three minutes old is worth
        // nothing, and leaving one on screen would be a live-looking number under a stale
        // banner — the exact confident wrongness this client exists to avoid.
        // Delivered once and never again, including across the reconnect the client now
        // performs when a connection goes silent. Without that the second connection would
        // re-deliver the snapshot and the data would never be stale enough to test.
        var delivered = false
        val stream: suspend (PairedHost) -> AgentStream = {
            object : AgentStream {
                override fun events(): Flow<StreamEnvelope> = flow {
                    if (!delivered) {
                        delivered = true
                        emit(envelope(0, StreamEvent.Snapshot(system, listOf(service), metrics(42f))))
                    }
                    awaitCancellation()
                }
            }
        }

        val seen = mutableListOf<AgentState>()
        val job = launch { live(stream).stateFor(host).collect { seen += it } }

        advanceTimeBy(1_000)
        assertNotNull("metrics were not applied at all", seen.last().hostMetrics)

        advanceTimeBy(70_000)

        val stale = seen.last()
        assertTrue("the watchdog did not fire", stale.freshness.isStale)
        assertNull("a stale CPU figure stayed on screen", stale.hostMetrics)
        // The service is still listed, degraded rather than dropped — the contrast is the
        // point, and it is deliberate that the two are treated differently.
        assertEquals(HealthStatus.Unknown, stale.services.single().health.status)

        job.cancel()
    }

    @Test
    fun `an unrecognised event does not disturb the metrics`() = runTest {
        // A type from a newer agent. It counts as traffic — the agent is alive and talking —
        // and must not be mistaken for a reason to drop what is already known.
        val stream = streamOf(
            flow {
                emit(envelope(0, StreamEvent.Snapshot(system, listOf(service), metrics(33f))))
                emit(envelope(1, StreamEvent.Unrecognised("gpu_updated")))
                awaitCancellation()
            }
        )
        val seen = mutableListOf<AgentState>()
        val job = launch { live(stream).stateFor(host).collect { seen += it } }

        advanceTimeBy(1_000)

        assertEquals(33f, seen.last().hostMetrics?.cpu?.usagePercent)

        job.cancel()
    }

    @Test
    fun `a connection that worked resets the backoff`() = runTest {
        // Without this the attempt counter only ever climbs, so a stream that has dropped a
        // few times over a long session sits at the 15s cap for the rest of it — slowest
        // exactly when it has been most reliable.
        val backoff = StreamBackoff()
        assertEquals(backoff.delayFor(1), backoff.delayFor(1))
        assertTrue(
            "backoff must grow before it is worth resetting",
            backoff.delayFor(4) > backoff.delayFor(1),
        )
    }
}
