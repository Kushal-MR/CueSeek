package dev.cueseek.core.api

import dev.cueseek.core.model.ApiResult
import kotlinx.coroutines.test.runTest
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The host metrics payload, crossing the wire.
 *
 * The rule under test is the same one the activity payloads answer to — **absent is not
 * zero** — applied where it is harder to get right. Hardware genuinely differs, so most
 * real machines will omit something, and a default anywhere in this layer would turn "this
 * box has no temperature sensor" into "this box is at 0°C".
 */
class HostMetricsMappingTest {

    private val server = MockWebServer().apply { start() }

    @After
    fun tearDown() = server.close()

    private suspend fun metrics() = when (val r = server.api("csk_token").hostMetrics()) {
        is ApiResult.Success -> r.value
        is ApiResult.Failure -> error("expected success, got ${r.error}")
    }

    @Test
    fun `a complete collection maps every field`() = runTest {
        server.enqueue(
            jsonResponse(
                200,
                """
                {
                  "collected_at": "2026-08-31T09:30:00Z",
                  "uptime_seconds": 350735,
                  "cpu": { "usage_percent": 23.5, "cores": 8, "load1": 1.25, "load5": 0.9, "load15": 0.4 },
                  "memory": {
                    "total_bytes": 16000000000,
                    "available_bytes": 11000000000,
                    "used_bytes": 5000000000
                  },
                  "storage": [
                    { "mount": "/", "filesystem": "/dev/sda2", "total_bytes": 500, "free_bytes": 120 }
                  ],
                  "thermal": [
                    { "label": "coretemp Package id 0", "celsius": 47.5, "high_celsius": 84.0 }
                  ]
                }
                """.trimIndent(),
            )
        )

        val got = requireNotNull(metrics()) { "the agent reported no metrics" }

        assertEquals(350735L, got.uptimeSeconds)
        assertEquals(23.5f, got.cpu?.usagePercent)
        assertEquals(8, got.cpu?.cores)
        assertEquals(5_000_000_000L, got.memory?.usedBytes)

        val volume = got.storage?.single()
        assertEquals("/", volume?.mount)
        assertEquals("/dev/sda2", volume?.filesystem)
        assertEquals(380L, volume?.usedBytes)

        val sensor = got.thermal?.single()
        assertEquals("coretemp Package id 0", sensor?.label)
        assertEquals(47.5f, sensor?.celsius)
        assertEquals(false, sensor?.isHot)
    }

    @Test
    fun `omitted fields stay null rather than becoming zero`() = runTest {
        // A machine that answered the clock and nothing else. Every field must be absent:
        // a zero here would report an idle, cold computer that was never measured.
        server.enqueue(jsonResponse(200, """{ "collected_at": "2026-08-31T09:30:00Z" }"""))

        val got = requireNotNull(metrics()) { "the agent reported no metrics" }

        assertNull("cpu", got.cpu)
        assertNull("memory", got.memory)
        assertNull("storage", got.storage)
        assertNull("thermal", got.thermal)
        assertNull("uptime", got.uptimeSeconds)
        assertTrue("a payload with nothing in it must know it", got.isEmpty)
    }

    @Test
    fun `an empty sensor list is not the same as no sensor list`() = runTest {
        // Every virtual machine reports this. It means "asked, and this hardware has no
        // sensors", which a screen must render as nothing rather than as a fault.
        server.enqueue(
            jsonResponse(
                200,
                """{ "collected_at": "2026-08-31T09:30:00Z", "thermal": [], "storage": [] }""",
            )
        )

        val got = requireNotNull(metrics()) { "the agent reported no metrics" }

        assertEquals(emptyList<Any>(), got.thermal)
        assertEquals(emptyList<Any>(), got.storage)
    }

    @Test
    fun `a first collection omits cpu usage and keeps the rest`() = runTest {
        // The agent cannot compute utilisation from one sample of a cumulative counter, so
        // the field is absent for one tick after a restart. Load and core count are still
        // real, and losing them because one sibling was missing would be the wrong trade.
        server.enqueue(
            jsonResponse(
                200,
                """
                {
                  "collected_at": "2026-08-31T09:30:00Z",
                  "cpu": { "cores": 8, "load1": 0.5 }
                }
                """.trimIndent(),
            )
        )

        val cpu = requireNotNull(metrics()?.cpu) { "cpu was absent" }

        assertNull("usage cannot exist from a single sample", cpu.usagePercent)
        assertEquals(8, cpu.cores)
        assertEquals(0.0625f, cpu.loadFraction)
    }

    @Test
    fun `204 is a successful answer of nothing`() = runTest {
        // Not an error. The agent may have started seconds ago, have metrics switched off,
        // or run on a platform that cannot read them — all of which are "no measurement
        // exists", which is a different fact from "we could not ask".
        server.enqueue(MockResponse.Builder().code(204).build())

        when (val result = server.api("csk_token").hostMetrics()) {
            is ApiResult.Success -> assertNull("204 must map to null", result.value)
            is ApiResult.Failure -> error("204 was treated as a failure: ${result.error}")
        }
    }

    @Test
    fun `an unknown field does not fail the response`() = runTest {
        // Forward tolerance. A newer agent adding a field must not break an older build,
        // which would turn every optional addition into a coordinated release.
        server.enqueue(
            jsonResponse(
                200,
                """{ "collected_at": "2026-08-31T09:30:00Z", "gpu": { "usage_percent": 12 } }""",
            )
        )

        assertNotNull("an unrecognised field broke the whole payload", metrics())
    }
}
