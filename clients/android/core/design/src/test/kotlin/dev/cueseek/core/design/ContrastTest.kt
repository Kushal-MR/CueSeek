package dev.cueseek.core.design

import androidx.compose.ui.graphics.Color
import dev.cueseek.core.design.token.CueSeekColors
import dev.cueseek.core.design.token.CueSeekStatusColors
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow

/**
 * Contrast, measured rather than eyeballed.
 *
 * ADR-0010 makes status rendering correctness-critical, and a sage palette is exactly the
 * kind of low-chroma family where contrast quietly fails — every one of these pairs looked
 * fine before it was measured, and two of them were not. Keeping the check in the build
 * means a future palette tweak cannot silently drop below the floor.
 *
 * WCAG AA: 4.5:1 for normal text, 3:1 for meaningful non-text (icons, boundaries).
 */
class ContrastTest {

    private fun luminance(c: Color): Double {
        fun channel(v: Float): Double {
            val d = v.toDouble()
            return if (d <= 0.03928) d / 12.92 else ((d + 0.055) / 1.055).pow(2.4)
        }
        return 0.2126 * channel(c.red) + 0.7152 * channel(c.green) + 0.0722 * channel(c.blue)
    }

    private fun ratio(a: Color, b: Color): Double {
        val la = luminance(a)
        val lb = luminance(b)
        return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
    }

    private fun assertContrast(name: String, fg: Color, bg: Color, min: Double) {
        val r = ratio(fg, bg)
        assertTrue(
            "$name: ${"%.2f".format(r)}:1, needs $min:1",
            r >= min,
        )
    }

    private val light = CueSeekColors.LightScheme
    private val dark = CueSeekColors.DarkScheme
    private val sLight = CueSeekStatusColors.Light
    private val sDark = CueSeekStatusColors.Dark

    @Test
    fun `light text meets AA`() {
        assertContrast("service name", light.onSurface, light.surfaceContainerLowest, 4.5)
        assertContrast("supporting", light.onSurfaceVariant, light.surfaceContainerLowest, 4.5)
        assertContrast("verdict", light.onSurface, light.surface, 4.5)
        assertContrast("eyebrow", light.onSurfaceVariant, light.surface, 4.5)
    }

    @Test
    fun `dark text meets AA`() {
        assertContrast("service name", dark.onSurface, dark.surfaceContainer, 4.5)
        assertContrast("supporting", dark.onSurfaceVariant, dark.surfaceContainer, 4.5)
        assertContrast("verdict", dark.onSurface, dark.surface, 4.5)
        assertContrast("eyebrow", dark.onSurfaceVariant, dark.surface, 4.5)
    }

    @Test
    fun `status icons meet the non-text floor`() {
        assertContrast("healthy light", sLight.healthy, sLight.healthyContainer, 3.0)
        assertContrast("degraded light", sLight.degraded, sLight.degradedContainer, 3.0)
        assertContrast("unreachable light", sLight.unreachable, sLight.unreachableContainer, 3.0)
        assertContrast("unknown light", sLight.unknown, light.surface, 3.0)

        assertContrast("healthy dark", sDark.healthy, sDark.healthyContainer, 3.0)
        assertContrast("degraded dark", sDark.degraded, sDark.degradedContainer, 3.0)
        assertContrast("unreachable dark", sDark.unreachable, sDark.unreachableContainer, 3.0)
        assertContrast("unknown dark", sDark.unknown, dark.surface, 3.0)
    }

    @Test
    fun `the open circle is a boundary, so it meets 3 to 1`() {
        // This is the pair that failed first time round at 2.00 and 2.95. The dashed ring
        // is the only thing distinguishing "no answer" from "an answer", so it has to be
        // visible on both the page and the roster.
        assertContrast("ring on page light", sLight.unknownOutline, light.surface, 3.0)
        assertContrast("ring on roster light", sLight.unknownOutline, light.surfaceContainerLowest, 3.0)
        assertContrast("ring on page dark", sDark.unknownOutline, dark.surface, 3.0)
        assertContrast("ring on roster dark", sDark.unknownOutline, dark.surfaceContainer, 3.0)
    }

    @Test
    fun `the tally rule is visible at 8dp in both themes`() {
        // The status containers measured 1.5:1 against the dark page and disappeared, which
        // is why the rule has its own dimmed values rather than reusing them.
        assertContrast("tally healthy light", sLight.tallyOnHealthy, light.surface, 3.0)
        assertContrast("tally degraded light", sLight.tallyOnDegraded, light.surface, 3.0)
        assertContrast("tally unreachable light", sLight.tallyOnUnreachable, light.surface, 3.0)

        assertContrast("tally healthy dark", sDark.tallyOnHealthy, dark.surface, 3.0)
        assertContrast("tally degraded dark", sDark.tallyOnDegraded, dark.surface, 3.0)
        assertContrast("tally unreachable dark", sDark.tallyOnUnreachable, dark.surface, 3.0)
    }

    @Test
    fun `the beat dot is visible`() {
        assertContrast("beat light", sLight.beat, light.surface, 3.0)
        assertContrast("beat dark", sDark.beat, dark.surface, 3.0)
    }

    @Test
    fun `unreachable and the error role agree`() {
        // Two names for the same fact. If they drift, an M3 component reaching for the
        // error role lands beside the status language instead of inside it.
        assertTrue("light", sLight.unreachable == light.error)
        assertTrue("dark", sDark.unreachable == dark.error)
    }

    @Test
    fun `colour alone does not distinguish healthy from unknown`() {
        // Not a failure - a design fact worth pinning. These are close in luminance by
        // design, which is why shape and icon carry the encoding. If someone "fixes" this
        // by pushing healthy toward a vivid green, this test says what they broke.
        val lightSeparation = ratio(sLight.healthy, sLight.unknown)
        val darkSeparation = ratio(sDark.healthy, sDark.unknown)
        assertTrue(
            "healthy/unknown should stay close so shape carries meaning, was $lightSeparation",
            lightSeparation < 2.0,
        )
        assertTrue(
            "healthy/unknown should stay close so shape carries meaning, was $darkSeparation",
            darkSeparation < 2.0,
        )
    }
}
