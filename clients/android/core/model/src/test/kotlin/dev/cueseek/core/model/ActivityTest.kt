package dev.cueseek.core.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The two rules the activity model exists to keep.
 *
 * **Absent is not empty.** `null` means the agent could not observe this; empty means it
 * observed nothing happening. Collapsing them would let a screen report an idle server on
 * the strength of a failed request.
 *
 * **A progress bar is a claim.** Anything it cannot honestly say must come back as `null`
 * rather than as zero, because a bar parked at the far left states that playback just
 * started — which for a live channel nobody said.
 */
class ActivityTest {

    private fun session(position: Int?, duration: Int?) = PlaybackSession(
        id = "s1",
        title = "The Bear",
        subtitle = null,
        user = null,
        client = null,
        positionSeconds = position,
        durationSeconds = duration,
        paused = false,
        transcoding = false,
    )

    // ---------------------------------------------------------------- progress

    @Test
    fun `progress is the fraction watched`() {
        assertEquals(0.5f, session(600, 1_200).progress!!, 0.001f)
    }

    @Test
    fun `live content has no progress rather than zero progress`() {
        // Duration is legitimately absent for a live stream, and zero for content the
        // server could not measure. Neither is "at the beginning".
        assertNull(session(600, null).progress)
        assertNull(session(600, 0).progress)
    }

    @Test
    fun `an unknown position has no progress`() {
        assertNull(session(null, 1_200).progress)
    }

    @Test
    fun `progress is clamped, because a bar cannot render past its own end`() {
        assertEquals(1f, session(2_000, 1_200).progress!!, 0.001f)
    }

    // ---------------------------------------------------------------- idle

    @Test
    fun `idle means observed and quiet, never unobserved`() {
        val quiet = NowPlaying(sessions = 0, transcoding = 0, items = emptyList())
        assertTrue(quiet.idle)

        val busy = NowPlaying(sessions = 2, transcoding = 1, items = emptyList())
        assertFalse(busy.idle)
    }

    @Test
    fun `transfers are idle when nothing is moving, even while tracking many`() {
        // A seedbox holding two hundred finished torrents is not "busy". Active is the
        // number that answers "is anything happening"; total answers a different question.
        val seeding = Transfers(
            active = 0,
            total = 200,
            downloadRateBytes = 0,
            uploadRateBytes = 5_000,
            items = emptyList(),
        )
        assertTrue(seeding.idle)
        assertEquals(200, seeding.total)
    }

    // ---------------------------------------------------------------- sampling

    @Test
    fun `counts are independent of the sample, which is the whole point`() {
        // The agent caps items at ten. A client that read items.size as the total would
        // understate a busy server at exactly the moment the number mattered.
        val many = Transfers(
            active = 40,
            total = 200,
            downloadRateBytes = 12_000_000,
            uploadRateBytes = 0,
            items = List(10) {
                TransferItem(
                    id = "t$it",
                    name = "item $it",
                    state = "downloading",
                    progress = 0.5f,
                    sizeBytes = null,
                    downloadRateBytes = null,
                    etaSeconds = null,
                )
            },
        )

        assertEquals(10, many.items.size)
        assertEquals(40, many.active)
        assertEquals(200, many.total)
        assertFalse(many.idle)
    }

    @Test
    fun `a service without the capability carries null, not an empty payload`() {
        val service = Service(
            id = "jellyfin",
            name = "Jellyfin",
            capabilities = emptyList(),
            health = Health(
                status = HealthStatus.Healthy,
                reachable = true,
                reportedStatus = null,
                reasons = emptyList(),
                observedAt = java.time.Instant.EPOCH,
            ),
            actions = emptyList(),
        )
        assertNull(service.nowPlaying)
        assertNull(service.transfers)
    }
}
