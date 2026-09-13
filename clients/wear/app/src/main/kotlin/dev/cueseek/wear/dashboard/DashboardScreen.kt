package dev.cueseek.wear.dashboard

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.produceState
import kotlinx.coroutines.delay
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.wear.compose.foundation.lazy.TransformingLazyColumn
import androidx.wear.compose.foundation.lazy.rememberTransformingLazyColumnState
import androidx.wear.compose.material3.CircularProgressIndicator
import androidx.wear.compose.material3.LinearProgressIndicator
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.ScreenScaffold
import androidx.wear.compose.material3.Text
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.model.CRITICAL
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.PRESSURE
import dev.cueseek.core.model.fullest
import dev.cueseek.wear.theme.WearType

/**
 * The watch's dashboard: the verdict, then the machine's vitals.
 *
 * # Why the verdict is the screen
 *
 * The phone answers "how is everything?" with a headline, a tally rule, a four-up vitals
 * grid and a service roster, and a thumb can reach all of it. A 233dp round screen read at
 * arm's length for two seconds cannot, and shrinking that layout is the most common way a
 * Wear app ends up looking like a port.
 *
 * So the order is inverted rather than compressed. **The verdict is the first and largest
 * thing**, because it is the whole question; the vitals sit under it for the case where the
 * answer is not "Operational" and you want to know why. The roster is M5.4b, below both.
 *
 * The verdict itself is computed by `:core:model` and shared with the phone — a watch that
 * disagreed with the phone in your pocket about the same machine would be a console
 * contradicting itself, which is worse than either being wrong alone.
 */
@Composable
fun DashboardScreen(model: DashboardViewModel = viewModel()) {
    val ui by model.ui.collectAsStateWithLifecycle()
    val listState = rememberTransformingLazyColumnState()

    // The whole poll schedule. A watch screen is on for seconds at a time, and a timer
    // behind a dark panel would spend battery producing readings nobody sees (ADR-0004).
    LaunchedEffect(Unit) { model.refresh() }

    // Staleness is a function of the clock, not of the fetch that produced the reading, so
    // it is re-evaluated while the screen is up rather than fixed at load. A reading taken
    // 30 seconds before you raised your wrist is fine; the same reading two minutes later
    // is not, and nothing new arrives to say so.
    //
    // The ticker lives here rather than in the ViewModel so it stops when the screen does —
    // this is the surface it exists to correct, and nothing off-screen needs it.
    val observedAt = (ui as? DashboardUi.Loaded)?.observedAt
    val stale by produceState(initialValue = false, key1 = observedAt) {
        if (observedAt == null) {
            value = false
            return@produceState
        }
        while (true) {
            value = DashboardViewModel.isStale(observedAt)
            delay(5_000)
        }
    }

    ScreenScaffold(scrollState = listState) { contentPadding ->
        TransformingLazyColumn(
            state = listState,
            contentPadding = contentPadding,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            when (val state = ui) {
                DashboardUi.Loading -> item {
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(6.dp),
                    ) {
                        CircularProgressIndicator()
                        Text(
                            "reading the agent…",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }

                DashboardUi.Unpaired -> item {
                    Text(
                        "Not paired",
                        style = MaterialTheme.typography.titleMedium,
                        textAlign = TextAlign.Center,
                    )
                }

                is DashboardUi.Failed -> item {
                    Text(
                        state.message,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                        textAlign = TextAlign.Center,
                    )
                }

                is DashboardUi.Loaded -> {
                    item { Verdict(state.copy(stale = stale)) }
                    item { Vitals(state.metrics) }
                }
            }
        }
    }
}

@Composable
private fun Verdict(state: DashboardUi.Loaded) {
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(2.dp),
        modifier = Modifier.padding(bottom = 8.dp),
    ) {
        Text(
            text = state.hostname,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
        Text(
            // The verdict the agent's data produced, unless the reading has aged out from
            // under it. "Unverified" is the phone's word for the same condition, and it
            // comes from the same shared function.
            text = if (state.stale) "Unverified" else state.verdict,
            style = MaterialTheme.typography.titleLarge,
            // The status palette, shared verbatim with the phone (ADR-0010). Colour is the
            // weakest of the three encodings DESIGN.md uses, which is why the word itself
            // carries the meaning and this only reinforces it.
            color = when {
                state.stale -> CueSeekStatus.colors.unknown
                state.tally.needingAttention > 0 -> CueSeekStatus.colors.degraded
                else -> MaterialTheme.colorScheme.onSurface
            },
            textAlign = TextAlign.Center,
        )
        if (state.tally.total > 0) {
            Text(
                text = "${state.tally.healthy}/${state.tally.total} healthy",
                style = WearType.DataSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
        }
    }
}

/**
 * The machine's own vitals.
 *
 * Absent values are omitted rather than zeroed, which is the rule the whole project turns
 * on and which M4.10 watched work on a phone: a VM exposes no thermal sensors, and rendering
 * `0°C` would claim a cold machine that never answered.
 */
@Composable
private fun Vitals(metrics: HostMetrics?) {
    if (metrics == null) return

    Column(
        modifier = Modifier.fillMaxWidth(),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        // usagePercent, not a fraction: CPU is the one metric the agent reports as 0..100.
        // Null on the agent's first collection after a restart, and absent rather than zero
        // for exactly the reason the whole vitals strip exists.
        metrics.cpu?.usagePercent?.let { Vital("CPU", it / 100f, judge = false) }
        metrics.memory?.usedFraction?.let { Vital("MEM", it, judge = true) }
        fullest(metrics.storage)?.let { disk ->
            disk.usedFraction?.let { Vital(disk.mount, it, judge = true) }
        }
        metrics.thermal?.firstOrNull()?.let { sensor ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
            ) {
                Text(
                    sensor.label,
                    style = MaterialTheme.typography.bodyExtraSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    "${sensor.celsius.toInt()}°C",
                    style = WearType.DataSmall,
                    color = if (sensor.isHot) {
                        CueSeekStatus.colors.unreachable
                    } else {
                        MaterialTheme.colorScheme.onSurface
                    },
                )
            }
        }
    }
}

/**
 * @param judge whether pressure is worth colouring. False for CPU, deliberately: a
 *   processor at 100% is a transcode doing its job, and colouring it would cry wolf every
 *   time somebody watched a film. The same reasoning `hostConcern` uses, and the same
 *   thresholds, because they now come from one place.
 */
@Composable
private fun Vital(label: String, fraction: Float, judge: Boolean) {
    Column(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
        ) {
            Text(
                label,
                style = MaterialTheme.typography.bodyExtraSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                "${(fraction * 100).toInt()}%",
                style = WearType.DataSmall,
                color = MaterialTheme.colorScheme.onSurface,
            )
        }
        LinearProgressIndicator(
            progress = { fraction.coerceIn(0f, 1f) },
            modifier = Modifier.fillMaxWidth(),
            colors = androidx.wear.compose.material3.ProgressIndicatorDefaults.colors(
                indicatorColor = when {
                    !judge -> MaterialTheme.colorScheme.primary
                    fraction >= CRITICAL -> CueSeekStatus.colors.unreachable
                    fraction >= PRESSURE -> CueSeekStatus.colors.degraded
                    else -> MaterialTheme.colorScheme.primary
                },
            ),
        )
    }
}
