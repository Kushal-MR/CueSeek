package dev.cueseek.core.design.token

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.LineHeightStyle
import androidx.compose.ui.unit.sp
import dev.cueseek.core.design.R

/**
 * CueSeek's type, as tokens.
 *
 * IBM Plex Sans, with IBM Plex Mono for anything numeric. Plex was drawn for a technology
 * company's own system software, and it shows: flat terminals, slightly squared curves, a
 * feel that reads precise rather than friendly. The mono sibling is why Plex was chosen
 * over an equally legible neutral like Inter — timestamps and ages need true tabular
 * figures, and taking them from the same superfamily keeps the screen one voice instead of
 * two.
 *
 * Mono is confined to **data**: ages, timestamps, counts. The interface language stays
 * proportional, which is the line between instrumentation and terminal pastiche.
 *
 * This departs from M3's Roboto default deliberately. The Material 3 skill permits exactly
 * that — "replace only when intentionally customizing the type scale" — so the roles stay
 * M3 while the faces and metrics become ours.
 *
 * Two weights only, 400 and 500. Hierarchy comes from size, tracking and colour; reaching
 * for a third weight is usually a sign the hierarchy is not working.
 *
 * Tracking tightens as size grows — negative at 24sp, neutral in the middle, open at 12sp.
 * Small type needs air; large type needs discipline.
 */
object CueSeekType {

    val PlexSans = FontFamily(
        Font(R.font.plex_sans_regular, FontWeight.Normal),
        Font(R.font.plex_sans_medium, FontWeight.Medium),
    )

    val PlexMono = FontFamily(
        Font(R.font.plex_mono_regular, FontWeight.Normal),
        Font(R.font.plex_mono_medium, FontWeight.Medium),
    )

    /**
     * Trims the extra leading Compose adds above the first line and below the last.
     *
     * Without this, a 24sp headline in a 30sp line box sits optically low inside its own
     * padding, and no amount of margin tuning fixes it — the gap is inside the text node.
     */
    private val Trim = LineHeightStyle(
        alignment = LineHeightStyle.Alignment.Center,
        trim = LineHeightStyle.Trim.None,
    )

    private fun sans(
        size: Int,
        line: Int,
        weight: FontWeight,
        tracking: Double,
    ) = TextStyle(
        fontFamily = PlexSans,
        fontWeight = weight,
        fontSize = size.sp,
        lineHeight = line.sp,
        letterSpacing = tracking.sp,
        lineHeightStyle = Trim,
    )

    private fun mono(
        size: Int,
        line: Int,
        weight: FontWeight,
    ) = TextStyle(
        fontFamily = PlexMono,
        fontWeight = weight,
        fontSize = size.sp,
        lineHeight = line.sp,
        // Mono is already even; added tracking only makes numbers drift apart.
        letterSpacing = 0.sp,
        lineHeightStyle = Trim,
    )

    /**
     * The M3 roles, in Plex.
     *
     * Only the roles CueSeek actually uses are tuned. Display is absent from the app on
     * purpose — an operational screen has no room for 36sp — but the scale still defines
     * it so a stray `displaySmall` does not fall back to Roboto and look foreign.
     */
    val Typography = Typography(
        displaySmall = sans(36, 44, FontWeight.Normal, -0.5),
        headlineLarge = sans(30, 38, FontWeight.Normal, -0.4),
        headlineMedium = sans(26, 34, FontWeight.Medium, -0.35),
        /** The verdict. The largest thing on screen and the only headline. */
        headlineSmall = sans(24, 30, FontWeight.Medium, -0.3),
        titleLarge = sans(20, 26, FontWeight.Medium, -0.2),
        /** Service names. */
        titleMedium = sans(16, 22, FontWeight.Medium, -0.1),
        titleSmall = sans(14, 20, FontWeight.Medium, 0.0),
        bodyLarge = sans(16, 24, FontWeight.Normal, 0.0),
        bodyMedium = sans(14, 20, FontWeight.Normal, 0.1),
        /**
         * Supporting lines. 13sp rather than M3's 12 — Plex has a more modest x-height
         * than Roboto and 12sp goes weak next to a 16sp Medium name.
         */
        bodySmall = sans(13, 18, FontWeight.Normal, 0.1),
        labelLarge = sans(14, 20, FontWeight.Medium, 0.1),
        /** The host eyebrow. Open tracking is what stops 12sp Medium reading as cramped. */
        labelMedium = sans(12, 16, FontWeight.Medium, 0.5),
        labelSmall = sans(11, 16, FontWeight.Medium, 0.5),
    )

    /**
     * Numerics. Not an M3 role, because M3 has no concept of "this text is data".
     *
     * Every age, timestamp and count uses one of these, which is what keeps the roster's
     * right edge from jittering as `4s` becomes `11s`.
     */
    object Data {
        /** Ages and timestamps in rows and metadata. */
        val Small = mono(12, 16, FontWeight.Normal)
        /** Counts in the summary, where the number is the point. */
        val Emphasis = mono(12, 16, FontWeight.Medium)
        val Medium = mono(14, 20, FontWeight.Normal)
    }
}
