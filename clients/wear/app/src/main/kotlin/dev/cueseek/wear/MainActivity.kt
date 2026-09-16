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
import androidx.navigation.NavType
import androidx.navigation.navArgument
import androidx.wear.compose.material3.AppScaffold
import androidx.wear.compose.material3.ScreenScaffold
import androidx.wear.compose.navigation.SwipeDismissableNavHost
import androidx.wear.compose.navigation.composable
import androidx.wear.compose.navigation.rememberSwipeDismissableNavController
import dev.cueseek.wear.dashboard.DashboardScreen
import dev.cueseek.wear.dashboard.DashboardUi
import dev.cueseek.wear.dashboard.DashboardViewModel
import dev.cueseek.wear.detail.ServiceDetailScreen
import dev.cueseek.wear.pairing.PairingScreen
import dev.cueseek.wear.power.HostPowerScreen
import dev.cueseek.wear.power.PowerAccess
import dev.cueseek.wear.power.powerAccess
import dev.cueseek.wear.theme.CueSeekWearTheme

/**
 * The dashboard, one service in full, and the machine itself.
 *
 * Routing between paired and unpaired comes from the store rather than from a flag this
 * class remembers — see [RootViewModel] for why that distinction cost a bug.
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
                    // sees it, and this recomposes — so the screen that paired does not also
                    // have to know what comes after it.
                    onPaired = {},
                )

                Root.Dashboard -> PairedApp()
            }
        }
    }
}

/**
 * The two screens a paired watch has, and the gesture between them.
 *
 * `SwipeDismissableNavHost` rather than a back stack of our own: on Wear the back gesture
 * *is* a swipe, and an app that implements it by hand gets the edge behaviour and the
 * animation subtly wrong. M5.8 makes rotary and haptics systematic across every screen;
 * this is the minimum that makes a detail screen reachable and leavable.
 *
 * One [DashboardViewModel] is shared by both destinations, so opening a service does not
 * re-poll the agent — the data is already in hand, and a watch should not spend a radio
 * round trip on a navigation.
 */
@Composable
private fun PairedApp(dashboard: DashboardViewModel = viewModel()) {
    val navController = rememberSwipeDismissableNavController()
    val ui by dashboard.ui.collectAsStateWithLifecycle()
    val action by dashboard.action.collectAsStateWithLifecycle()

    SwipeDismissableNavHost(
        navController = navController,
        startDestination = ROUTE_DASHBOARD,
    ) {
        composable(ROUTE_DASHBOARD) {
            DashboardScreen(
                model = dashboard,
                onServiceClick = { id -> navController.navigate("$ROUTE_SERVICE/$id") },
                onPowerClick = { navController.navigate(ROUTE_POWER) },
            )
        }

        composable(ROUTE_POWER) {
            val loaded = ui as? DashboardUi.Loaded

            androidx.compose.runtime.LaunchedEffect(Unit) { dashboard.clearAction() }

            HostPowerScreen(
                // Recomputed from the current reading rather than passed in at navigation
                // time, so a revoked scope or an agent that stopped offering power takes
                // effect on the next poll instead of on the next launch.
                access = loaded
                    ?.let { powerAccess(it.scopes, it.hostActions) }
                    ?: PowerAccess.Ungranted,
                services = loaded?.services.orEmpty(),
                action = action,
                onInvoke = { actionId, label -> dashboard.invokePower(actionId, label) },
            )
        }

        composable(
            route = "$ROUTE_SERVICE/{$ARG_SERVICE}",
            arguments = listOf(navArgument(ARG_SERVICE) { type = NavType.StringType }),
        ) { entry ->
            val wanted = entry.arguments?.getString(ARG_SERVICE)
            // A lookup by key, not a branch on identity: what the service *is* never
            // decides what gets drawn (ADR-0005, and WearCapabilityTest enforces it).
            val service = (ui as? DashboardUi.Loaded)?.services?.firstOrNull { it.id == wanted }

            if (service == null) {
                // The agent dropped it between the tap and the draw — a service removed from
                // the configuration, or a poll that arrived in between. Going back is the
                // honest response; an empty detail screen would imply the service still
                // exists and is simply doing nothing.
                navController.popBackStack()
            } else {
                // The action banner is cleared on entry rather than on exit, so arriving at
                // a service never shows the outcome of something done to a different one.
                androidx.compose.runtime.LaunchedEffect(service.id) { dashboard.clearAction() }

                ServiceDetailScreen(
                    service = service,
                    stale = false,
                    action = action,
                    onInvoke = { actionId, label ->
                        dashboard.invoke(service.id, actionId, label)
                    },
                )
            }
        }
    }
}

private const val ROUTE_DASHBOARD = "dashboard"
private const val ROUTE_SERVICE = "service"
private const val ROUTE_POWER = "power"
private const val ARG_SERVICE = "id"

@Preview(device = "id:wearos_large_round", showSystemUi = true)
@Composable
private fun WearAppPreview() = WearApp(deviceName = "Watch")
