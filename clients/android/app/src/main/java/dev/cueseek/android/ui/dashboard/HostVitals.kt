package dev.cueseek.android.ui.dashboard

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.cueseek.android.ui.byteSize
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.design.token.CueSeekShapes
import dev.cueseek.core.design.token.CueSeekSpacing
import dev.cueseek.core.design.token.CueSeekType
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.StorageMetrics
import dev.cueseek.core.model.ThermalMetrics
import kotlin.math.roundToInt

/**
 * The machine's own vitals, under the summary.
 *
 * Three meters and a footnote, and deliberately not a card. The roster below is the one
 * substantial object on screen; giving the host a surface of its own would create a second
 * and flatten the hierarchy the header exists to establish. This is the same rule motif the
 * tally uses, at the same 8dp, because the reader has already been taught what a rule means
 * here — proportion, nothing else.
 *
 * **Nothing absent is drawn.** A machine with no temperature sensors shows no temperature
 * row, not an empty one; a first collection with no CPU figure shows two meters rather than
 * three with a dash. Placeholders would imply the agent measured something and found it
 * blank, which is the one claim this payload is shaped never to make.
 */
@Composable
fun HostVitals(metrics: HostMetrics?, modifier: Modifier = Modifier) {
    // Crossfades rather than snapping in and out, because metrics legitimately come and go:
    // they vanish when the data goes stale and return on the next collection, and a strip
    // that appeared abruptly would read as a layout fault rather than as an arrival.
    AnimatedVisibility(
        visible = metrics != null && !metrics.isEmpty,
        enter = fadeIn(tween(CueSeekMotion.DurationEnter, easing = CueSeekMotion.EmphasizedDecelerate)),
        exit = fadeOut(tween(CueSeekMotion.DurationExit, easing = CueSeekMotion.EmphasizedAccelerate)),
        modifier = modifier,
    ) {
        // Held so the exit animation has something to draw while it fades out.
        val shown = remembered(metrics)
        if (shown != null) Vitals(shown)
    }
}

/**
 * Keeps the last non-null metrics for the duration of the exit transition.
 *
 * Without this the content disappears instantly and the fade animates an empty box, which
 * looks like a glitch rather than a departure.
 */
@Composable
private fun remembered(metrics: HostMetrics?): HostMetrics? {
    val holder = remember { mutableStateOf<HostMetrics?>(null) }
    if (metrics != null) holder.value = metrics
    return holder.value
}

@Composable
private fun Vitals(metrics: HostMetrics) {
    Column(verticalArrangement = Arrangement.spacedBy(9.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(14.dp), modifier = Modifier.fillMaxWidth()) {
            metrics.cpu?.usagePercent?.let { usage ->
                Meter(
                    label = "CPU",
                    value = "${usage.roundToInt()}%",
                    fraction = usage / 100f,
                    // Never coloured. A CPU at 95% is a transcode doing its job, not a
                    // fault, and painting it red would train the reader to ignore the one
                    // colour this palette reserves for things that need a decision.
                    tint = null,
                    detail = metrics.cpu?.cores?.let { "$it cores" },
                    description = "Processor ${usage.roundToInt()} percent",
                )
            }

            metrics.memory?.usedFraction?.let { used ->
                Meter(
                    label = "MEM",
                    value = "${(used * 100).roundToInt()}%",
                    fraction = used,
                    tint = pressureTint(used),
                    detail = metrics.memory?.availableBytes?.let { "${byteSize(it)} free" },
                    description = "Memory ${(used * 100).roundToInt()} percent used",
                )
            }

            // One filesystem, because three would turn a glance into a table. The fullest
            // is the one worth showing: a console answers "is anything wrong" before it
            // answers "what is where", and the rest are in the agent's payload for a screen
            // that wants them later.
            fullest(metrics.storage)?.let { volume ->
                volume.usedFraction?.let { used ->
                    Meter(
                        label = volume.mount,
                        // A percentage, like its two neighbours, so every value on this row
                        // describes the bar beneath it. It used to read "175 GB free" over
                        // a two-thirds-full bar, which made the one column that mattered
                        // most the only one whose number and rule disagreed.
                        value = "${(used * 100).roundToInt()}%",
                        fraction = used,
                        tint = pressureTint(used),
                        detail = "${byteSize(volume.freeBytes)} free",
                        description = "${volume.mount}, ${byteSize(volume.freeBytes)} free of " +
                            "${byteSize(volume.totalBytes)}",
                    )
                }
            }
        }

        Footnote(metrics)
    }
}

