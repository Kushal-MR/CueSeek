package dev.cueseek.core.api

import dev.cueseek.core.model.ActionRisk
import dev.cueseek.core.model.ActionStatus
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.Platform
import dev.cueseek.core.model.Scope
import java.time.Instant
import kotlinx.coroutines.test.runTest
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The read and write paths against payloads taken from a live agent.
 *
 * These run on the JVM in milliseconds because `:core:api` is not an Android module, which
 * is the whole reason it is worth covering every documented shape rather than the happy
 * path (ADR-0013).
 */
class CueSeekApiTest {

    private val server = MockWebServer().apply { start() }

    @After
    fun tearDown() = server.close()

    private fun <T> ApiResult<T>.expectSuccess(): T = when (this) {
        is ApiResult.Success -> value
        is ApiResult.Failure -> error("expected success, got $error")
    }

    @Test
    fun `system parses identity and timestamp`() = runTest {
        server.enqueue(
            jsonResponse(
                200,
                """
                {
                  "host_id": "664917f8b739290c57d971481accef0e",
                  "hostname": "kushal-HP-paviliong6",
                  "agent_version": "m1.8-listenretry",
                  "api_version": "0.1.0",
                  "started_at": "2026-08-08T14:56:49.7878093Z"
                }
                """.trimIndent(),
            )
        )

        val system = server.api("csk_token").system().expectSuccess()

        assertEquals("664917f8b739290c57d971481accef0e", system.hostId.value)
        assertEquals("kushal-HP-paviliong6", system.hostname)
        assertEquals("0.1.0", system.apiVersion)
        assertEquals(Instant.parse("2026-08-08T14:56:49.7878093Z"), system.startedAt)
    }

    @Test
    fun `services parses capabilities actions and health`() = runTest {
        server.enqueue(jsonResponse(200, "[$JELLYFIN_SERVICE_JSON]"))

        val services = server.api("csk_token").services().expectSuccess()

        assertEquals(1, services.size)
        val jellyfin = services.single()
        assertEquals("jellyfin", jellyfin.id)
        assertEquals(listOf("health", "control"), jellyfin.capabilities.map { it.id })
        assertEquals(listOf("Health", "Controls"), jellyfin.capabilities.map { it.label })
        assertEquals(HealthStatus.Healthy, jellyfin.health.status)
        assertTrue(jellyfin.health.reachable)
        assertTrue(jellyfin.health.reasons.isEmpty())
        assertEquals(ActionRisk.Disruptive, jellyfin.actions.single().risk)
    }

    @Test
    fun `reported_status is absent for services that publish none`() = runTest {
        server.enqueue(jsonResponse(200, JELLYFIN_SERVICE_JSON))

        val service = server.api("csk_token").service("jellyfin").expectSuccess()

        // Jellyfin publishes no self-assessment. Absent must stay absent rather than
        // becoming an empty string that a UI would then render as a blank status line.
        assertNull(service.health.reportedStatus)
    }

    @Test
    fun `degraded and reachable are independent facts`() = runTest {
        server.enqueue(
            jsonResponse(
                200,
                """
                {
                  "id": "jellyfin", "name": "Jellyfin",
                  "capabilities": [], "actions": [],
                  "health": {
                    "status": "degraded",
                    "reachable": true,
                    "reported_status": "Unhealthy",
                    "reasons": [{"code": "auth_failed", "message": "API key rejected"}],
                    "observed_at": "2026-08-08T14:56:51Z"
                  }
                }
                """.trimIndent(),
            )
        )

        val health = server.api("csk_token").service("jellyfin").expectSuccess().health

        assertEquals(HealthStatus.Degraded, health.status)
        assertTrue("degraded does not imply unreachable", health.reachable)
        assertEquals("Unhealthy", health.reportedStatus)
        assertEquals("auth_failed", health.reasons.single().code)
    }

