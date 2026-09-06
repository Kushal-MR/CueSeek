package dev.cueseek.wear.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.wear.compose.material3.MaterialTheme
import dev.cueseek.core.design.LocalStatusColors
import dev.cueseek.core.design.token.CueSeekStatusColors

/**
 * CueSeek's theme on Wear.
 *
 * The watch half of ADR-0010. The phone's [dev.cueseek.core.design.CueSeekTheme] and this
 * one share their tokens and share nothing else: this wraps Wear's `MaterialTheme`, that
 * one wraps the phone's, and the two `MaterialTheme`s are unrelated types from unrelated
 * libraries.
 *
 * # The status palette crosses unchanged, and that is the point
 *
 * `CueSeekStatusColors` is provided through the same `LocalStatusColors` the phone uses,
 * holding the same values from the same file. It could be, because ADR-0010 put the status
 * roles **outside** `ColorScheme` on purpose — meaning must not be themeable, so it was
 * never entangled with a scheme that turned out to be form-factor-specific.
 *
 * That decision was made in M2 to keep `error` from being repainted by a wallpaper. It
 * paid for itself here, three milestones later, in a place nobody was aiming at: every
 * other colour needed remapping onto Wear's roles, and the ones that carry meaning needed
 * nothing at all.
 *
 * Always the dark variant — see [WearColors] for why the watch has no light theme.
 */
@Composable
fun CueSeekWearTheme(content: @Composable () -> Unit) {
    CompositionLocalProvider(LocalStatusColors provides CueSeekStatusColors.Dark) {
        MaterialTheme(
            colorScheme = WearColors.Scheme,
            typography = WearType.Typography,
            content = content,
        )
    }
}
