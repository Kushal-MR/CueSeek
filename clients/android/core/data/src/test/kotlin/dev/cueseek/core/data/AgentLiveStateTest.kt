package dev.cueseek.core.data

import dev.cueseek.core.api.AgentStream
import dev.cueseek.core.api.StreamFailure
import dev.cueseek.core.model.ActionInvocationId
import dev.cueseek.core.model.ActionProgress
import dev.cueseek.core.model.ActionStatus
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.Capability
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.HostId
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Scope
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.StreamEnvelope
import dev.cueseek.core.model.StreamEvent
import dev.cueseek.core.model.StreamStatus
import dev.cueseek.core.model.SystemInfo
import java.io.IOException
import java.time.Duration
import java.time.Instant
import java.util.concurrent.atomic.AtomicInteger
import kotlin.math.max
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The freshness watchdog and the connection loop, on virtual time.
 *
 * Every `delay` here is fake, so a three-minute Doze stall costs no wall-clock time and the
 * result is deterministic. That matters more than usual: the behaviour under test is
 * defined entirely by *elapsed time with nothing happening*, which is impossible to
 * observe reliably any other way.
 */
class AgentLiveStateTest {

    private val host = PairedHost(
        hostId = HostId("h1"),
        address = AgentAddress("100.92.18.125", 7777),
        hostname = "box",
        agentVersion = "m1.8",
        apiVersion = "0.1.0",
        deviceId = DeviceId("d1"),
        deviceName = "Pixel 8",
        scopes = setOf(Scope.Read, Scope.ServiceControl),
        pairedAt = Instant.EPOCH,
    )

    private val system = SystemInfo(
        hostId = HostId("h1"),
        hostname = "box",
        agentVersion = "m1.8",
        apiVersion = "0.1.0",
        startedAt = Instant.EPOCH,
    )

    private fun service(status: HealthStatus = HealthStatus.Healthy, id: String = "jellyfin") =
        Service(
            id = id,
            name = id.replaceFirstChar { it.uppercase() },
            capabilities = listOf(Capability("health", "Health")),
            health = Health(
                status = status,
                reachable = true,
                reportedStatus = null,
                reasons = emptyList(),
                observedAt = Instant.EPOCH,
            ),
            actions = emptyList(),
        )

    private fun snapshot(seq: Long = 0, services: List<Service> = listOf(service())) =
        StreamEnvelope(
            seq = seq,
            emittedAt = Instant.EPOCH,
            schemaVersion = "1",
            event = StreamEvent.Snapshot(system, services),
        )

    private fun heartbeat(seq: Long) = StreamEnvelope(
        seq = seq,
        emittedAt = Instant.EPOCH,
        schemaVersion = "1",
        event = StreamEvent.Heartbeat,
    )

    private fun streamOf(vararg flows: Flow<StreamEnvelope>): suspend (PairedHost) -> AgentStream {
        val queue = ArrayDeque(flows.toList())
        return { object : AgentStream {
            override fun events(): Flow<StreamEnvelope> =
                queue.removeFirstOrNull() ?: flow { throw StreamFailure(ApiError.Transport(IOException("exhausted"))) }
        } }
    }

    /** Collects into a list on the test scope's virtual clock. */
    private fun TestScope.liveState(
        streams: suspend (PairedHost) -> AgentStream?,
        snapshots: suspend (PairedHost) -> ApiResult<StreamEvent.Snapshot> = {
            ApiResult.Failure(ApiError.Transport(IOException("no refresh in this test")))
        },
        nudge: suspend (PairedHost) -> ApiResult<Unit> = { ApiResult.Success(Unit) },
    ): Pair<MutableList<AgentState>, AgentLiveState> {
        val seen = mutableListOf<AgentState>()
        val live = AgentLiveState(
            streams = streams,
            snapshots = snapshots,
            nudge = nudge,
            now = { Instant.ofEpochMilli(testScheduler.currentTime) },
            staleAfter = Duration.ofSeconds(30),
            checkInterval = Duration.ofSeconds(1),
        )
        return seen to live
    }

    @Test
    fun `a snapshot replaces state wholesale`() = runTest {
        val (seen, live) = liveState(streamOf(flow { emit(snapshot()); kotlinx.coroutines.awaitCancellation() }))
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(1_000)

        val latest = seen.last()
        assertEquals(listOf("jellyfin"), latest.services.map { it.id })
        assertEquals(HealthStatus.Healthy, latest.services.single().health.status)
        assertFalse(latest.freshness.isStale)
        job.cancel()
    }

