package dev.cueseek.core.design.token

import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Immutable
import androidx.compose.ui.graphics.Color

/**
 * CueSeek's colour, as tokens.
 *
 * The palette is a muted sage/eucalyptus family, and the governing rule is that **chroma
 * means attention**. Healthy is very nearly achromatic, so a machine with nothing wrong
 * renders calm and grey-green; the only real colour on screen belongs to the services that
 * need someone. A dashboard where green is always present is a dashboard where green means
 * nothing.
 *
 * Neither theme uses pure white or pure black — both ends carry a green cast, so the app
 * reads as one material rather than as ink on paper.
 */
object CueSeekColors {

    // ---------------------------------------------------------------- light
    val LightScheme = lightColorScheme(
        primary = Color(0xFF4C6650),
        onPrimary = Color(0xFFFFFFFF),
        primaryContainer = Color(0xFFD3E3D2),
        onPrimaryContainer = Color(0xFF0A2010),
        secondary = Color(0xFF5A6459),
        onSecondary = Color(0xFFFFFFFF),
        secondaryContainer = Color(0xFFDEE5DB),
        onSecondaryContainer = Color(0xFF171D16),
        tertiary = Color(0xFF3B6664),
        onTertiary = Color(0xFFFFFFFF),
        tertiaryContainer = Color(0xFFCDE3E0),
        onTertiaryContainer = Color(0xFF00201F),
        background = Color(0xFFF1F4EE),
        onBackground = Color(0xFF191D18),
        surface = Color(0xFFF1F4EE),
        onSurface = Color(0xFF191D18),
        onSurfaceVariant = Color(0xFF434840),
        surfaceContainerLowest = Color(0xFFFCFDFA),
        surfaceContainerLow = Color(0xFFF1F4EE),
        surfaceContainer = Color(0xFFEBEEE8),
        surfaceContainerHigh = Color(0xFFE5E9E2),
        surfaceContainerHighest = Color(0xFFE0E4DC),
        outline = Color(0xFF737870),
        outlineVariant = Color(0xFFC3C8BE),
        // Unreachable and `error` are the same fact wearing two names, so they are the
        // same colour. Keeping them in step means an M3 component that reaches for the
        // error role lands inside the status language rather than beside it.
        error = Color(0xFF8C3A31),
        onError = Color(0xFFFFFFFF),
        errorContainer = Color(0xFFF0DCD8),
        onErrorContainer = Color(0xFF35120E),
    )

    // ----------------------------------------------------------------- dark
    //
    // Not the light scheme inverted. Light puts brighter content on a tinted ground and
    // separates rows with hairlines; dark raises the container above a deeper page and
    // separates with tone, because a drop shadow does nothing on a dark ground. Container
    // chroma is pulled back so amber and clay do not glow at low brightness.
    val DarkScheme = darkColorScheme(
        primary = Color(0xFFB1CDB2),
        onPrimary = Color(0xFF1D3722),
        primaryContainer = Color(0xFF344E38),
        onPrimaryContainer = Color(0xFFCDE8CF),
        secondary = Color(0xFFC2CBBF),
        onSecondary = Color(0xFF2C332C),
        secondaryContainer = Color(0xFF424A41),
        onSecondaryContainer = Color(0xFFDEE5DB),
        tertiary = Color(0xFFA3CDC9),
        onTertiary = Color(0xFF043735),
        tertiaryContainer = Color(0xFF224E4C),
        onTertiaryContainer = Color(0xFFCDE3E0),
        background = Color(0xFF0E1210),
        onBackground = Color(0xFFE2E7DE),
        surface = Color(0xFF0E1210),
        onSurface = Color(0xFFE2E7DE),
        onSurfaceVariant = Color(0xFFB4BCB1),
        surfaceContainerLowest = Color(0xFF090C09),
        surfaceContainerLow = Color(0xFF161B15),
        surfaceContainer = Color(0xFF191E18),
        surfaceContainerHigh = Color(0xFF232821),
        surfaceContainerHighest = Color(0xFF272D26),
        outline = Color(0xFF8C948A),
        outlineVariant = Color(0xFF333A32),
        error = Color(0xFFE0A79E),
        onError = Color(0xFF52241E),
        errorContainer = Color(0xFF52241E),
        onErrorContainer = Color(0xFFF0DCD8),
    )
}

/**
 * The status palette.
 *
 * Deliberately **not** part of [androidx.compose.material3.ColorScheme] and deliberately
 * **not** derived from dynamic colour. M3 already establishes the precedent: `error` is a
 * static role that does not follow the wallpaper, because it means something. These roles
 * mean something too, so ADR-0010's rule — "status colours win; meaning must not be
 * themeable" — is expressed by keeping them outside the scheme entirely.
 *
 * [tallyOn] exists because the container colours, which read correctly behind a 17dp icon,
 * measure 1.5:1 against the dark page and vanish in an 8dp rule. The rule gets its own
 * dimmed values rather than reusing containers at a size they were not chosen for.
 */
@Immutable
data class CueSeekStatusColors(
    val healthy: Color,
    val healthyContainer: Color,
    val degraded: Color,
    val degradedContainer: Color,
    val unreachable: Color,
    val unreachableContainer: Color,
    val unknown: Color,
    /** Outline for the open circle and the dashed tally rule. Meets 3:1 on both surfaces. */
    val unknownOutline: Color,
    /** The beat dot: every statement about freshness carries one. */
    val beat: Color,
    val tallyOnHealthy: Color,
    val tallyOnDegraded: Color,
    val tallyOnUnreachable: Color,
) {
    companion object {
        val Light = CueSeekStatusColors(
            healthy = Color(0xFF3F5E46),
            healthyContainer = Color(0xFFE3EBE0),
            degraded = Color(0xFF7A5215),
            degradedContainer = Color(0xFFF3E2C4),
            unreachable = Color(0xFF8C3A31),
            unreachableContainer = Color(0xFFF0DCD8),
            unknown = Color(0xFF5E6560),
            // 3.12:1 on the page, 3.40:1 on the roster. The first value that passes; any
            // lighter and the open circle stops being a boundary.
            unknownOutline = Color(0xFF848C82),
            beat = Color(0xFF4C6650),
            // Dimmed foregrounds, not the containers. The containers measured 1.10 /
            // 1.15 / 1.19 against the page and the rule was effectively invisible - caught
            // by the golden, not by eye. 3.35 / 3.57 / 3.56 now.
            tallyOnHealthy = Color(0xFF6E8C70),
            tallyOnDegraded = Color(0xFF9E7A38),
            tallyOnUnreachable = Color(0xFFAE7068),
        )

        val Dark = CueSeekStatusColors(
            healthy = Color(0xFFA8C4A6),
            healthyContainer = Color(0xFF29392B),
            degraded = Color(0xFFE5BE84),
            degradedContainer = Color(0xFF46320F),
            unreachable = Color(0xFFE0A79E),
            unreachableContainer = Color(0xFF52241E),
            unknown = Color(0xFF9AA298),
            // 3.80:1 on the page, 3.38:1 on the roster.
            unknownOutline = Color(0xFF6B736B),
            beat = Color(0xFFA8C4A6),
            // Dimmed foregrounds rather than containers: 3.93 / 4.33 / 3.69 against the
            // page, where the containers managed 1.54 / 1.55 / 1.46 and disappeared.
            tallyOnHealthy = Color(0xFF5A7A5C),
            tallyOnDegraded = Color(0xFF9A722C),
            tallyOnUnreachable = Color(0xFF9A5E55),
        )
    }
}
