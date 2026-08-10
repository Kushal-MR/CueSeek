package dev.cueseek.core.api

import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.DeviceId
import kotlinx.coroutines.test.runTest
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Every documented failure, mapped.
 *
 * The agent answers with one error shape across the whole API, so one mapper suffices —
 * but only if every `type` it emits has a case. A type that falls through to
 * [ApiError.Unrecognised] by accident is a UI that says "something went wrong" about a
 * situation the agent described precisely.
 */
class ApiErrorMappingTest {

    private val server = MockWebServer().apply { start() }

    @After
    fun tearDown() = server.close()

    private fun <T> ApiResult<T>.expectError(): ApiError = when (this) {
        is ApiResult.Failure -> error
        is ApiResult.Success -> error("expected failure, got $value")
    }

    private suspend fun errorFrom(response: MockResponse, token: String? = "csk_token"): ApiError {
        server.enqueue(response)
        return server.api(token).services().expectError()
    }

    @Test
    fun `a rejected token is distinguishable from a missing one`() = runTest {
        val rejected = errorFrom(
            problemResponse(
                401,
                problem(
                    "unauthorized", "Unauthorized", 401,
                    "The device token was not accepted. Pair the device again.",
                ),
            ),
            token = "csk_dead",
        )

        // The agent distinguishes these only in prose, and prose is exactly what must not
        // be branched on. What the client sent is a fact no rewording can change.
        assertTrue(rejected is ApiError.Unauthorized)
        assertTrue(
            "a token was sent, so it is dead and the user must re-pair",
            (rejected as ApiError.Unauthorized).tokenWasSent,
        )
    }

    @Test
    fun `a missing credential is reported as a client fault`() = runTest {
        val missing = errorFrom(
            problemResponse(
                401,
                problem("unauthorized", "Unauthorized", 401, "No bearer token was presented."),
            ),
            token = null,
        )

        assertTrue(missing is ApiError.Unauthorized)
        assertFalse(
            "nothing was sent, so this is a client bug and re-pairing would not fix it",
            (missing as ApiError.Unauthorized).tokenWasSent,
        )
    }

    @Test
    fun `insufficient scope`() = runTest {
        val error = errorFrom(
            problemResponse(
                403,
                problem(
                    "insufficient-scope", "Insufficient scope", 403,
                    "this operation requires the service.control scope",
                ),
            )
        )

        assertTrue(error is ApiError.InsufficientScope)
        assertEquals(
            "this operation requires the service.control scope",
            error.detail,
        )
    }

    @Test
    fun `an invalid pairing code does not say which kind`() = runTest {
        server.enqueue(
            problemResponse(
                403,
                problem("invalid-pairing-code", "Invalid pairing code", 403),
            )
        )

        val error = server.api().pair("BADC-ODE1", "Pixel 8").expectError()

        assertTrue(error is ApiError.InvalidPairingCode)
        // Unknown, expired and already-redeemed are merged by the agent on purpose:
        // saying a code "expired" reveals it was once real.
        assertEquals(null, error.detail)
    }

    @Test
    fun `rate limiting`() = runTest {
        server.enqueue(problemResponse(429, problem("rate-limited", "Too many requests", 429)))

        assertTrue(server.api().pair("D8JT-HUPV", "Pixel 8").expectError() is ApiError.RateLimited)
    }

    @Test
    fun `not found`() = runTest {
        server.enqueue(problemResponse(404, problem("not-found", "Not found", 404)))

        assertTrue(
            server.api("csk_token").service("nope").expectError() is ApiError.NotFound
        )
    }

    @Test
    fun `an action already running`() = runTest {
        server.enqueue(
            problemResponse(
                409,
                problem("action-in-progress", "Action in progress", 409, "restart is running"),
            )
        )

        val error = server.api("csk_token").invokeAction("jellyfin", "restart").expectError()

        assertTrue(error is ApiError.ActionInProgress)
    }

