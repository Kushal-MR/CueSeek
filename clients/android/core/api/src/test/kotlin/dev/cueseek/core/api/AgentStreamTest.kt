package dev.cueseek.core.api

import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.DeviceToken
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.StreamEnvelope
import dev.cueseek.core.model.StreamEvent
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.runTest
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The SSE transport against real frames.
 *
 * `:core:data`'s state-machine tests stub the stream out entirely, which is the right way
 * to test time-dependent behaviour but proves nothing about framing, headers, or how a
 * refusal is reported. This is where those are checked.
 */
class AgentStreamTest {

    private val server = MockWebServer().apply { start() }

    @After
    fun tearDown() = server.close()

    private fun stream(token: String? = "csk_token"): AgentStream =
        CueSeekApiFactory.createStream(
            address = AgentAddress(server.hostName, server.port),
            tokens = token?.let { t -> TokenProvider { DeviceToken(t) } } ?: TokenProvider.None,
        )

    private fun sse(body: String): MockResponse = MockResponse.Builder()
        .code(200)
        .setHeader("Content-Type", "text/event-stream")
        .setHeader("Cache-Control", "no-cache")
        .body(body)
        .build()

    /**
     * Builds one SSE frame.
     *
     * Explicit `\n\n` rather than a raw string literal, because the blank line **is** the
     * frame terminator and `trimIndent()` deletes trailing blank lines — which silently
     * produces a frame the reader waits forever to finish.
     */
    private fun frame(type: String, data: String) = "event: $type\ndata: $data\n\n"

    private val snapshotFrame = frame(
        "snapshot",
        """{"type":"snapshot","seq":0,"emitted_at":"2026-08-08T14:56:53.7967252Z","schema_version":"1","snapshot":{"system":{"host_id":"h1","hostname":"box","agent_version":"m1.8","api_version":"0.1.0","started_at":"2026-08-08T14:56:49Z"},"services":[{"id":"jellyfin","name":"Jellyfin","capabilities":[{"id":"health","label":"Health"}],"actions":[],"health":{"status":"healthy","reachable":true,"reasons":[],"observed_at":"2026-08-08T14:56:51Z"}}]}}""",
    )

    /** Collects until the stream fails, returning what arrived and why it stopped. */
    private suspend fun AgentStream.drain(): Pair<List<StreamEnvelope>, ApiError?> {
        var error: ApiError? = null
        val events = events()
            .catch { if (it is StreamFailure) error = it.error else throw it }
            .toList()
        return events to error
    }

    @Test
    fun `a snapshot frame becomes a snapshot event`() = runTest {
        server.enqueue(sse(snapshotFrame))

        val (events, _) = stream().drain()

        val envelope = events.first()
        assertEquals(0L, envelope.seq)
        assertEquals("1", envelope.schemaVersion)
        val snapshot = envelope.event as StreamEvent.Snapshot
        assertEquals("box", snapshot.system.hostname)
        assertEquals("jellyfin", snapshot.services.single().id)
        assertEquals(HealthStatus.Healthy, snapshot.services.single().health.status)
    }

    @Test
    fun `heartbeats and updates parse in sequence`() = runTest {
        server.enqueue(
            sse(
                snapshotFrame +
                    frame(
                        "heartbeat",
                        """{"type":"heartbeat","seq":1,"emitted_at":"2026-08-08T14:57:08Z","schema_version":"1"}""",
                    ) +
                    frame(
                        "service_updated",
                        """{"type":"service_updated","seq":2,"emitted_at":"2026-08-08T14:57:09Z","schema_version":"1","service":{"id":"jellyfin","name":"Jellyfin","capabilities":[],"actions":[],"health":{"status":"degraded","reachable":true,"reasons":[{"code":"auth_failed","message":"API key rejected"}],"observed_at":"2026-08-08T14:57:09Z"}}}""",
                    )
            )
        )

        val (events, _) = stream().drain()

        assertEquals(listOf(0L, 1L, 2L), events.map { it.seq })
        assertTrue(events[1].event is StreamEvent.Heartbeat)
        val updated = events[2].event as StreamEvent.ServiceUpdated
        assertEquals(HealthStatus.Degraded, updated.service.health.status)
        assertEquals("auth_failed", updated.service.health.reasons.single().code)
    }

