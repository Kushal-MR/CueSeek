package dev.cueseek.android.ui.dashboard

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Snackbar
import androidx.compose.material3.SnackbarDuration
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.cueseek.android.ui.CueSeekViewModel
import dev.cueseek.android.ui.SUPPORTED_API_VERSION
import dev.cueseek.android.ui.explain
import dev.cueseek.android.ui.RowDestination
import dev.cueseek.android.ui.openWebUi
import dev.cueseek.android.ui.rowDestination
import dev.cueseek.core.model.ActionStatus
import dev.cueseek.core.model.AgentState
import kotlinx.coroutines.delay
import java.time.Instant

/**
 * The dashboard.
 *
 * Summary, then roster, then everything else on demand. The screen answers "is everything
 * fine?" before it offers anything to do, because that is the question it exists for.
 */
@Composable
fun DashboardScreen(
    state: AgentState,
    viewModel: CueSeekViewModel,
    modifier: Modifier = Modifier,
) {
    // A ticking clock, so ages count up while nothing arrives. Without it the screen would
    // silently freeze its own timestamps and look fresher than it is.
    var now by remember { mutableStateOf(Instant.now()) }
    LaunchedEffect(Unit) {
        while (true) {
            now = Instant.now()
            delay(1_000)
        }
    }

    val theme by viewModel.theme.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val snackbars = remember { SnackbarHostState() }
    val stale = state.freshness.isStale
    val open = viewModel.openServiceId?.let { id -> state.services.firstOrNull { it.id == id } }

    Column(modifier = modifier.fillMaxSize()) {
        Column(
            modifier = Modifier
                .weight(1f)
                .verticalScroll(rememberScrollState()),
        ) {
            SummaryHeader(
                state = state,
                now = now,
                theme = theme,
                onThemeChange = viewModel::setTheme,
                onForgetRequested = viewModel::askToForget,
            )

            SkewBanner(state)

            Spacer(Modifier.height(20.dp))

            ServiceRoster(
                services = state.services,
                host = state.host,
                stale = stale,
                now = now,
                // The row's two destinations, decided here rather than inside the roster:
                // composing the URL needs the paired address, and launching it needs an
                // Android context, neither of which a presentational list should hold.
                //
                // Falling back to the sheet — rather than to nothing — is the point. A
                // service with no configured interface, or a phone with nothing that can
                // open one, still answers the tap with something worth reading.
                onServiceClick = { service ->
                    when (val to = rowDestination(service, state.host.address)) {
                        // Even a resolved URL can fail to launch on a device with nothing
                        // that opens links, so the fallback covers that too.
                        is RowDestination.WebUi ->
                            if (!openWebUi(context, to.url)) viewModel.openService(service)

                        RowDestination.Details -> viewModel.openService(service)
                    }
                },
                onInvoke = { service, action ->
                    viewModel.invoke(state.host, service.id, action.id, action.label)
                },
            )

            Spacer(Modifier.height(24.dp))
        }

        SnackbarHost(snackbars)
    }

    open?.let { service ->
        ServiceSheet(
            service = service,
            stale = stale,
            onDismiss = viewModel::closeService,
        )
    }

    // The outcome of an action arrives only over the stream, so this waits for it rather
    // than claiming success at the 202.
    viewModel.pending?.let { pending ->
        val outcome = state.outcomeOf(pending.id)
        LaunchedEffect(pending.id, outcome?.status) {
            when (outcome?.status) {
                null -> snackbars.showSnackbar(
                    "${pending.label} accepted — waiting for the agent",
                    duration = SnackbarDuration.Indefinite,
                )

                ActionStatus.Succeeded -> {
                    snackbars.currentSnackbarData?.dismiss()
                    snackbars.showSnackbar("${pending.label} finished")
                    viewModel.dismissPending()
                }

                ActionStatus.Failed -> {
                    snackbars.currentSnackbarData?.dismiss()
                    snackbars.showSnackbar(outcome.error ?: "${pending.label} failed")
                    viewModel.dismissPending()
                }

                else -> Unit
            }
        }
    }

    if (viewModel.confirmingForget) {
        AlertDialog(
            onDismissRequest = viewModel::cancelForget,
            title = { Text("Forget ${state.host.hostname}?") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(
                        "This phone will lose its credentials and you will need a new " +
                            "pairing code to connect again.",
                        style = MaterialTheme.typography.bodyMedium,
                    )
                    // Said plainly because it is easy to assume otherwise: revoking the
                    // device on the agent needs the devices.manage scope, which the CLI
                    // does not grant by default, so this device almost certainly cannot.
                    Text(
                        "The agent will not be told. Its record of this device stays until " +
                            "you remove it there.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            },
            confirmButton = {
                TextButton(
                    onClick = { viewModel.forget(state.host) },
                    colors = ButtonDefaults.textButtonColors(
                        contentColor = MaterialTheme.colorScheme.error,
                    ),
                ) { Text("Forget") }
            },
            dismissButton = {
                TextButton(onClick = viewModel::cancelForget) { Text("Cancel") }
            },
        )
    }

    viewModel.actionError?.let { error ->
        val copy = error.explain()
        AlertDialog(
            onDismissRequest = viewModel::dismissActionError,
            title = { Text(copy.title) },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(copy.body, style = MaterialTheme.typography.bodyMedium)
                    // Shown verbatim: for action-unavailable the agent's own words name the
                    // polkit rule or missing unit, which is more use than our paraphrase.
                    copy.detail?.let {
                        Text(
                            it,
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            },
            confirmButton = {
                TextButton(onClick = viewModel::dismissActionError) { Text("Close") }
            },
        )
    }
}

/**
 * Says which side is behind, rather than mis-rendering quietly (ADR-0007).
 */
@Composable
private fun SkewBanner(state: AgentState) {
    val system = state.system
    val skewed = system != null && state.apiVersionSkew(SUPPORTED_API_VERSION)

    AnimatedVisibility(
        visible = skewed,
        enter = fadeIn() + expandVertically(),
        exit = fadeOut() + shrinkVertically(),
    ) {
        val agentVersion = system?.apiVersion ?: return@AnimatedVisibility
        val appIsBehind = agentVersion > SUPPORTED_API_VERSION

        OutlinedCard(
            modifier = Modifier
                .fillMaxWidth()
                .padding(start = 16.dp, end = 16.dp, top = 16.dp),
        ) {
            Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    if (appIsBehind) "CueSeek is out of date" else "The agent is out of date",
                    style = MaterialTheme.typography.titleSmall,
                )
                Text(
                    if (appIsBehind) {
                        "The agent speaks $agentVersion; this app was built for " +
                            "$SUPPORTED_API_VERSION. Some things may not appear."
                    } else {
                        "This app expects $SUPPORTED_API_VERSION; the agent speaks " +
                            "$agentVersion. Updating the agent is the fix."
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}
