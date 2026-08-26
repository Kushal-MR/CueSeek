package dev.cueseek.android.ui.capability

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.cueseek.android.ui.byteSize
import dev.cueseek.android.ui.etaOrNull
import dev.cueseek.android.ui.percent
import dev.cueseek.android.ui.playbackPosition
import dev.cueseek.android.ui.rateOrNull
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.design.token.CueSeekShapes
import dev.cueseek.core.design.token.CueSeekType
import dev.cueseek.core.model.NowPlaying
import dev.cueseek.core.model.PlaybackSession
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.TransferItem
import dev.cueseek.core.model.Transfers

/**
 * The two activity capabilities, rendered.
 *
 * Both follow the same shape: a **headline of counts**, then a bounded list. That ordering
 * is the product in miniature — the two-second glance is answered by the counts, and the
 * list is for when the answer was interesting.
 *
 * Both reuse the rule motif rather than introducing a component. A progress bar here is
 * the same 4dp rounded strip the tally rule is, at a smaller size, because this screen has
 * already taught the reader what a rule means.
 *
 * Neither offers a control. Pausing a torrent and seeking a stream belong to the service's
 * own interface, which the row body already opens — a console that grew half a torrent
 * manager would be worse at it than the real one and would still not replace it.
 */

// ---------------------------------------------------------------- now playing

@Composable
internal fun NowPlayingCapability(service: Service, stale: Boolean) {
    val playing = service.nowPlaying

    // Null is not empty. Null means the agent could not ask — a media server that answers
    // its health endpoint and refuses /Sessions is up, and claiming "nothing is playing"
    // would be reporting an observation nobody made.
    if (playing == null) {
        Unavailable("Playback could not be read.")
        return
    }
    if (playing.idle) {
        Quiet("Nothing playing.")
        return
    }

    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Headline(summary = sessionSummary(playing), stale = stale)
        playing.items.forEach { SessionRow(it, stale) }
        Remainder(shown = playing.items.size, total = playing.sessions, noun = "session")
    }
}

/**
 * The counts, as one line.
 *
 * Transcoding is named rather than left to be inferred from a badge, because it is the
 * number that explains a hot machine: direct play is nearly free, and one transcode can
 * saturate the CPU every other service on the host is sharing.
 */
private fun sessionSummary(playing: NowPlaying): String {
    val sessions = "${playing.sessions} " + plural(playing.sessions, "session")
    if (playing.transcoding == 0) return "$sessions · direct play"
    return "$sessions · ${playing.transcoding} transcoding"
}

