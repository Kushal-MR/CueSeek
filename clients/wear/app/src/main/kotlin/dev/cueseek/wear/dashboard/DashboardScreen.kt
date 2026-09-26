package dev.cueseek.wear.dashboard

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.produceState
import androidx.compose.ui.text.style.TextOverflow
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
import androidx.wear.compose.material3.Button
import androidx.wear.compose.material3.ButtonDefaults
import androidx.wear.compose.material3.CircularProgressIndicator
import androidx.wear.compose.material3.EdgeButton
import androidx.wear.compose.material3.LinearProgressIndicator
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.ScreenScaffold
import androidx.wear.compose.material3.Text
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.design.status.statusStyle
import dev.cueseek.core.model.CRITICAL
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.PRESSURE
import dev.cueseek.core.model.fullest
import dev.cueseek.wear.power.PowerAccess
import dev.cueseek.wear.power.powerAccess
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
fun DashboardScreen(
    model: DashboardViewModel = viewModel(),
    stale: Boolean = false,
    onServiceClick: (String) -> Unit = {},
    onPowerClick: () -> Unit = {},
) {
    val ui by model.ui.collectAsStateWithLifecycle()
    val listState = rememberTransformingLazyColumnState()

    // The poll is *not* triggered here. It was until M5.10, and then ambient gave the app
    // a second way to become visible — leaving this in would have meant two fetches on
    // every wrist-raise that landed on the dashboard, on the one phase whose whole subject
    // is not spending battery. It lives in `PairedApp`, which is the boundary every
    // destination enters through.

    // The way to the machine itself, and the only one. An [EdgeButton] rather than a row in
    // the roster: it is pinned to the bottom bezel instead of riding the scroll, so reaching
    // it is a decision rather than the end of a flick — and a service list that contained
    // "Shut down" would be presenting the machine as one of its own services.
    //
    // Absent entirely without the `host.power` grant, which is where this differs from the
    // phone's greyed menu item. See `powerAccess` for why hiding is right on a wrist and
    // wrong in a pocket.
    val loaded = ui as? DashboardUi.Loaded
    val access = loaded?.let { powerAccess(it.scopes, it.hostActions) } ?: PowerAccess.Ungranted

    // Two scaffolds rather than one with an empty button, because the overload that takes an
    // edge button reserves the space for it. An ungranted watch would get a permanent gap at
    // the bottom of the roster — a dead control drawn as nothing at all, which is the one
    // outcome worse than drawing it greyed.
    val body: @Composable BoxScope.(PaddingValues) -> Unit = { contentPadding ->
        TransformingLazyColumn(
            state = listState,
            contentPadding = contentPadding,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            when (val state = ui) {
                // Never a blank screen, and never a bare spinner either: a spinner alone
                // does not say whether the app is thinking or the agent is slow.
                DashboardUi.Loading -> item {
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(6.dp),
                        // Same bezel inset as the error state, for the same reason.
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 14.dp),
                    ) {
                        CircularProgressIndicator()
                        Text(
                            "reading the agent…",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            textAlign = TextAlign.Center,
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

                // The agent's own words, already shortened for a wrist by `shortMessage` —
                // never a status code and never a stack trace. Below it, the one thing that
                // is actually actionable from here: ask again.
                is DashboardUi.Failed -> item {
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(8.dp),
                        // Pushed down and narrowed, because a round screen is not a
                        // rectangle with corners missing: at the top of a 466px circle the
                        // chord is far shorter than the screen is wide, so a full-width line
                        // drawn there runs off the glass at both ends. "Could not reach the
                        // agent" rendered as "ould not reach the agen", and padding alone
                        // did not fix it — the content had to move to where the circle is
                        // wide. Measured on the device, not derived.
                        modifier = Modifier
                            .fillMaxWidth(0.78f)
                            .padding(top = 40.dp),
                    ) {
                        Text(
                            state.message,
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                            textAlign = TextAlign.Center,
                        )
                        Button(
                            onClick = { model.refresh() },
                            colors = ButtonDefaults.filledTonalButtonColors(),
                        ) {
                            Text("Try again", maxLines = 1)
                        }
                    }
                }

                is DashboardUi.Loaded -> {
                    item { Verdict(state.copy(stale = stale)) }
                    item { Vitals(state.metrics) }

                    // Configured nothing, which is a working install rather than a fault:
                    // `services: []` is what ships, and the machine's own vitals above need
                    // no configuration and no privilege. So this says what is true and
                    // points at the fix, in the register of an instruction rather than an
                    // error — it must not look like the agent is broken, because it is not.
                    if (state.services.isEmpty()) {
                        item {
                            Text(
                                text = "No services configured.\nAdd them on the host.",
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                textAlign = TextAlign.Center,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(top = 10.dp),
                            )
                        }
                    }

                    // The roster, keyed by service id so recomposition is stable when the
                    // agent reorders. Rendered from capabilities and health only -- what
                    // service this *is* never appears in a branch (ADR-0005, ADR-0007),
                    // and WearCapabilityTest enforces that rather than trusting review.
                    items(
                        count = state.services.size,
                        key = { state.services[it].id },
                    ) { index ->
                        ServiceRow(
                            service = state.services[index],
                            stale = stale,
                            onClick = onServiceClick,
                        )
                    }
                }
            }
        }
    }

    if (access is PowerAccess.Ungranted) {
        ScreenScaffold(scrollState = listState, content = body)
    } else {
        ScreenScaffold(
            scrollState = listState,
            edgeButton = {
                EdgeButton(
                    onClick = onPowerClick,
                    colors = ButtonDefaults.filledTonalButtonColors(),
                ) {
                    Text("Machine", maxLines = 1)
                }
            },
            content = body,
        )
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

/**
 * One service: a status mark, its name, and what it is doing.
 *
 * Three encodings of the same fact, exactly as `DESIGN.md` §3 requires and for the reason it
 * gives — healthy and unknown differ by 1.21:1 in luminance, so anyone who cannot separate
 * the hues is reading the shape and the word:
 *
 *  1. the mark's **colour**, from the palette shared verbatim with the phone
 *  2. its **shape** — filled when the status is a fact, hollow when it is not
 *  3. the **label**, which is also what a screen reader announces
 *
 * Nothing here asks which service it is.
 */
@Composable
private fun ServiceRow(
    service: Service,
    stale: Boolean,
    onClick: (String) -> Unit,
) {
    val style = statusStyle(service.health.status, stale)

    Row(
        modifier = Modifier
            .fillMaxWidth()
            // The id is passed along, never inspected. The row does not know or care which
            // service this is.
            .clickable { onClick(service.id) }
            .padding(vertical = 6.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier = Modifier
                .size(10.dp)
                .then(
                    if (style.verified) {
                        Modifier.background(style.content, CircleShape)
                    } else {
                        // Hollow, because "we do not have an answer" must not look like an
                        // answer. The phone draws a dashed ring; at 10dp a ring is all the
                        // distinction that survives.
                        Modifier.border(1.dp, style.content, CircleShape)
                    },
                ),
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = service.name,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            // Activity when there is any, the status word when there is not. An idle
            // service saying "0 playing" would spend the line on a non-event.
            Text(
                text = wearActivityLine(service) ?: style.label,
                style = MaterialTheme.typography.bodyExtraSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}
