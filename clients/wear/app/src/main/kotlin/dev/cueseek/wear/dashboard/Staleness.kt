package dev.cueseek.wear.dashboard

import androidx.compose.runtime.Composable
import androidx.compose.runtime.State
import androidx.compose.runtime.produceState
import kotlinx.coroutines.delay
import java.time.Instant

/**
 * Whether the reading on screen is still worth believing.
 *
 * # Why this is a clock and not a connection
 *
 * A client that trusted "connected" to mean "current" would show confident green while the
 * agent was unreachable — the failure the phone was built to avoid, and one a watch is more
 * exposed to, because its radio sleeps far more aggressively than a phone's. So staleness is
 * a function of **when the reading was taken**, re-evaluated against the wall clock, and
 * nothing about the transport can suppress it.
 *
 * # Why it is hoisted rather than owned by the dashboard
 *
 * It used to live inside the dashboard, with a comment saying it belonged there because that
 * was the surface it existed to correct and nothing off-screen needed it. That was true when
 * the dashboard was the only screen. It stopped being true at M5.5 and M5.7, and nobody
 * noticed: the detail screen was passed a **hardcoded `false`**, so a reading could age out
 * while you looked at it and still render as fact.
 *
 * That is worse on the detail screen than on the dashboard, because the detail screen is
 * where you *act*. Deciding to restart something on a reading two minutes dead is a
 * different class of mistake from merely reading one.
 *
 * So the clock is hoisted to the one place all three screens share. It is still composable
 * scope rather than the ViewModel — it stops when the app is off screen, because a ticker
 * running behind a dark panel spends battery correcting a display nobody is looking at.
 *
 * @param observedAt when the reading on screen was taken, or null when there is no reading.
 */
@Composable
fun rememberStaleness(observedAt: Instant?): State<Boolean> =
    produceState(initialValue = false, key1 = observedAt) {
        if (observedAt == null) {
            // No reading is not a stale reading. Loading and error states say their own
            // thing, and marking them stale would be a second claim on top of a screen that
            // is already explaining itself.
            value = false
            return@produceState
        }
        while (true) {
            value = DashboardViewModel.isStale(observedAt)
            // Coarse on purpose. STALE_AFTER is 90 seconds, so a five-second tick is already
            // far finer than the thing it measures, and anything finer would be a wakeup per
            // second to answer a question whose answer changes twice an hour.
            delay(5_000)
        }
    }
