package dev.cueseek.core.design.status

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.unit.dp
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.design.token.CueSeekShapes
import dev.cueseek.core.design.token.CueSeekSpacing
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.Service

/** How many services sit in each status. */
data class Tally(
    val healthy: Int = 0,
    val degraded: Int = 0,
    val unreachable: Int = 0,
    val unknown: Int = 0,
) {
    val total: Int get() = healthy + degraded + unreachable + unknown
    val needingAttention: Int get() = degraded + unreachable

    companion object {
        fun of(services: List<Service>): Tally {
            var h = 0; var d = 0; var u = 0; var k = 0
            services.forEach {
                when (it.health.status) {
                    HealthStatus.Healthy -> h++
                    HealthStatus.Degraded -> d++
                    HealthStatus.Unreachable -> u++
                    HealthStatus.Unknown -> k++
                }
            }
            return Tally(h, d, u, k)
        }
    }
}

/**
 * The whole fleet's composition, as an 8dp rule.
 *
 * Deliberately a rule and not a component. An earlier draft made this a 28dp pill with
 * counts inside, and it competed with the roster for the eye — two rounded masses of equal
 * weight and no clear entry point. Reduced to a rule it informs without dominating, and
 * the counts moved to the headline where they read as language.
 *
 * It encodes **proportion only**. The semantics live in the verdict above it and in the
 * rows below, both of which carry icon and text, so nothing here is the sole conveyor of
 * meaning. That is the one place in this design system where colour stands alone, and it
 * is acceptable precisely because it is redundant.
 *
 * When stale it stops being drawn as segments at all and becomes a dashed hairline: the
 * distribution is no longer knowable, so it is no longer claimed.
 */
@Composable
fun TallyRule(
    tally: Tally,
    modifier: Modifier = Modifier,
    stale: Boolean = false,
) {
    val colors = CueSeekStatus.colors

    val description = if (stale) {
        "Composition unknown; ${tally.total} services unverified"
    } else {
        buildList {
            if (tally.healthy > 0) add("${tally.healthy} healthy")
            if (tally.degraded > 0) add("${tally.degraded} degraded")
            if (tally.unreachable > 0) add("${tally.unreachable} unreachable")
            if (tally.unknown > 0) add("${tally.unknown} unknown")
        }.joinToString(", ")
    }

    if (stale) {
        Box(
            modifier = modifier
                .fillMaxWidth()
                .height(CueSeekSpacing.tallyHeight)
                .drawBehind {
                    drawLine(
                        color = colors.unknownOutline,
                        start = Offset(0f, size.height / 2f),
                        end = Offset(size.width, size.height / 2f),
                        strokeWidth = 1.5.dp.toPx(),
                        pathEffect = PathEffect.dashPathEffect(
                            floatArrayOf(4.dp.toPx(), 3.dp.toPx()),
                        ),
                    )
                }
                .clearAndSetSemantics { contentDescription = description }
        )
        return
    }

    // Segments animate their weight rather than snapping, so a service changing status
    // reads as a change to the fleet rather than a flicker.
    val healthy by animateFloatAsState(tally.healthy.toFloat(), CueSeekMotion.tallySpring(), label = "h")
    val degraded by animateFloatAsState(tally.degraded.toFloat(), CueSeekMotion.tallySpring(), label = "d")
    val unreachable by animateFloatAsState(tally.unreachable.toFloat(), CueSeekMotion.tallySpring(), label = "u")
    val unknown by animateFloatAsState(tally.unknown.toFloat(), CueSeekMotion.tallySpring(), label = "k")

    Row(
        modifier = modifier
            .fillMaxWidth()
            .height(CueSeekSpacing.tallyHeight)
            .clearAndSetSemantics { contentDescription = description },
        horizontalArrangement = Arrangement.spacedBy(3.dp),
    ) {
        // Dimmed foregrounds, not the status containers. The containers read correctly
        // behind a 17dp icon but measure 1.5:1 against the dark page and vanish at 8dp.
        Segment(healthy, colors.tallyOnHealthy)
        Segment(degraded, colors.tallyOnDegraded)
        Segment(unreachable, colors.tallyOnUnreachable)
        Segment(unknown, colors.unknownOutline)
    }
}

@Composable
private fun androidx.compose.foundation.layout.RowScope.Segment(weight: Float, color: androidx.compose.ui.graphics.Color) {
    if (weight <= 0.01f) return
    Box(
        modifier = Modifier
            .weight(weight)
            .height(CueSeekSpacing.tallyHeight)
            .background(color, CueSeekShapes.TallySegment)
    )
}
