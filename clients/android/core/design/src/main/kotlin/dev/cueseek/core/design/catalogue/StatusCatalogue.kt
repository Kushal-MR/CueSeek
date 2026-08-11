package dev.cueseek.core.design.catalogue

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import dev.cueseek.core.design.CueSeekTheme
import dev.cueseek.core.design.status.BeatDot
import dev.cueseek.core.design.status.StatusMark
import dev.cueseek.core.design.status.Tally
import dev.cueseek.core.design.status.TallyRule
import dev.cueseek.core.design.status.statusStyle
import dev.cueseek.core.design.token.CueSeekType
import dev.cueseek.core.model.HealthStatus

/**
 * Every state the status language can be in, on one screen.
 *
 * This is the artifact ADR-0010 asks for: a catalogue that can be reviewed and
 * screenshot-tested without a running agent. It is also the honest test of the claim that
 * colour is not load-bearing — rendered in greyscale, all five rows must still be
 * distinguishable.
 */
@Composable
fun StatusCatalogue(modifier: Modifier = Modifier) {
    Surface(modifier = modifier, color = MaterialTheme.colorScheme.surface) {
        Column(
            modifier = Modifier.padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(18.dp),
        ) {
            Section("Status marks") {
                Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    HealthStatus.entries.forEach { status ->
                        MarkRow(status = status, stale = false)
                    }
                    // Stale is a modifier on top of a status, not a fifth status, so it is
                    // shown against a healthy service - the case where getting it wrong
                    // would be most dangerous.
                    MarkRow(status = HealthStatus.Healthy, stale = true)
                }
            }

            Section("Tally rule") {
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    TallyRule(Tally(healthy = 4, degraded = 1, unreachable = 1))
                    TallyRule(Tally(healthy = 6))
                    TallyRule(Tally(healthy = 1))
                    TallyRule(Tally(healthy = 4, degraded = 1, unreachable = 1), stale = true)
                }
            }

            Section("Beat") {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    BeatDot(live = true)
                    Text("Checked 4s ago", style = MaterialTheme.typography.bodySmall)
                    BeatDot(live = false)
                    Text("no data 34s", style = CueSeekType.Data.Emphasis)
                }
            }
        }
    }
}

@Composable
private fun MarkRow(status: HealthStatus, stale: Boolean) {
    val style = statusStyle(status, stale)
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(16.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        StatusMark(status = status, stale = stale)
        Text(style.label, style = MaterialTheme.typography.titleMedium)
        Text(
            when {
                stale -> "confidence, not failure"
                style.verified -> "verified"
                else -> "no answer yet"
            },
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun Section(title: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text(
            title,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        content()
    }
}

/**
 * The type scale, so a weight or tracking change is visible in a diff rather than only in
 * the app.
 */
@Composable
fun TypeCatalogue(modifier: Modifier = Modifier) {
    Surface(modifier = modifier, color = MaterialTheme.colorScheme.surface) {
        Column(
            modifier = Modifier.padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text("kushal-HP-paviliong6", style = MaterialTheme.typography.labelMedium)
            Text("2 need attention", style = MaterialTheme.typography.headlineSmall)
            Text("Jellyfin", style = MaterialTheme.typography.titleMedium)
            Text("API key rejected", style = MaterialTheme.typography.bodySmall)
            Text("06:32:03 · 4s · 11s", style = CueSeekType.Data.Small)
            Text("4  1  1", style = CueSeekType.Data.Emphasis)
            Text("Restart", style = MaterialTheme.typography.labelLarge)
        }
    }
}

@Preview(name = "Status · light", showBackground = true)
@Composable
private fun StatusCataloguePreviewLight() {
    CueSeekTheme(darkTheme = false, dynamicColor = false) { StatusCatalogue() }
}

@Preview(name = "Status · dark", showBackground = true)
@Composable
private fun StatusCataloguePreviewDark() {
    CueSeekTheme(darkTheme = true, dynamicColor = false) { StatusCatalogue() }
}

@Preview(name = "Type · light", showBackground = true)
@Composable
private fun TypeCataloguePreviewLight() {
    CueSeekTheme(darkTheme = false, dynamicColor = false) { TypeCatalogue() }
}
