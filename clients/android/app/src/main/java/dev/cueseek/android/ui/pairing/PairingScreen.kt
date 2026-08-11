package dev.cueseek.android.ui.pairing

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.isSystemInDarkTheme
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
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import dev.cueseek.android.ui.CueSeekViewModel
import dev.cueseek.android.ui.explain
import dev.cueseek.core.design.icon.CueSeekIcons
import dev.cueseek.core.design.token.CueSeekShapes
import dev.cueseek.core.design.token.CueSeekSpacing
import dev.cueseek.core.design.token.CueSeekType

/**
 * First run: point the app at an agent and redeem a code.
 *
 * Built on the same structure as the dashboard rather than as a plain form, because it is
 * the first thing anyone sees and it was the one screen that did not look like the app.
 * The signature is the same: a flat header of type on the page, then **one** container at
 * 28dp holding rows divided at the text column. Light casts a soft shadow, dark is raised
 * by tone — the same rule the roster follows.
 *
 * There is no discovery and no QR. The agent emits neither, so the honest first screen asks
 * for what the operator already has in front of them.
 */
@Composable
fun PairingScreen(viewModel: CueSeekViewModel, modifier: Modifier = Modifier) {
    val form = viewModel.form
    val dark = isSystemInDarkTheme()

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState()),
    ) {
        // Header: no container, type on the page. Matches the dashboard exactly.
        Column(Modifier.padding(start = 16.dp, end = 16.dp, top = 40.dp)) {
            Text(
                "Set up",
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(3.dp))
            Text("Pair with an agent", style = MaterialTheme.typography.headlineSmall)
            Spacer(Modifier.height(10.dp))
            Text(
                "Run cueseekd pair on the host. It prints a code that works once and " +
                    "expires in a few minutes.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        Spacer(Modifier.height(24.dp))

        Surface(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = CueSeekSpacing.rosterInset)
                .then(
                    if (dark) Modifier
                    else Modifier.shadow(2.dp, CueSeekShapes.Roster, clip = false)
                ),
            shape = CueSeekShapes.Roster,
            color = if (dark) {
                MaterialTheme.colorScheme.surfaceContainer
            } else {
                MaterialTheme.colorScheme.surfaceContainerLowest
            },
        ) {
            Column {
                Field(
                    label = "Address",
                    value = form.host,
                    onValueChange = { v -> viewModel.edit { copy(host = v) } },
                    placeholder = "100.92.18.125",
                    enabled = !form.busy,
                    mono = true,
                    keyboard = KeyboardOptions(
                        keyboardType = KeyboardType.Uri,
                        imeAction = ImeAction.Next,
                        autoCorrectEnabled = false,
                    ),
                )
                RowDivider(dark)
                Field(
                    label = "Port",
                    value = form.port,
                    onValueChange = { v -> viewModel.edit { copy(port = v.filter(Char::isDigit)) } },
                    placeholder = "7777",
                    enabled = !form.busy,
                    mono = true,
                    keyboard = KeyboardOptions(
                        keyboardType = KeyboardType.Number,
                        imeAction = ImeAction.Next,
                    ),
                )
                RowDivider(dark)
                Field(
                    label = "Pairing code",
                    value = form.code,
                    onValueChange = { v -> viewModel.edit { copy(code = v) } },
                    // The agent strips anything outside its alphabet and uppercases before
                    // matching, so there is nothing to validate and nothing to correct.
                    placeholder = "D8JT-HUPV",
                    enabled = !form.busy,
                    mono = true,
                    keyboard = KeyboardOptions(
                        capitalization = KeyboardCapitalization.Characters,
                        autoCorrectEnabled = false,
                        imeAction = ImeAction.Next,
                    ),
                )
                RowDivider(dark)
                Field(
                    label = "This device",
                    value = form.deviceName,
                    onValueChange = { v -> viewModel.edit { copy(deviceName = v) } },
                    placeholder = "Phone",
                    enabled = !form.busy,
                    mono = false,
                    keyboard = KeyboardOptions(imeAction = ImeAction.Done),
                )
            }
        }

        Spacer(Modifier.height(10.dp))

        Text(
            "Spacing and case in the code do not matter.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp),
        )

        Spacer(Modifier.height(24.dp))

        Button(
            onClick = viewModel::pair,
            enabled = !form.busy && form.host.isNotBlank() && form.code.isNotBlank(),
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp)
                .height(52.dp),
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

        AnimatedVisibility(
            visible = form.error != null,
            enter = fadeIn() + expandVertically(),
            exit = fadeOut() + shrinkVertically(),
        ) {
            val copy = form.error?.explain() ?: return@AnimatedVisibility
            Surface(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(start = 16.dp, end = 16.dp, top = 16.dp),
                shape = CueSeekShapes.Shapes.large,
                color = Color.Transparent,
                border = BorderStroke(1.dp, MaterialTheme.colorScheme.error),
            ) {
                Row(
                    Modifier.padding(14.dp),
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    Icon(
                        CueSeekIcons.Warning,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.error,
                        modifier = Modifier.size(18.dp),
                    )
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(copy.title, style = MaterialTheme.typography.titleSmall)
                        Text(
                            copy.body,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        copy.detail?.let {
                            Text(
                                it,
                                style = CueSeekType.Data.Small,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
            }
        }

        Spacer(Modifier.height(40.dp))
    }
}

/**
 * One row of the form.
 *
 * A label above an unadorned field rather than an `OutlinedTextField`, because an outlined
 * box inside a rounded container gives two competing borders — the same reason roster rows
 * are square inside their container. Addresses, ports and codes are set in Plex Mono: they
 * are data, and the code in particular is read character by character off another screen.
 */
@Composable
private fun Field(
    label: String,
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    enabled: Boolean,
    mono: Boolean,
    keyboard: KeyboardOptions,
) {
    Column(Modifier.padding(start = 16.dp, end = 8.dp, top = 10.dp, bottom = 4.dp)) {
        Text(
            label,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        TextField(
            value = value,
            onValueChange = onValueChange,
            enabled = enabled,
            singleLine = true,
            placeholder = {
                Text(
                    placeholder,
                    style = if (mono) CueSeekType.Data.Medium else LocalTextStyle.current,
                    color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f),
                )
            },
            textStyle = if (mono) {
                CueSeekType.Data.Medium.copy(color = MaterialTheme.colorScheme.onSurface)
            } else {
                TextStyle(color = MaterialTheme.colorScheme.onSurface)
            },
            keyboardOptions = keyboard,
            colors = TextFieldDefaults.colors(
                // Transparent everything: the container is the shape, the field is content.
                focusedContainerColor = Color.Transparent,
                unfocusedContainerColor = Color.Transparent,
                disabledContainerColor = Color.Transparent,
                focusedIndicatorColor = Color.Transparent,
                unfocusedIndicatorColor = Color.Transparent,
                disabledIndicatorColor = Color.Transparent,
            ),
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

@Composable
private fun RowDivider(dark: Boolean) {
    HorizontalDivider(
        color = MaterialTheme.colorScheme.outlineVariant.copy(alpha = if (dark) 0.35f else 1f),
        modifier = Modifier.padding(start = 16.dp),
    )
}
