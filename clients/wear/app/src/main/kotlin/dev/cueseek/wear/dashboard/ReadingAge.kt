package dev.cueseek.wear.dashboard

import java.time.Duration
import java.time.Instant

/**
 * How old the reading on screen is, in the fewest characters that stay honest.
 *
 * # Why ambient needs this and the interactive screen does not
 *
 * Interactive has a live status word — `Running`, `Unverified` — and a poll that fires the
 * moment the screen comes up, so "how old is this?" is answered by the fact you are looking
 * at a fresh fetch. **Ambient has neither.** It deliberately does not poll, so what it shows
 * is by definition the last thing known, and the only honest way to present that is to say
 * *when*.
 *
 * A dimmed screen saying `Operational` with no age is the exact failure this project keeps
 * legislating against — confident green with nothing behind it — except worse, because
 * ambient can sit on a wrist for an hour.
 *
 * # The rounding is deliberately coarse
 *
 * Seconds are not rendered beyond the first minute. A wrist glanced at in ambient is asking
 * "is this current or is this old?", and `3m` answers it; `3m 42s` spends three extra
 * characters on precision nobody acts on, on the screen with the least room and the least
 * light.
 */
fun readingAge(observedAt: Instant, now: Instant = Instant.now()): String {
    val seconds = Duration.between(observedAt, now).seconds

    return when {
        // A clock that went backwards — a time-zone change, an NTP correction, a resumed
        // VM. "Now" is the least wrong thing to say, and a negative age would be nonsense
        // rendered with total confidence.
        seconds <= 0L -> "now"

        // Inside the first minute there is nothing to report: the reading is current by
        // any standard this app applies, and STALE_AFTER is 90 seconds away.
        seconds < 60L -> "now"

        seconds < 3_600L -> "${seconds / 60}m ago"

        // Hours, then days. Both are failure territory rather than normal operation — an
        // ambient screen this old means the app has been resident and unrefreshed for a
        // very long time — but saying "59m ago" forever would be a lie of omission.
        seconds < 86_400L -> "${seconds / 3_600}h ago"

        else -> "${seconds / 86_400}d ago"
    }
}
