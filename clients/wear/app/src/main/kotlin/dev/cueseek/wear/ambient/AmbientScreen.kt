package dev.cueseek.wear.ambient

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.Text
import dev.cueseek.wear.dashboard.DashboardUi
import dev.cueseek.wear.dashboard.readingAge
import dev.cueseek.wear.theme.WearType

/**
 * What the watch shows once the screen has dimmed.
 *
 * # It says when, because it cannot say now
 *
 * Ambient **does not poll** — that is most of the point of it — so everything here is by
 * definition the last thing known. A dimmed screen reading `Operational` with no age would
 * be confident green with nothing behind it, which is the failure this project keeps
 * legislating against, except worse: ambient can sit on a wrist for an hour.
 *
 * So the age is not a detail in the corner. It is the second line, and it is the line that
 * makes the first one honest.
 *
 * # Why it is this sparse
 *
 * Not minimalism. Ambient keeps the panel lit for as long as the wrist stays up, and every
 * lit pixel is both battery and, on OLED, a burn-in risk that accumulates over a wear-day.
 * So: no progress bars, no status dots, no filled containers, no roster. Three short lines
 * of unfilled text on black.
 *
 * The status **colour is dropped too**, and that is a deliberate loss. Colour is the weakest
 * of the three encodings `DESIGN.md` §3 requires, and the word survives without it — whereas
 * a coloured block is exactly the shape of thing that burns into a panel. The interactive
 * screen keeps all three; ambient keeps the one that carries the meaning.
 *
 * @param stale whether the reading had already aged out before the screen dimmed. Ambient
 *   shows a stale reading rather than hiding it, for the same reason it shows the age: the
 *   last thing known is still the most useful thing there is, as long as it is labelled.
 */
@Composable
fun AmbientScreen(ui: DashboardUi, stale: Boolean) {
    Box(
        modifier = Modifier.fillMaxSize(),
        contentAlignment = Alignment.Center,
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(4.dp),
            modifier = Modifier.fillMaxWidth(0.8f),
        ) {
            when (ui) {
                is DashboardUi.Loaded -> {
                    Text(
                        text = ui.hostname,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        textAlign = TextAlign.Center,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Text(
                        // The same word the interactive screen would show, including
                        // "Unverified" when the reading had already aged out. Ambient does
                        // not get a softer vocabulary than the screen it replaces.
                        text = if (stale) "Unverified" else ui.verdict,
                        style = MaterialTheme.typography.titleMedium,
                        color = MaterialTheme.colorScheme.onSurface,
                        textAlign = TextAlign.Center,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Text(
                        text = readingAge(ui.observedAt),
                        style = WearType.DataSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                // Nothing was ever read, so there is no verdict to dim. Saying which of the
                // three it is costs one line and prevents a dark screen from reading as a
                // crash.
                DashboardUi.Loading -> AmbientNote("Reading…")
                DashboardUi.Unpaired -> AmbientNote("Not paired")
                is DashboardUi.Failed -> AmbientNote("No reading")
            }
        }
    }
}

@Composable
private fun AmbientNote(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.titleMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        textAlign = TextAlign.Center,
        modifier = Modifier.fillMaxWidth(),
    )
}
