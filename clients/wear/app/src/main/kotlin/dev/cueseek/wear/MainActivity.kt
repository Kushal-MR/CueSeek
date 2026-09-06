package dev.cueseek.wear

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.wear.compose.material3.AppScaffold
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.ScreenScaffold
import androidx.wear.compose.material3.Text
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.wear.theme.CueSeekWearTheme

/**
 * M5.1: the skeleton. One screen that names itself and proves the build works end to end.
 *
 * Everything here is temporary except the imports, which are the point. `MaterialTheme`,
 * `Text` and the scaffolds all come from `androidx.wear.compose.material3` — a different
 * library from the phone's `androidx.compose.material3`, with different components and a
 * different scaffold. Importing the phone's Material 3 in this module is a defect
 * (ADR-0010: the tokens are shared, the components are not).
 *
 * The real theme lands in M5.2, which maps DESIGN.md's palette onto Wear's ColorScheme.
 * Until then this is the library default, and it is meant to look unfinished.
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent { WearApp() }
    }
}

@Composable
fun WearApp() {
    CueSeekWearTheme {
        // AppScaffold owns what persists across screens — the clock at the top. Each
        // screen then supplies its own ScreenScaffold. This pairing has no phone
        // equivalent and is the first thing that makes a Wear layout behave correctly.
        //
        // The default timeText is deliberate: overriding it is how apps end up with a
        // clock that disagrees with the watch face.
        AppScaffold {
            ScreenScaffold {
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
                        style = MaterialTheme.typography.titleLarge,
                    )
                    // Not decoration. This line is the phase's actual acceptance test:
                    // a type from :core:model, resolved and executed inside a Wear
                    // process. ADR-0013 claimed the module was plain Kotlin/JVM precisely
                    // so a second consumer could do this with no audit and no change.
                    //
                    // It round-trips through fromWire() rather than naming the constant,
                    // so the companion object and the wire mapping are exercised too — a
                    // reference the compiler could not have optimised into nothing.
                    Text(
                        text = "core:model -> " + HealthStatus.fromWire("healthy").name,
                        textAlign = TextAlign.Center,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@Preview(device = "id:wearos_large_round", showSystemUi = true)
@Composable
private fun WearAppPreview() = WearApp()
