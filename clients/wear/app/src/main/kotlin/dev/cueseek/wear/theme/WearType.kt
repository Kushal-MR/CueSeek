package dev.cueseek.wear.theme

import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.wear.compose.material3.Typography
import dev.cueseek.core.design.R

/**
 * IBM Plex on a watch.
 *
 * # The faces are shared; the scale is not
 *
 * The four Plex faces come from `:core:design`'s resources — one copy of the fonts for the
 * whole project, which is what makes "the tokens are shared" true rather than aspirational.
 * The *sizes* are Wear's own: DESIGN.md's scale was built for a 6-inch screen, and a 233dp
 * round display is a different problem.
 *
 * # Wear has a numeral scale, and it is exactly the rule DESIGN.md already had
 *
 * DESIGN.md §4: *"Mono is confined to data — ages, timestamps, counts. Interface language
 * stays proportional. That line is what separates instrumentation from terminal pastiche."*
 *
 * On the phone that rule is carried by two hand-rolled roles, `Data.Small` and
 * `Data.Emphasis`, because Material 3's type scale has nowhere to put it. Wear Material 3
 * has five `numeral*` roles as a first-class part of its scale. So the rule stops being a
 * convention the codebase maintains and becomes the thing the type system already models:
 * **every numeral role is Plex Mono, every other role is Plex Sans.**
 *
 * That is the tidiest thing found so far about building on Wear, and it should feed back
 * into DESIGN.md's open questions rather than staying here.
 *
 * # Weights
 *
 * Two, 400 and 500, exactly as DESIGN.md mandates. Hierarchy comes from size and colour.
 * Only the roles this app actually renders are overridden; the rest keep Wear's defaults,
 * which are already tuned for the form factor and are better than a guess.
 */
internal object WearType {

    val PlexSans = FontFamily(
        Font(R.font.plex_sans_regular, FontWeight.Normal),
        Font(R.font.plex_sans_medium, FontWeight.Medium),
    )

    val PlexMono = FontFamily(
        Font(R.font.plex_mono_regular, FontWeight.Normal),
        Font(R.font.plex_mono_medium, FontWeight.Medium),
    )

    /**
     * Wear's default scale with CueSeek's faces substituted.
     *
     * Built by copying each default and replacing only `fontFamily`, so the sizes, line
     * heights and tracking Google measured for a round screen survive. Choosing our own
     * numbers here would mean re-deriving work that has already been done on hardware we
     * do not have, to fix a problem we have not yet observed.
     *
     * The sizes get revisited in M5.17, when the type has been read on a wrist in daylight
     * — which is the only evidence that would justify changing them.
     */
    val Typography: Typography = Typography().let { d ->
        Typography(
            arcLarge = d.arcLarge.copy(fontFamily = PlexSans),
            arcMedium = d.arcMedium.copy(fontFamily = PlexSans),
            arcSmall = d.arcSmall.copy(fontFamily = PlexSans),

            displayLarge = d.displayLarge.copy(fontFamily = PlexSans),
            displayMedium = d.displayMedium.copy(fontFamily = PlexSans),
            displaySmall = d.displaySmall.copy(fontFamily = PlexSans),

            titleLarge = d.titleLarge.copy(fontFamily = PlexSans),
            titleMedium = d.titleMedium.copy(fontFamily = PlexSans),
            titleSmall = d.titleSmall.copy(fontFamily = PlexSans),

            labelLarge = d.labelLarge.copy(fontFamily = PlexSans),
            labelMedium = d.labelMedium.copy(fontFamily = PlexSans),
            labelSmall = d.labelSmall.copy(fontFamily = PlexSans),

            bodyLarge = d.bodyLarge.copy(fontFamily = PlexSans),
            bodyMedium = d.bodyMedium.copy(fontFamily = PlexSans),
            bodySmall = d.bodySmall.copy(fontFamily = PlexSans),
            bodyExtraSmall = d.bodyExtraSmall.copy(fontFamily = PlexSans),

            // Mono, all five. This is DESIGN.md's rule expressed as configuration rather
            // than as discipline.
            numeralExtraLarge = d.numeralExtraLarge.copy(fontFamily = PlexMono),
            numeralLarge = d.numeralLarge.copy(fontFamily = PlexMono),
            numeralMedium = d.numeralMedium.copy(fontFamily = PlexMono),
            numeralSmall = d.numeralSmall.copy(fontFamily = PlexMono),
            numeralExtraSmall = d.numeralExtraSmall.copy(fontFamily = PlexMono),
        )
    }
}
