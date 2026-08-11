package dev.cueseek.core.design.status

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.unit.dp
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.design.token.CueSeekShapes
import dev.cueseek.core.design.token.CueSeekSpacing
import dev.cueseek.core.model.HealthStatus

/**
 * A service's status, as a 32dp mark.
 *
 * The silhouette is the primary encoding. A **closed, filled circle** means we hold an
 * answer; an **open, dashed circle** means we do not. That reads at a glance, survives
 * greyscale, and does not depend on telling sage from grey — which matters, because
 * healthy and unknown are only 1.21:1 apart in luminance.
 *
 * The whole mark carries one content description and hides its internals from
 * accessibility services, so TalkBack announces "Jellyfin, Healthy" rather than reading a
 * decorative icon separately from its label.
 */
@Composable
fun StatusMark(
    status: HealthStatus,
    modifier: Modifier = Modifier,
    stale: Boolean = false,
) {
    val style = statusStyle(status, stale)
    val outline = dev.cueseek.core.design.CueSeekStatus.colors.unknownOutline

    val container by animateColorAsState(
        targetValue = style.container,
        animationSpec = tween(CueSeekMotion.DurationStateChange, easing = CueSeekMotion.Emphasized),
        label = "statusContainer",
    )
    val content by animateColorAsState(
        targetValue = style.content,
        animationSpec = tween(CueSeekMotion.DurationStateChange, easing = CueSeekMotion.Emphasized),
        label = "statusContent",
    )

    Box(
        modifier = modifier
            .size(CueSeekSpacing.markSize)
            .then(
                if (style.verified) {
                    Modifier.background(container, CueSeekShapes.Mark)
                } else {
                    // Dashed rather than solid: an outline alone would read as a disabled
                    // control. A broken line reads as an unfinished statement.
                    Modifier.drawBehind {
                        drawCircle(
                            color = outline,
                            radius = size.minDimension / 2f - 1.dp.toPx(),
                            style = Stroke(
                                width = 1.5.dp.toPx(),
                                pathEffect = PathEffect.dashPathEffect(
                                    floatArrayOf(3.dp.toPx(), 2.5.dp.toPx()),
                                ),
                            ),
                        )
                    }
                }
            )
            .clearAndSetSemantics { contentDescription = style.label },
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = style.icon,
            contentDescription = null,
            tint = content,
            modifier = Modifier.size(17.dp),
        )
    }
}

/**
 * The beat dot.
 *
 * CueSeek's one recurring detail. It appears immediately before any statement about
 * freshness and nowhere else, so its meaning never dilutes: filled and pulsing when events
 * are arriving, open when they are not.
 *
 * The pulse is driven by [pulseKey] — pass the sequence number or arrival timestamp of the
 * last event, and the dot beats once each time it changes. It is not a looping animation:
 * a dot that pulses on a timer would keep beating while the stream is frozen, which is
 * exactly the lie this app exists to avoid.
 */
@Composable
fun BeatDot(
    modifier: Modifier = Modifier,
    live: Boolean = true,
    pulseKey: Any? = null,
) {
    val colors = dev.cueseek.core.design.CueSeekStatus.colors
    val scale = remember { Animatable(1f) }

    LaunchedEffect(pulseKey, live) {
        if (!live || pulseKey == null) return@LaunchedEffect
        scale.animateTo(
            CueSeekMotion.Beat.PeakScale,
            tween(CueSeekMotion.Beat.ExpandMillis, easing = CueSeekMotion.EmphasizedDecelerate),
        )
        scale.animateTo(
            1f,
            tween(CueSeekMotion.Beat.SettleMillis, easing = CueSeekMotion.Emphasized),
        )
    }

    Box(
        modifier = modifier
            .size(CueSeekSpacing.beatSize)
            .graphicsLayer { scaleX = scale.value; scaleY = scale.value }
            .then(
                if (live) {
                    Modifier.background(colors.beat, CueSeekShapes.Mark)
                } else {
                    Modifier.border(1.5.dp, colors.unknownOutline, CueSeekShapes.Mark)
                }
            )
            .clearAndSetSemantics { }
    )
}
