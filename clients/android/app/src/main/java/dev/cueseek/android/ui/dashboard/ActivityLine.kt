package dev.cueseek.android.ui.dashboard

import dev.cueseek.android.ui.rateOrNull
import dev.cueseek.core.model.Service

/**
 * The service row's activity line, as a pure function.
 *
 * Activity belongs on the row rather than only behind a tap. "Healthy" is uninformative
 * when the same line could say "2 playing · 1 transcoding" — and this is a console whose
 * whole purpose is a two-second glance, so a fact that needs navigation to reach is a fact
 * most people never see.
 *
 * Returns null when there is nothing worth saying, and the row keeps its status label. In
 * particular an idle service says nothing: "0 playing" spends a line to report an absence,
 * and the status mark beside it already covers the "everything is fine" case.
 *
 * A service implementing both capabilities gets both, joined — a media server that also
 * moves files is a real configuration, and picking one to show would be arbitrary.
 *
 * Pure, so the decisions here are testable without a screen. Which of these lines a busy
 * row shows is the kind of thing that looks obvious and turns out to have four cases.
 */
fun activityLine(service: Service): String? {
    val parts = listOfNotNull(
        service.nowPlaying?.let(::playbackPhrase),
        service.transfers?.let(::transferPhrase),
    )
    return parts.takeIf { it.isNotEmpty() }?.joinToString("  ·  ")
}

/**
 * Playback, in as few words as carry the fact.
 *
 * Transcoding is named whenever it is happening, because it is the number that explains a
 * hot machine: direct play is nearly free, and one 4K transcode saturates the CPU every
 * other service on the host is sharing. It is *not* named when it is zero — a line reading
 * "2 playing · 0 transcoding" spends half its width on a non-event.
 */
private fun playbackPhrase(playing: dev.cueseek.core.model.NowPlaying): String? {
    if (playing.idle) return null
    val sessions = "${playing.sessions} playing"
    return if (playing.transcoding > 0) {
        "$sessions · ${playing.transcoding} transcoding"
    } else {
        sessions
    }
}

/**
 * Transfers, leading with what is moving.
 *
 * `active` comes first because it answers "is anything happening"; `total` follows only
 * when it differs, since "3 of 3 active" is a longer way of saying "3 active".
 *
 * A client that is tracking torrents but moving none says nothing at all. A seedbox holding
 * two hundred finished torrents is not doing anything, and reporting a number that never
 * changes trains the reader to stop looking at the line.
 */
private fun transferPhrase(transfers: dev.cueseek.core.model.Transfers): String? {
    if (transfers.idle) return null

    val counts = if (transfers.active == transfers.total) {
        "${transfers.active} active"
    } else {
        "${transfers.active} of ${transfers.total} active"
    }
    // Only the download rate on the row. Upload matters in the detail sheet; on a line
    // competing with a service name and an age it is the less urgent of the two.
    val rate = rateOrNull(transfers.downloadRateBytes)
    return if (rate != null) "$counts · ↓ $rate" else counts
}