    @Test
    fun `devices parses scopes and an absent last_seen_at`() = runTest {
        server.enqueue(
            jsonResponse(
                200,
                """
                [
                  {
                    "id": "217f2f3dbf991996", "name": "Pixel 8", "platform": "android",
                    "scopes": ["read", "service.control"],
                    "created_at": "2026-08-08T05:38:14Z"
                  }
                ]
                """.trimIndent(),
            )
        )

        val device = server.api("csk_token").devices().expectSuccess().single()

        assertEquals(DeviceId("217f2f3dbf991996"), device.id)
        assertEquals(Platform.Android, device.platform)
        assertEquals(setOf(Scope.Read, Scope.ServiceControl), device.scopes)
        assertNull("never used means unknown, not epoch", device.lastSeenAt)
    }

    @Test
    fun `pairing sends no credential and returns a token`() = runTest {
        server.enqueue(
            jsonResponse(
                201,
                """
                {
                  "device": {
                    "id": "217f2f3dbf991996", "name": "Pixel 8", "platform": "android",
                    "scopes": ["read", "service.control"],
                    "created_at": "2026-08-08T05:38:14Z"
                  },
                  "token": "csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU"
                }
                """.trimIndent(),
            )
        )

        // A token is held, and must still not be sent: pairing is the one unauthenticated
        // operation, and re-pairing while already paired is a normal thing to do.
        val pairing = server.api("csk_existing")
            .pair(code = "D8JT-HUPV", deviceName = "Pixel 8")
            .expectSuccess()

        assertEquals("csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU", pairing.token.value)
        assertEquals("Pixel 8", pairing.device.name)

        val request = server.takeRequest()
        assertNull(request.headers["Authorization"])
        assertNull("the marker header must not reach the wire", request.headers["X-CueSeek-No-Auth"])
        val body = request.body?.utf8().orEmpty()
        assertTrue(body, body.contains(""""code":"D8JT-HUPV""""))
        assertTrue(body, body.contains(""""device_name":"Pixel 8""""))
        assertTrue(body, body.contains(""""platform":"android""""))
    }

    @Test
    fun `authenticated calls carry the bearer token`() = runTest {
        server.enqueue(jsonResponse(200, "[]"))

        server.api("csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU").services().expectSuccess()

        assertEquals(
            "Bearer csk_jsT8Bn6sLXCp1JXu6OtHmD2n6crjjjF7nw9nxVSQvHU",
            server.takeRequest().headers["Authorization"],
        )
    }

    @Test
    fun `invoking an action yields a correlation id`() = runTest {
        server.enqueue(
            jsonResponse(
                202,
                """
                {
                  "action_id": "2944a731d4a8af63",
                  "service_id": "jellyfin",
                  "action": "restart",
                  "status": "running",
                  "accepted_at": "2026-08-09T03:48:17.000890051Z"
                }
                """.trimIndent(),
            )
        )

        val accepted = server.api("csk_token").invokeAction("jellyfin", "restart").expectSuccess()

        assertEquals("2944a731d4a8af63", accepted.actionId.value)
        assertEquals(ActionStatus.Running, accepted.status)
        assertTrue("the 202 is an acknowledgement, not an outcome", !accepted.status.isTerminal)

        val request = server.takeRequest()
        assertEquals("POST", request.method)
        assertEquals("/v1/services/jellyfin/actions/restart", request.url.encodedPath)
    }

    @Test
    fun `revoking a device succeeds on an empty 204`() = runTest {
        server.enqueue(MockResponse.Builder().code(204).build())

        val result = server.api("csk_token").revokeDevice(DeviceId("217f2f3dbf991996"))

        assertTrue(result.isSuccess)
        assertEquals("DELETE", server.takeRequest().method)
    }

    @Test
    fun `fields added by a newer agent are ignored`() = runTest {
        server.enqueue(
            jsonResponse(
                200,
                """
                {
                  "host_id": "abc", "hostname": "box", "agent_version": "9.9.9",
                  "api_version": "0.2.0", "started_at": "2026-08-08T14:56:49Z",
                  "uptime_seconds": 42,
                  "cpu": { "load": 0.3 }
                }
                """.trimIndent(),
            )
        )

        // Version skew is permanent (ADR-0007). An agent that grows a field must not take
        // the client down with it.
        val system = server.api("csk_token").system().expectSuccess()

        assertEquals("0.2.0", system.apiVersion)
    }
}
