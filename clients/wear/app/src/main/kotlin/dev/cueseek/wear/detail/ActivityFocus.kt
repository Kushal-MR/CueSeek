package dev.cueseek.wear.detail

import dev.cueseek.core.model.NowPlaying
import dev.cueseek.core.model.PlaybackSession
import dev.cueseek.core.model.TransferItem
import dev.cueseek.core.model.Transfers

/**
 * Which single item a wrist sees when the service is doing several things.
 *
 * # Why choosing is better than scrolling
 *
 * The phone lists every session and every torrent, and that is right there — a thumb can
 * flick through twelve rows in a second. On a watch the same list is twelve rotations of a
 * crown to answer a question that was supposed to take two seconds, so the screen shows
 * **one**, and the count says how many it stands for.
 *
 * That makes *which one* a decision rather than an accident, and a wrong choice here is
 * worse than a list: showing the idle session while another is pinning the CPU would be a
 * console pointing at the wrong thing.
 *
 * Both rules below are pure, so the cases are testable without a screen — and they have more
 * cases than they look like.
 */

/**
 * The session worth showing.
 *
 * **Transcoding first**, because it is the one that explains the machine. A direct play is
 * nearly free; one 4K transcode saturates the CPU every other service on that host is
 * sharing, so if a wrist shows a single stream it should be that one.
 *
 * Then **playing over paused**: a paused stream is not consuming anything and is not what
 * "what is happening right now" is asking about.
 *
 * Then the first, which is the agent's own order and stable between polls — a tiebreak that
 * reordered under a raised wrist would be worse than an arbitrary one.
 */
fun focusedSession(playing: NowPlaying?): PlaybackSession? {
    val items = playing?.items?.takeIf { it.isNotEmpty() } ?: return null
    return items.firstOrNull { it.transcoding && !it.paused }
        ?: items.firstOrNull { it.transcoding }
        ?: items.firstOrNull { !it.paused }
        ?: items.first()
}

/**
 * The transfer worth showing.
 *
 * **Fastest first**, among those actually moving. Speed is the only ranking that means
 * anything here: everything paused, seeding or finished sits at zero, so ranking the whole
 * list by it would rank nothing — the same finding the qBittorrent adapter recorded when it
 * stopped sorting by `dlspeed` (see `agent/internal/adapters/qbittorrent`).
 *
 * When nothing is moving, the first item, which is the agent's order — newest first — and
 * therefore "what I did most recently" rather than a random survivor.
 */
fun focusedTransfer(transfers: Transfers?): TransferItem? {
    val items = transfers?.items?.takeIf { it.isNotEmpty() } ?: return null
    val moving = items.filter { (it.downloadRateBytes ?: 0L) > 0L }
    return moving.maxByOrNull { it.downloadRateBytes ?: 0L } ?: items.first()
}

/**
 * `1:23` or `1:23:45`, and never a total that was not reported.
 *
 * Live or unbounded content has a position and no end. Saying where you are is useful;
 * inventing a duration to divide it by is not — the same rule the phone applies, arrived at
 * for the same reason and kept identical because a watch showing a different position for
 * the same stream would be a defect rather than a form-factor choice.
 */
fun playbackClock(positionSeconds: Int?, durationSeconds: Int?): String? {
    val position = positionSeconds ?: return null
    if (durationSeconds == null || durationSeconds <= 0) return clock(position)
    return "${clock(position)} / ${clock(durationSeconds)}"
}

private fun clock(seconds: Int): String {
    val safe = seconds.coerceAtLeast(0)
    val hours = safe / 3600
    val minutes = (safe % 3600) / 60
    val secs = safe % 60
    return if (hours > 0) {
        "%d:%02d:%02d".format(hours, minutes, secs)
    } else {
        "%d:%02d".format(minutes, secs)
    }
}
