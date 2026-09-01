package dev.cueseek.android.ui.dashboard

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.dp
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.model.Action
import dev.cueseek.core.model.Service

/**
 * Powering the machine off, and the one sentence that makes it a considered decision.
 *
 * The confirmation does not block on what the machine is doing, and deliberately so: the
 * operator owns this box and may have excellent reasons to shut it down mid-transcode.
 * Refusing would make the tool argue with the person it exists to serve. What it does
 * instead is *say* what is going on — which is the one thing a console is uniquely placed
 * to know at that moment, and the difference between a considered decision and an accident
 * (ADR-0002 Amendment 2).
 */

/**
 * What is currently in flight across every service, as one phrase, or null when nothing is.
 *
 * Built from the activity capabilities M3.5 already collects, so this costs no new request
 * and no new state. Counts rather than titles: at the instant somebody is deciding whether
 * to shut a machine down, "2 playing" is the fact, and which episode it was is not.
 *
 * Null when nothing is happening, so the confirmation stays quiet rather than reassuring —
 * "nothing is running" is a claim about services whose activity the agent could not read
 * either, and this cannot tell those apart.
 */
internal fun activeWorkSummary(services: List<Service>): String? {
    val playing = services.sumOf { it.nowPlaying?.sessions ?: 0 }
    val transferring = services.sumOf { it.transfers?.active ?: 0 }

    val parts = buildList {
        if (playing > 0) add(if (playing == 1) "1 stream playing" else "$playing streams playing")
        if (transferring > 0) {
            add(if (transferring == 1) "1 transfer running" else "$transferring transfers running")
        }
    }
    return parts.takeIf { it.isNotEmpty() }?.joinToString(" and ")
}

/**
 * Asks before a power action, and says what it will interrupt.
 *
 * Reuses [HoldToConfirmButton] rather than growing a second gesture: both power actions are
 * `destructive`, which is the same tier as stopping a service, and the risk vocabulary is
 * public API shared with clients that already exist. Adding a fourth level to express that
 * a power-off is worse than a stop would force every client to handle a value it has no
 * interaction for. The consequence is stated in words instead, which is where it is read.
 */
@Composable
internal fun HostPowerConfirmation(
    action: Action,
    services: List<Service>,
    onDismiss: () -> Unit,
    onConfirmed: () -> Unit,
) {
    val busy = activeWorkSummary(services)

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("${action.label}?") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Text(
                    // The agent's own description. It is the half that knows whether this
                    // machine comes back on its own.
                    action.description ?: "This affects the whole machine.",
                    style = MaterialTheme.typography.bodyMedium,
                )
                if (busy != null) {
                    Text(
                        "Right now: $busy.",
                        style = MaterialTheme.typography.bodyMedium,
                        // Coloured, because this is the sentence that should give somebody
                        // pause. It is information, not an obstacle — the button below it
                        // works exactly the same either way.
                        color = CueSeekStatus.colors.degraded,
                    )
                }
                HoldToConfirmButton(label = action.label, onConfirmed = onConfirmed)
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
        dismissButton = null,
    )
}