    @Test
    fun `silence makes data unknown even though the transport says connected`() = runTest {
        val (seen, live) = liveState(streamOf(flow { emit(snapshot()); kotlinx.coroutines.awaitCancellation() }))
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(1_000)
        assertEquals(HealthStatus.Healthy, seen.last().services.single().health.status)

        // A7: with the screen off the stream freezes for 108-168s while still reporting
        // itself connected. Nothing arrives, and nothing tells us anything is wrong.
        advanceTimeBy(31_000)

        val latest = seen.last()
        assertTrue("the transport still believes it is open", latest.status is StreamStatus.Open)
        assertTrue("but the data must not be believed", latest.freshness.isStale)
        assertEquals(
            "showing stale green is worse than showing nothing",
            HealthStatus.Unknown,
            latest.services.single().health.status,
        )
        job.cancel()
    }

    @Test
    fun `a heartbeat is enough to keep data fresh`() = runTest {
        val (seen, live) = liveState(
            streamOf(
                flow {
                    emit(snapshot())
                    kotlinx.coroutines.delay(20_000)
                    emit(heartbeat(1))
                    kotlinx.coroutines.awaitCancellation()
                }
            )
        )
        val job = launch { live.stateFor(host).collect { seen += it } }

        // 25s in: past nothing, because the heartbeat at 20s reset the timer.
        advanceTimeBy(25_000)
        assertFalse(seen.last().freshness.isStale)

        // 52s in, so the watchdog tick at 51s has actually run: 31s since that heartbeat,
        // and heartbeats are the agent's promise that silence means something is wrong.
        advanceTimeBy(27_000)
        assertTrue(seen.last().freshness.isStale)
        job.cancel()
    }

    @Test
    fun `stale data keeps its observed timestamp so the user can judge it`() = runTest {
        val observed = Instant.parse("2026-08-10T09:00:00Z")
        val stamped = service().let { it.copy(health = it.health.copy(observedAt = observed)) }
        val (seen, live) = liveState(
            streamOf(flow { emit(snapshot(services = listOf(stamped))); kotlinx.coroutines.awaitCancellation() })
        )
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(32_000)

        val health = seen.last().services.single().health
        // Staleness is rendered from observed_at, never from arrival time.
        assertEquals(observed, health.observedAt)
        assertEquals(HealthStatus.Unknown, health.status)
        assertEquals("client_stale", health.reasons.first().code)
        job.cancel()
    }

    @Test
    fun `an update replaces one service by id`() = runTest {
        val (seen, live) = liveState(
            streamOf(
                flow {
                    emit(snapshot(services = listOf(service(id = "jellyfin"), service(id = "qbittorrent"))))
                    emit(
                        StreamEnvelope(
                            1, Instant.EPOCH, "1",
                            StreamEvent.ServiceUpdated(service(HealthStatus.Degraded, "jellyfin")),
                        )
                    )
                    kotlinx.coroutines.awaitCancellation()
                }
            )
        )
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(1_000)

        val services = seen.last().services.associateBy { it.id }
        assertEquals(2, services.size)
        assertEquals(HealthStatus.Degraded, services.getValue("jellyfin").health.status)
        assertEquals(HealthStatus.Healthy, services.getValue("qbittorrent").health.status)
        job.cancel()
    }

    @Test
    fun `a reconnect replaces state rather than merging it`() = runTest {
        val (seen, live) = liveState(
            streamOf(
                flow {
                    emit(snapshot(services = listOf(service(id = "jellyfin"), service(id = "qbittorrent"))))
                    throw StreamFailure(ApiError.Transport(IOException("frozen")))
                },
                // The second connection knows about one service only - qbittorrent was
                // removed from the agent's config while we were away.
                flow { emit(snapshot(services = listOf(service(id = "jellyfin")))); kotlinx.coroutines.awaitCancellation() },
            )
        )
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(5_000)

        assertEquals(listOf("jellyfin"), seen.last().services.map { it.id })
        job.cancel()
    }

