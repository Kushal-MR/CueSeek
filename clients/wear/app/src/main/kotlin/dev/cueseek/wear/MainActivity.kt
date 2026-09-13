package dev.cueseek.wear

import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.wear.compose.material3.AppScaffold
import androidx.wear.compose.material3.ScreenScaffold
import dev.cueseek.wear.dashboard.DashboardScreen
import dev.cueseek.wear.pairing.PairingScreen
import dev.cueseek.wear.theme.CueSeekWearTheme

/**
 * M5.4a: the dashboard, once there is something to show.
 *
 * Routing comes from the store rather than from a flag this class remembers — see
 * [RootViewModel] for why that distinction cost a bug. Navigation proper, and
 * swipe-to-dismiss between screens, arrive in M5.8.
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Build.MODEL is what the operator sees in the phone's device list, so it has to be
        // recognisable there rather than pretty here.
        setContent { WearApp(deviceName = Build.MODEL) }
    }
}

@Composable
fun WearApp(
    deviceName: String,
    model: RootViewModel = viewModel(),
) {
    CueSeekWearTheme {
        AppScaffold {
            val root by model.root.collectAsStateWithLifecycle()

            when (root) {
                // Nothing, deliberately. The store answers in milliseconds, and a spinner
                // that appears for one frame is worse than a dark screen for one frame.
                Root.Deciding -> ScreenScaffold {
                    Box(modifier = Modifier.fillMaxSize()) {}
                }

                Root.Pairing -> PairingScreen(
                    deviceName = deviceName,
                    // No navigation call. Pairing writes a host to the store, the root flow
                    // sees it, and this recomposes to the dashboard — so the screen that
                    // paired does not also have to know what comes after it.
                    onPaired = {},
                )

                Root.Dashboard -> DashboardScreen()
            }
        }
    }
}

@Preview(device = "id:wearos_large_round", showSystemUi = true)
@Composable
private fun WearAppPreview() = WearApp(deviceName = "Watch")
