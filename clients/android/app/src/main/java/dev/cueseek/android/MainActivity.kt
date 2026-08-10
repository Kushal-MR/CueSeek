package dev.cueseek.android

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.cueseek.core.design.CueSeekTheme
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.Freshness
import dev.cueseek.core.model.StreamStatus
import java.time.Duration
import java.time.Instant

/**
 * A deliberately plain integration surface.
 *
 * This is not the app. It exists so P3's stream and freshness machinery can be exercised
 * against a real agent on a real device before any design work happens. The status
 * language, the capability registry and the risk-gated confirmation are P4 and P5; nothing
 * here should survive them.
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            CueSeekTheme {
                CueSeekApp()
            }
        }
    }
}

@Composable
private fun CueSeekApp(
    viewModel: CueSeekViewModel = viewModel(factory = CueSeekViewModel.Factory),
) {
    // collectAsStateWithLifecycle is repeatOnLifecycle: collection starts at STARTED and
    // stops at STOPPED, so backgrounding the app tears the stream down rather than holding
    // a connection that Doze would silently freeze anyway.
    val screen by viewModel.screen.collectAsStateWithLifecycle(initialValue = Screen.Loading)

    Scaffold(modifier = Modifier.fillMaxSize()) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            when (val current = screen) {
                Screen.Loading -> CircularProgressIndicator()
                Screen.Unpaired -> PairingForm(viewModel)
                is Screen.Paired -> AgentReadout(current.state, viewModel)
            }
        }
    }
}

@Composable
private fun PairingForm(viewModel: CueSeekViewModel) {
    val form = viewModel.form

    Text("Pair with an agent", style = MaterialTheme.typography.titleLarge)
    Text(
        "Run `cueseekd pair` on the host and enter the code it prints.",
        style = MaterialTheme.typography.bodyMedium,
    )

    OutlinedTextField(
        value = form.host,
        onValueChange = { v -> viewModel.edit { copy(host = v) } },
        label = { Text("Host (tailnet address)") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = form.port,
        onValueChange = { v -> viewModel.edit { copy(port = v) } },
        label = { Text("Port") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = form.code,
        onValueChange = { v -> viewModel.edit { copy(code = v) } },
        label = { Text("Pairing code") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )
    OutlinedTextField(
        value = form.deviceName,
        onValueChange = { v -> viewModel.edit { copy(deviceName = v) } },
        label = { Text("Device name") },
        singleLine = true,
        modifier = Modifier.fillMaxWidth(),
    )

    Button(onClick = viewModel::pair, enabled = !form.busy) {
        Text(if (form.busy) "Pairing…" else "Pair")
    }

    form.error?.let { error ->
        // Shown raw on purpose. This surface exists to diagnose, and the error model is
        // what P5 will render properly.
        Text("Failed: ${error::class.simpleName}", style = MaterialTheme.typography.titleSmall)
        error.detail?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
    }
}

@Composable
private fun AgentReadout(state: AgentState, viewModel: CueSeekViewModel) {
    Text(state.host.hostname, style = MaterialTheme.typography.titleLarge)
    Text("${state.host.address}  ·  ${state.host.hostId.value.take(8)}…")

    Text(
        when (val status = state.status) {
            StreamStatus.Idle -> "stream: idle"
            StreamStatus.Connecting -> "stream: connecting"
            is StreamStatus.Open -> "stream: open"
            is StreamStatus.Retrying -> "stream: retrying (attempt ${status.attempt})"
            is StreamStatus.Stopped -> "stream: stopped — ${status.error::class.simpleName}"
        }
    )

    // The point of the whole phase: what the transport claims and what the clock says are
    // reported separately, and the clock wins.
    Text(
        when (val freshness = state.freshness) {
            Freshness.Fresh -> "data: fresh"
            is Freshness.Stale -> "data: STALE" + (freshness.lastEventAt?.let {
                " — nothing for ${Duration.between(it, Instant.now()).seconds}s"
            } ?: " — nothing received yet")
        },
        style = MaterialTheme.typography.titleMedium,
    )

    state.system?.let { Text("agent ${it.agentVersion}, api ${it.apiVersion}") }

    state.services.forEach { service ->
        Card(modifier = Modifier.fillMaxWidth()) {
            Column(
                modifier = Modifier.padding(12.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                Text(service.name, style = MaterialTheme.typography.titleMedium)
                Text("status: ${service.health.status}  ·  reachable: ${service.health.reachable}")
                Text("observed: ${service.health.observedAt}")
                service.health.reasons.forEach { Text("· ${it.code}: ${it.message}") }
                Text("capabilities: " + service.capabilities.joinToString { "${it.id} (${it.label})" })

                service.actions.forEach { action ->
                    OutlinedButton(
                        onClick = { viewModel.invoke(state.host, service.id, action.id) },
                        enabled = state.host.canControlServices(),
                    ) {
                        Text("${action.label} [${action.risk}]")
                    }
                }
            }
        }
    }

    viewModel.pendingAction?.let { id ->
        val outcome = state.outcomeOf(id)
        if (outcome == null) {
            Text("action ${id.value.take(8)}… accepted, awaiting outcome over the stream")
        } else {
            Text("action ${id.value.take(8)}… → ${outcome.status}" + (outcome.error?.let { ": $it" } ?: ""))
            OutlinedButton(onClick = viewModel::clearPendingAction) { Text("Clear") }
        }
    }

    viewModel.lastActionError?.let { error ->
        Text("action failed: ${error::class.simpleName}")
        error.detail?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
    }

    OutlinedButton(onClick = { viewModel.forget(state.host) }) {
        Text("Forget this host (local only)")
    }
    Text(
        "Forgetting clears this device's credentials. It does not revoke the device on the " +
            "agent — that needs the devices.manage scope, which the CLI does not grant by default.",
        style = MaterialTheme.typography.bodySmall,
    )
}