    @Test
    fun `a sequence gap forces a reconnect instead of guessing`() = runTest {
        val (seen, live) = liveState(
            streamOf(
                flow {
                    emit(snapshot(seq = 0))
                    emit(heartbeat(1))
                    // 2 never arrives. There is no way to ask for it, so the only correct
                    // response is a reconnect, which by contract yields a fresh snapshot.
                    emit(heartbeat(5))
                    kotlinx.coroutines.awaitCancellation()
                },
                flow { emit(snapshot(seq = 0)); kotlinx.coroutines.awaitCancellation() },
            )
        )
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(5_000)

        assertTrue(
            "the gap should have been noticed and retried",
            seen.any { it.status is StreamStatus.Retrying },
        )
        assertTrue(seen.last().status is StreamStatus.Open)
        job.cancel()
    }

    @Test
    fun `seq restarting at zero is a new connection, not a gap`() = runTest {
        val (seen, live) = liveState(
            streamOf(
                flow {
                    emit(snapshot(seq = 0))
                    emit(heartbeat(1))
                    throw StreamFailure(ApiError.Transport(IOException("dropped")))
                },
                flow { emit(snapshot(seq = 0)); emit(heartbeat(1)); kotlinx.coroutines.awaitCancellation() },
            )
        )
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(5_000)

        assertEquals(1, seen.count { it.status is StreamStatus.Retrying })
        assertTrue(seen.last().status is StreamStatus.Open)
        job.cancel()
    }

    @Test
    fun `a rejected token stops rather than retrying forever`() = runTest {
        val (seen, live) = liveState(
            streamOf(
                flow {
                    throw StreamFailure(ApiError.Unauthorized(tokenWasSent = true, detail = "rejected"))
                }
            )
        )
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(60_000)

        val status = seen.last().status
        assertTrue("got $status", status is StreamStatus.Stopped)
        assertTrue((status as StreamStatus.Stopped).error is ApiError.Unauthorized)
        job.cancel()
    }

    @Test
    fun `a shutting-down agent is retried`() = runTest {
        val (seen, live) = liveState(
            streamOf(
                flow { throw StreamFailure(ApiError.NotImplemented("shutting down")) },
                flow { emit(snapshot()); kotlinx.coroutines.awaitCancellation() },
            )
        )
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(5_000)

        // 503 there means the agent is restarting, which is temporary by definition.
        assertTrue(seen.any { it.status is StreamStatus.Retrying })
        assertTrue(seen.last().status is StreamStatus.Open)
        job.cancel()
    }

    @Test
    fun `an action outcome is retained for correlation`() = runTest {
        val id = ActionInvocationId("2944a731d4a8af63")
        val (seen, live) = liveState(
            streamOf(
                flow {
                    emit(snapshot())
                    emit(
                        StreamEnvelope(
                            1, Instant.EPOCH, "1",
                            StreamEvent.ActionOutcome(
                                ActionProgress(
                                    actionId = id,
                                    serviceId = "jellyfin",
                                    action = "restart",
                                    status = ActionStatus.Succeeded,
                                    at = Instant.EPOCH,
                                    error = null,
                                )
                            ),
                        )
                    )
                    kotlinx.coroutines.awaitCancellation()
                }
            )
        )
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(1_000)

        // The stream is the only delivery mechanism - there is no endpoint to ask an
        // action how it went - so the outcome has to be kept when it passes by.
        assertEquals(ActionStatus.Succeeded, seen.last().outcomeOf(id)?.status)
        job.cancel()
    }

    @Test
    fun `state starts unknown rather than empty-and-healthy`() = runTest {
        val (seen, live) = liveState(streamOf(flow { kotlinx.coroutines.awaitCancellation() }))
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(100)

        // Before the first event nothing is known. Starting Fresh would briefly render an
        // empty service list as a healthy empty server.
        assertTrue(seen.first().freshness.isStale)
        job.cancel()
    }

    @Test
    fun `missing credentials stop the stream with an honest reason`() = runTest {
        val (seen, live) = liveState({ null })
        val job = launch { live.stateFor(host).collect { seen += it } }

        advanceTimeBy(1_000)

        val status = seen.last().status
        assertTrue("got $status", status is StreamStatus.Stopped)
        val error = (status as StreamStatus.Stopped).error
        assertTrue(error is ApiError.Unauthorized)
        assertFalse("nothing was sent, because there was nothing to send", (error as ApiError.Unauthorized).tokenWasSent)
        job.cancel()
    }

