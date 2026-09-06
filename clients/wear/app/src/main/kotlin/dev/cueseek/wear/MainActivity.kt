package dev.cueseek.wear

import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.wear.compose.material3.AppScaffold
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.ScreenScaffold
import androidx.wear.compose.material3.Text
import dev.cueseek.wear.pairing.PairingScreen
import dev.cueseek.wear.theme.CueSeekWearTheme

/**
 * M5.3b: pair the watch with an agent.
 *
 * Still one screen and a `Boolean`. Navigation, the dashboard and swipe-to-dismiss arrive
 * in M5.4 and M5.8; building a nav graph around a single destination now would be
 * scaffolding for a shape nobody knows yet.
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Build.MODEL is what the operator will see in the phone's device list, so it has
        // to be recognisable there rather than pretty here.
        setContent { WearApp(deviceName = Build.MODEL) }
    }
}

@Composable
fun WearApp(deviceName: String) {
    CueSeekWearTheme {
        AppScaffold {
            var paired by remember { mutableStateOf(false) }
            if (paired) {
                PairedPlaceholder()
            } else {
                PairingScreen(deviceName = deviceName, onPaired = { paired = true })
            }
        }
    }
}

/**
 * Where the dashboard goes in M5.4a.
 *
 * Deliberately blunt, so nobody mistakes it for a screen that has been designed.
 */
@Composable
private fun PairedPlaceholder() {
    ScreenScaffold {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(horizontal = 16.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = "Paired",
                style = MaterialTheme.typography.titleMedium,
                textAlign = TextAlign.Center,
            )
            Text(
                text = "dashboard lands in M5.4",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
        }
    }
}

@Preview(device = "id:wearos_large_round", showSystemUi = true)
@Composable
private fun PairingPreview() = WearApp(deviceName = "Watch")
