package dev.cueseek.core.design.token

import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.Easing
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.SpringSpec
import androidx.compose.animation.core.spring
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/**
 * CueSeek's motion.
 *
 * One rule governs everything: **motion means data arrived.** The beat pulses when an
 * event lands, the tally resizes when the counts change, and a status mark settles when it
 * changes. Nothing else on the screen moves, so movement is never decorative — if it
 * animates, something happened.
 *
 * That rule is why there is no ambient motion, no shimmer, and no looping placeholder.
 * A shimmering skeleton would mean "we are working"; on this screen the honest signal for
 * "we do not know yet" is stillness plus [dev.cueseek.core.model.HealthStatus.Unknown].
 *
 * Spring physics for anything that changes size or position, per M3 Expressive; the legacy
 * easing curves remain correct for enter/exit transitions.
 */
object CueSeekMotion {

    /** M3 emphasized easing, for content entering and leaving. */
    val Emphasized: Easing = CubicBezierEasing(0.2f, 0f, 0f, 1f)
    val EmphasizedDecelerate: Easing = CubicBezierEasing(0.05f, 0.7f, 0.1f, 1f)
    val EmphasizedAccelerate: Easing = CubicBezierEasing(0.3f, 0f, 0.8f, 0.15f)

    const val DurationEnter: Int = 400
    const val DurationExit: Int = 200
    const val DurationStandard: Int = 300

    /**
     * The tally rule resizing as counts change.
     *
     * Low stiffness with no bounce: a service going degraded should not feel playful.
     */
    fun <T> tallySpring(): SpringSpec<T> = spring(
        dampingRatio = Spring.DampingRatioNoBouncy,
        stiffness = Spring.StiffnessMediumLow,
    )

    /** A status mark changing state. Fast enough to feel immediate, slow enough to notice. */
    fun <T> markSpring(): SpringSpec<T> = spring(
        dampingRatio = Spring.DampingRatioNoBouncy,
        stiffness = Spring.StiffnessMedium,
    )

    /** Colour and alpha crossfades, including fill to outline when data goes stale. */
    const val DurationStateChange: Int = 220

    /**
     * The beat.
     *
     * One pulse per received event — roughly every 15 seconds against a live agent, which
     * is deliberately slow. A dot that blinks constantly is noise; a dot that moves twice a
     * minute is a pulse you notice the absence of.
     */
    object Beat {
        const val ExpandMillis: Int = 180
        const val SettleMillis: Int = 520
        const val PeakScale: Float = 1.55f
        const val TroughAlpha: Float = 0.55f
    }
}

/** Spacing, on the 8dp grid M3 Expressive asks for. */
object CueSeekSpacing {
    val screenMargin: Dp = 16.dp
    val rosterInset: Dp = 8.dp
    val rowHorizontal: Dp = 16.dp
    val rowVertical: Dp = 12.dp

    /**
     * Dark rows are 2dp taller than light ones.
     *
     * Not an oversight. Dark mode drops divider contrast almost to nothing and separates by
     * tone instead, and low-contrast type needs air more than it needs lines.
     */
    val rowVerticalDark: Dp = 14.dp

    /** The gap between the status mark and the text column. */
    val markGap: Dp = 16.dp

    /** 16 margin + 32 mark + 16 gap. Dividers inset to exactly where text starts. */
    val textStart: Dp = 64.dp

    val markSize: Dp = 32.dp
    val beatSize: Dp = 6.dp
    val tallyHeight: Dp = 8.dp
    val touchTarget: Dp = 48.dp
}
