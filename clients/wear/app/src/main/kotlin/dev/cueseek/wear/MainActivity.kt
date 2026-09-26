package dev.cueseek.wear

import android.os.Build
import android.os.Bundle
import android.util.Log
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableStateOf
import androidx.wear.ambient.AmbientLifecycleObserver
import dev.cueseek.wear.ambient.AmbientScreen
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
import dev.cueseek.wear.dashboard.rememberStaleness
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
private const val TAG = "CueSeekWear"

class MainActivity : ComponentActivity() {

    /**
     * Whether the screen has dimmed.
     *
     * Plain activity state rather than something in a ViewModel, because that is what it
     * is: ambient is a property of *this window*, delivered by the framework to this
     * class, and routing it through a ViewModel would add an owner that has no opinion
     * about it. Compose reads it directly.
     */
    private val ambient = mutableStateOf(false)

    /**
     * The only thing `androidx.wear:wear` is used for.
     *
     * There is no Compose-level signal for "the screen dimmed" — it is an Activity
     * lifecycle fact, and this observer is how the framework reports it. Registered
     * against the lifecycle rather than driven by hand so it is torn down with the
     * activity and cannot outlive it.
     */
    private val ambientObserver = AmbientLifecycleObserver(this, object : AmbientLifecycleObserver.AmbientLifecycleCallback {
        override fun onEnterAmbient(details: AmbientLifecycleObserver.AmbientDetails) {
            Log.i(TAG, "onEnterAmbient burnIn=${details.burnInProtectionRequired} lowBit=${details.deviceHasLowBitAmbient}")
            ambient.value = true
        }

        override fun onExitAmbient() {
            Log.i(TAG, "onExitAmbient")
            ambient.value = false
        }

        // Fires about once a minute while dimmed. Deliberately does nothing: this is the
        // hook an app would use to redraw a clock, and CueSeek's ambient screen shows a
        // *reading* rather than the time. Refreshing here would mean polling the agent
        // from a dark wrist, which is exactly what M5.10 exists to prevent — and the one
        // thing that does change, the age, is recomputed when the screen is drawn.
        override fun onUpdateAmbient() = Unit
    })

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Logged on purpose, and kept. Ambient is the one behaviour in this app with no
        // visible evidence when it fails: the screen simply goes dark, which looks
        // identical whether the system declined to grant ambient or this class never asked
        // for it. Distinguishing those two cost three build cycles on the Watch 2R, and
        // this line is what settles it next time.
        runCatching { lifecycle.addObserver(ambientObserver) }
            .onSuccess { Log.i(TAG, "ambient observer registered") }
            .onFailure { Log.w(TAG, "ambient unavailable: $it") }
        // Build.MODEL is what the operator sees in the phone's device list, so it has to be
        // recognisable there rather than pretty here.
        setContent { WearApp(deviceName = Build.MODEL, ambient = ambient.value) }
    }
}

@Composable
fun WearApp(
    deviceName: String,
    ambient: Boolean = false,
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

                Root.Dashboard -> PairedApp(ambient = ambient)
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
private fun PairedApp(ambient: Boolean, dashboard: DashboardViewModel = viewModel()) {
    val navController = rememberSwipeDismissableNavController()
    val ui by dashboard.ui.collectAsStateWithLifecycle()
    val action by dashboard.action.collectAsStateWithLifecycle()

    // One clock for every screen. Hoisted here at M5.9 because the detail screen had been
    // receiving a hardcoded `false` since M5.5 — see [rememberStaleness] for why that was
    // worse on the screen you act from than on the one you read.
    val stale by rememberStaleness((ui as? DashboardUi.Loaded)?.observedAt)

    // Ambient replaces the whole navigation graph rather than dimming whatever screen
    // happened to be open. Two reasons, and the second is the one that matters:
    //
    //  1. A dimmed detail screen would keep a service's controls on a wrist that is down.
    //  2. Ambient is answering a different question. Interactive is "what is going on with
    //     this service"; ambient is "is everything still fine", which is the dashboard's
    //     question and the only one worth keeping a panel lit for.
    //
    // The back stack is untouched underneath, so lowering and raising a wrist returns to
    // the screen that was open rather than to the top.
    if (ambient) {
        AmbientScreen(ui = ui, stale = stale)
        return
    }

    // Coming back from ambient re-reads the agent. Ambient deliberately does not poll, so
    // whatever is on screen at this moment is at least as old as the dim — and the first
    // thing an operator does on raising a wrist is believe it.
    LaunchedEffect(Unit) { dashboard.refresh() }

    SwipeDismissableNavHost(
        navController = navController,
        startDestination = ROUTE_DASHBOARD,
    ) {
        composable(ROUTE_DASHBOARD) {
            DashboardScreen(
                model = dashboard,
                stale = stale,
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
                // Staleness matters more here than anywhere. Everything on this screen is a
                // decision about the whole machine, taken from a reading that may be dead.
                stale = stale,
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
                    stale = stale,
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
