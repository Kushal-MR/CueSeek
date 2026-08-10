package dev.cueseek.core.design

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalContext
import dev.cueseek.core.design.token.CueSeekColors
import dev.cueseek.core.design.token.CueSeekShapes
import dev.cueseek.core.design.token.CueSeekStatusColors
import dev.cueseek.core.design.token.CueSeekType

/**
 * Status colours, reached through the theme rather than imported directly.
 *
 * A `CompositionLocal` rather than a global object so that light and dark resolve the same
 * way every other colour does, and so a preview or a screenshot test can pin a theme
 * without the status palette silently staying light.
 */
val LocalStatusColors = staticCompositionLocalOf { CueSeekStatusColors.Light }

/**
 * CueSeek's theme.
 *
 * @param dynamicColor when true and the device supports it (API 31+), **surfaces** follow
 *   the user's wallpaper. Status colours never do — see the note below.
 */
@Composable
fun CueSeekTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    dynamicColor: Boolean = true,
    content: @Composable () -> Unit,
) {
    val colorScheme = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> {
            val context = LocalContext.current
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        }

        darkTheme -> CueSeekColors.DarkScheme
        else -> CueSeekColors.LightScheme
    }

    // Never dynamic. ADR-0010: status colours carry meaning, and meaning must not be
    // themeable. M3 sets the precedent with `error`, which is also static.
    val statusColors =
        if (darkTheme) CueSeekStatusColors.Dark else CueSeekStatusColors.Light

    CompositionLocalProvider(LocalStatusColors provides statusColors) {
        MaterialTheme(
            colorScheme = colorScheme,
            typography = CueSeekType.Typography,
            shapes = CueSeekShapes.Shapes,
            content = content,
        )
    }
}

/** Entry point for composables: `CueSeekStatus.colors`. */
object CueSeekStatus {
    val colors: CueSeekStatusColors
        @Composable
        @ReadOnlyComposable
        get() = LocalStatusColors.current
}
