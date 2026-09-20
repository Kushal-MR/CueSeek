package dev.cueseek.wear.feedback

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.hapticfeedback.HapticFeedback
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import dev.cueseek.wear.dashboard.ActionUi

/**
 * What the watch says through the wrist.
 *
 * # Three events, and deliberately only three
 *
 * A watch is often operated without looking at it — that is most of why the app exists on
 * one. So the wrist has to carry the facts a glance would otherwise have to: **that a
 * control committed**, and **whether the agent took it or refused it**.
 *
 * Everything else stays silent. Taps, scrolls, navigation and arriving at a screen produce
 * nothing, because a device that buzzes at everything communicates nothing — the signal stops
 * being information and becomes texture, and then the one buzz that mattered is
 * indistinguishable from the twenty that did not.
 *
 * | | when | why it is felt |
 * | --- | --- | --- |
 * | [committed] | a hold crosses its threshold, or a disruptive action is confirmed | the decision is now made; you can let go |
 * | [accepted] | the agent took the request | it is on its way |
 * | [refused] | the agent declined it, or the call failed | it is *not* on its way |
 *
 * The last row is the one that earns this file. Without it, a failed action on a screen
 * nobody is looking at is indistinguishable from a successful one, and the watch becomes a
 * thing you have to verify on your phone — which is the opposite of the point.
 *
 * # The constants are semantic, not sounds
 *
 * These map to `HapticFeedbackConstants` the platform chooses the waveform for, rather than
 * to durations picked here. `GestureThresholdActivate` exists precisely for "a held gesture
 * has crossed the line"; `Confirm` and `Reject` are the platform's own pair for exactly the
 * distinction above. Hand-rolling amplitudes would mean overruling a vendor's tuning for
 * their own motor — and `aw-haptic-hv` on the Watch 2R is not the motor this would be tuned
 * against anyway.
 */
class CueSeekHaptics(private val haptics: HapticFeedback) {

    /**
     * A control has committed.
     *
     * Fires when the hold **reaches** its threshold rather than when the finger lifts, and
     * that ordering is the whole ergonomic point: it is the signal that says *you can stop
     * pressing now*, which is worth nothing if it arrives after you already have.
     *
     * The cost of that choice, stated rather than hidden: a hold that reaches the threshold
     * and is then cancelled — a scroll stealing the pointer — will have buzzed for something
     * that did not happen. The alternative is a control you must watch to use, on the device
     * least suited to being watched, so the trade goes this way.
     */
    fun committed() = haptics.performHapticFeedback(HapticFeedbackType.GestureThresholdActivate)

    /** The agent took the request. Not that it finished — see `DashboardViewModel.invoke`. */
    fun accepted() = haptics.performHapticFeedback(HapticFeedbackType.Confirm)

    /** The agent refused it, or the call never arrived. */
    fun refused() = haptics.performHapticFeedback(HapticFeedbackType.Reject)
}

@Composable
fun rememberCueSeekHaptics(): CueSeekHaptics {
    val haptics = LocalHapticFeedback.current
    return remember(haptics) { CueSeekHaptics(haptics) }
}

/**
 * Feels an action's outcome, once, wherever it is shown.
 *
 * Lives here rather than in each screen so the mapping from outcome to sensation is decided
 * in one place. Two screens invoke actions today — a service's lifecycle controls and the
 * machine's power controls — and a build where a failed restart buzzed differently from a
 * failed reboot would be teaching the wrist something untrue about the difference.
 *
 * Keyed on the state itself, so it fires on the transition and not on every recomposition
 * that happens to arrive while the banner is still up.
 */
@Composable
fun ActionOutcomeHaptics(action: ActionUi) {
    val haptics = rememberCueSeekHaptics()
    LaunchedEffect(action) {
        when (action) {
            is ActionUi.Accepted -> haptics.accepted()
            is ActionUi.Failed -> haptics.refused()
            // Working and Idle are not outcomes. The wrist stays quiet until there is
            // something to report.
            is ActionUi.Working, ActionUi.Idle -> Unit
        }
    }
}