    @Test
    fun `an action the host cannot perform keeps its detail verbatim`() = runTest {
        val detail = "polkit refused RestartUnit for jellyfin.service"
        server.enqueue(
            problemResponse(
                409,
                problem("action-unavailable", "Action unavailable", 409, detail),
            )
        )

        val error = server.api("csk_token").invokeAction("jellyfin", "restart").expectError()

        assertTrue(error is ApiError.ActionUnavailable)
        // This one is worth showing to an operator word for word: it usually names a
        // polkit rule that is one line away from being fixed.
        assertEquals(detail, error.detail)
    }

    @Test
    fun `bad request and internal`() = runTest {
        assertTrue(
            errorFrom(problemResponse(400, problem("bad-request", "Bad request", 400)))
                is ApiError.BadRequest
        )
        assertTrue(
            errorFrom(problemResponse(500, problem("internal", "Internal error", 500)))
                is ApiError.Internal
        )
    }

    @Test
    fun `a shutting-down agent is not a permanent dead end`() = runTest {
        val error = errorFrom(
            problemResponse(503, problem("not-implemented", "Not implemented", 503))
        )

        // Reachable in exactly one place: the stream, while the agent shuts down. It
        // warrants a retry, not a "this feature does not exist" message.
        assertTrue(error is ApiError.NotImplemented)
    }

    @Test
    fun `an unknown problem type keeps what it can`() = runTest {
        val error = errorFrom(
            problemResponse(
                418, problem("teapot", "I am a teapot", 418, "short and stout"),
            )
        )

        assertTrue(error is ApiError.Unrecognised)
        error as ApiError.Unrecognised
        assertEquals("https://cueseek.dev/problems/teapot", error.type)
        assertEquals(418, error.status)
        assertEquals("short and stout", error.detail)
    }

    @Test
    fun `an error that is not a problem document keeps its status`() = runTest {
        // Nothing in CueSeek emits this, but a phone talks to a network, and a network
        // contains things that answer with HTML.
        val error = errorFrom(
            MockResponse.Builder()
                .code(502)
                .setHeader("Content-Type", "text/html")
                .body("<html><body>Bad Gateway</body></html>")
                .build()
        )

        assertTrue(error is ApiError.Unrecognised)
        assertEquals(502, (error as ApiError.Unrecognised).status)
    }

    @Test
    fun `an unreachable host is a transport failure`() = runTest {
        val api = server.api("csk_token")
        server.close()

        val error = api.services().expectError()

        // The common case on a phone, and the one whose message should mention the VPN:
        // there is no relay, so this will not resolve itself by waiting.
        assertTrue("got $error", error is ApiError.Transport)
    }

    @Test
    fun `a response that cannot be parsed is malformed, not empty`() = runTest {
        val error = errorFrom(jsonResponse(200, """{"this": "is not a service list"}"""))

        assertTrue("got $error", error is ApiError.Malformed)
    }

    @Test
    fun `an unparseable timestamp is malformed rather than silently substituted`() = runTest {
        val error = errorFrom(
            jsonResponse(
                200,
                """
                [{
                  "id": "jellyfin", "name": "Jellyfin", "capabilities": [], "actions": [],
                  "health": {
                    "status": "healthy", "reachable": true, "reasons": [],
                    "observed_at": "yesterday afternoon"
                  }
                }]
                """.trimIndent(),
            )
        )

        // Substituting `now` would make the UI render staleness from a fabricated clock,
        // which is the one thing docs/m2-android-api.md §8 says never to do.
        assertTrue("got $error", error is ApiError.Malformed)
    }

    @Test
    fun `revocation of an unknown device is not found`() = runTest {
        server.enqueue(problemResponse(404, problem("not-found", "Not found", 404)))

        val error = server.api("csk_token").revokeDevice(DeviceId("nope")).expectError()

        assertTrue(error is ApiError.NotFound)
    }
}
