package dev.cueseek.android.ui

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The formatting rules for activity numbers.
 *
 * These are tested rather than eyeballed because they are read at a glance while doing
 * something else, and the failure mode is silent: a wrong unit or a stray decimal does not
 * crash, it just quietly misinforms.
 */
class UnitsTest {

    // ---------------------------------------------------------------- sizes

    @Test
    fun `sizes use decimal units, matching what download clients quote`() {
        // Decimal rather than binary on purpose. qBittorrent saying 1.2 MB/s beside a
        // console saying 1.1 MiB/s looks like a defect in the console.
        assertEquals("1.0 kB", byteSize(1_000))
        assertEquals("1.5 MB", byteSize(1_500_000))
        assertEquals("2.0 GB", byteSize(2_000_000_000))
    }

    @Test
    fun `bytes below a kilobyte are exact`() {
        assertEquals("0 B", byteSize(0))
        assertEquals("999 B", byteSize(999))
    }

    @Test
    fun `precision drops above one hundred, where the decimal is noise`() {
        assertEquals("99.9 MB", byteSize(99_900_000))
        assertEquals("947 MB", byteSize(947_300_000))
    }

    // ---------------------------------------------------------------- rates

    @Test
    fun `a zero rate says nothing rather than zero`() {
        // A stopped transfer should not invite the reader to wonder whether it is stalled
        // or idle. That is what the state word is for.
        assertNull(rateOrNull(0))
        assertNull(rateOrNull(null))
        assertNull(rateOrNull(-1))
    }

    @Test
    fun `a real rate carries its unit`() {
        assertEquals("1.2 MB/s", rateOrNull(1_200_000))
    }

    // ---------------------------------------------------------------- eta

    @Test
    fun `eta coarsens as it grows`() {
        assertEquals("45s", etaOrNull(45))
        assertEquals("5m", etaOrNull(300))
        assertEquals("2h 5m", etaOrNull(7_500))
        assertEquals("1d 3h", etaOrNull(97_200))
    }

    @Test
    fun `an absent or nonsensical eta is nothing`() {
        assertNull(etaOrNull(null))
        assertNull(etaOrNull(0))
        assertNull(etaOrNull(-5))
    }

    // ---------------------------------------------------------------- playback

    @Test
    fun `playback shows hours only when there are hours`() {
        // The leading zeroes on 00:12:34 are pure column noise for an episode.
        assertEquals("12:34 / 48:20", playbackPosition(754, 2_900))
        assertEquals("1:02:03 / 2:10:00", playbackPosition(3_723, 7_800))
    }

    @Test
    fun `live content shows a position without inventing a total`() {
        assertEquals("12:34", playbackPosition(754, null))
        assertEquals("12:34", playbackPosition(754, 0))
    }

    @Test
    fun `no position at all produces nothing`() {
        assertNull(playbackPosition(null, 2_900))
    }

    // ---------------------------------------------------------------- percent

    @Test
    fun `percent rounds and clamps to the contract`() {
        assertEquals("0%", percent(0f))
        assertEquals("42%", percent(0.4239f))
        assertEquals("100%", percent(1f))
        assertEquals("100%", percent(1.4f))
        assertEquals("0%", percent(-0.2f))
    }
}
