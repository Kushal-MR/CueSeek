package dev.cueseek.android.ui

import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * Timestamps, rendered in the reader's own timezone.
 *
 * The agent emits `observed_at` as RFC 3339 in UTC, and an earlier version printed it by
 * slicing the ISO string — which showed `04:14:32` to someone whose clock read `09:45`.
 * In an app whose whole claim is that it never looks fresher or staler than it is, a
 * timestamp five and a half hours out is worse than no timestamp at all.
 *
 * Found by looking at the running app on a phone in IST. No unit test would have caught
 * it, because the JVM's default zone in CI is UTC and the bug is invisible there.
 */
private val CLOCK: DateTimeFormatter = DateTimeFormatter.ofPattern("HH:mm:ss")

/** Wall-clock time, as the person holding the phone would read it. */
fun localClock(instant: Instant, zone: ZoneId = ZoneId.systemDefault()): String =
    CLOCK.format(instant.atZone(zone))

/**
 * "4s ago", "3m ago", then an absolute local time.
 *
 * Precision degrades as a fact ages, because past an hour the exact clock time is more use
 * than a number that keeps growing.
 */
fun observedPhrase(
    observedAt: Instant,
    now: Instant = Instant.now(),
    zone: ZoneId = ZoneId.systemDefault(),
): String {
    val seconds = Duration.between(observedAt, now).seconds
    return when {
        seconds < 0 -> "just now"
        seconds < 60 -> "${seconds}s ago"
        seconds < 3600 -> "${seconds / 60}m ago"
        else -> "at " + localClock(observedAt, zone)
    }
}
