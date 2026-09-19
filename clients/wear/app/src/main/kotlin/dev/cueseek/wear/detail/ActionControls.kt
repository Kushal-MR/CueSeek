package dev.cueseek.wear.detail

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.wear.compose.material3.Button
import androidx.wear.compose.material3.ButtonDefaults
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.Text
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.model.Action
import dev.cueseek.core.model.ActionRisk
import dev.cueseek.wear.feedback.rememberCueSeekHaptics
import kotlinx.coroutines.launch

/**
 * How long a destructive action must be held.
 *
 * **Not yet measured, and that is the honest state of it.** The M5 plan says this value is
 * measured rather than inherited; it currently *is* the phone's 1200ms, and M5.17 is where a
 * wrist decides whether that is right.
 *
 * The reasoning for starting there rather than lengthening it: the risk a watch adds is not
 * longer accidental contact — a sleeve brush does not sustain 1.2 seconds any more than a
 * pocket does — it is that a *deliberate* hold is harder, on a small target, on a raised arm.
 * Making the duration longer would trade a safety margin that is already sufficient for
 * ergonomics that are already worse. The watch-specific answer is a bigger target, which is
 * below, and haptics, which are M5.8.
 *
 * If M5.17 finds 1200ms unholdable on a raised wrist, the number moves and this comment
 * becomes the record of why it started here.
 */
internal const val HOLD_MILLIS = 1200

/**
 * The actions a service offers, exactly as the agent reports them.
 *
 * State-dependence is the agent's business, not this screen's: `Start` appears only when the
 * unit is inactive because the agent only offers it then (ADR-0002 Amendment 1). A client
 * that decided for itself which actions made sense would be reimplementing the host layer
 * from a snapshot, and would be wrong every time the two disagreed.
 *
 * Risk decides the ceremony, and it comes from the agent too:
 *
 * | risk | what it takes |
 * | --- | --- |
 * | `Safe` | a tap |
 * | `Disruptive` | a tap, then confirm |
 * | `Destructive` | a press and hold |
 * | `Unrecognised` | treated as destructive |
 *
 * **`Unrecognised` gets the strictest treatment**, which is the only safe direction: a risk level
 * this build has never heard of came from a newer agent, and guessing it is mild is the one
 * mistake that cannot be undone by asking again.
 */
@Composable
internal fun ActionButton(
    action: Action,
    enabled: Boolean,
    onConfirmed: () -> Unit,
) {
    var confirming by remember(action.id) { mutableStateOf(false) }
    val haptics = rememberCueSeekHaptics()

    when (action.risk) {
        ActionRisk.Safe -> PlainButton(action.label, enabled, onConfirmed)

        ActionRisk.Disruptive -> {
            if (confirming) {
                ConfirmRow(
                    label = action.label,
                    description = action.description,
                    // The same commitment the hold's threshold marks, reached by a second
                    // tap instead of by time. One vocabulary: whatever the ceremony, the
                    // moment it ends feels the same.
                    onConfirm = { confirming = false; haptics.committed(); onConfirmed() },
                    onCancel = { confirming = false },
                )
            } else {
                PlainButton(action.label, enabled) { confirming = true }
            }
        }

        ActionRisk.Destructive, ActionRisk.Unrecognised ->
            HoldButton(action = action, enabled = enabled, onConfirmed = onConfirmed)
    }
}

@Composable
private fun PlainButton(label: String, enabled: Boolean, onClick: () -> Unit) {
    Button(
        onClick = onClick,
        enabled = enabled,
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 4.dp),
    ) {
        Text(label, maxLines = 1)
    }
}

/**
 * The confirmation for a disruptive action.
 *
 * Inline rather than a dialog: on 233dp a dialog is the screen, so it costs a transition to
 * show what two buttons already say. The service's own description is carried when the agent
 * supplies one — "Anything currently watching will be interrupted" is the agent's sentence,
 * not ours, and it is the part that makes the choice informed.
 */
@Composable
private fun ConfirmRow(
    label: String,
    description: String?,
    onConfirm: () -> Unit,
    onCancel: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 4.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        description?.takeIf { it.isNotBlank() }?.let {
            Text(
                text = it,
                style = MaterialTheme.typography.bodyExtraSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
                modifier = Modifier.fillMaxWidth(),
            )
        }
        Button(
            onClick = onConfirm,
            modifier = Modifier.fillMaxWidth(),
            colors = ButtonDefaults.filledTonalButtonColors(),
        ) {
            Text("$label — confirm", maxLines = 1)
        }
        Button(onClick = onCancel, modifier = Modifier.fillMaxWidth()) {
            Text("Cancel", maxLines = 1)
        }
    }
}

/**
 * Press and hold, for anything that cannot be undone by asking again.
 *
 * Fills from the leading edge while held and fires only on completion. Releasing early
 * animates back rather than snapping, so an interrupted hold reads as "not yet" instead of
 * "nothing happened" — and so the control is legible without a label saying how it works.
 */
@Composable
private fun HoldButton(action: Action, enabled: Boolean, onConfirmed: () -> Unit) {
    val scope = rememberCoroutineScope()
    val progress = remember(action.id) { Animatable(0f) }
    var holding by remember(action.id) { mutableStateOf(false) }
    val haptics = rememberCueSeekHaptics()

    Box(
        modifier = Modifier
            .fillMaxWidth()
            .height(48.dp)
            .padding(top = 4.dp)
            .clip(MaterialTheme.shapes.large)
            .background(CueSeekStatus.colors.unreachableContainer)
            .pointerInput(action.id, enabled) {
                if (!enabled) return@pointerInput
                detectTapGestures(
                    onPress = {
                        holding = true
                        val completed = scope.launch {
                            progress.animateTo(1f, tween(HOLD_MILLIS))
                            // At the threshold, not at the lift. This is the signal that
                            // says "you can stop pressing now", which is worth nothing if it
                            // arrives after you already have. See [CueSeekHaptics.committed]
                            // for what that costs in the cancelled case.
                            haptics.committed()
                        }
                        // Waits for the finger. tryAwaitRelease returns false when the
                        // gesture is cancelled — a scroll stealing the pointer — and that
                        // must not count as a confirmation any more than an early lift does.
                        val released = tryAwaitRelease()
                        holding = false
                        val reached = progress.value >= 1f
                        completed.cancel()
                        if (released && reached) {
                            onConfirmed()
                        }
                        scope.launch { progress.animateTo(0f, tween(180)) }
                    },
                )
            },
        contentAlignment = Alignment.Center,
    ) {
        Box(
            modifier = Modifier
                .fillMaxWidth(progress.value)
                .fillMaxHeight()
                .background(CueSeekStatus.colors.unreachable),
        )
        Text(
            text = if (holding) "Hold…" else "${action.label} — hold",
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurface,
            maxLines = 1,
        )
    }
}
