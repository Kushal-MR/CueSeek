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
import dev.cueseek.core.data.ThemeChoice
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.design.icon.CueSeekIcons
import dev.cueseek.core.design.status.BeatDot
import dev.cueseek.core.design.status.Tally
import dev.cueseek.core.design.status.TallyRule
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.design.token.CueSeekType
import dev.cueseek.core.model.Action
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.Freshness
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.Scope
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
    theme: ThemeChoice,
    onThemeChange: (ThemeChoice) -> Unit,
    onForgetRequested: () -> Unit,
    modifier: Modifier = Modifier,
    onPowerRequested: (Action) -> Unit = {},
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

            HostMenu(
                theme = theme,
                onThemeChange = onThemeChange,
                onForgetRequested = onForgetRequested,
                modifier = Modifier.padding(top = 2.dp),
                hostActions = state.hostActions,
                // What this device may do, from the token it was paired with — a separate
                // question from what the agent offers, and the user has to be able to tell
                // the two apart.
                canPower = state.host.scopes.contains(Scope.HostPower),
                onPowerRequested = onPowerRequested,
            )
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

        // Below the provenance line on purpose. The order down this column is the order of
        // the questions being asked: is anything wrong, can I believe this, and only then
        // how is the machine itself. Reversing the last two would put a CPU figure above
        // the line that says whether to trust it.
        //
        // Null when stale, which is why nothing here re-checks freshness: the data layer
        // has already dropped the metrics rather than degraded them, so this simply has
        // nothing to draw.
        HostVitals(
            metrics = state.hostMetrics,
            modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 14.dp),
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

/**
 * The verdict, in the user's terms rather than the system's.
 *
 * Ordered by how definite the problem is, not by subject. Services needing attention come
 * first because that is what the console is for. **Host pressure comes next, ahead of
 * unknown services**, and that ordering is the point of it existing: a filesystem at 97% is
 * a fact somebody has to act on, while `unknown` is the absence of a fact. Before this, a
 * disk could sit at 97% with its rule drawn red while the headline directly above it read
 * "All good" — the console being cheerful about the one thing on screen that was not.
 *
 * It stops at the headline. The tally rule and the roster stay about services, because a
 * machine is not one of its own services and counting it as one would make "2 healthy" mean
 * two different kinds of thing at once (ADR-0004 Amendment 4).
 */
internal fun verdict(state: AgentState, tally: Tally): String {
    if (state.freshness.isStale) return "Unverified"
    if (state.services.isEmpty()) return "No services"

    val attention = tally.needingAttention
    return when {
        attention == 1 -> "1 needs attention"
        attention > 1 -> "$attention need attention"
        else -> hostConcern(state.hostMetrics)
            ?: if (tally.unknown > 0) "${tally.unknown} unknown" else "Operational"
    }
}

/**
 * What the machine itself is complaining about, or null when it is not.
 *
 * Thresholds match the vitals strip exactly rather than being restated, so the headline and
 * the rule beneath it can never disagree about whether something is wrong — which was the
 * original defect in a different form.
 *
 * Temperature is judged against the sensor's own stated limit, never a number invented here.
 * CPU is deliberately absent: a processor at 100% is a transcode doing its job, and a
 * console that announced it would be crying wolf every time somebody watched a film.
 */
internal fun hostConcern(metrics: HostMetrics?): String? {
    if (metrics == null) return null

    val disk = fullest(metrics.storage)?.usedFraction
    val memory = metrics.memory?.usedFraction

    // Critical first, across both resources, so the worse of the two wins rather than
    // whichever happens to be checked first.
    if (disk != null && disk >= CRITICAL) return "Disk almost full"
    if (memory != null && memory >= CRITICAL) return "Memory almost full"

    if (metrics.thermal?.any { it.isHot } == true) return "Running hot"

    if (disk != null && disk >= PRESSURE) return "Disk filling up"
    if (memory != null && memory >= PRESSURE) return "Memory under pressure"

    return null
}

