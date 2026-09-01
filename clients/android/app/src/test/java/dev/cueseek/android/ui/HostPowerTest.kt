package dev.cueseek.android.ui

import dev.cueseek.android.ui.dashboard.activeWorkSummary
import dev.cueseek.core.model.Capability
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
 * The sentence that turns a power-off into a considered decision.
 *
 * The console does not block the action — the operator owns the machine and may have good
 * reasons to shut it down mid-transcode — so this text is the entire mechanism. If it says
 * nothing when something is running, the feature does not exist.
 */
class HostPowerTest {

    private fun service(
        id: String,
        nowPlaying: NowPlaying? = null,
        transfers: Transfers? = null,
    ) = Service(
        id = id,
        name = id,
        capabilities = listOf(Capability("health", "Health")),
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

    private fun playing(sessions: Int) =
        NowPlaying(sessions = sessions, transcoding = 0, items = emptyList())

    private fun transferring(active: Int) = Transfers(
        active = active,
        total = active,
        downloadRateBytes = 0,
        uploadRateBytes = 0,
        items = emptyList(),
    )

    @Test
    fun `an idle machine says nothing`() {
        // Quiet rather than reassuring. "Nothing is running" would also be what a machine
        // whose activity could not be read looks like, and this cannot tell them apart.
        assertNull(activeWorkSummary(emptyList()))
        assertNull(activeWorkSummary(listOf(service("jellyfin"))))
        assertNull(activeWorkSummary(listOf(service("jellyfin", nowPlaying = playing(0)))))
    }

    @Test
    fun `playback is named before the machine goes down`() {
        val services = listOf(service("jellyfin", nowPlaying = playing(1)))
        assertEquals("1 stream playing", activeWorkSummary(services))

        assertEquals(
            "2 streams playing",
            activeWorkSummary(listOf(service("jellyfin", nowPlaying = playing(2)))),
        )
    }

    @Test
    fun `transfers are named too`() {
        assertEquals(
            "1 transfer running",
            activeWorkSummary(listOf(service("qbittorrent", transfers = transferring(1)))),
        )
    }

    @Test
    fun `work is summed across services`() {
        // The question is what shutting down the machine interrupts, which is everything on
        // it, not everything on one service.
        val services = listOf(
            service("jellyfin", nowPlaying = playing(2)),
            service("qbittorrent", transfers = transferring(3)),
        )
        assertEquals("2 streams playing and 3 transfers running", activeWorkSummary(services))
    }

    @Test
    fun `finished transfers do not count as work`() {
        // `active` rather than `total`: a seeding or completed torrent is not something a
        // shutdown interrupts, and counting it would cry wolf on every power-off.
        val idle = Transfers(
            active = 0,
            total = 40,
            downloadRateBytes = 0,
            uploadRateBytes = 0,
            items = emptyList(),
        )
        assertNull(activeWorkSummary(listOf(service("qbittorrent", transfers = idle))))
    }
}
