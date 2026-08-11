package dev.cueseek.android.ui.dashboard

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.cueseek.core.data.ThemeChoice
import dev.cueseek.core.design.icon.CueSeekIcons
import dev.cueseek.core.design.token.CueSeekShapes

/**
 * The overflow menu.
 *
 * It used to be a single button wired straight to "forget this host", so opening the menu
 * unpaired the device. Ordinary navigation must never be destructive: the destructive item
 * now lives *inside* the menu, is worded as what it does rather than where it goes, and
 * asks before it acts.
 */
@Composable
fun HostMenu(
    theme: ThemeChoice,
    onThemeChange: (ThemeChoice) -> Unit,
    onForgetRequested: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var open by remember { mutableStateOf(false) }

    Box(modifier) {
        IconButton(onClick = { open = true }) {
            Icon(
                CueSeekIcons.More,
                contentDescription = "Menu",
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
            Text(
                "Appearance",
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 10.dp, bottom = 4.dp),
            )

            ThemeChoice.entries.forEach { choice ->
                DropdownMenuItem(
                    text = { Text(choice.label()) },
                    onClick = {
                        onThemeChange(choice)
                        open = false
                    },
                    // A check on the current one rather than a radio: the menu closes on
                    // selection, so this is a statement of what is set, not a control.
                    trailingIcon = {
                        if (choice == theme) {
                            Icon(
                                CueSeekIcons.Check,
                                contentDescription = "Selected",
                                tint = MaterialTheme.colorScheme.primary,
                                modifier = Modifier.size(18.dp),
                            )
                        }
                    },
                )
            }

            HorizontalDivider(
                color = MaterialTheme.colorScheme.outlineVariant,
                modifier = Modifier.padding(vertical = 4.dp),
            )

            DropdownMenuItem(
                // Named for what it does to this phone. "Log out" would imply the agent
                // forgets us too, and it does not — revoking needs a scope this device
                // almost certainly was not granted.
                text = {
                    Text(
                        "Forget this host",
                        color = MaterialTheme.colorScheme.error,
                    )
                },
                onClick = {
                    open = false
                    onForgetRequested()
                },
                trailingIcon = {
                    Icon(
                        CueSeekIcons.Block,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.error,
                        modifier = Modifier.size(18.dp),
                    )
                },
            )
        }
    }
}

private fun ThemeChoice.label(): String = when (this) {
    ThemeChoice.System -> "System"
    ThemeChoice.Light -> "Light"
    ThemeChoice.Dark -> "Dark"
}
