package dev.cueseek.wear.dashboard

import dev.cueseek.core.model.NowPlaying
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.Transfers

/**
 * One short line saying what a service is doing, or null when there is nothing worth saying.
 *
 * # Why this is not the phone's `activityLine`
 *
 * The phone's version is shared logic in all but name — a pure function over the same model
 * types — and the obvious move was to lift it into `:core:model` the way M5.4a lifted the
 * verdict. That would have been wrong, and the difference is worth stating because the two
 * cases look identical from a distance.
 *
 * The **verdict** had to be shared: "is everything fine?" is one question about one machine,
 * and two clients answering it differently is a console contradicting itself.
 *
 * An activity line is not a judgement, it is a phrasing of the same fact — and the phone's
 * phrasing does not fit here. `3 of 12 active · ↓ 4.2 MB/s` is 28 characters; a watch row
 * has room for about half that beside a status mark and a name. Lifting it would also have
 * dragged `byteSize` and its decimal-places rule into the domain module, which is unit
 * formatting and belongs nowhere near it.
 *
 * So the watch says less, on purpose, and this is the M5.3b error-copy precedent applied
 * again rather than a new argument (ADR-0007: the same capability, rendered differently).
 *
 * # What is deliberately identical
 *
 * The *decisions*, which are the part that would be a defect if they diverged:
 *
 *  - **Idle says nothing.** "0 playing" spends a row on a non-event, and the status mark
 *    beside it already covers "everything is fine".
 *  - **Transcoding is named only when it is happening**, because it is the number that
 *    explains a hot machine — and never when it is zero.
 *  - **A service doing both gets both**, joined. A media server that also moves files is a
 *    real configuration and picking one would be arbitrary.
 *
 * Pure, so every one of those cases is testable without a screen.
 */
fun wearActivityLine(service: Service): String? {
    val parts = listOfNotNull(
        service.nowPlaying?.let(::playback),
        service.transfers?.let(::transfers),
    )
    return parts.takeIf { it.isNotEmpty() }?.joinToString(" · ")
}

private fun playback(playing: NowPlaying): String? {
    if (playing.idle) return null
    val sessions = "${playing.sessions} playing"
    // "+1 transcoding" rather than the phone's "· 1 transcoding": the fact survives, six
    // characters do not.
    return if (playing.transcoding > 0) "$sessions +${playing.transcoding} tc" else sessions
}

private fun transfers(transfers: Transfers): String? {
    if (transfers.idle) return null
    // The count alone. The phone adds the total and the download rate; both are useful on a
    // row a thumb can reach and neither survives the width here. The detail screen (M5.5)
    // is where a wrist gets the rest.
    return "${transfers.active} active"
}
