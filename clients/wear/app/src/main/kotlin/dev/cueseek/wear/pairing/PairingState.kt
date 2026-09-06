package dev.cueseek.wear.pairing

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import dev.cueseek.core.data.HostRepository
import dev.cueseek.core.data.PairingRepository
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.Platform
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Where the address the watch is about to pair against came from. */
enum class AddressSource {
    /** Handed over by a paired phone (ADR-0014). Nobody typed it here. */
    Phone,

    /** Typed on the watch. The fallback that always works, and is never the default. */
    Manual,
}

sealed interface PairingUi {
    /** Looking for an address from the phone. */
    data object Preparing : PairingUi

    /** Ready for a code. [address] is null when the phone gave nothing. */
    data class Ready(val address: AgentAddress?, val source: AddressSource) : PairingUi

    data class Working(val address: AgentAddress) : PairingUi

    data class Paired(val hostName: String) : PairingUi

    /**
     * [message] is shown verbatim.
     *
     * Never reworded into a claim about *why* a code was rejected: the agent merges
     * unknown, expired and already-redeemed on purpose, and a client that guessed between
     * them would leak what the agent declined to say (ADR-0006).
     */
    data class Failed(val address: AgentAddress?, val message: String) : PairingUi
}

/**
 * The watch's half of pairing.
 *
 * Reuses `:core:data`'s [PairingRepository] rather than reimplementing it. That module was
 * not one ADR-0013 listed as shared, and it turned out to carry nothing phone-specific —
 * which matters most for the part nobody should write twice: the token is sealed by the
 * same `TokenCipher`, against the same Android Keystore, with the same "does not survive a
 * restore" property.
 *
 * The token this mints belongs to the watch alone. The phone supplied an address and has
 * never seen a credential (ADR-0014).
 */
class PairingViewModel(app: Application) : AndroidViewModel(app) {

    private val hosts = HostRepository(app)
    private val pairing = PairingRepository(hosts)
    private val fromPhone = AddressFromPhone(app)

    private val _ui = MutableStateFlow<PairingUi>(PairingUi.Preparing)
    val ui: StateFlow<PairingUi> = _ui.asStateFlow()

    init {
        viewModelScope.launch {
            val address = fromPhone.read()
            _ui.value = PairingUi.Ready(
                address = address,
                source = if (address == null) AddressSource.Manual else AddressSource.Phone,
            )
        }
    }

    /** The fallback path: somebody typed an address on the watch. */
    fun addressEntered(raw: String) {
        val parsed = parseAddress(raw)
        _ui.value = PairingUi.Ready(address = parsed, source = AddressSource.Manual)
    }

    fun pair(address: AgentAddress, code: String, deviceName: String) {
        _ui.value = PairingUi.Working(address)
        viewModelScope.launch {
            // Platform.WearOs, not Android. The device list and the audit log exist to say
            // which device did something; a watch calling itself a phone would make both
            // of them lie.
            val result = pairing.pair(
                address = address,
                code = code.trim(),
                deviceName = deviceName,
                platform = Platform.WearOs,
            )
            _ui.value = when (result) {
                is ApiResult.Success -> PairingUi.Paired(result.value.hostname)
                is ApiResult.Failure -> PairingUi.Failed(address, shortMessage(result.error))
            }
        }
    }

    companion object {

        /**
         * A watch-sized sentence for a failure.
         *
         * Deliberately not `:app`'s `ErrorCopy`. That lives in the phone module and is
         * shaped for it — a title, a body and the agent's own words, three lines that do
         * not fit on 233dp. ADR-0007's position is that form factors render the same thing
         * differently, and copy is presentation.
         *
         * What is **not** presentation, and holds identically here: an invalid code is
         * never reworded into a claim about *why*. The agent merges unknown, expired and
         * already-redeemed on purpose, and a client that guessed between them would leak
         * exactly what the agent declined to say (ADR-0006).
         */
        fun shortMessage(error: ApiError): String = when (error) {
            is ApiError.InvalidPairingCode -> "Code not accepted"
            is ApiError.RateLimited -> "Too many attempts"
            is ApiError.Transport -> "Could not reach the agent"
            is ApiError.Unauthorized -> "The agent refused this device"
            is ApiError.InsufficientScope -> "Not allowed with these scopes"
            is ApiError.NotFound -> "The agent does not have that"
            else -> "Pairing failed"
        }

        /**
         * `host:port`, or `host` with the default port.
         *
         * Deliberately not the handoff URI: this is what a person types, and asking
         * somebody to type `cueseek://pair?host=` on a watch would be worse than the
         * problem ADR-0014 set out to solve.
         */
        fun parseAddress(raw: String): AgentAddress? {
            val text = raw.trim()
            if (text.isEmpty()) return null
            val host = text.substringBefore(':').trim()
            if (host.isEmpty()) return null
            val port = if (':' in text) {
                text.substringAfter(':').trim().toIntOrNull() ?: return null
            } else {
                AgentAddress.DEFAULT_PORT
            }
            if (port !in 1..65535) return null
            return AgentAddress(host, port)
        }
    }
}
