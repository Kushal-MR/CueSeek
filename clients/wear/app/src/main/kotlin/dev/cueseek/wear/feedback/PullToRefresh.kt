package dev.cueseek.wear.feedback

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.nestedscroll.NestedScrollConnection
import androidx.compose.ui.input.nestedscroll.NestedScrollSource
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.Velocity
import androidx.compose.ui.unit.dp
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.wear.compose.material3.MaterialTheme
import dev.cueseek.wear.detail.animationsEnabled

/**
 * Pull down at the top of a list to read the agent again.
 *
 * # Why the watch needed it
 *
 * The watch reads the agent when the app opens and on returning from ambient, and otherwise
 * not at all — it holds no stream (ADR-0004), on purpose, because a radio kept awake for a
 * screen nobody is looking at is the battery cost M5.10 exists to avoid. That left one gap,
 * and Kushal found it on his wrist in M5.17: start playback on Jellyfin while CueSeek is open,
 * and the watch keeps showing the moment it opened until you leave and come back.
 *
 * The phone answers "show me now" with pull-to-refresh, so the watch does the same rather
 * than inventing a second gesture for one idea.
 *
 * # Why it is written here
 *
 * Wear Compose has no pull-to-refresh, and the phone's lives in phone Material 3, which this
 * module cannot see (ADR-0010, enforced since M5.2). It is small: a nested-scroll connection
 * that collects the downward drag a list at its top cannot use, a threshold felt as the same
 * haptic every other commitment in the app uses, and a refresh on release past it.
 *
 * Every gesture needs a non-gesture equivalent (DESIGN.md §9), so the list also carries a
 * "Refresh" accessibility action.
 */
@Stable
class PullToRefreshState internal constructor(
    private val thresholdPx: Float,
    private val onThreshold: () -> Unit,
    private val onRefresh: () -> Unit,
) {
    /** How far the pull has gone, from 0 to 1 at the threshold. Drawn by [PullIndicator]. */
    var progress by mutableFloatStateOf(0f)
        private set

    /** How far the list is drawn down to make room for the ring, in pixels. */
    var offsetPx by mutableFloatStateOf(0f)
        private set

    internal val restingOffsetPx = thresholdPx

    private var pulled = 0f

    private fun update(delta: Float) {
        val before = pulled >= thresholdPx
        pulled = (pulled + delta).coerceIn(0f, thresholdPx * 1.5f)
        progress = (pulled / thresholdPx).coerceAtMost(1f)
        offsetPx = pulled.coerceAtMost(thresholdPx)
        if (!before && pulled >= thresholdPx) onThreshold()
    }

    internal val connection = object : NestedScrollConnection {
        // Pushing back up while a pull is in progress shrinks the pull before the list
        // scrolls, so a change of mind is not a refresh.
        override fun onPreScroll(available: Offset, source: NestedScrollSource): Offset {
            if (pulled > 0f && available.y < 0f) {
                val before = pulled
                update(available.y)
                return Offset(0f, pulled - before)
            }
            return Offset.Zero
        }

        // Only what the list could not use: a downward drag left over means it is already at
        // its top. A fling's leftovers are ignored — a refresh is asked for by a finger, not
        // by momentum from a scroll that happened to end at the top.
        override fun onPostScroll(
            consumed: Offset,
            available: Offset,
            source: NestedScrollSource,
        ): Offset {
            if (source == NestedScrollSource.UserInput && available.y > 0f) {
                update(available.y * 0.5f)
                return Offset(0f, available.y)
            }
            return Offset.Zero
        }

        override suspend fun onPreFling(available: Velocity): Velocity {
            val fire = pulled >= thresholdPx
            pulled = 0f
            progress = 0f
            offsetPx = 0f
            if (fire) onRefresh()
            return Velocity.Zero
        }
    }
}

