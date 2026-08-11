package dev.cueseek.android.ui.dashboard

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import dev.cueseek.android.ui.capability.CapabilitySection
import dev.cueseek.core.design.status.riskStyle
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.model.Action
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Service
import kotlinx.coroutines.launch

/**
 * One service, in detail.
 *
 * A sheet rather than a screen: inspecting a service is a glance deeper, not a journey,
 * and predictive back handles dismissal for free. Capabilities render through the registry,
 * so this composable never learns what a Jellyfin is.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ServiceSheet(
    service: Service,
    host: PairedHost,
    stale: Boolean,
    onDismiss: () -> Unit,
    onInvoke: (Action) -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val scope = rememberCoroutineScope()

    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(start = 24.dp, end = 24.dp, bottom = 32.dp),
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            Text(service.name, style = MaterialTheme.typography.headlineSmall)

            service.capabilities.forEach { capability ->
                CapabilitySection(capability = capability, service = service, stale = stale)
            }

            if (service.actions.isNotEmpty()) {
                if (!host.canControlServices()) {
                    // Client-side gating is user experience, not a control - the agent
                    // enforces scopes and every call still handles a 403. Saying why the
                    // buttons are absent is better than silently omitting them.
                    Text(
                        "This device was paired without permission to control services.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        service.actions.forEach { action ->
                            ActionControl(action = action) {
                                scope.launch {
                                    sheetState.hide()
                                    onInvoke(action)
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

/**
 * An action, with as much ceremony as its risk warrants.
 *
 * The policy comes from the domain rather than from here, so a new risk level ships to an
 * existing client with the right prompt already attached (ADR-0005). `safe` invokes on
 * tap; `disruptive` asks; `destructive` and anything unrecognised must be held.
 */
@Composable
private fun ActionControl(action: Action, onConfirmed: () -> Unit) {
    val style = riskStyle(action.risk)
    var asking by remember { mutableStateOf(false) }

    when {
        !style.confirm -> {
            Button(onClick = onConfirmed, modifier = Modifier.fillMaxWidth()) {
                Text(action.label)
            }
        }

        style.emphatic -> HoldToConfirmButton(label = action.label, onConfirmed = onConfirmed)

        !asking -> {
            Button(onClick = { asking = true }, modifier = Modifier.fillMaxWidth()) {
                Text(action.label)
            }
        }

        else -> {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    action.description ?: "This interrupts the service while it restarts.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = { asking = false }, modifier = Modifier.weight(1f)) {
                        Text("Cancel")
                    }
                    Button(onClick = onConfirmed, modifier = Modifier.weight(1f)) {
                        Text("Confirm")
                    }
                }
            }
        }
    }
}

/**
 * A button that must be held.
 *
 * Effort in proportion to consequence: the one gesture in the app that is deliberately
 * harder than a tap is the one that can take something down. The fill is the progress, so
 * releasing early visibly abandons it rather than silently doing nothing.
 */
@Composable
private fun HoldToConfirmButton(label: String, onConfirmed: () -> Unit) {
    val progress = remember { Animatable(0f) }
    val scope = rememberCoroutineScope()
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

private const val HOLD_MILLIS = 1200
