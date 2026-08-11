package dev.cueseek.core.design

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.Color
import app.cash.paparazzi.DeviceConfig
import app.cash.paparazzi.Paparazzi
import com.android.ide.common.rendering.api.SessionParams
import dev.cueseek.core.design.catalogue.StatusCatalogue
import dev.cueseek.core.design.catalogue.TypeCatalogue
import dev.cueseek.core.design.token.CueSeekStatusColors
import org.junit.Rule
import org.junit.Test
import kotlin.math.pow

/**
 * Golden images for the status language.
 *
 * Scoped to `:core:design`'s catalogue and nothing else. Screenshot tests are famously
 * noisy, and they earn their keep here only because status rendering is correctness-
 * critical and the subject barely changes — pinning feature screens that are still being
 * designed would train everyone to re-record without looking.
 */
class CatalogueGoldenTest {

    @get:Rule
    val paparazzi = Paparazzi(
        deviceConfig = DeviceConfig.PIXEL_5,
        renderingMode = SessionParams.RenderingMode.SHRINK,
        showSystemUi = false,
        // These goldens are recorded on Windows and verified on Linux CI, and layoutlib's
        // text rasterisation is not bit-identical across platforms. A small tolerance keeps
        // the suite from failing on sub-pixel antialiasing while still catching every change
        // that a person would actually see - a colour, a shape, a weight, a position.
        maxPercentDifference = 0.25,
    )

    @Test
    fun status_light() {
        paparazzi.snapshot {
            CueSeekTheme(darkTheme = false, dynamicColor = false) { StatusCatalogue() }
        }
    }

    @Test
    fun status_dark() {
        paparazzi.snapshot {
            CueSeekTheme(darkTheme = true, dynamicColor = false) { StatusCatalogue() }
        }
    }

    @Test
    fun type_light() {
        paparazzi.snapshot {
            CueSeekTheme(darkTheme = false, dynamicColor = false) { TypeCatalogue() }
        }
    }

    @Test
    fun type_dark() {
        paparazzi.snapshot {
            CueSeekTheme(darkTheme = true, dynamicColor = false) { TypeCatalogue() }
        }
    }

    /**
     * The catalogue with every status colour replaced by its own luminance grey.
     *
     * This is the claim "colour is not load-bearing", rendered. Hue is gone; shape, icon
     * and label are all that remain. If a future change makes two statuses tell apart only
     * by colour, this golden is where it shows — and it shows as an image a person can
     * look at, which no contrast assertion can do.
     */
    @Test
    fun status_light_greyscale() {
        paparazzi.snapshot {
            CueSeekTheme(darkTheme = false, dynamicColor = false) {
                CompositionLocalProvider(
                    LocalStatusColors provides CueSeekStatusColors.Light.desaturated()
                ) {
                    StatusCatalogue()
                }
            }
        }
    }

    @Test
    fun status_dark_greyscale() {
        paparazzi.snapshot {
            CueSeekTheme(darkTheme = true, dynamicColor = false) {
                CompositionLocalProvider(
                    LocalStatusColors provides CueSeekStatusColors.Dark.desaturated()
                ) {
                    StatusCatalogue()
                }
            }
        }
    }
}

/** Replaces each colour with the grey of identical relative luminance. */
private fun CueSeekStatusColors.desaturated(): CueSeekStatusColors = CueSeekStatusColors(
    healthy = healthy.grey(),
    healthyContainer = healthyContainer.grey(),
    degraded = degraded.grey(),
    degradedContainer = degradedContainer.grey(),
    unreachable = unreachable.grey(),
    unreachableContainer = unreachableContainer.grey(),
    unknown = unknown.grey(),
    unknownOutline = unknownOutline.grey(),
    beat = beat.grey(),
    tallyOnHealthy = tallyOnHealthy.grey(),
    tallyOnDegraded = tallyOnDegraded.grey(),
    tallyOnUnreachable = tallyOnUnreachable.grey(),
)

private fun Color.grey(): Color {
    if (alpha == 0f) return this
    fun lin(v: Float): Double {
        val d = v.toDouble()
        return if (d <= 0.03928) d / 12.92 else ((d + 0.055) / 1.055).pow(2.4)
    }
    val y = 0.2126 * lin(red) + 0.7152 * lin(green) + 0.0722 * lin(blue)
    val srgb = if (y <= 0.0031308) y * 12.92 else 1.055 * y.pow(1 / 2.4) - 0.055
    val v = srgb.toFloat().coerceIn(0f, 1f)
    return Color(v, v, v, alpha)
}
