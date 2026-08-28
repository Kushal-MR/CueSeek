package dev.cueseek.android.ui

import java.util.Locale
import kotlin.math.roundToInt

/**
 * Formatting for the numbers activity capabilities carry.
 *
 * Kept out of the composables so the rounding decisions are testable, and out of
 * `:core:design` because they are language rather than tokens.
 *
 * The governing rule is **one glance, no arithmetic**. An operations console is read while
 * doing something else, so "1.2 MB/s" beats "1,258,291 B/s" every time, and a value that
 * needs mental division has failed at the only job it had.
 */

/**
 * A transfer rate.
 *
 * Decimal units, matching what every download client and every ISP quotes — a torrent
 * client saying 1.2 MB/s next to a console saying 1.1 MiB/s would look like a defect in the
 * console, and being pedantically correct is not worth that.
 *
 * Returns null at zero rather than "0 B/s". A stopped transfer should say nothing about
 * speed at all; printing a zero invites the reader to wonder whether it is stalled or
 * simply idle, which is what the state word is for.
 */
fun rateOrNull(bytesPerSecond: Long?): String? {
    val bytes = bytesPerSecond ?: return null
    if (bytes <= 0) return null
    return "${byteSize(bytes)}/s"
}

/**
 * A size, in decimal units.
 *
 * One decimal place below 100, none above: "9.4 GB" carries information, "947.3 MB" is
 * three digits of noise in a column that has to stay narrow.
 */
fun byteSize(bytes: Long): String {
    if (bytes < 1_000) return "$bytes B"

    val units = listOf("kB", "MB", "GB", "TB", "PB")
    var value = bytes.toDouble() / 1_000
    var unit = 0
    while (value >= 1_000 && unit < units.lastIndex) {
        value /= 1_000
        unit++
    }
    return if (value < 100) {
        String.format(Locale.US, "%.1f %s", value, units[unit])
    } else {
        String.format(Locale.US, "%.0f %s", value, units[unit])
    }
}

/**
 * A countdown, coarsening as it grows.
 *
 * Seconds matter when there are seconds left; days do not need minutes. Returns null for
 * an unknown or absent estimate — the agent already normalises "effectively never" to
 * absent, so anything reaching here is a real number.
 */
fun etaOrNull(seconds: Int?): String? {
    val total = seconds ?: return null
    if (total <= 0) return null

    val minutes = total / 60
    val hours = minutes / 60
    val days = hours / 24

    return when {
        total < 60 -> "${total}s"
        minutes < 60 -> "${minutes}m"
        hours < 24 -> "${hours}h ${minutes % 60}m"
        else -> "${days}d ${hours % 24}h"
    }
}

/**
 * Elapsed and total playback, as one phrase.
 *
 * Hours only when there are hours, so an episode reads "12:34 / 48:20" rather than
 * "00:12:34 / 00:48:20" — the leading zeroes are pure column noise on the shorter form.
 */
fun playbackPosition(positionSeconds: Int?, durationSeconds: Int?): String? {
    val position = positionSeconds ?: return null
    val duration = durationSeconds
    // Live or unbounded content has a position but no end. Saying where you are is still
    // useful; inventing a total is not.
    if (duration == null || duration <= 0) return clock(position)
    return "${clock(position)} / ${clock(duration)}"
}

private fun clock(seconds: Int): String {
    val safe = seconds.coerceAtLeast(0)
    val hours = safe / 3600
    val minutes = (safe % 3600) / 60
    val secs = safe % 60
    return if (hours > 0) {
        String.format(Locale.US, "%d:%02d:%02d", hours, minutes, secs)
    } else {
        String.format(Locale.US, "%d:%02d", minutes, secs)
    }
}

/** A fraction as a whole-number percentage, for a label beside a progress rule. */
fun percent(fraction: Float): String = "${(fraction.coerceIn(0f, 1f) * 100).roundToInt()}%"
