package dev.cueseek.android.ui.dashboard

import android.provider.Settings
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.width
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.pulltorefresh.PullToRefreshDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.layout.layout
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.design.token.CueSeekShapes
import kotlin.math.absoluteValue

/**
 * The pull-to-refresh indicator, as a rule rather than a spinner.
 *
 * A circular spinner would be the default and would be wrong here. This screen already has
 * a rule that means "the state of everything" — the tally under the headline — and pulling
 * downward should feel like reaching for *that*, not like summoning a generic progress
 * widget from another app. So the indicator is the same 8dp rounded rule, and the gesture
 * reads as drawing the summary down to ask it again.
 *
 * Three states, and each is a statement:
 *
 * - **Pulling.** The rule fills from the centre in proportion to the pull, dim and
 *   unsaturated. Chroma means attention in this palette, so something that has not yet
 *   committed to anything has almost none.
 * - **Armed.** Crossing the threshold takes the rule to full width and to full primary in
 *   one step. Deliberately a step and not a ramp: the user needs to know they have crossed
 *   a line, and a gradient cannot say "now".
 * - **Refreshing.** A segment sweeps the track while the request is genuinely outstanding.
 *
 * That sweep is the one repeating animation in the app, and it is admissible under the
 * motion rule for the same reason a hold-to-confirm fill is: it is bounded by a real
 * operation and stops when the operation does. Nothing here loops while the app is idle.
 *
 * There is no completion flourish, on purpose. When the data lands the beat dot pulses and
 * the tally rule resizes — the app already says "something arrived" in its own vocabulary,
 * and a tick here would be a second, louder answer to a question already answered.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BoxScope.RefreshRule(
    state: PullToRefreshState,
    refreshing: Boolean,
    modifier: Modifier = Modifier,
) {
    val fraction = state.distanceFraction
    if (fraction <= 0f && !refreshing) return

    val armed = fraction >= 1f || refreshing

    val color by animateColorAsState(
        targetValue = if (armed) {
            MaterialTheme.colorScheme.primary
        } else {
            MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.45f)
        },
        animationSpec = tween(CueSeekMotion.DurationStateChange),
        label = "refreshArmed",
    )

    // Settles to a full track the moment a refresh starts, so the transition from "you let
    // go" to "it is working" is one continuous object rather than a swap.
    val fill by animateFloatAsState(
        targetValue = if (refreshing) 1f else fraction.coerceAtMost(1f),
        animationSpec = tween(CueSeekMotion.DurationExit, easing = CueSeekMotion.Emphasized),
        label = "refreshFill",
    )

    Box(
        modifier = modifier
            .align(Alignment.TopCenter)
            .graphicsLayer {
                // Rides down with the pull and parks just below the top edge while
                // refreshing. Overshoot past the threshold is damped to a quarter so the
                // rule stops travelling once the gesture has already been understood.
                val overshoot = (fraction - 1f).coerceAtLeast(0f) * 0.25f
                val travel = (fraction.coerceAtMost(1f) + overshoot) * Threshold.toPx()
                translationY = travel - size.height
                alpha = if (refreshing) 1f else fraction.coerceIn(0f, 1f)
            }
            .width(TrackWidth)
            .height(RuleHeight)
            .clip(CueSeekShapes.Shapes.extraSmall)
            .semantics {
                if (refreshing) contentDescription = "Refreshing"
            },
        contentAlignment = Alignment.Center,
    ) {
        Box(
            Modifier
                .fillMaxWidth()
                .fillMaxHeight()
                .background(MaterialTheme.colorScheme.onSurface.copy(alpha = 0.08f))
        )

        if (refreshing && animationsEnabled()) {
            SweepingSegment(color)
        } else if (refreshing) {
            // Animations are switched off system-wide. The request is still real, so the
            // track still says so — as a full armed rule rather than as nothing, because
            // the honest alternative to a moving indicator is a static one, not silence.
            Box(Modifier.fillMaxWidth().fillMaxHeight().background(color))
        } else {
            Box(
                Modifier
                    .fillMaxWidth(fill)
                    .fillMaxHeight()
                    .background(color)
            )
        }
    }
}

/**
 * Whether the system wants animation at all.
 *
 * Accessibility settings and battery savers both zero the animator scale, and a user who
 * has asked for stillness has asked for it here too. Read once and remembered: this is a
 * settings lookup, not something to do on every frame of a gesture.
 */
@Composable
private fun animationsEnabled(): Boolean {
    val context = LocalContext.current
    return remember(context) {
        Settings.Global.getFloat(
            context.contentResolver,
            Settings.Global.ANIMATOR_DURATION_SCALE,
            1f,
        ) != 0f
    }
}

/**
 * A segment travelling the track while a request is outstanding.
 *
 * It eases at the turns rather than sliding linearly, which is what keeps it from reading
 * as a loading bar borrowed from a web page. The reverse repeat means it never jumps: the
 * segment always occupies a position it could have travelled to.
 */
@Composable
private fun SweepingSegment(color: Color) {
    val transition = rememberInfiniteTransition(label = "refreshSweep")
    val position by transition.animateFloat(
        initialValue = 0f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(
            animation = tween(SweepMillis, easing = LinearEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "refreshSweepPosition",
    )

    // Eased by hand rather than by the animation spec: a reversing tween would pause at
    // each end, and a rule that hesitates looks like a rule that has stalled.
    val eased = 1f - (2f * position - 1f).absoluteValue

    Box(
        Modifier
            .fillMaxHeight()
            .fillMaxWidth(SegmentFraction)
            .layout { measurable, constraints ->
                val placeable = measurable.measure(constraints)
                val travel = constraints.maxWidth - placeable.width
                layout(constraints.maxWidth, placeable.height) {
                    placeable.placeRelative((travel * eased).toInt(), 0)
                }
            }
            .clip(CueSeekShapes.Shapes.extraSmall)
            .background(color)
    )
}

/** Matches [androidx.compose.material3.pulltorefresh.PullToRefreshDefaults.PositionalThreshold]. */
private val Threshold = PullToRefreshDefaults.PositionalThreshold

/** The tally rule's height, because this is meant to read as the same object. */
private val RuleHeight = 8.dp

private val TrackWidth = 96.dp

private const val SegmentFraction = 0.4f
private const val SweepMillis = 900
