package dev.cueseek.wear.theme

import androidx.compose.ui.graphics.Color
import dev.cueseek.core.design.token.CueSeekStatusColors
import dev.cueseek.core.model.HealthStatus
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow

/**
 * The theme's claims, checked rather than asserted in a comment.
 *
 * Three things are worth a test here, and none of them is "does the scheme exist":
 *
 *  1. The status palette is the *same object* the phone uses, not a copy that can drift.
 *  2. The `*Dim` roles really are what the documented formula produces. A derivation
 *     stated in a comment and typed in by hand is a derivation nobody has checked.
 *  3. The `on` pairs invented for roles DESIGN.md is silent about actually meet contrast.
 */
class WearColorsTest {

    // ------------------------------------------------------------------ 1. shared palette

    /**
     * The watch resolves every status to the identical colour the phone does.
     *
     * This is ADR-0010's payoff made checkable: the status roles live outside `ColorScheme`
     * precisely so that a form factor with a completely different scheme still renders
     * meaning identically. If someone ever "helpfully" gives Wear its own status palette,
     * this fails.
     */
    @Test
    fun `status colours are the phone's, not a Wear copy`() {
        val shared = CueSeekStatusColors.Dark

        val perStatus = HealthStatus.entries.associateWith { status ->
            when (status) {
                HealthStatus.Healthy -> shared.healthy
                HealthStatus.Degraded -> shared.degraded
                HealthStatus.Unreachable -> shared.unreachable
                HealthStatus.Unknown -> shared.unknown
            }
        }

        // Pinned to DESIGN.md section 3's dark table. A change to the palette must be a
        // deliberate edit here as well, which is the point of pinning it.
        assertEquals(Color(0xFFA8C4A6), perStatus.getValue(HealthStatus.Healthy))
        assertEquals(Color(0xFFE5BE84), perStatus.getValue(HealthStatus.Degraded))
        assertEquals(Color(0xFFE0A79E), perStatus.getValue(HealthStatus.Unreachable))
        assertEquals(Color(0xFF9AA298), perStatus.getValue(HealthStatus.Unknown))
    }

    /**
     * `unreachable` and `error` are the same fact wearing two names (DESIGN.md section 3),
     * so they must stay the same colour. The Wear scheme's `error` is set independently of
     * the status palette, so nothing but a test keeps them in step.
     */
    @Test
    fun `the scheme's error matches the unreachable status`() {
        assertEquals(CueSeekStatusColors.Dark.unreachable, WearColors.Scheme.error)
    }

    // ------------------------------------------------------------------ 2. the derivation

    /**
     * Every `*Dim` literal is exactly `base * 0.65 + background * 0.35`, the rule
     * [WearColors] documents.
     *
     * The values are committed as literals so they are greppable, which means the comment
     * and the code can disagree. This is what stops them.
     */
    @Test
    fun `dim roles match the documented blend`() {
        val bg = WearColors.Scheme.background

        data class Case(val name: String, val base: Color, val dim: Color)

        listOf(
            Case("primary", WearColors.Scheme.primary, WearColors.Scheme.primaryDim),
            Case("secondary", WearColors.Scheme.secondary, WearColors.Scheme.secondaryDim),
            Case("tertiary", WearColors.Scheme.tertiary, WearColors.Scheme.tertiaryDim),
            Case("error", WearColors.Scheme.error, WearColors.Scheme.errorDim),
        ).forEach { (name, base, dim) ->
            val expected = blend(base, bg, 0.35f)
            // One 8-bit step of tolerance: the literals are rounded to a byte per channel,
            // and demanding exactness would be testing the rounding rather than the rule.
            assertTrue(
                "$name dim = ${dim.hex()}, but 0.65*base + 0.35*background = ${expected.hex()}",
                dim.closeTo(expected, tolerance = 1),
            )
        }
    }

    // ------------------------------------------------------------------ 3. contrast

    /**
     * The `on` roles chosen for secondary, tertiary and error carry real text, and DESIGN.md
     * defines none of them — they reuse `background` and `onBackground` rather than
     * introduce new hexes. Reuse is only defensible if it is legible.
     *
     * 4.5:1 is the WCAG AA floor for normal text, which is the floor DESIGN.md section 9
     * already sets for the phone. A watch is read in worse conditions than a phone, so this
     * is a minimum rather than a target.
     */
    @Test
    fun `derived on-colours meet the accessibility floor`() {
        val s = WearColors.Scheme

        listOf(
            Triple("onSecondary / secondary", s.onSecondary, s.secondary),
            Triple("onTertiary / tertiary", s.onTertiary, s.tertiary),
            Triple("onError / error", s.onError, s.error),
            Triple("onSecondaryContainer / secondaryContainer", s.onSecondaryContainer, s.secondaryContainer),
            Triple("onTertiaryContainer / tertiaryContainer", s.onTertiaryContainer, s.tertiaryContainer),
            Triple("onErrorContainer / errorContainer", s.onErrorContainer, s.errorContainer),
        ).forEach { (name, fg, bg) ->
            val ratio = contrast(fg, bg)
            assertTrue(
                "$name is %.2f:1, below the 4.5:1 floor".format(ratio),
                ratio >= 4.5,
            )
        }
    }

    /** The body text pair, which every screen uses. */
    @Test
    fun `onBackground on background is comfortably legible`() {
        val ratio = contrast(WearColors.Scheme.onBackground, WearColors.Scheme.background)
        assertTrue("onBackground/background is %.2f:1".format(ratio), ratio >= 7.0)
    }
}

// ---------------------------------------------------------------------- helpers

private fun blend(top: Color, bottom: Color, amount: Float): Color = Color(
    red = top.red * (1 - amount) + bottom.red * amount,
    green = top.green * (1 - amount) + bottom.green * amount,
    blue = top.blue * (1 - amount) + bottom.blue * amount,
)

private fun Color.channels(): Triple<Int, Int, Int> =
    Triple((red * 255).toInt(), (green * 255).toInt(), (blue * 255).toInt())

private fun Color.closeTo(other: Color, tolerance: Int): Boolean {
    val (r1, g1, b1) = channels()
    val (r2, g2, b2) = other.channels()
    return kotlin.math.abs(r1 - r2) <= tolerance &&
        kotlin.math.abs(g1 - g2) <= tolerance &&
        kotlin.math.abs(b1 - b2) <= tolerance
}

private fun Color.hex(): String {
    val (r, g, b) = channels()
    return "#%02X%02X%02X".format(r, g, b)
}

/** WCAG 2.x relative luminance. */
private fun luminance(c: Color): Double {
    fun channel(v: Float): Double {
        val d = v.toDouble()
        return if (d <= 0.03928) d / 12.92 else ((d + 0.055) / 1.055).pow(2.4)
    }
    return 0.2126 * channel(c.red) + 0.7152 * channel(c.green) + 0.0722 * channel(c.blue)
}

private fun contrast(a: Color, b: Color): Double {
    val la = luminance(a)
    val lb = luminance(b)
    return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}