    @Test
    fun `backoff grows and then stops growing`() {
        val backoff = StreamBackoff()

        assertEquals(Duration.ofSeconds(1), backoff.delayFor(1))
        assertEquals(Duration.ofSeconds(2), backoff.delayFor(2))
        assertEquals(Duration.ofSeconds(4), backoff.delayFor(3))
        assertEquals(Duration.ofSeconds(8), backoff.delayFor(4))
        // Capped: a phone that has been in a tunnel for an hour should still come back
        // promptly, not fifteen minutes after the signal returns.
        assertEquals(Duration.ofSeconds(15), backoff.delayFor(5))
        assertEquals(Duration.ofSeconds(15), backoff.delayFor(50))
    }

    // ---------------------------------------------------------------- manual refresh

    /** A stream that connects, says nothing, and never drops. The A7 freeze, exactly. */
    private fun frozenStream() =
        streamOf(flow<StreamEnvelope> { kotlinx.coroutines.awaitCancellation() })

    @Test
    fun `a refresh that succeeds resets the freshness clock`() = runTest {
        // The case pull-to-refresh actually exists for: the connection still claims to be
        // open, nothing has arrived for a minute, and an HTTP round trip is the only way to
        // find out whether the agent is alive.
        val refreshes = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
        val (seen, live) = liveState(
            streams = frozenStream(),
            snapshots = { ApiResult.Success(StreamEvent.Snapshot(system, listOf(service()))) },
        )
        val job = launch { live.stateFor(host, refreshes).collect { seen += it } }

        advanceTimeBy(60_000)
        assertTrue("should have gone stale in silence", seen.last().freshness.isStale)

        refreshes.emit(Unit)
        // Past the observe window: the refresh now nudges the agent and waits for it to
        // look before reading, so the answer is deliberately later than it used to be.
        advanceTimeBy(5_000)

        assertFalse("a delivered snapshot is fresh data", seen.last().freshness.isStale)
        assertEquals(HealthStatus.Healthy, seen.last().services.single().health.status)
        job.cancel()
    }

    @Test
    fun `a refresh that fails leaves the data exactly as stale as it was`() = runTest {
        // The guarantee that matters most. Pulling must never be able to make a dead agent
        // look alive, so a failure moves nothing but the in-flight flag.
        val refreshes = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
        val (seen, live) = liveState(
            // A snapshot lands, then the stream freezes: there is real data to protect.
            streams = streamOf(flow { emit(snapshot()); kotlinx.coroutines.awaitCancellation() }),
            snapshots = { ApiResult.Failure(ApiError.Transport(IOException("unreachable"))) },
        )
        val job = launch { live.stateFor(host, refreshes).collect { seen += it } }

        advanceTimeBy(60_000)
        val before = seen.last().freshness

        refreshes.emit(Unit)
        advanceTimeBy(5_000)

        assertEquals(before, seen.last().freshness)
        assertTrue(seen.last().freshness.isStale)
        assertEquals(HealthStatus.Unknown, seen.last().services.single().health.status)
        assertFalse("the flag must clear even on failure", seen.last().refreshing)
        job.cancel()
    }

    @Test
    fun `refreshes never overlap, however hard the user pulls`() = runTest {
        // The guarantee is that two requests are never in flight at once, not that a burst
        // produces exactly one. A pull that arrives *after* a fetch began is a request for
        // state newer than that fetch will return, so one follow-up is correct rather than
        // wasteful — what must never happen is two talking to the agent simultaneously.
        val inFlight = AtomicInteger()
        val peak = AtomicInteger()
        val calls = AtomicInteger()
        val refreshes = MutableSharedFlow<Unit>(extraBufferCapacity = 8)
        val (seen, live) = liveState(
            streams = frozenStream(),
            snapshots = {
                calls.incrementAndGet()
                peak.getAndUpdate { max(it, inFlight.incrementAndGet()) }
                try {
                    // Long enough that the later pulls all land while this is outstanding.
                    delay(5_000)
                    ApiResult.Success(StreamEvent.Snapshot(system, listOf(service())))
                } finally {
                    inFlight.decrementAndGet()
                }
            },
        )
        val job = launch { live.stateFor(host, refreshes).collect { seen += it } }
        advanceTimeBy(1_000)

        repeat(5) { refreshes.emit(Unit) }
        advanceTimeBy(30_000)

        assertEquals("never two requests at once", 1, peak.get())
        assertTrue("five pulls must not become five requests: ${calls.get()}", calls.get() <= 2)
        assertFalse(seen.last().refreshing)
        job.cancel()
    }

