package dev.cueseek.android.ui.pairing

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import dev.cueseek.android.ui.CueSeekViewModel
import dev.cueseek.android.ui.explain

/**
 * First run: point the app at an agent and redeem a code.
 *
 * There is no discovery and no QR. The agent emits neither, so the honest first screen asks
 * for what the operator already has in front of them — the address and the code that
 * `cueseekd pair` just printed.
 */
@Composable
fun PairingScreen(viewModel: CueSeekViewModel, modifier: Modifier = Modifier) {
    val form = viewModel.form

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Spacer(Modifier.height(40.dp))

        Text("Pair with an agent", style = MaterialTheme.typography.headlineSmall)
        Text(
            "Run cueseekd pair on the host. It prints a code that works once and expires " +
                "in a few minutes.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        Spacer(Modifier.height(4.dp))

        OutlinedTextField(
            value = form.host,
            onValueChange = { v -> viewModel.edit { copy(host = v) } },
            label = { Text("Address") },
            supportingText = { Text("The host's address on your VPN or LAN") },
            singleLine = true,
            enabled = !form.busy,
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Uri,
                imeAction = ImeAction.Next,
                autoCorrectEnabled = false,
            ),
            modifier = Modifier.fillMaxWidth(),
        )

        OutlinedTextField(
            value = form.port,
            onValueChange = { v -> viewModel.edit { copy(port = v.filter(Char::isDigit)) } },
            label = { Text("Port") },
            singleLine = true,
            enabled = !form.busy,
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Number,
                imeAction = ImeAction.Next,
            ),
            modifier = Modifier.fillMaxWidth(),
        )

        OutlinedTextField(
            value = form.code,
            onValueChange = { v -> viewModel.edit { copy(code = v) } },
            label = { Text("Pairing code") },
            // The agent strips anything outside its alphabet and uppercases before
            // matching, so there is nothing to validate here and nothing to correct.
            supportingText = { Text("Spacing and case do not matter") },
            singleLine = true,
            enabled = !form.busy,
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.Characters,
                autoCorrectEnabled = false,
                imeAction = ImeAction.Next,
            ),
            modifier = Modifier.fillMaxWidth(),
        )

        OutlinedTextField(
            value = form.deviceName,
            onValueChange = { v -> viewModel.edit { copy(deviceName = v) } },
            label = { Text("Name this device") },
            supportingText = { Text("Shown in the agent's device list") },
            singleLine = true,
            enabled = !form.busy,
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
            modifier = Modifier.fillMaxWidth(),
        )

        Button(
            onClick = viewModel::pair,
            enabled = !form.busy && form.host.isNotBlank() && form.code.isNotBlank(),
            modifier = Modifier.fillMaxWidth().height(52.dp),
        ) {
            if (form.busy) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(18.dp),
                        strokeWidth = 2.dp,
                        color = MaterialTheme.colorScheme.onPrimary,
                    )
                    Text("Pairing")
                }
            } else {
                Text("Pair")
            }
        }

        AnimatedVisibility(visible = form.error != null, enter = fadeIn(), exit = fadeOut()) {
            val copy = form.error?.explain() ?: return@AnimatedVisibility
            OutlinedCard(Modifier.fillMaxWidth()) {
                Column(
                    Modifier.padding(14.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    Text(copy.title, style = MaterialTheme.typography.titleSmall)
                    Text(
                        copy.body,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    copy.detail?.let {
                        Text(
                            it,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }

        Spacer(Modifier.height(32.dp))
    }
}
