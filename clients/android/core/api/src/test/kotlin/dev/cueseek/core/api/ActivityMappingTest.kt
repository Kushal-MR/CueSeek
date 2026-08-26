package dev.cueseek.core.api

import dev.cueseek.core.model.ApiResult
import kotlinx.coroutines.test.runTest
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The activity payloads, crossing the wire.
 *
 * The rule under test throughout is **absent is not empty**. The agent omits a field it
 * could not observe and sends a zeroed one it observed as quiet, and that difference has
 * to survive the mapping intact — a default of `emptyList()` anywhere in this layer would
 * turn "we could not ask" into "nothing is happening" for every screen downstream.
 */
class ActivityMappingTest {

    private val server = MockWebServer().apply { start() }

    @After
    fun tearDown() = server.close()

    private fun serviceJson(extra: String) = """
        {
          "id": "jellyfin",
          "name": "Jellyfin",
          "capabilities": [{ "id": "health", "label": "Health" }],
          "actions": [],
          "health": {
            "status": "healthy",
            "reachable": true,
            "reasons": [],
            "observed_at": "2026-08-08T14:56:51.7940666Z"
          }$extra
        }
    """.trimIndent()

    private suspend fun singleService() = when (val r = server.api("csk_token").services()) {
        is ApiResult.Success -> r.value.single()
        is ApiResult.Failure -> error("expected success, got ${'$'}{r.error}")
    }

    @Test
    fun `an omitted capability maps to null, not to an empty payload`() = runTest {
        server.enqueue(jsonResponse(200, "[${serviceJson("")}]"))

        val service = singleService()

        assertNull("now_playing was absent and must stay absent", service.nowPlaying)
        assertNull("transfers was absent and must stay absent", service.transfers)
    }

    @Test
    fun `an observed but idle server maps to a present, empty payload`() = runTest {
        val idle = ""","now_playing": { "sessions": 0, "transcoding": 0, "items": [] }"""
        server.enqueue(jsonResponse(200, "[${serviceJson(idle)}]"))

        val playing = singleService().nowPlaying
            ?: error("now_playing was present on the wire and must survive mapping")

        assertEquals(0, playing.sessions)
        assertTrue(playing.items.isEmpty())
        assertTrue("an observed quiet server is idle", playing.idle)
    }

    @Test
    fun `a playback session carries everything the agent sent`() = runTest {
        val body = """,
          "now_playing": {
            "sessions": 2,
            "transcoding": 1,
            "items": [{
              "id": "s1",
              "title": "The Bear",
              "subtitle": "S2E7",
              "user": "kushal",
              "client": "Living Room TV",
              "position_seconds": 754,
              "duration_seconds": 2900,
              "paused": false,
              "transcoding": true
            }]
          }"""
        server.enqueue(jsonResponse(200, "[${serviceJson(body)}]"))

        val playing = singleService().nowPlaying!!

        assertEquals(2, playing.sessions)
        assertEquals(1, playing.transcoding)

        val item = playing.items.single()
        assertEquals("The Bear", item.title)
        assertEquals("S2E7", item.subtitle)
        assertEquals("Living Room TV", item.client)
        assertTrue(item.transcoding)
        assertEquals(0.26f, item.progress!!, 0.01f)
    }

    @Test
    fun `optional session fields the agent omitted stay null`() = runTest {
        // A live stream: a position but no end, no reported user, no device name.
        val body = """,
          "now_playing": {
            "sessions": 1,
            "transcoding": 0,
            "items": [{ "id": "s1", "title": "BBC News", "paused": false, "transcoding": false }]
          }"""
        server.enqueue(jsonResponse(200, "[${serviceJson(body)}]"))

        val item = singleService().nowPlaying!!.items.single()

        assertNull(item.subtitle)
        assertNull(item.user)
        assertNull(item.client)
        assertNull(item.positionSeconds)
        assertNull(item.durationSeconds)
        // And therefore no progress claim at all, rather than a bar at the far left.
        assertNull(item.progress)
    }

    @Test
    fun `transfers carry aggregate rates and a verbatim state`() = runTest {
        val body = """,
          "transfers": {
            "active": 2,
            "total": 47,
            "download_rate_bytes": 12000000,
            "upload_rate_bytes": 800000,
            "items": [{
              "id": "abc",
              "name": "ubuntu.iso",
              "state": "stalledDL",
              "progress": 0.42,
              "size_bytes": 4700000000,
              "eta_seconds": 900
            }]
          }"""
        server.enqueue(jsonResponse(200, "[${serviceJson(body)}]"))

        val transfers = singleService().transfers!!

        assertEquals(2, transfers.active)
        assertEquals(47, transfers.total)
        assertEquals(12_000_000L, transfers.downloadRateBytes)

        val item = transfers.items.single()
        // Unmapped: the difference between stalled and queued is what tells an operator
        // whether to care, and this client must display words it has never seen.
        assertEquals("stalledDL", item.state)
        assertEquals(4_700_000_000L, item.sizeBytes)
        assertEquals(900, item.etaSeconds)
        assertNull("an omitted per-item rate stays null", item.downloadRateBytes)
    }

    @Test
    fun `a state this build has never seen still maps`() = runTest {
        // The forward-compatibility case. An agent newer than this client will report
        // states from a qBittorrent release that postdates it, and a whole response must
        // not fail over one unfamiliar word.
        val body = """,
          "transfers": {
            "active": 1, "total": 1,
            "download_rate_bytes": 0, "upload_rate_bytes": 0,
            "items": [{ "id": "x", "name": "n", "state": "movingFilesInQbit6", "progress": 1.0 }]
          }"""
        server.enqueue(jsonResponse(200, "[${serviceJson(body)}]"))

        val item = singleService().transfers!!.items.single()
        assertEquals("movingFilesInQbit6", item.state)
    }

    @Test
    fun `a sampled list does not overwrite the true counts`() = runTest {
        // The agent caps items at ten. `total` remains what the service reported, and a
        // client that trusted items.size would understate a busy server.
        val items = (1..10).joinToString(",") {
            """{ "id": "t$it", "name": "item $it", "state": "downloading", "progress": 0.5 }"""
        }
        val body = """,
          "transfers": {
            "active": 40, "total": 200,
            "download_rate_bytes": 1, "upload_rate_bytes": 0,
            "items": [$items]
          }"""
        server.enqueue(jsonResponse(200, "[${serviceJson(body)}]"))

        val transfers = singleService().transfers!!

        assertEquals(10, transfers.items.size)
        assertEquals(40, transfers.active)
        assertEquals(200, transfers.total)
    }
}
