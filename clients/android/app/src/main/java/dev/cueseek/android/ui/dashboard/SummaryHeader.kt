package dev.cueseek.android.ui.dashboard

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.design.icon.CueSeekIcons
import dev.cueseek.core.design.status.BeatDot
import dev.cueseek.core.design.status.Tally
import dev.cueseek.core.design.status.TallyRule
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.design.token.CueSeekType
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.Freshness
import java.time.Duration
import java.time.Instant

/**
 * The summary: host, verdict, tally, and one line of provenance.
 *
 * No container and no card. The header is type on the page, which is what lets the roster
 * below read as the one substantial object on screen — giving this a surface of its own
 * would flatten the hierarchy it exists to create.
 */
@Composable
fun SummaryHeader(
    state: AgentState,
    now: Instant,
    onOverflow: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val stale = state.freshness.isStale
    val tally = Tally.of(state.services)

    Column(modifier = modifier.fillMaxWidth()) {
        Row(
            verticalAlignment = Alignment.Top,
            modifier = Modifier.fillMaxWidth().padding(start = 16.dp, end = 4.dp, top = 18.dp),
        ) {
            Column(Modifier.weight(1f)) {
                Text(
                    state.host.hostname,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Spacer(Modifier.height(3.dp))
                // The verdict crossfades rather than snapping: a machine going from fine
                // to not-fine should register as a change, not as a different screen.
                AnimatedContent(
                    targetState = verdict(state, tally),
                    transitionSpec = {
                        fadeIn(tween(CueSeekMotion.DurationEnter, easing = CueSeekMotion.EmphasizedDecelerate)) togetherWith
                            fadeOut(tween(CueSeekMotion.DurationExit, easing = CueSeekMotion.EmphasizedAccelerate))
                    },
                    label = "verdict",
                ) { text ->
                    Text(text, style = MaterialTheme.typography.headlineSmall)
                }
            }

            androidx.compose.material3.IconButton(onClick = onOverflow) {
                Icon(
                    CueSeekIcons.More,
                    contentDescription = "More",
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(18.dp),
                )
            }
        }

        TallyRule(
            tally = tally,
            stale = stale,
            modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 15.dp),
        )

        ProvenanceLine(
            state = state,
            tally = tally,
            now = now,
            modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 11.dp),
        )
    }
}

/**
 * The breakdown when live; the two facts that matter when not.
 *
 * Stale is where this line earns its place: "Stream open" and "no data 34s" are both true
 * at once, and stating them side by side is the clearest way to say that the connection
 * being alive is not evidence the data is.
 */
@Composable
private fun ProvenanceLine(
    state: AgentState,
    tally: Tally,
    now: Instant,
    modifier: Modifier = Modifier,
) {
    val colors = CueSeekStatus.colors
    val stale = state.freshness.isStale
    val lastEventAt = (state.freshness as? Freshness.Stale)?.lastEventAt

    Row(
        modifier = modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        if (stale) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(5.dp),
            ) {
                Icon(
                    CueSeekIcons.Sensors,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(14.dp),
                )
                Text(
                    connectionPhrase(state),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                BeatDot(live = false)
                Text(
                    lastEventAt?.let { "no data ${Duration.between(it, now).seconds}s" }
                        ?: "no data yet",
                    style = CueSeekType.Data.Emphasis,
                    color = MaterialTheme.colorScheme.onSurface,
                )
            }
        } else {
            Row(horizontalArrangement = Arrangement.spacedBy(11.dp)) {
                CountChip(CueSeekIcons.Check, colors.healthy, tally.healthy)
                CountChip(CueSeekIcons.Warning, colors.degraded, tally.degraded)
                CountChip(CueSeekIcons.Block, colors.unreachable, tally.unreachable)
                CountChip(CueSeekIcons.Question, colors.unknown, tally.unknown)
            }
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                // The key is the last event's arrival, so the dot beats once per event
                // rather than on a timer that would keep ticking through a frozen stream.
                BeatDot(live = true, pulseKey = state.services.size to freshnessKey(state))
                Text(
                    "live",
                    style = CueSeekType.Data.Small,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun CountChip(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    tint: androidx.compose.ui.graphics.Color,
    count: Int,
) {
    if (count == 0) return
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        modifier = Modifier.clearAndSetSemantics { contentDescription = "$count" },
    ) {
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(13.dp))
        Text(
            count.toString(),
            style = CueSeekType.Data.Emphasis,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

private fun freshnessKey(state: AgentState): Any =
    state.services.joinToString { it.health.observedAt.toString() }

private fun connectionPhrase(state: AgentState): String =
    when (state.status) {
        is dev.cueseek.core.model.StreamStatus.Open -> "Stream open"
        is dev.cueseek.core.model.StreamStatus.Connecting -> "Connecting"
        is dev.cueseek.core.model.StreamStatus.Retrying -> "Reconnecting"
        is dev.cueseek.core.model.StreamStatus.Stopped -> "Stream stopped"
        dev.cueseek.core.model.StreamStatus.Idle -> "Not connected"
    }

/** The verdict, in the user's terms rather than the system's. */
private fun verdict(state: AgentState, tally: Tally): String {
    if (state.freshness.isStale) return "Unverified"
    if (state.services.isEmpty()) return "No services"
    val attention = tally.needingAttention
    return when {
        attention == 0 && tally.unknown == 0 -> "All good"
        attention == 0 -> "${tally.unknown} unknown"
        attention == 1 -> "1 needs attention"
        else -> "$attention need attention"
    }
}
