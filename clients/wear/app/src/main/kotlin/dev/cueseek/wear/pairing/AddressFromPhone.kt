package dev.cueseek.wear.pairing

import android.content.Context
import android.util.Log
import com.google.android.gms.wearable.Wearable
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.AgentHandoff
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume

/**
 * Reads the agent address a paired phone published, if it published one.
 *
 * The watch half of [ADR-0014]. It receives **an address and nothing else** — and
 * [AgentHandoff.decode] refuses a payload carrying a pairing code, so that is enforced on
 * this side too rather than trusted from the other.
 *
 * # Everything here can legitimately be absent
 *
 * No paired phone, no CueSeek on the phone, no host selected there yet, Play services
 * missing, the data item not synced yet. Every one of those returns null, and none of them
 * is an error worth showing — the fallback is a manual address field, which always works
 * and is the reason the watch can still call itself standalone.
 */
class AddressFromPhone(context: Context) {

    private val dataClient = Wearable.getDataClient(context.applicationContext)

    /**
     * The most recently published address, or null.
     *
     * Reads the current data item rather than listening for changes. A one-shot read is
     * what the pairing screen actually needs — the address is looked up once, when
     * somebody opens the screen — and a listener would keep a callback alive across a flow
     * that exists for about thirty seconds.
     */
    suspend fun read(): AgentAddress? = suspendCancellableCoroutine { cont ->
        dataClient.dataItems
            .addOnSuccessListener { buffer ->
                val uri = try {
                    buffer.firstOrNull { it.uri.path == AgentHandoff.PATH }
                        ?.let { item ->
                            com.google.android.gms.wearable.DataMapItem.fromDataItem(item)
                                .dataMap
                                .getString(AgentHandoff.KEY_URI)
                        }
                } finally {
                    // The buffer holds native memory and is not garbage collected. Leaking
                    // it is the classic Data Layer mistake and it is silent.
                    buffer.release()
                }

                val address = uri?.let(AgentHandoff::decode)
                if (uri != null && address == null) {
                    // Not merely absent: something published a payload this refuses. Worth
                    // a log line, because the most interesting reason is a code in it.
                    Log.w(TAG, "a published handoff was rejected by the parser")
                }
                cont.resume(address)
            }
            .addOnFailureListener {
                Log.d(TAG, "no address from a phone: ${it.message}")
                cont.resume(null)
            }
    }

    private companion object {
        const val TAG = "CueSeekWearHandoff"
    }
}
