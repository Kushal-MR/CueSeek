package dev.cueseek.wear.dashboard

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Duration
import java.time.Instant

/**
 * Staleness, which is the one thing on this screen that must not be optimistic.
 *
 * The whole point is that a reading stops being presented as fact on its own clock, without
 * anything arriving to say so. A client that waited to be told would show confident green
 * while the agent was unreachable, which is the failure this project has refused since M2.
 */
class StalenessTest {

    private val now: Instant = Instant.parse("2026-09-06T10:00:00Z")

    @Test
    fun `a fresh reading is not stale`() {
        assertFalse(DashboardViewModel.isStale(now.minusSeconds(5), now))
    }

    @Test
    fun `a reading one agent poll old is not stale`() {
        // The agent polls every 30s by default. Calling that stale would flag the most
        // recent reading that can exist.
        assertFalse(DashboardViewModel.isStale(now.minusSeconds(30), now))
    }

    @Test
    fun `the boundary is not stale, one second past it is`() {
        val edge = now.minus(DashboardViewModel.STALE_AFTER)
        assertFalse("exactly at the threshold should still count", DashboardViewModel.isStale(edge, now))
        assertTrue(DashboardViewModel.isStale(edge.minusSeconds(1), now))
    }

    @Test
    fun `a clearly old reading is stale`() {
        assertTrue(DashboardViewModel.isStale(now.minus(Duration.ofMinutes(10)), now))
    }

    /**
     * A watch sleeps, and its clock does not. If the device wakes hours later the reading on
     * screen is ancient, and nothing will have arrived to say so — which is exactly the case
     * the clock-based rule exists for.
     */
    @Test
    fun `a reading from before a long sleep is stale`() {
        assertTrue(DashboardViewModel.isStale(now.minus(Duration.ofHours(8)), now))
    }

    @Test
    fun `the threshold is three agent poll intervals`() {
        // Pinned so that changing it is a decision rather than a drift. The phone uses the
        // same multiple for the same reason.
        assertTrue(DashboardViewModel.STALE_AFTER == Duration.ofSeconds(90))
    }
}