    @Test
    fun `a refresh does not claim the stream is open`() = runTest {
        // A successful HTTP call says the agent answered; it says nothing about the stream.
        // Reporting Open here would tell the user a connection exists that does not, which
        // is the same class of lie as showing stale data as fresh.
        val refreshes = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
        val (seen, live) = liveState(
            streams = streamOf(flow { throw StreamFailure(ApiError.Transport(IOException("down"))) }),
            snapshots = { ApiResult.Success(StreamEvent.Snapshot(system, listOf(service()))) },
        )
        val job = launch { live.stateFor(host, refreshes).collect { seen += it } }
        advanceTimeBy(500)

        refreshes.emit(Unit)
        advanceTimeBy(5_000)

        val latest = seen.last()
        assertFalse("data arrived, so it is fresh", latest.freshness.isStale)
        assertTrue(
            "but the stream is not open: ${latest.status}",
            latest.status is StreamStatus.Retrying || latest.status is StreamStatus.Connecting,
        )
        job.cancel()
    }

    @Test
    fun `a refresh ends the reconnect backoff early`() = runTest {
        // By the fourth attempt the wait is 8s. A user who pulls should not be told to sit
        // through the rest of it.
        val connects = AtomicInteger()
        val refreshes = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
        val (seen, live) = liveState(
            streams = {
                connects.incrementAndGet()
                object : AgentStream {
                    override fun events(): Flow<StreamEnvelope> =
                        flow { throw StreamFailure(ApiError.Transport(IOException("down"))) }
                }
            },
            snapshots = { ApiResult.Failure(ApiError.Transport(IOException("also down"))) },
        )
        val job = launch { live.stateFor(host, refreshes).collect { seen += it } }

        // Let the backoff grow to a wait long enough to be interrupted meaningfully.
        advanceTimeBy(4_000)
        val before = connects.get()

        refreshes.emit(Unit)
        advanceTimeBy(200)

        assertTrue(
            "expected a reconnect attempt without waiting out the backoff",
            connects.get() > before,
        )
        job.cancel()
    }

    @Test
    fun `a refresh asks the agent to look before it reads`() = runTest {
        // The ordering is the feature. Reading first would return the agent's cache — the
        // state from before whatever the user is trying to verify (ADR-0003 Amendment 1).
        val order = mutableListOf<String>()
        val refreshes = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
        val (seen, live) = liveState(
            streams = frozenStream(),
            snapshots = {
                order += "read"
                ApiResult.Success(StreamEvent.Snapshot(system, listOf(service())))
            },
            nudge = {
                order += "nudge"
                ApiResult.Success(Unit)
            },
        )
        val job = launch { live.stateFor(host, refreshes).collect { seen += it } }
        advanceTimeBy(1_000)

        refreshes.emit(Unit)
        advanceTimeBy(5_000)

        assertEquals(listOf("nudge", "read"), order)
        assertFalse(seen.last().freshness.isStale)
        job.cancel()
    }

    @Test
    fun `an agent too old to know the refresh endpoint still gets read`() = runTest {
        // Forward compatibility in the awkward direction: a client newer than its agent.
        // The nudge 404s, the read still happens, and the refresh degrades to what it did
        // before the endpoint existed rather than failing outright.
        val refreshes = MutableSharedFlow<Unit>(extraBufferCapacity = 1)
        val (seen, live) = liveState(
            streams = frozenStream(),
            snapshots = { ApiResult.Success(StreamEvent.Snapshot(system, listOf(service()))) },
            nudge = { ApiResult.Failure(ApiError.NotFound("no such endpoint")) },
        )
        val job = launch { live.stateFor(host, refreshes).collect { seen += it } }

        advanceTimeBy(60_000)
        assertTrue(seen.last().freshness.isStale)

        refreshes.emit(Unit)
        advanceTimeBy(5_000)

        assertFalse("the read still delivered data", seen.last().freshness.isStale)
        job.cancel()
    }
}