    @Test
    fun `an action outcome carries its correlation id`() = runTest {
        server.enqueue(
            sse(
                frame(
                    "action_progress",
                    """{"type":"action_progress","seq":7,"emitted_at":"2026-08-09T03:48:20Z","schema_version":"1","action_progress":{"action_id":"2944a731d4a8af63","service_id":"jellyfin","action":"restart","status":"succeeded","at":"2026-08-09T03:48:20Z"}}""",
                )
            )
        )

        val (events, _) = stream().drain()

        val outcome = events.single().event as StreamEvent.ActionOutcome
        assertEquals("2944a731d4a8af63", outcome.progress.actionId.value)
        assertTrue(outcome.progress.status.isTerminal)
        assertNull(outcome.progress.error)
    }

    @Test
    fun `an event type this build predates is kept, not dropped`() = runTest {
        server.enqueue(
            sse(
                frame(
                    "host_metrics",
                    """{"type":"host_metrics","seq":3,"emitted_at":"2026-08-08T14:57:10Z","schema_version":"2"}""",
                )
            )
        )

        val (events, _) = stream().drain()

        // A silently discarded event is indistinguishable from a quiet stream, and quiet is
        // the one thing this client must never misread.
        val unrecognised = events.single().event as StreamEvent.Unrecognised
        assertEquals("host_metrics", unrecognised.type)
        assertEquals("2", events.single().schemaVersion)
    }

    @Test
    fun `a frame that cannot be parsed is skipped rather than fatal`() = runTest {
        server.enqueue(
            sse(
                frame("service_updated", """{"type":"service_updated","seq":1,""") + snapshotFrame
            )
        )

        val (events, _) = stream().drain()

        // Failing the connection over one bad frame would produce a reconnect loop against
        // an agent that is otherwise working; the next snapshot rebuilds state anyway.
        assertEquals(1, events.size)
        assertTrue(events.single().event is StreamEvent.Snapshot)
    }

    @Test
    fun `the stream carries the bearer token`() = runTest {
        server.enqueue(sse(snapshotFrame))

        stream("csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU").drain()

        val request = server.takeRequest()
        assertEquals(
            "Bearer csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU",
            request.headers["Authorization"],
        )
        assertEquals("/v1/stream", request.url.encodedPath)
        assertEquals("text/event-stream", request.headers["Accept"])
    }

    @Test
    fun `a rejected token fails the stream as unauthorized`() = runTest {
        server.enqueue(
            MockResponse.Builder()
                .code(401)
                .setHeader("Content-Type", "application/problem+json")
                .body(
                    problem(
                        "unauthorized", "Unauthorized", 401,
                        "The device token was not accepted.",
                    )
                )
                .build()
        )

        val (_, error) = stream("csk_dead").drain()

        assertTrue("got $error", error is ApiError.Unauthorized)
        // The collector uses this to stop retrying: a dead token cannot be revived by
        // reconnecting, and the loop would otherwise 401 forever.
        assertTrue((error as ApiError.Unauthorized).tokenWasSent)
    }

    @Test
    fun `a shutting-down agent fails as not-implemented, which is retryable`() = runTest {
        server.enqueue(
            MockResponse.Builder()
                .code(503)
                .setHeader("Content-Type", "application/problem+json")
                .body(problem("not-implemented", "Not implemented", 503, "shutting down"))
                .build()
        )

        val (_, error) = stream().drain()

        assertTrue("got $error", error is ApiError.NotImplemented)
    }

    @Test
    fun `the agent closing the stream is reported as a failure to reconnect from`() = runTest {
        server.enqueue(sse(snapshotFrame))

        val (events, error) = stream().drain()

        // The agent closes cleanly when it shuts down, and drops clients too slow to drain
        // its 16-event buffer. Both mean "reconnect", so a clean close is not a completed
        // stream from the collector's point of view.
        assertEquals(1, events.size)
        assertTrue("got $error", error is ApiError.Transport)
    }
}
