package dev.cueseek.wear.theme

import androidx.compose.ui.graphics.Color
import androidx.wear.compose.material3.ColorScheme

/**
 * DESIGN.md's palette, mapped onto Wear's colour roles.
 *
 * # Why this is not a copy of the phone's scheme
 *
 * Wear Material 3's `ColorScheme` is a different shape. It has `primaryDim`,
 * `secondaryDim`, `tertiaryDim` and `errorDim`, which the phone has no equivalent for; it
 * has no plain `surface` and no `surfaceContainerLowest`/`Highest`. Copying field by field
 * is not possible, which is ADR-0010's "tokens shared, components not" showing up as a
 * concrete constraint rather than a principle.
 *
 * # Dark only, and that is a decision
 *
 * `CueSeekTheme` on the phone follows the system setting. This does not, and the parameter
 * is absent rather than defaulted so nobody has to guess whether it was considered.
 *
 * A watch panel is OLED and a light theme on it costs battery for every pixel, all day, on
 * a device with a fraction of a phone's cell. DESIGN.md's dark palette is also the one
 * whose contrast was tuned at low brightness — the tally-rule finding was measured there.
 * Wear OS supports a light theme; CueSeek declines it for the same reason it declines
 * dynamic colour.
 *
 * # Where the values come from
 *
 * Every hex below is either **lifted verbatim from DESIGN.md §3 (Dark)** or **derived from
 * one by a stated rule**. None is invented, because a colour nobody can trace is a colour
 * nobody can review.
 */
internal object WearColors {

    // ------------------------------------------------------------------ DESIGN.md §3 Dark
    private val Primary = Color(0xFFB1CDB2)
    private val OnPrimary = Color(0xFF1D3722)
    private val PrimaryContainer = Color(0xFF344E38)
    private val OnPrimaryContainer = Color(0xFFCDE8CF)

    private val Secondary = Color(0xFFC2CBBF)
    private val SecondaryContainer = Color(0xFF424A41)

    private val Tertiary = Color(0xFFA3CDC9)
    private val TertiaryContainer = Color(0xFF224E4C)

    private val Background = Color(0xFF0E1210)
    private val OnBackground = Color(0xFFE2E7DE)
    private val OnSurfaceVariant = Color(0xFFB4BCB1)

    private val SurfaceContainerLow = Color(0xFF161B15)
    private val SurfaceContainer = Color(0xFF191E18)
    private val SurfaceContainerHigh = Color(0xFF232821)

    private val Outline = Color(0xFF8C948A)
    private val OutlineVariant = Color(0xFF333A32)

    private val Error = Color(0xFFE0A79E)
    private val ErrorContainer = Color(0xFF52241E)

    // ------------------------------------------------------------------ derived
    //
    // The `*Dim` roles have no DESIGN.md answer, because the phone's scheme has no such
    // role to have defined one. Wear uses them for a fill that is present but not the
    // primary action.
    //
    // Rule, applied identically to all four: the base colour composited 35% toward the
    // background. Reproducible, reviewable, and it preserves hue — which matters, because
    // DESIGN.md's palette earns its identity from staying in one family.
    //
    //   dim = base * 0.65 + Background * 0.35
    //
    // Recorded as literals rather than computed at runtime so the values are greppable and
    // a golden test pins them.
    private val PrimaryDim = Color(0xFF788B79)
    private val SecondaryDim = Color(0xFF838A82)
    private val TertiaryDim = Color(0xFF6F8B88)
    private val ErrorDim = Color(0xFF97736C)

    // DESIGN.md defines no `on` role for secondary, tertiary or error. Rather than invent
    // four hexes, each reuses the value that already carries the same job:
    //
    //   on<X>          the base is light, so the darkest thing in the palette sits on it
    //   on<X>Container the container is dark, so the page's own foreground sits on it
    //
    // Zero new colours, and the contrast is checked in WearColorsTest rather than assumed.
    private val OnLightFill = Background
    private val OnDarkContainer = OnBackground

    val Scheme = ColorScheme(
        primary = Primary,
        primaryDim = PrimaryDim,
        primaryContainer = PrimaryContainer,
        onPrimary = OnPrimary,
        onPrimaryContainer = OnPrimaryContainer,

        secondary = Secondary,
        secondaryDim = SecondaryDim,
        secondaryContainer = SecondaryContainer,
        onSecondary = OnLightFill,
        onSecondaryContainer = OnDarkContainer,

        tertiary = Tertiary,
        tertiaryDim = TertiaryDim,
        tertiaryContainer = TertiaryContainer,
        onTertiary = OnLightFill,
        onTertiaryContainer = OnDarkContainer,

        surfaceContainerLow = SurfaceContainerLow,
        surfaceContainer = SurfaceContainer,
        surfaceContainerHigh = SurfaceContainerHigh,
        onSurface = OnBackground,
        onSurfaceVariant = OnSurfaceVariant,

        outline = Outline,
        outlineVariant = OutlineVariant,

        background = Background,
        onBackground = OnBackground,

        error = Error,
        errorDim = ErrorDim,
        errorContainer = ErrorContainer,
        onError = OnLightFill,
        onErrorContainer = OnDarkContainer,
    )
}