@Composable
fun rememberPullToRefresh(onRefresh: () -> Unit): PullToRefreshState {
    val haptics = rememberCueSeekHaptics()
    val threshold = with(LocalDensity.current) { 48.dp.toPx() }
    val latest by rememberUpdatedState(onRefresh)
    return remember(threshold) {
        PullToRefreshState(
            thresholdPx = threshold,
            onThreshold = { haptics.committed() },
            onRefresh = { latest() },
        )
    }
}

/**
 * Attach to the scrolling list. Also exposes the gesture as an accessibility action.
 *
 * The list is drawn down while pulled, and held down while the agent is read, so the ring has
 * its own space between the clock and the first item. The first version drew the ring over
 * the top of the list instead, which put it on top of the time — the one thing the top of a
 * watch screen is always showing (M5.17, on the wrist).
 */
@Composable
fun Modifier.pullToRefresh(
    state: PullToRefreshState,
    refreshing: Boolean,
    onRefresh: () -> Unit,
): Modifier {
    val target = if (refreshing) state.restingOffsetPx else state.offsetPx
    val offset by animateFloatAsState(target, label = "pull offset")
    return this
        .nestedScroll(state.connection)
        .graphicsLayer { translationY = offset }
        .semantics {
            customActions = listOf(CustomAccessibilityAction("Refresh") { onRefresh(); true })
        }
}

/**
 * A screen with the pull ring layered over it: filling while pulled, turning while the agent is
 * being read, nothing otherwise. Under the clock, in the gap [pullToRefresh] opens by drawing
 * the list down.
 *
 * **Over the scaffold, not inside it.** The ring was first drawn inside `ScreenScaffold`'s
 * content box, aligned top-centre — which is the screen's top centre only for the scaffold
 * without an edge button. The one *with* an edge button (the dashboard of any watch granted
 * `host.power`) does not span the screen, so the ring landed as a sliver on the left edge. It
 * passed on a debug watch paired without power and failed on the release watch paired with it
 * (after v0.1.3). A full-screen layer has one centre whatever the scaffold.
 */
@Composable
fun PullRefreshFrame(
    state: PullToRefreshState,
    refreshing: Boolean,
    content: @Composable () -> Unit,
) {
    PullRefreshFrame(
        showRing = refreshing || state.progress > 0f,
        progress = if (refreshing) null else state.progress,
        content = content,
    )
}

@Composable
internal fun PullRefreshFrame(showRing: Boolean, progress: Float?, content: @Composable () -> Unit) {
    Box(modifier = Modifier.fillMaxSize()) {
        content()
        if (showRing) {
            PullRing(
                progress = progress,
                modifier = Modifier
                    .align(Alignment.TopCenter)
                    // Below the time text, which owns roughly the top 30dp of a round screen.
                    .padding(top = 34.dp),
            )
        }
    }
}

/** @param progress 0..1 while pulled; null while reading, when it turns instead. */
@Composable
internal fun PullRing(progress: Float?, modifier: Modifier = Modifier) {
    val track = MaterialTheme.colorScheme.surfaceContainerHigh
    val arc = MaterialTheme.colorScheme.primary
    val turning = if (progress == null && animationsEnabled()) {
        val spin = rememberInfiniteTransition(label = "pull ring")
        spin.animateFloat(
            initialValue = 0f,
            targetValue = 360f,
            animationSpec = infiniteRepeatable(tween(900, easing = LinearEasing)),
            label = "pull ring angle",
        ).value
    } else {
        0f
    }
    Canvas(modifier = modifier.size(24.dp)) {
        val stroke = 3.dp.toPx()
        val inset = stroke / 2
        val box = Size(size.width - stroke, size.height - stroke)
        val origin = Offset(inset, inset)
        drawArc(track, 0f, 360f, false, origin, box, style = Stroke(stroke))
        val sweep = progress?.let { 360f * it } ?: 100f
        val start = if (progress == null) turning - 90f else -90f
        drawArc(arc, start, sweep, false, origin, box, style = Stroke(stroke, cap = StrokeCap.Round))
    }
}
