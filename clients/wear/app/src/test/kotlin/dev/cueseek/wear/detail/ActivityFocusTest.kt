package dev.cueseek.wear.detail

import dev.cueseek.core.model.NowPlaying
import dev.cueseek.core.model.PlaybackSession
import dev.cueseek.core.model.TransferItem
import dev.cueseek.core.model.Transfers
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Which one item a wrist sees.
 *
 * The screen shows a single session and a single transfer, so *which* is a decision rather
 * than an accident — and the wrong one is worse than a list. Showing an idle stream while
 * another pins the CPU would be a console pointing at the wrong thing.
 */
class ActivityFocusTest {

    // ------------------------------------------------------------------ playback

    @Test
    fun `nothing playing focuses nothing`() {
        assertNull(focusedSession(null))
        assertNull(focusedSession(NowPlaying(sessions = 0, transcoding = 0, items = emptyList())))
    }

    /**
     * The rule that matters. A direct play is nearly free; one transcode saturates the CPU
     * every other service on that host is sharing, so it is the stream that explains the
     * machine and the one a single-item screen must pick.
     */
    @Test
    fun `a transcoding session wins over a direct play listed first`() {
        val playing = playing(
            session("direct", transcoding = false),
            session("transcode", transcoding = true),
        )
        assertEquals("transcode", focusedSession(playing)?.id)
    }

    /** Paused is not what "what is happening right now" is asking about. */
    @Test
    fun `a playing session wins over a paused one listed first`() {
        val playing = playing(
            session("paused", transcoding = false, paused = true),
            session("playing", transcoding = false),
        )
        assertEquals("playing", focusedSession(playing)?.id)
    }

    /**
     * Transcoding outranks paused, but an *active* transcode outranks a paused one — a
     * paused transcode is not currently costing anything.
     */
    @Test
    fun `an active transcode beats a paused transcode`() {
        val playing = playing(
            session("paused-transcode", transcoding = true, paused = true),
            session("live-transcode", transcoding = true),
        )
        assertEquals("live-transcode", focusedSession(playing)?.id)
    }

    /** A paused transcode still beats a direct play: it is the heavier thing on the host. */
    @Test
    fun `a paused transcode still beats a direct play`() {
        val playing = playing(
            session("direct", transcoding = false),
            session("paused-transcode", transcoding = true, paused = true),
        )
        assertEquals("paused-transcode", focusedSession(playing)?.id)
    }

    /**
     * Everything paused is still worth showing — the answer is "one paused stream", not
     * silence. Falls back to the agent's own order, which is stable between polls; a
     * tiebreak that reordered under a raised wrist would be worse than an arbitrary one.
     */
    @Test
    fun `all paused falls back to the agent's order`() {
        val playing = playing(
            session("first", transcoding = false, paused = true),
            session("second", transcoding = false, paused = true),
        )
        assertEquals("first", focusedSession(playing)?.id)
    }

    // ------------------------------------------------------------------ transfers

    @Test
    fun `nothing transferring focuses nothing`() {
        assertNull(focusedTransfer(null))
        assertNull(focusedTransfer(transfers()))
    }

    @Test
    fun `the fastest transfer wins`() {
        val moving = transfers(
            item("slow", rate = 1_000),
            item("fast", rate = 9_000_000),
            item("medium", rate = 500_000),
        )
        assertEquals("fast", focusedTransfer(moving)?.id)
    }

    /**
     * Speed only ranks the things that are moving. Everything paused, seeding or finished
     * sits at zero, so ranking the whole list by it ranks nothing — the same finding the
     * qBittorrent adapter recorded when it stopped sorting by `dlspeed`.
     */
    @Test
    fun `nothing moving falls back to the agent's order rather than a zero-rate tie`() {
        val idle = transfers(
            item("newest", rate = 0),
            item("older", rate = 0),
            item("oldest", rate = null),
        )
        assertEquals("newest", focusedTransfer(idle)?.id)
    }

    @Test
    fun `a single mover beats a stack of finished torrents`() {
        val mixed = transfers(
            item("done-1", rate = 0),
            item("done-2", rate = null),
            item("moving", rate = 42),
            item("done-3", rate = 0),
        )
        assertEquals("moving", focusedTransfer(mixed)?.id)
    }

    // ------------------------------------------------------------------ the clock

    @Test
    fun `position and duration read as a clock`() {
        assertEquals("0:47 / 2:35:24", playbackClock(47, 9324))
        assertEquals("1:05 / 3:20", playbackClock(65, 200))
    }

    /**
     * Live or unbounded content has a position and no end. Saying where you are is useful;
     * inventing a total to divide it by is not.
     */
    @Test
    fun `an unbounded stream shows a position and no total`() {
        assertEquals("12:30", playbackClock(750, null))
        assertEquals("12:30", playbackClock(750, 0))
    }

    @Test
    fun `no position means no clock at all`() {
        assertNull(playbackClock(null, 3600))
    }

    @Test
    fun `a negative position is clamped rather than rendered`() {
        assertEquals("0:00", playbackClock(-5, null))
    }

    // ------------------------------------------------------------------ fixtures

    private fun playing(vararg items: PlaybackSession) = NowPlaying(
        sessions = items.size,
        transcoding = items.count { it.transcoding },
        items = items.toList(),
    )

    private fun session(id: String, transcoding: Boolean, paused: Boolean = false) =
        PlaybackSession(
            id = id,
            title = id,
            subtitle = null,
            user = null,
            client = null,
            positionSeconds = null,
            durationSeconds = null,
            paused = paused,
            transcoding = transcoding,
        )

    private fun transfers(vararg items: TransferItem) = Transfers(
        active = items.count { (it.downloadRateBytes ?: 0L) > 0L },
        total = items.size,
        downloadRateBytes = items.sumOf { it.downloadRateBytes ?: 0L },
        uploadRateBytes = 0,
        items = items.toList(),
    )

    private fun item(id: String, rate: Long?) = TransferItem(
        id = id,
        name = id,
        state = "downloading",
        progress = 0.5f,
        sizeBytes = null,
        downloadRateBytes = rate,
        etaSeconds = null,
    )
}