/**
 * One labelled rule: name and percentage above it, the absolute figure below.
 *
 * Three lines rather than two, and that third line is what makes this a grid instead of a
 * row with a caption. The absolute numbers — free bytes, load average — used to live in a
 * single full-width line of dot-separated facts underneath, which belonged to none of the
 * three columns and read as something left over. Each column now carries its own.
 *
 * Label and value share a line above the rule rather than sitting beside it, so three of
 * these fit across a phone without any of them truncating a mount point to "/mn…".
 */
@Composable
private fun RowScope.Meter(
    label: String,
    value: String,
    fraction: Float,
    tint: Color?,
    detail: String?,
    description: String,
) {
    val filled by animateFloatAsState(
        targetValue = fraction.coerceIn(0f, 1f),
        animationSpec = CueSeekMotion.tallySpring(),
        label = "vital",
    )

    Column(
        modifier = Modifier
            .weight(1f)
            .clearAndSetSemantics { contentDescription = description },
        verticalArrangement = Arrangement.spacedBy(5.dp),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.Bottom,
        ) {
            Text(
                label,
                style = CueSeekType.Data.Small,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                // The gap is padding on the label rather than spacing on the row, because
                // the label is the element that grows: a long mount point takes the whole
                // width and would otherwise run its ellipsis straight into the number,
                // which on a real machine read as "/srv/cuese…88%".
                modifier = Modifier.weight(1f, fill = false).padding(end = 6.dp),
            )
            Text(
                value,
                style = CueSeekType.Data.Emphasis,
                color = MaterialTheme.colorScheme.onSurface,
                maxLines = 1,
            )
        }

        val track = CueSeekStatus.colors.unknownOutline.copy(alpha = 0.28f)
        val fill = tint ?: MaterialTheme.colorScheme.onSurfaceVariant

        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(CueSeekSpacing.tallyHeight)
                .background(track, CueSeekShapes.TallySegment),
        ) {
            // Drawn as a fraction of the track rather than as two weighted siblings, so an
            // empty meter is an empty track instead of a zero-width sliver that rounds up
            // to a visible dot.
            if (filled > 0.005f) {
                Box(
                    modifier = Modifier
                        .fillMaxWidth(filled)
                        .height(CueSeekSpacing.tallyHeight)
                        .background(fill, CueSeekShapes.TallySegment),
                )
            }
        }

        if (detail != null) {
            Text(
                detail,
                style = CueSeekType.Data.Small,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

/**
 * The two facts that belong to the machine rather than to any one meter: how long it has
 * been up, and how hot it is.
 *
 * **On the same three-column grid as the meters above**, with uptime in the first column and
 * temperature in the third, both left-aligned exactly like the detail lines they sit under.
 * An earlier version spanned the full width with the two anchored to opposite edges, which
 * put "up 43m" under the left edge of one column and the temperature against the screen edge
 * under the right edge of another — so nothing on the strip lined up with anything, and the
 * line read as text that had overflowed rather than as a row that was placed.
 *
 * Neither is a proportion, so neither gets a rule. Temperature has no ceiling this app
 * knows: that is the hardware's own business, which is why the agent carries the threshold
 * and the client never invents one.
 */
@Composable
private fun Footnote(metrics: HostMetrics) {
    val sensor = mostConcerning(metrics.thermal)
    val uptime = metrics.uptimeSeconds?.let { "up ${uptimePhrase(it)}" }
    val load = metrics.cpu?.let(::loadPhrase)
    if (uptime == null && sensor == null && load == null) return

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clearAndSetSemantics {
                // The full sensor name is spoken even though only the chip is drawn, so the
                // shortening stays a layout decision rather than a loss of information.
                contentDescription = listOfNotNull(
                    uptime,
                    load?.let { "load average $it" },
                    sensor?.let { "${it.celsius.roundToInt()} degrees, ${it.label}" },
                ).joinToString(", ")
            },
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        FootnoteCell(uptime, MaterialTheme.colorScheme.onSurfaceVariant)

        // Load lives here rather than under the CPU meter, where it belongs by subject but
        // would not fit: "4 cores · 0.1 queued" truncates to "4 cores · 0.1…" in a third of
        // a phone. This row is machine-level facts placed on the grid rather than facts
        // about the column above them — uptime is not about the processor either — so the
        // move costs nothing and the word carries its own label.
        FootnoteCell(load, MaterialTheme.colorScheme.onSurfaceVariant)

        FootnoteCell(
            text = sensor?.let { "${it.celsius.roundToInt()}°C ${chipOf(it.label)}" },
            // Coloured only when the hardware itself says the reading is high. The
            // threshold comes from the sensor, so a laptop CPU at 85°C and a drive at
            // 85°C are judged by their own limits rather than by a number this app
            // made up.
            color = if (sensor?.isHot == true) {
                CueSeekStatus.colors.degraded
            } else {
                MaterialTheme.colorScheme.onSurfaceVariant
            },
        )
    }
}

/** One cell of the footnote row, on the same weights as the meters above it. */
@Composable
private fun RowScope.FootnoteCell(text: String?, color: Color) {
    Text(
        text.orEmpty(),
        style = CueSeekType.Data.Small,
        color = color,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = Modifier.weight(1f),
    )
}

/**
 * The one-minute load average, worded so it does not claim to be something else.
 *
 * "load 0.1 of 4" read as cores in use, which is wrong in a way that matters. Load average
 * counts processes waiting to run *or* blocked on disk; it can legitimately exceed the core
 * count, and it is a different measurement from the percentage above it — which is the whole
 * reason both are on screen. A machine thrashing its swap reads near-zero usage and
 * near-unusable load, and only the pair catches that.
 *
 * "queued" is the plainest true word available. It is not perfect — a running process is
 * counted and is not queued — but it is honest about the shape of the number, where the old
 * phrasing was confidently wrong about its meaning. The core count moved under the CPU
 * meter, where it belongs and where it reads as capacity rather than as consumption.
 */
internal fun loadPhrase(cpu: dev.cueseek.core.model.CpuMetrics): String? =
    cpu.load1?.let { "${trimZero(it)} queued" }

/**
 * The sensor worth showing, which is not simply the hottest one.
 *
 * Picking by raw temperature made the label flicker between unrelated pieces of hardware:
 * on the test machine `acpitz` (a chassis sensor with no stated limit) and `coretemp` (the
 * CPU, limit 87°C) sit within a few degrees of each other and trade places minute to minute,
 * so the strip kept changing what it was talking about while nothing was happening.
 *
 * So: rank by **how close each sensor is to its own limit**, and prefer sensors that state
 * one. A CPU at 61 of 87 is less concerning than a drive at 50 of 60, and saying so requires
 * the hardware's own threshold rather than a number this app invents. Sensors with no stated
 * limit cannot be compared that way and are used only when nothing else is available, which
 * on ordinary hardware also makes the choice stable.
 */
internal fun mostConcerning(sensors: List<ThermalMetrics>?): ThermalMetrics? {
    if (sensors.isNullOrEmpty()) return null
    val rated = sensors.mapNotNull { sensor ->
        sensor.highCelsius?.takeIf { it > 0f }?.let { limit -> sensor to sensor.celsius / limit }
    }
    if (rated.isEmpty()) return sensors.maxByOrNull { it.celsius }
    return rated.maxByOrNull { (_, headroom) -> headroom }?.first
}

/**
 * The colour of a filling resource.
 *
 * Nothing until 85%, and only then. Memory and disk differ from CPU in that filling them up
 * is a state somebody has to act on rather than a machine doing its job, but a box sitting
 * at 70% is entirely normal and colouring it would spend attention on nothing.
 */
@Composable
private fun pressureTint(fraction: Float): Color? = when {
    fraction >= 0.95f -> CueSeekStatus.colors.unreachable
    fraction >= 0.85f -> CueSeekStatus.colors.degraded
    else -> null
}

/**
 * The chip a sensor belongs to — the first word of its label.
 *
 * Found on hardware. The footnote is one line of four short facts, and a full sensor name
 * is long enough to push it onto a second: "coretemp Core 0" wrapped and left "Core 0"
 * stranded on its own line, while "acpitz" on the same machine fitted. Which one is hottest
 * changes minute to minute, so the layout broke and healed on its own — the worst kind of
 * defect to find later.
 *
 * The chip is the part that answers the question a glance is asking. "Which of four cores"
 * is detail for a screen that has room for it; "the CPU package is at 61°" is the fact.
 * Screen readers still get the full label.
 */
internal fun chipOf(label: String): String = label.trim().substringBefore(' ')

/** Coarse uptime. Nobody reads a server's uptime to the minute after the first hour. */
internal fun uptimePhrase(seconds: Long): String {
    val minutes = seconds / 60
    val hours = minutes / 60
    val days = hours / 24
    return when {
        minutes < 60 -> "${minutes}m"
        hours < 48 -> "${hours}h"
        else -> "${days}d"
    }
}

/** "1.2" rather than "1.20", and "3" rather than "3.0". */
internal fun trimZero(value: Float): String {
    val rounded = (value * 10).roundToInt() / 10f
    return if (rounded % 1f == 0f) rounded.toInt().toString() else rounded.toString()
}

/** The fullest of the reported filesystems, or null when none can be judged. */
internal fun fullest(storage: List<StorageMetrics>?): StorageMetrics? =
    storage?.filter { it.usedFraction != null }?.maxByOrNull { it.usedFraction ?: 0f }
