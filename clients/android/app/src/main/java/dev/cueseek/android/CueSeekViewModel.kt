package dev.cueseek.android

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import androidx.lifecycle.ViewModelProvider.AndroidViewModelFactory.Companion.APPLICATION_KEY
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.ActionInvocationId
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.PairedHost
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/** What the single screen is showing. */
sealed interface Screen {
    data object Loading : Screen
    data object Unpaired : Screen
    data class Paired(val state: AgentState) : Screen
}

/** The pairing form's contents and the last thing that went wrong with it. */
data class PairingForm(
    val host: String = "",
    val port: String = AgentAddress.DEFAULT_PORT.toString(),
    val code: String = "",
    val deviceName: String = android.os.Build.MODEL ?: "Android",
    val busy: Boolean = false,
    val error: ApiError? = null,
)

class CueSeekViewModel(private val container: AppContainer) : ViewModel() {

    /**
     * The screen, as a **cold** flow.
     *
     * Nothing connects until something collects, and collection is scoped to the lifecycle
     * by the UI. That is what makes the stream a foreground affordance rather than a
     * background service pretending to be one.
     */
    @OptIn(ExperimentalCoroutinesApi::class)
    val screen: Flow<Screen> = container.hosts.selectedHost.flatMapLatest { host ->
        if (host == null) flowOf(Screen.Unpaired)
        else container.live.stateFor(host).map { Screen.Paired(it) }
    }

    var form by mutableStateOf(PairingForm())
        private set

    /** The invocation we are waiting on an outcome for, if any. */
    var pendingAction by mutableStateOf<ActionInvocationId?>(null)
        private set

    var lastActionError by mutableStateOf<ApiError?>(null)
        private set

    fun edit(update: PairingForm.() -> PairingForm) {
        form = form.update()
    }

    fun pair() {
        val port = form.port.toIntOrNull()
        if (form.host.isBlank() || port == null) {
            form = form.copy(error = ApiError.BadRequest("Enter a host and a numeric port."))
            return
        }

        form = form.copy(busy = true, error = null)
        viewModelScope.launch {
            val result = container.pairing.pair(
                address = AgentAddress(form.host.trim(), port),
                // Passed through as typed: the agent strips anything outside its alphabet
                // and uppercases before matching.
                code = form.code,
                deviceName = form.deviceName.ifBlank { "Android" },
            )
            form = when (result) {
                is ApiResult.Success -> PairingForm()
                is ApiResult.Failure -> form.copy(busy = false, error = result.error)
            }
        }
    }

    fun invoke(host: PairedHost, serviceId: String, actionId: String) {
        lastActionError = null
        viewModelScope.launch {
            when (val result = container.services.invokeAction(host, serviceId, actionId)) {
                // Keep the id: the outcome arrives only as a stream event carrying it, and
                // there is no endpoint to ask again.
                is ApiResult.Success -> pendingAction = result.value.actionId
                is ApiResult.Failure -> lastActionError = result.error
            }
        }
    }

    fun clearPendingAction() {
        pendingAction = null
    }

    fun forget(host: PairedHost) {
        viewModelScope.launch { container.hosts.forget(host.hostId) }
    }

    companion object {
        val Factory: ViewModelProvider.Factory = viewModelFactory {
            initializer {
                val app = this[APPLICATION_KEY] as CueSeekApplication
                CueSeekViewModel(app.container)
            }
        }
    }
}