@Composable
private fun SessionRow(session: PlaybackSession, stale: Boolean) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            session.title,
            style = MaterialTheme.typography.bodyMedium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )

        // One line of context, assembled only from what the agent actually sent. Every
        // part is optional, and an absent one leaves no separator behind.
        val context = listOfNotNull(
            session.subtitle,
            session.user,
            session.client,
        ).joinToString(" · ")
        if (context.isNotEmpty()) {
            Text(
                context,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }

        session.progress?.let { ProgressRule(it, stale) }

        val facts = listOfNotNull(
            playbackPosition(session.positionSeconds, session.durationSeconds),
            if (session.paused) "paused" else null,
            if (session.transcoding) "transcoding" else null,
        )
        if (facts.isNotEmpty()) {
            Text(
                facts.joinToString("  ·  "),
                style = CueSeekType.Data.Small,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

// ---------------------------------------------------------------- transfers

@Composable
internal fun TransfersCapability(service: Service, stale: Boolean) {
    val transfers = service.transfers

    if (transfers == null) {
        Unavailable("Transfers could not be read.")
        return
    }

    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Headline(summary = transferSummary(transfers), stale = stale)

        if (transfers.items.isEmpty()) {
            Quiet("Nothing queued.")
        } else {
            transfers.items.forEach { TransferRow(it, stale) }
            Remainder(shown = transfers.items.size, total = transfers.total, noun = "transfer")
        }
    }
}

/**
 * Counts first, then rates.
 *
 * `active` and `total` are both shown because they answer different questions — "is
 * anything happening" and "how much is this thing looking after" — and a client that
 * collapsed them would have to mislead about one.
 */
private fun transferSummary(transfers: Transfers): String {
    val counts = if (transfers.active == transfers.total) {
        "${transfers.total} " + plural(transfers.total, "transfer")
    } else {
        "${transfers.active} of ${transfers.total} active"
    }
    val rates = listOfNotNull(
        rateOrNull(transfers.downloadRateBytes)?.let { "↓ $it" },
        rateOrNull(transfers.uploadRateBytes)?.let { "↑ $it" },
    ).joinToString("  ")

    return if (rates.isEmpty()) counts else "$counts · $rates"
}

@Composable
private fun TransferRow(item: TransferItem, stale: Boolean) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
            item.name,
            style = MaterialTheme.typography.bodyMedium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )

        ProgressRule(item.progress, stale)

        // The service's own state word, verbatim. Not translated into a shared vocabulary:
        // the difference between "stalled" and "queued" is exactly what tells an operator
        // whether to care, and this client must display words it has never seen.
        val facts = listOfNotNull(
            percent(item.progress),
            item.state,
            rateOrNull(item.downloadRateBytes),
            etaOrNull(item.etaSeconds),
            item.sizeBytes?.let(::byteSize),
        )
        Text(
            facts.joinToString("  ·  "),
            style = CueSeekType.Data.Small,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

// ---------------------------------------------------------------- shared parts

/**
 * A determinate rule.
 *
 * The tally rule's shape at a smaller size, so the reader is not asked to learn a second
 * vocabulary for "a proportion". It animates on change with `tallySpring` — no bounce,
 * because progress moving backwards would be a lie and a spring that overshoots draws one.
 *
 * Goes achromatic when stale, alongside everything else on the screen: a filled bar is a
 * claim about right now, and right now is exactly what is not known.
 */
@Composable
private fun ProgressRule(progress: Float, stale: Boolean) {
    val target = progress.coerceIn(0f, 1f)
    val animated by animateFloatAsState(
        targetValue = target,
        animationSpec = CueSeekMotion.tallySpring(),
        label = "activityProgress",
    )

    Box(
        Modifier
            .fillMaxWidth()
            .height(4.dp)
            .clip(CueSeekShapes.Shapes.extraSmall)
            .background(MaterialTheme.colorScheme.onSurface.copy(alpha = 0.08f))
            // The rule is a redundant encoding of the percentage printed beside it, so it
            // is not the sole conveyor of anything and needs no description of its own.
            .clearAndSetSemantics {},
    ) {
        Box(
            Modifier
                .fillMaxWidth(animated)
                .height(4.dp)
                .clip(CueSeekShapes.Shapes.extraSmall)
                .background(
                    if (stale) {
                        MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.4f)
                    } else {
                        MaterialTheme.colorScheme.primary
                    }
                )
        )
    }
}

@Composable
private fun Headline(summary: String, stale: Boolean) {
    Text(
        if (stale) "—" else summary,
        style = CueSeekType.Data.Emphasis,
        color = MaterialTheme.colorScheme.onSurface,
    )
}

/** The agent asked and there was nothing to report. A fact, not a failure. */
@Composable
private fun Quiet(message: String) {
    Text(
        message,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

/**
 * The agent could not observe this.
 *
 * Worded as the agent's failure rather than the service's, because that is what it is:
 * the service may be perfectly happy and simply not answering this one endpoint.
 */
@Composable
private fun Unavailable(message: String) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier.padding(vertical = 2.dp),
    ) {
        Text(
            message,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/**
 * Says what the list is not showing.
 *
 * The agent caps every sample, so a busy server sends ten items and a count of two
 * hundred. Without this line the list would silently misrepresent the fleet at exactly the
 * moment the number mattered most.
 */
@Composable
private fun Remainder(shown: Int, total: Int, noun: String) {
    if (total <= shown) return
    val hidden = total - shown
    Text(
        "and $hidden more ${plural(hidden, noun)}",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.clearAndSetSemantics {
            contentDescription = "and $hidden more ${plural(hidden, noun)}"
        },
    )
}

private fun plural(count: Int, noun: String) = if (count == 1) noun else "${noun}s"
