package dev.cueseek.android

import android.app.Activity
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalView
import androidx.core.view.WindowCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.cueseek.android.ui.CueSeekViewModel
import dev.cueseek.android.ui.Screen
import dev.cueseek.android.ui.dashboard.DashboardScreen
import dev.cueseek.android.ui.pairing.PairingScreen
import androidx.compose.foundation.isSystemInDarkTheme
import dev.cueseek.core.data.ThemeChoice
import dev.cueseek.core.design.CueSeekTheme
import dev.cueseek.core.design.token.CueSeekMotion

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            CueSeekApp()
        }
    }
}

/**
 * The whole app.
 *
 * Two destinations, chosen by whether a host is paired, so there is no navigation graph —
 * a `NavHost` for a binary condition would be machinery describing a boolean. A service's
 * detail is a sheet, not a destination, and predictive back dismisses it.
 */
@Composable
private fun CueSeekApp(
    viewModel: CueSeekViewModel = viewModel(factory = CueSeekViewModel.Factory),
) {
    // The theme wraps the app rather than the activity, because the choice lives in the
    // ViewModel. System means follow the device, which is not the same as light.
    val theme by viewModel.theme.collectAsStateWithLifecycle()
    val dark = when (theme) {
        ThemeChoice.System -> isSystemInDarkTheme()
        ThemeChoice.Light -> false
        ThemeChoice.Dark -> true
    }

    // The system bars are not ours to theme through MaterialTheme, and enableEdgeToEdge
    // picks their icon contrast from the *system* setting. Forcing Light while the device
    // is in dark mode therefore left white status icons on a pale background. The bars
    // follow the app's resolved theme, not the device's.
    val view = LocalView.current
    if (!view.isInEditMode) {
        val window = (view.context as Activity).window
        SideEffect {
            WindowCompat.getInsetsController(window, view).apply {
                isAppearanceLightStatusBars = !dark
                isAppearanceLightNavigationBars = !dark
            }
        }
    }

    CueSeekTheme(darkTheme = dark) { CueSeekContent(viewModel) }
}

@Composable
private fun CueSeekContent(viewModel: CueSeekViewModel) {
    // collectAsStateWithLifecycle is repeatOnLifecycle: collection starts at STARTED and
    // stops at STOPPED, so backgrounding tears the stream down rather than holding a
    // connection that Doze would freeze into a lie anyway.
    val screen by viewModel.screen.collectAsStateWithLifecycle(initialValue = Screen.Loading)

    Scaffold(
        modifier = Modifier.fillMaxSize(),
        containerColor = MaterialTheme.colorScheme.surface,
    ) { padding ->
        AnimatedContent(
            targetState = screen,
            transitionSpec = {
                fadeIn(tween(CueSeekMotion.DurationEnter, easing = CueSeekMotion.EmphasizedDecelerate)) togetherWith
                    fadeOut(tween(CueSeekMotion.DurationExit, easing = CueSeekMotion.EmphasizedAccelerate))
            },
            contentKey = { it::class },
            label = "screen",
            modifier = Modifier.padding(padding),
        ) { current ->
            when (current) {
                // Deliberately blank rather than a spinner. This state lasts one frame
                // while DataStore answers, and a spinner that flashes is worse than nothing.
                Screen.Loading -> Box(Modifier.fillMaxSize())
                Screen.Unpaired -> PairingScreen(viewModel)
                is Screen.Paired -> DashboardScreen(current.state, viewModel)
            }
        }
    }
}
