package dev.cueseek.wear.dashboard

import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.Instant

/**
 * How old the reading is, as ambient has to say it.
 *
 * Ambient does not poll, so this line is the only thing standing between a dimmed
 * `Operational` and a claim with nothing behind it. Worth pinning, including the cases that
 * only happen when something has gone wrong.
 */
class ReadingAgeTest {

    private val now: Instant = Instant.parse("2026-09-26T12:00:00Z")

    private fun ageOf(secondsAgo: Long) = readingAge(now.minusSeconds(secondsAgo), now)

    @Test
    fun `inside the first minute there is nothing to report`() {
        assertEquals("now", ageOf(0))
        assertEquals("now", ageOf(1))
        assertEquals("now", ageOf(59))
    }

    @Test
    fun `minutes, from the first one`() {
        assertEquals("1m ago", ageOf(60))
        assertEquals("1m ago", ageOf(119))
        assertEquals("2m ago", ageOf(120))
        assertEquals("59m ago", ageOf(3_599))
    }

    @Test
    fun `hours once minutes stop being useful`() {
        assertEquals("1h ago", ageOf(3_600))
        assertEquals("23h ago", ageOf(86_399))
    }

    /**
     * Failure territory rather than normal operation, but "59m ago" forever would be a lie
     * of omission — and a resident ambient screen is exactly where that could sit unnoticed.
     */
    @Test
    fun `days rather than an ever-growing hour count`() {
        assertEquals("1d ago", ageOf(86_400))
        assertEquals("3d ago", ageOf(3 * 86_400))
    }

    /**
     * A clock that went backwards: a time-zone change, an NTP correction, a VM resumed from
     * a saved state. A negative age would be nonsense rendered with total confidence, which
     * is worse than rounding to the nearest true thing.
     */
    @Test
    fun `a reading from the future reads as now rather than as nonsense`() {
        assertEquals("now", readingAge(now.plusSeconds(30), now))
        assertEquals("now", readingAge(now.plusSeconds(86_400), now))
    }
}
