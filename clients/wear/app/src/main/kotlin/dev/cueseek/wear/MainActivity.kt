package dev.cueseek.wear

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.wear.compose.material3.AppScaffold
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.ScreenScaffold
import androidx.wear.compose.material3.Text
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.wear.pairing.AddressFromPhone
import dev.cueseek.wear.theme.CueSeekWearTheme
import dev.cueseek.wear.theme.WearType

/**
 * M5.3a: does the address arrive from the phone?
 *
 * Still a probe rather than a screen. The pairing flow it feeds — a code field and nothing
 * else — is M5.3b. This exists so the handoff can be watched working on hardware before
 * anything is built on top of it, which is the order every phase of this project has used.
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { WearApp() }
    }
}

/** What the watch knows about where the agent is, before anybody has typed anything. */
private sealed interface Handoff {
    data object Looking : Handoff
    data class Received(val address: AgentAddress) : Handoff
    data object Absent : Handoff
}

@Composable
fun WearApp() {
    CueSeekWearTheme {
        AppScaffold {
            ScreenScaffold {
                val context = LocalContext.current

                // produceState rather than a ViewModel: one read, nothing to survive a
                // rotation a watch cannot do, and M5.3b moves this behind the pairing
                // screen's own state holder anyway.
                val handoff by produceState<Handoff>(initialValue = Handoff.Looking) {
                    val address = AddressFromPhone(context).read()
                    value = if (address == null) Handoff.Absent else Handoff.Received(address)
                }

                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(horizontal = 16.dp),
                    verticalArrangement = Arrangement.Center,
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text(
                        text = "CueSeek",
                        textAlign = TextAlign.Center,
                        style = MaterialTheme.typography.titleMedium,
                    )

                    when (val state = handoff) {
                        Handoff.Looking -> Text(
                            text = "asking the phone…",
                            textAlign = TextAlign.Center,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )

                        is Handoff.Received -> Text(
                            // WearType.DataSmall, not numeralSmall. An address is data,
                            // but it is an identifier rather than a magnitude, and Wear's
                            // numeral roles start at 24sp -- see WearType for what that
                            // looked like on the watch.
                            text = state.address.toString(),
                            textAlign = TextAlign.Center,
                            style = WearType.DataSmall,
                            color = MaterialTheme.colorScheme.primary,
                        )

                        Handoff.Absent -> Text(
                            text = "no address from a phone",
                            textAlign = TextAlign.Center,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}

@Preview(device = "id:wearos_large_round", showSystemUi = true)
@Composable
private fun WearAppPreview() = WearApp()
