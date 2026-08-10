package dev.cueseek.android.ui

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.ViewModelProvider.AndroidViewModelFactory.Companion.APPLICATION_KEY
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import dev.cueseek.android.AppContainer
import dev.cueseek.android.CueSeekApplication
import dev.cueseek.core.model.ActionInvocationId
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Service
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/** The API version this build was written against (`docs/m2-android-api.md`). */
const val SUPPORTED_API_VERSION: String = "0.1.0"

sealed interface Screen {
    data object Loading : Screen
    data object Unpaired : Screen
    data class Paired(val state: AgentState) : Screen
}

data class PairingForm(
    val host: String = "",
    val port: String = AgentAddress.DEFAULT_PORT.toString(),
    val code: String = "",
    val deviceName: String = android.os.Build.MODEL ?: "Android",
    val busy: Boolean = false,
    val error: ApiError? = null,
)

/** An invocation we are holding open, waiting for the stream to say how it went. */
data class PendingAction(
    val id: ActionInvocationId,
    val serviceId: String,
    val label: String,
)

class CueSeekViewModel(private val container: AppContainer) : ViewModel() {

    /**
     * The screen, as a **cold** flow.
     *
     * Nothing connects until something collects, and the UI scopes collection to the
     * lifecycle. That is what keeps the stream a foreground affordance rather than a
     * background service in disguise (ADR-0004 Amendment 2).
     */
    @OptIn(ExperimentalCoroutinesApi::class)
    val screen: Flow<Screen> = container.hosts.selectedHost.flatMapLatest { host ->
        if (host == null) flowOf(Screen.Unpaired)
        else container.live.stateFor(host).map { Screen.Paired(it) }
    }

    var form by mutableStateOf(PairingForm())
        private set

    var pending by mutableStateOf<PendingAction?>(null)
        private set

    var actionError by mutableStateOf<ApiError?>(null)
        private set

    /** The service whose sheet is open, by id. Held by id so a stream update refreshes it. */
    var openServiceId by mutableStateOf<String?>(null)
        private set

    fun edit(update: PairingForm.() -> PairingForm) {
        form = form.update()
    }

    fun pair() {
        val port = form.port.trim().toIntOrNull()
        if (form.host.isBlank()) {
            form = form.copy(error = ApiError.BadRequest("Enter the agent's address."))
            return
        }
        if (port == null || port !in 1..65535) {
            form = form.copy(error = ApiError.BadRequest("Enter a port between 1 and 65535."))
            return
        }

        form = form.copy(busy = true, error = null)
        viewModelScope.launch {
            val result = container.pairing.pair(
                address = AgentAddress(form.host.trim(), port),
                // Passed through exactly as typed: the agent strips anything outside its
                // alphabet and uppercases before matching.
                code = form.code,
                deviceName = form.deviceName.ifBlank { "Android" },
            )
            form = when (result) {
                is ApiResult.Success -> PairingForm()
                is ApiResult.Failure -> form.copy(busy = false, error = result.error)
            }
        }
    }

    fun openService(service: Service) {
        actionError = null
        openServiceId = service.id
    }

    fun closeService() {
        openServiceId = null
    }

    fun invoke(host: PairedHost, serviceId: String, actionId: String, label: String) {
        actionError = null
        viewModelScope.launch {
            when (val result = container.services.invokeAction(host, serviceId, actionId)) {
                is ApiResult.Success -> {
                    // The 202 is an acknowledgement, not an outcome. Keep the id: the
                    // result arrives only as a stream event carrying it, and there is no
                    // endpoint to ask again.
                    pending = PendingAction(result.value.actionId, serviceId, label)
                    openServiceId = null
                }

                is ApiResult.Failure -> actionError = result.error
            }
        }
    }

    fun dismissPending() {
        pending = null
    }

    fun dismissActionError() {
        actionError = null
    }

    fun forget(host: PairedHost) {
        viewModelScope.launch {
            container.hosts.forget(host.hostId)
            pending = null
            openServiceId = null
        }
    }

    companion object {
        val Factory: ViewModelProvider.Factory = viewModelFactory {
            initializer {
                CueSeekViewModel((this[APPLICATION_KEY] as CueSeekApplication).container)
            }
        }
    }
}
