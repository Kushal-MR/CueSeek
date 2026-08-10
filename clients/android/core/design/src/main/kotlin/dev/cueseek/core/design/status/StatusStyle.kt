package dev.cueseek.core.design.status

import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.design.icon.CueSeekIcons
import dev.cueseek.core.model.ActionRisk
import dev.cueseek.core.model.HealthStatus

/**
 * How one status looks and what it is called.
 *
 * @param verified whether this style states a fact we currently hold. When false the mark
 *   is drawn as an open, dashed circle rather than a filled one — the difference between
 *   "this is the answer" and "we do not have one".
 */
@Immutable
data class StatusStyle(
    val icon: ImageVector,
    val content: Color,
    val container: Color,
    val label: String,
    val verified: Boolean,
)

/**
 * The status language, as a total function.
 *
 * Deliberately a `when` with no `else`. `HealthStatus` is a closed set the contract owns,
 * and if the agent grows a fifth value this stops compiling — which is the correct
 * outcome. A default branch would render the new status as whatever the fallback happened
 * to be and nobody would find out.
 *
 * Three independent encodings, and colour is the weakest of them:
 *
 *  1. **shape** — a closed circle when we have an answer, an open dashed one when we do not
 *  2. **icon** — check, warning, block, or a clock when the answer is out of date
 *  3. **label** — the word itself, which is what a screen reader announces
 *
 * That redundancy is not belt-and-braces. Healthy and unknown differ by only 1.21:1 in
 * luminance, so anyone who cannot separate the hues is reading the shape and the glyph.
 */
@Composable
@ReadOnlyComposable
fun statusStyle(status: HealthStatus, stale: Boolean = false): StatusStyle {
    val c = CueSeekStatus.colors

    // Stale is not a fifth status. It is a statement about confidence that applies on top
    // of whatever the status was, so it overrides presentation without replacing meaning.
    if (stale) {
        return StatusStyle(
            icon = CueSeekIcons.History,
            content = c.unknown,
            container = Color.Transparent,
            label = "Unverified",
            verified = false,
        )
    }

    return when (status) {
        HealthStatus.Healthy -> StatusStyle(
            icon = CueSeekIcons.Check,
            content = c.healthy,
            container = c.healthyContainer,
            label = "Healthy",
            verified = true,
        )

        HealthStatus.Degraded -> StatusStyle(
            icon = CueSeekIcons.Warning,
            content = c.degraded,
            container = c.degradedContainer,
            label = "Degraded",
            verified = true,
        )

        HealthStatus.Unreachable -> StatusStyle(
            icon = CueSeekIcons.Block,
            content = c.unreachable,
            container = c.unreachableContainer,
            label = "Unreachable",
            verified = true,
        )

        // Unknown before any data is a different sentence from unknown because data went
        // quiet, but both are honestly "we do not know", and both are drawn open.
        HealthStatus.Unknown -> StatusStyle(
            icon = CueSeekIcons.Question,
            content = c.unknown,
            container = Color.Transparent,
            label = "Unknown",
            verified = false,
        )
    }
}

/**
 * How much ceremony an action's risk warrants.
 *
 * The policy already lives in the domain — [ActionRisk.requiresConfirmation] and
 * [ActionRisk.requiresEmphaticConfirmation] — so this only decides how it looks. A screen
 * that re-derives the policy is a screen that will eventually disagree with another one.
 */
@Immutable
data class RiskStyle(
    val confirm: Boolean,
    val emphatic: Boolean,
    val label: String,
)

/** Total over [ActionRisk], for the same reason [statusStyle] is total over status. */
fun riskStyle(risk: ActionRisk): RiskStyle = when (risk) {
    ActionRisk.Safe -> RiskStyle(confirm = false, emphatic = false, label = "Safe")
    ActionRisk.Disruptive -> RiskStyle(confirm = true, emphatic = false, label = "Disruptive")
    ActionRisk.Destructive -> RiskStyle(confirm = true, emphatic = true, label = "Destructive")
    // An unrecognised risk is treated as the most dangerous one we know about. Defaulting
    // to safe would invoke a future action of unknown consequence with no prompt on every
    // client that predates it.
    ActionRisk.Unrecognised -> RiskStyle(confirm = true, emphatic = true, label = "Unknown risk")
}
