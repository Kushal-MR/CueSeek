package dev.cueseek.core.api

import dev.cueseek.core.model.ActionRisk
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.HealthStatus
import kotlinx.coroutines.test.runTest
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * What happens when the agent is newer than the app.
 *
 * This is a permanent condition, not a transitional one (ADR-0007), and it is the reason
 * the enum-valued fields are strings on the wire: the generated enums throw, and one
 * unrecognised value would otherwise cost the whole response rather than one field.
 */
class VersionSkewTest {

    private val server = MockWebServer().apply { start() }

    @After
    fun tearDown() = server.close()

    private fun <T> ApiResult<T>.expectSuccess(): T = when (this) {
        is ApiResult.Success -> value
        is ApiResult.Failure -> error("expected success, got $error")
    }

    @Test
    fun `a health status this build predates renders as unknown`() = runTest {
        server.enqueue(
            jsonResponse(
                200,
                """
                {
                  "id": "jellyfin", "name": "Jellyfin", "capabilities": [], "actions": [],
                  "health": {
                    "status": "quarantined", "reachable": true, "reasons": [],
                    "observed_at": "2026-08-08T14:56:51Z"
                  }
                }
                """.trimIndent(),
            )
        )

        val health = server.api("csk_token").service("jellyfin").expectSuccess().health

        // "I don't know" is the honest rendering of a status this build has never heard
        // of, and it is a state the design system already has to draw (ADR-0008).
        assertEquals(HealthStatus.Unknown, health.status)
    }

    @Test
    fun `a risk level this build predates is treated as dangerous`() = runTest {
        server.enqueue(
            jsonResponse(
                200,
                """
                {
                  "id": "host", "name": "Host", "capabilities": [],
                  "actions": [
                    { "id": "wipe", "label": "Wipe", "risk": "catastrophic" }
                  ],
                  "health": {
                    "status": "healthy", "reachable": true, "reasons": [],
                    "observed_at": "2026-08-08T14:56:51Z"
                  }
                }
                """.trimIndent(),
            )
        )

        val action = server.api("csk_token").service("host").expectSuccess().actions.single()

        assertEquals(ActionRisk.Unrecognised, action.risk)
        assertTrue(action.risk.requiresConfirmation)
        // Defaulting an unknown risk to `safe` would invoke a future action of unknown
        // consequence with no prompt, on every client that predates it.
        assertTrue(action.risk.requiresEmphaticConfirmation)
    }

    @Test
    fun `a capability this build cannot render still arrives with its label`() = runTest {
        server.enqueue(
            jsonResponse(
                200,
                """
                {
                  "id": "immich", "name": "Immich",
                  "capabilities": [{ "id": "immich_jobs", "label": "Immich Jobs" }],
                  "actions": [],
                  "health": {
                    "status": "healthy", "reachable": true, "reasons": [],
                    "observed_at": "2026-08-08T14:56:51Z"
                  }
                }
                """.trimIndent(),
            )
        )

        val capability = server.api("csk_token").service("immich").expectSuccess()
            .capabilities.single()

        // The label is why an old client can render "Immich Jobs - update CueSeek to view
        // this" rather than an empty box. The renderer lookup is P5's job; the data
        // reaching it intact is this layer's.
        assertEquals("immich_jobs", capability.id)
        assertEquals("Immich Jobs", capability.label)
    }

    @Test
    fun `an unknown scope is dropped rather than failing the device list`() = runTest {
        server.enqueue(
            jsonResponse(
                200,
                """
                [{
                  "id": "d1", "name": "Pixel 8", "platform": "foldable",
                  "scopes": ["read", "host.thermal"],
                  "created_at": "2026-08-08T05:38:14Z"
                }]
                """.trimIndent(),
            )
        )

        val device = server.api("csk_token").devices().expectSuccess().single()

        // Dropping is safe in this direction only: an unknown scope can cause the client
        // to hide UI it might have shown, never to offer something the agent will refuse.
        assertEquals(setOf(dev.cueseek.core.model.Scope.Read), device.scopes)
        assertEquals(dev.cueseek.core.model.Platform.Unknown, device.platform)
    }
}
