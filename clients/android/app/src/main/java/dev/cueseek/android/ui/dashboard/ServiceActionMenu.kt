package dev.cueseek.android.ui.dashboard

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import dev.cueseek.core.design.icon.CueSeekIcons
import dev.cueseek.core.design.status.riskStyle
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.design.token.CueSeekShapes
import dev.cueseek.core.model.Action
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Service

/**
 * A service's operational actions, behind the row's trailing button.
 *
 * The row now has two jobs and therefore two targets. Tapping the body *uses* the service —
 * it opens the thing itself. This button *operates on* it: restart, stop, start. Keeping
 * them apart is what makes the body safe to tap, and it is the same separation a desktop
 * makes between opening a file and right-clicking it.
 *
 * Nothing here knows what a Jellyfin is. The menu lists whatever [Service.actions] contains
 * and gates each entry on the risk the agent assigned, so a verb this build has never heard
 * of arrives with the right amount of ceremony already attached (ADR-0005).
 */
@Composable
fun ServiceActionMenu(
    service: Service,
    host: PairedHost,
    onInvoke: (Action) -> Unit,
    modifier: Modifier = Modifier,
) {
    var open by remember { mutableStateOf(false) }
    var confirming by remember { mutableStateOf<Action?>(null) }

    Box(modifier) {
        IconButton(
            onClick = { open = true },
            // The two targets are announced differently on purpose. A screen reader user
            // moving through the row hears "Jellyfin, Healthy" then "Actions for Jellyfin",
            // which is the whole interaction model stated out loud.
            modifier = Modifier.semantics {
                contentDescription = "Actions for ${service.name}"
            },
        ) {
            Icon(
                CueSeekIcons.More,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(18.dp),
            )
        }

        DropdownMenu(
            expanded = open,
            onDismissRequest = { open = false },
            shape = CueSeekShapes.Shapes.large,
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ) {
            if (!host.canControlServices()) {
                // Client-side gating is user experience, not a control: the agent enforces
                // scopes and every call still handles a 403. An empty menu would read as a
                // bug, so it says why instead.
                Text(
                    "This device was paired without permission to control services.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
                )
            } else {
                service.actions.forEach { action ->
                    val style = riskStyle(action.risk)
                    DropdownMenuItem(
                        text = {
                            Text(
                                action.label,
                                color = if (style.emphatic) {
                                    MaterialTheme.colorScheme.error
                                } else {
                                    MaterialTheme.colorScheme.onSurface
                                },
                            )
                        },
                        onClick = {
                            open = false
                            // Safe verbs fire from the menu; anything heavier leaves the
                            // menu and asks in a dialog, because a consequence explained
                            // inside a menu item is a consequence nobody reads.
                            if (style.confirm) confirming = action else onInvoke(action)
                        },
                    )
                }
            }
        }
    }

    confirming?.let { action ->
        ActionConfirmation(
            action = action,
            onDismiss = { confirming = null },
            onConfirmed = {
                confirming = null
                onInvoke(action)
            },
        )
    }
}

/**
 * Asks before an action that costs something.
 *
 * Two tiers, both driven by [riskStyle] rather than by the verb: `disruptive` is a dialog
 * with a button, `destructive` and anything unrecognised is a dialog with a gesture that
 * has to be sustained. The description is the agent's own — it is the only text that knows
 * what stopping *this* service actually costs.
 */
@Composable
private fun ActionConfirmation(
    action: Action,
    onDismiss: () -> Unit,
    onConfirmed: () -> Unit,
) {
    val style = riskStyle(action.risk)

    AlertDialog(
        onDismissRequest = onDismiss,
        // The label alone. The agent already names the service in it — "Stop Jellyfin" —
        // and appending our own produced "Stop Jellyfin Jellyfin?" on the device. Copy the
        // agent owns is copy this screen does not get to decorate.
        title = { Text("${action.label}?") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Text(
                    action.description ?: "This interrupts the service while it runs.",
                    style = MaterialTheme.typography.bodyMedium,
                )
                // The hold bar lives in the body rather than the button row so it gets the
                // dialog's full width. A gesture control squeezed to the size of a text
                // button would be a target people miss while trying to be careful.
                if (style.emphatic) {
                    HoldToConfirmButton(label = action.label, onConfirmed = onConfirmed)
                }
            }
        },
        confirmButton = {
            if (style.emphatic) {
                TextButton(onClick = onDismiss) { Text("Cancel") }
            } else {
                TextButton(onClick = onConfirmed) { Text(action.label) }
            }
        },
        dismissButton = if (style.emphatic) {
            null
        } else {
            { TextButton(onClick = onDismiss) { Text("Cancel") } }
        },
    )
}

/**
 * A button that must be held.
 *
 * Effort in proportion to consequence: the one gesture in the app that is deliberately
 * harder than a tap is the one that can take something down. The fill is the progress, so
 * releasing early visibly abandons it rather than silently doing nothing.
 *
 * Shared by the menu and the detail sheet so the two can never drift into asking for
 * different amounts of certainty about the same action.
 */
@Composable
internal fun HoldToConfirmButton(label: String, onConfirmed: () -> Unit) {
    val progress = remember { Animatable(0f) }
    var holding by remember { mutableStateOf(false) }

    LaunchedEffect(holding) {
        if (holding) {
            progress.animateTo(1f, tween(HOLD_MILLIS, easing = CueSeekMotion.Emphasized))
            if (progress.value >= 1f) onConfirmed()
        } else {
            progress.animateTo(0f, tween(CueSeekMotion.DurationExit))
        }
    }

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(48.dp)
            .background(
                MaterialTheme.colorScheme.errorContainer,
                MaterialTheme.shapes.extraLarge,
            )
            .pointerInput(Unit) {
                detectTapGestures(
                    onPress = {
                        holding = true
                        tryAwaitRelease()
                        holding = false
                    },
                )
            }
            .semantics { contentDescription = "$label. Press and hold to confirm." },
        contentAlignment = Alignment.Center,
    ) {
        Box(
            Modifier
                .fillMaxWidth(progress.value)
                .height(48.dp)
                .background(MaterialTheme.colorScheme.error, MaterialTheme.shapes.extraLarge)
        )
        Text(
            if (holding) "Hold…" else "$label — hold",
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onErrorContainer,
        )
    }
}

internal const val HOLD_MILLIS = 1200
