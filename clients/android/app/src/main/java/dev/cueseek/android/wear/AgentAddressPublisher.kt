package dev.cueseek.android.wear

import android.content.Context
import android.util.Log
import com.google.android.gms.wearable.PutDataMapRequest
import com.google.android.gms.wearable.Wearable
import dev.cueseek.core.data.HostRepository
import dev.cueseek.core.model.AgentHandoff
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/**
 * Publishes the selected agent's address where a paired watch can read it.
 *
 * The phone half of [ADR-0014]. It sends **an address and nothing else** — never a token,
 * never a pairing code. The watch redeems its own code against the agent and holds a
 * credential this device has never seen, which is what keeps per-device revocation and the
 * audit log meaningful (ADR-0006).
 *
 * # Why a data item and not a message
 *
 * `DataClient` items persist and sync; `MessageClient` requires both devices awake at the
 * same instant. The watch reads this whenever the user opens it, which may be hours after
 * the phone last changed hosts — so the address has to be sitting there already.
 *
 * # Why this is fire-and-forget
 *
 * There may be no watch, no Play services, or no paired device at all, and every one of
 * those is the ordinary case rather than an error. Publishing failure must never surface on
 * the phone: the phone user did not ask for this and cannot act on it. It is logged at
 * debug and dropped.
 */
class AgentAddressPublisher(
    context: Context,
    private val hosts: HostRepository,
) {
    private val dataClient = Wearable.getDataClient(context.applicationContext)

    /**
     * Mirrors the selected host's address for as long as [scope] lives.
     *
     * `distinctUntilChanged` matters more than it looks: `selectedHost` re-emits on every
     * unrelated change to the record — a rename, a last-seen bump — and each emission
     * would otherwise be a Bluetooth write. A watch is the wrong device to spend radio on
     * re-sending a value that did not change.
     */
    fun start(scope: CoroutineScope) {
        scope.launch {
            hosts.selectedHost
                .map { it?.address }
                .distinctUntilChanged()
                .collect { address ->
                    if (address == null) return@collect

                    val request = PutDataMapRequest.create(AgentHandoff.PATH).apply {
                        dataMap.putString(AgentHandoff.KEY_URI, AgentHandoff.encode(address))
                    }

                    // Not awaited, and therefore no dependency on
                    // kotlinx-coroutines-play-services. Nothing downstream needs the
                    // result: there is no retry that would help and no user to tell.
                    dataClient.putDataItem(request.asPutDataRequest().setUrgent())
                        .addOnSuccessListener { Log.d(TAG, "address published for the watch") }
                        // No watch, no Play services, no paired device: all ordinary.
                        .addOnFailureListener { Log.d(TAG, "address not published: ${it.message}") }
                }
        }
    }

    private companion object {
        const val TAG = "CueSeekWearHandoff"
    }
}
