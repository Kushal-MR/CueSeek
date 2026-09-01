package dev.cueseek.core.design.icon

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.addPathNodes
import androidx.compose.ui.unit.dp

/**
 * One icon family, drawn from Material Symbols geometry.
 *
 * Declared as vectors here rather than pulled from `material-icons-extended` for two
 * reasons: that artifact is large and mostly unused, and pinning the exact paths keeps the
 * screenshot goldens stable across dependency bumps — an icon set that silently redraws is
 * a golden suite that fails for no reason.
 *
 * Everything the app draws comes from this file. A second icon language on the same screen
 * is the single most reliable way to make an interface look assembled.
 */
object CueSeekIcons {

    private fun icon(name: String, pathData: String): ImageVector =
        ImageVector.Builder(
            name = name,
            defaultWidth = 24.dp,
            defaultHeight = 24.dp,
            viewportWidth = 24f,
            viewportHeight = 24f,
        ).addPath(
            pathData = addPathNodes(pathData),
            fill = SolidColor(Color.Black),
        ).build()

    /** Healthy. */
    val Check: ImageVector = icon(
        "cueseek_check",
        "M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4z",
    )

    /** Degraded — the service answered and something is wrong. */
    val Warning: ImageVector = icon(
        "cueseek_warning",
        "M1 21h22L12 2 1 21zm12-3h-2v-2h2v2zm0-4h-2v-4h2v4z",
    )

    /** Unreachable — no contact at all. Distinct from degraded on purpose. */
    val Block: ImageVector = icon(
        "cueseek_block",
        "M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zM4 12c0-4.42 " +
            "3.58-8 8-8 1.85 0 3.55.63 4.9 1.69L5.69 16.9C4.63 15.55 4 13.85 4 12zm8 8c-1.85 " +
            "0-3.55-.63-4.9-1.69L18.31 7.1C19.37 8.45 20 10.15 20 12c0 4.42-3.58 8-8 8z",
    )

    /** Unknown — no answer yet. */
    val Question: ImageVector = icon(
        "cueseek_question",
        "M11.07 12.85c.77-1.39 2.25-2.21 3.11-3.44.91-1.29.4-3.7-2.18-3.7-1.69 0-2.52 " +
            "1.28-2.87 2.34L6.54 6.96C7.25 4.83 9.18 3 11.99 3c2.35 0 3.96 1.07 4.78 2.41.7 " +
            "1.15 1.11 3.3.03 4.9-1.2 1.77-2.35 2.31-2.97 3.45-.25.46-.35.76-.35 2.24h-2.89" +
            "c-.01-.78-.13-2.05.48-3.15zM14 20c0 1.1-.9 2-2 2s-2-.9-2-2 .9-2 2-2 2 .9 2 2z",
    )

    /**
     * Stale. A clock, not a warning.
     *
     * The distinction is the whole point: stale is a statement about *time*, not about
     * failure. An alert glyph here would make a working service look broken.
     */
    val History: ImageVector = icon(
        "cueseek_history",
        "M13 3a9 9 0 0 0-9 9H1l3.89 3.89.07.14L9 12H6c0-3.87 3.13-7 7-7s7 3.13 7 7-3.13 " +
            "7-7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42A8.954 8.954 0 0 0 13 21a9 9 0 0 0 " +
            "0-18zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z",
    )

    /** The stream is connected. Stated separately from whether data is fresh. */
    val Sensors: ImageVector = icon(
        "cueseek_sensors",
        "M7.76 16.24A5.98 5.98 0 0 1 6 12c0-1.66.67-3.16 1.76-4.24L6.34 6.34A7.97 7.97 0 0 " +
            "0 4 12c0 2.21.9 4.21 2.34 5.66l1.42-1.42zm8.48 0A5.98 5.98 0 0 0 18 12c0-1.66-.67" +
            "-3.16-1.76-4.24l1.42-1.42A7.97 7.97 0 0 1 20 12a7.97 7.97 0 0 1-2.34 5.66l-1.42" +
            "-1.42zM12 10a2 2 0 1 0 0 4 2 2 0 0 0 0-4z",
    )

    /** Restart, the only action M2 ships. */
    val Restart: ImageVector = icon(
        "cueseek_restart",
        "M12 5V2L8 6l4 4V7c3.31 0 6 2.69 6 6 0 2.97-2.17 5.43-5 5.91v2.02c3.95-.49 " +
            "7-3.85 7-7.93 0-4.42-3.58-8-8-8zm-6 8c0-1.65.67-3.15 1.76-4.24L6.34 7.34A7.92 " +
            "7.92 0 0 0 4 13c0 4.08 3.05 7.44 7 7.93v-2.02c-2.83-.48-5-2.94-5-5.91z",
    )

    val More: ImageVector = icon(
        "cueseek_more",
        "M12 8c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2zm0 2c-1.1 0-2 .9-2 2s.9 2 2 2 " +
            "2-.9 2-2-.9-2-2-2zm0 6c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2z",
    )

    /**
     * Host power. The universal power symbol, deliberately: this is the one action in the
     * app whose meaning should be obvious before the label is read.
     */
    val Power: ImageVector = icon(
        "cueseek_power",
        "M13 3h-2v10h2V3zm4.83 2.17-1.42 1.42A6.92 6.92 0 0 1 19 12c0 3.87-3.13 7-7 " +
            "7s-7-3.13-7-7c0-2.19 1.01-4.14 2.58-5.42L6.17 5.17A8.93 8.93 0 0 0 3 12a9 9 " +
            "0 0 0 18 0c0-2.74-1.23-5.19-3.17-6.83z",
    )
}
