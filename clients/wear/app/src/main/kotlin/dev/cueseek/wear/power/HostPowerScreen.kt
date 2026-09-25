package dev.cueseek.wear.power

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.wear.compose.foundation.lazy.TransformingLazyColumn
import androidx.wear.compose.foundation.lazy.rememberTransformingLazyColumnState
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.ScreenScaffold
import androidx.wear.compose.material3.Text
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.model.Service
import dev.cueseek.wear.dashboard.ActionUi
import dev.cueseek.wear.detail.ActionButton
import dev.cueseek.wear.feedback.ActionOutcomeHaptics

/**
 * The machine itself: reboot, and shut down.
 *
 * # A screen, not a row
 *
 * It would have been less code to append these to the dashboard's roster, and that is the
 * reason not to. The roster is the thing a wrist scrolls through, and a control that ends the
 * machine must not be reachable by momentum — the last flick of a scroll should never land a
 * thumb on "Shut down". A deliberate tap on a deliberate entry point, then a hold, is two
 * gestures neither of which happens by accident.
 *
 * It is also not a service. The phone keeps host power out of the service list for the same
 * reason its acceptance type carries no `serviceId`: a machine is not one of its own
 * services, and putting it in the roster would make it look like one that could be restarted
 * and come back.
 *
 * # Ceremony comes from the agent, exactly as it does for a service
 *
 * [ActionButton] is reused rather than reimplemented, so `destructive` gets the same
 * press-and-hold here as stopping a unit does, and an agent that one day classified a
 * suspend as `disruptive` would get the confirm flow with no change on this side. The risk
 * vocabulary is the agent's, not this screen's opinion of what is frightening — which is
 * also why there is no fourth level meaning "worse than destructive". The consequence is
 * stated in words instead, which is where it is read.
 */
@Composable
fun HostPowerScreen(
    access: PowerAccess,
    services: List<Service>,
    /**
     * Whether the reading these controls were drawn from has aged out.
     *
     * Not a lock. The operator owns the machine and may have excellent reasons to shut it
     * down on a reading they know is old — refusing would make the tool argue with the
     * person it exists to serve (ADR-0002 Amendment 2). What it changes is the **claim**:
     * "Right now: 2 playing" becomes a lie the moment the reading dies, so it is withdrawn
     * and replaced with the fact that nothing recent is known.
     */
    stale: Boolean,
    action: ActionUi,
    onInvoke: (actionId: String, label: String) -> Unit,
) {
    val listState = rememberTransformingLazyColumnState()

    // Felt, not read — and this is the screen where that matters most. A power action's
    // success is silence by design, so the *refusal* is the only thing there is to report,
    // and it must not need a glance to notice.
    ActionOutcomeHaptics(action)

    ScreenScaffold(scrollState = listState) { contentPadding ->
        TransformingLazyColumn(
            state = listState,
            contentPadding = contentPadding,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            item {
                Text(
                    text = "Machine",
                    style = MaterialTheme.typography.titleMedium,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.padding(bottom = 4.dp),
                )
            }

            when (access) {
                // Unreachable through the dashboard, which shows no entry point without the
                // grant. Handled anyway because a token can be revoked between the tap and
                // the draw, and arriving at a blank screen would look like a crash.
                PowerAccess.Ungranted -> item {
                    Note("This watch was not given permission to power the machine.")
                }

                PowerAccess.NoneOffered -> item {
                    // The distinction the agent preserves by returning the same list to
                    // everyone: this is the agent's limitation, not this device's.
                    Note("This agent offers no power actions.")
                }

                is PowerAccess.Offered -> {
                    // What the machine is in the middle of. Stated, never enforced: the
                    // operator owns this box and may have excellent reasons to shut it down
                    // mid-transcode, and a tool that argued would be arguing with the person
                    // it exists to serve (ADR-0002 Amendment 2).
                    when (val claim = busyClaim(services, stale)) {
                        // Said rather than omitted: silence here would read as "nothing is
                        // running", which is the one thing this screen must never imply
                        // when it does not know. The buttons below still work.
                        BusyClaim.Unknown -> item {
                            Note(
                                text = "Last reading is out of date — what this would interrupt is unknown.",
                                color = CueSeekStatus.colors.unknown,
                            )
                        }

                        is BusyClaim.Busy -> item {
                            Note(
                                text = "Right now: ${claim.summary}",
                                color = CueSeekStatus.colors.degraded,
                            )
                        }

                        // Deliberately nothing. "Nothing is running" would be a claim about
                        // services whose activity the agent could not read either.
                        BusyClaim.Quiet -> Unit
                    }

                    item { PowerOutcome(action) }

                    items(
                        count = access.actions.size,
                        key = { access.actions[it].id },
                    ) { index ->
                        val power = access.actions[index]
                        // One [Column], not two siblings. A lazy item slot takes a single
                        // composable, and emitting two put the button and its description in
                        // the same place — which rendered as the description alone and cost a
                        // watch to find, because every unit test still passed.
                        Column(modifier = Modifier.fillMaxWidth()) {
                            ActionButton(
                                action = power,
                                enabled = action !is ActionUi.Working,
                                onConfirmed = { onInvoke(power.id, power.label) },
                            )
                            // The agent's own sentence, under its own button rather than
                            // above it: it is the half that knows whether this machine comes
                            // back on its own, and it is read after the label has raised the
                            // question.
                            power.description?.takeIf { it.isNotBlank() }?.let { Note(it) }
                        }
                    }
                }
            }
        }
    }
}

/**
 * What became of a power request.
 *
 * "Asked", and then a sentence saying the agent is expected to disappear — because on this
 * screen silence is the success, and a watch that said nothing would leave somebody holding
 * the button again. Nothing here ever says "Rebooted": a power action that worked took the
 * connection that would have reported it, so no client can honestly claim the outcome.
 */
@Composable
private fun PowerOutcome(action: ActionUi) {
    when (action) {
        ActionUi.Idle -> Unit

        is ActionUi.Working -> Note("asking the agent…")

        is ActionUi.Accepted -> Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 4.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            Text(
                text = "${action.label} — asked",
                style = MaterialTheme.typography.bodyExtraSmall,
                color = CueSeekStatus.colors.beat,
                textAlign = TextAlign.Center,
            )
            Text(
                text = "The agent will go quiet now.",
                style = MaterialTheme.typography.bodyExtraSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
        }

        is ActionUi.Failed -> Text(
            text = action.message,
            style = MaterialTheme.typography.bodyExtraSmall,
            color = MaterialTheme.colorScheme.error,
            textAlign = TextAlign.Center,
            modifier = Modifier
                .fillMaxWidth()
                .padding(bottom = 4.dp),
        )
    }
}

@Composable
private fun Note(text: String, color: Color = Color.Unspecified) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyExtraSmall,
        color = if (color == Color.Unspecified) {
            MaterialTheme.colorScheme.onSurfaceVariant
        } else {
            color
        },
        textAlign = TextAlign.Center,
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 4.dp, bottom = 4.dp),
    )
}
