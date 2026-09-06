package dev.cueseek.wear.pairing

import android.app.Activity
import android.app.RemoteInput
import android.content.Intent
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.wear.compose.foundation.lazy.TransformingLazyColumn
import androidx.wear.compose.foundation.lazy.rememberTransformingLazyColumnState
import androidx.wear.compose.material3.Button
import androidx.wear.compose.material3.CircularProgressIndicator
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.ScreenScaffold
import androidx.wear.compose.material3.Text
import androidx.wear.input.RemoteInputIntentHelper
import androidx.wear.input.wearableExtender
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.wear.theme.WearType

/**
 * Pair this watch with an agent.
 *
 * The screen ADR-0014 was written to make possible: **one field, and it is the code.** The
 * address arrives from a paired phone, because fifteen characters of IP address is the step
 * where somebody stops setting the app up.
 *
 * # Why RemoteInput and not a text field
 *
 * A `BasicTextField` on a 233dp round screen is a 20dp target and an on-watch keyboard.
 * `RemoteInput` hands off to the system input, which offers a keyboard, **voice** and
 * handwriting, and returns one string. For an eight-character code that is the whole
 * interaction.
 *
 * `setEmojisAllowed(false)` asks the Wear input UI not to offer its emoji option. It does
 * not remove the system keyboard's own emoji key — observed on the Watch 2R — so it is a
 * hint rather than a guarantee. Worth setting, not worth relying on: the agent rejects a
 * malformed code anyway, and merges that with expired and already-redeemed on purpose.
 *
 * # A note for whoever tries to automate this
 *
 * Driving this flow with `adb shell input tap` does not work: the chooser opens, the
 * keyboard accepts text, and the send never returns a result. The same flow completes
 * immediately under a real finger. Synthetic taps are not equivalent to touch for this
 * system UI, so M5.17's checklist item here has to be performed by a person.
 */
@Composable
fun PairingScreen(
    deviceName: String,
    onPaired: () -> Unit,
    model: PairingViewModel = viewModel(),
) {
    val ui by model.ui.collectAsStateWithLifecycle()
    val listState = rememberTransformingLazyColumnState()

    val codeInput = rememberRemoteInput(KEY_CODE) { code ->
        val state = ui
        val address = when (state) {
            is PairingUi.Ready -> state.address
            is PairingUi.Failed -> state.address
            else -> null
        }
        if (address != null && code.isNotBlank()) {
            model.pair(address, code, deviceName)
        }
    }

    val addressInput = rememberRemoteInput(KEY_ADDRESS) { model.addressEntered(it) }

    ScreenScaffold(scrollState = listState) { contentPadding ->
        TransformingLazyColumn(
            state = listState,
            contentPadding = contentPadding,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            when (val state = ui) {
                PairingUi.Preparing -> item {
                    Centred {
                        CircularProgressIndicator()
                        Text(
                            "asking the phone…",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            textAlign = TextAlign.Center,
                        )
                    }
                }

                is PairingUi.Ready -> {
                    item { Header(state.address, state.source) }
                    if (state.address != null) {
                        item {
                            Button(
                                onClick = { codeInput(PROMPT_CODE) },
                                modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                            ) { Text("Enter code") }
                        }
                    }
                    item {
                        Button(
                            onClick = { addressInput(PROMPT_ADDRESS) },
                            modifier = Modifier.fillMaxWidth().padding(top = 4.dp),
                        ) {
                            // The fallback ADR-0014 insisted on keeping. Present, and
                            // never the first thing offered.
                            Text(
                                if (state.address == null) "Enter address" else "Change address",
                                style = MaterialTheme.typography.labelMedium,
                            )
                        }
                    }
                }

                is PairingUi.Working -> item {
                    Centred {
                        CircularProgressIndicator()
                        Text(
                            "pairing…",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }

                is PairingUi.Paired -> item {
                    Centred {
                        Text("Paired", style = MaterialTheme.typography.titleMedium)
                        Text(
                            state.hostName,
                            style = WearType.DataSmall,
                            color = MaterialTheme.colorScheme.primary,
                            textAlign = TextAlign.Center,
                        )
                        Button(
                            onClick = onPaired,
                            modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                        ) { Text("Done") }
                    }
                }

                is PairingUi.Failed -> {
                    item {
                        Centred {
                            Text(
                                state.message,
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.error,
                                textAlign = TextAlign.Center,
                            )
                        }
                    }
                    if (state.address != null) {
                        item {
                            Button(
                                onClick = { codeInput(PROMPT_CODE) },
                                modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                            ) { Text("Try again") }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun Header(address: AgentAddress?, source: AddressSource) {
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Text("Pair", style = MaterialTheme.typography.titleMedium)
        Text(
            text = address?.toString() ?: "no address yet",
            // An identifier, so mono at body size rather than the numeral scale — see
            // WearType.DataSmall for what the numeral roles did to this exact string.
            style = WearType.DataSmall,
            color = if (address == null) {
                MaterialTheme.colorScheme.onSurfaceVariant
            } else {
                MaterialTheme.colorScheme.primary
            },
            textAlign = TextAlign.Center,
        )
        Text(
            text = when {
                address == null -> "type it, or open CueSeek on your phone"
                source == AddressSource.Phone -> "from your phone"
                else -> "typed here"
            },
            style = MaterialTheme.typography.bodyExtraSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
    }
}

@Composable
private fun Centred(content: @Composable () -> Unit) {
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(4.dp),
        modifier = Modifier.fillMaxWidth(),
    ) { content() }
}

/**
 * Launches Wear's system text input and delivers the result.
 *
 * Returns a function taking the prompt, so a screen can reuse one launcher for a field and
 * pass different wording per use.
 */
@Composable
private fun rememberRemoteInput(key: String, onResult: (String) -> Unit): (String) -> Unit {
    val launcher = rememberLauncherForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        if (result.resultCode != Activity.RESULT_OK) return@rememberLauncherForActivityResult
        result.data
            ?.let { RemoteInput.getResultsFromIntent(it) }
            ?.getCharSequence(key)
            ?.toString()
            ?.let(onResult)
    }

    return { prompt ->
        val remoteInput = RemoteInput.Builder(key)
            .setLabel(prompt)
            // A code is [A-Z0-9-] and an address is [a-z0-9.:-]. Neither is ever an emoji,
            // and offering that keyboard only produces failures the agent has to merge
            // into "not accepted".
            .wearableExtender { setEmojisAllowed(false) }
            .build()

        val intent: Intent = RemoteInputIntentHelper.createActionRemoteInputIntent()
        RemoteInputIntentHelper.putRemoteInputsExtra(intent, listOf(remoteInput))
        launcher.launch(intent)
    }
}

private const val KEY_CODE = "cueseek_pairing_code"
private const val KEY_ADDRESS = "cueseek_agent_address"
private const val PROMPT_CODE = "Pairing code"
private const val PROMPT_ADDRESS = "Agent address"
