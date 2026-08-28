package dev.cueseek.android.ui

import dev.cueseek.android.ui.dashboard.activityLine
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.NowPlaying
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.Transfers
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * What the service row says when there is nothing wrong.
 *
 * This line only appears when health has no reason to speak, so every case here is a
 * healthy service. The decisions being pinned are about restraint: which facts earn a place
 * on a line that competes with a service name and an age, and which are noise.
 */
class ActivityLineTest {

    private fun service(
        nowPlaying: NowPlaying? = null,
        transfers: Transfers? = null,
    ) = Service(
        id = "svc",
        name = "Service",
        capabilities = emptyList(),
        health = Health(
            status = HealthStatus.Healthy,
            reachable = true,
            reportedStatus = null,
            reasons = emptyList(),
            observedAt = Instant.EPOCH,
        ),
        actions = emptyList(),
        nowPlaying = nowPlaying,
        transfers = transfers,
    )

    private fun playing(sessions: Int, transcoding: Int) =
        NowPlaying(sessions = sessions, transcoding = transcoding, items = emptyList())

    private fun moving(active: Int, total: Int, down: Long = 0) = Transfers(
        active = active,
        total = total,
        downloadRateBytes = down,
        uploadRateBytes = 0,
        items = emptyList(),
    )

    // ---------------------------------------------------------------- nothing to say

    @Test
    fun `a service with no activity capabilities says nothing`() {
        assertNull(activityLine(service()))
    }

    @Test
    fun `an idle server says nothing rather than reporting an absence`() {
        // "0 playing" spends a whole line to report that nothing is happening, which the
        // status mark beside it already covers.
        assertNull(activityLine(service(nowPlaying = playing(0, 0))))
    }

    @Test
    fun `a client tracking torrents but moving none says nothing`() {
        // A seedbox holding two hundred finished torrents is not doing anything. A number
        // that never changes trains the reader to stop looking at the line.
        assertNull(activityLine(service(transfers = moving(active = 0, total = 200))))
    }

    // ---------------------------------------------------------------- playback

    @Test
    fun `playback reports its sessions`() {
        assertEquals("2 playing", activityLine(service(nowPlaying = playing(2, 0))))
    }

    @Test
    fun `transcoding is named whenever it is happening`() {
        // The number that explains a hot machine, so it earns the width.
        assertEquals(
            "2 playing · 1 transcoding",
            activityLine(service(nowPlaying = playing(2, 1))),
        )
    }

    @Test
    fun `zero transcoding is not mentioned`() {
        val line = activityLine(service(nowPlaying = playing(3, 0)))!!
        assertEquals("3 playing", line)
    }

    // ---------------------------------------------------------------- transfers

    @Test
    fun `transfers lead with what is moving`() {
        assertEquals(
            "3 of 47 active · ↓ 12.0 MB/s",
            activityLine(service(transfers = moving(3, 47, 12_000_000))),
        )
    }

    @Test
    fun `total is omitted when everything tracked is active`() {
        // "3 of 3 active" is a longer way of saying "3 active".
        assertEquals("3 active", activityLine(service(transfers = moving(3, 3))))
    }

    @Test
    fun `a zero rate is omitted rather than printed`() {
        assertEquals("2 of 9 active", activityLine(service(transfers = moving(2, 9, 0))))
    }

    // ---------------------------------------------------------------- both

    @Test
    fun `a service doing both reports both`() {
        // A media server that also moves files is a real configuration, and picking one to
        // show would be arbitrary.
        val line = activityLine(
            service(nowPlaying = playing(1, 1), transfers = moving(2, 5, 1_000_000)),
        )
        assertEquals("1 playing · 1 transcoding  ·  2 of 5 active · ↓ 1.0 MB/s", line)
    }

    @Test
    fun `one capability idle and the other busy shows only the busy one`() {
        val line = activityLine(
            service(nowPlaying = playing(0, 0), transfers = moving(2, 5)),
        )
        assertEquals("2 of 5 active", line)
    }
}
