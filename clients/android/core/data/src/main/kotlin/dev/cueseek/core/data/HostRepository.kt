package dev.cueseek.core.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dev.cueseek.core.data.internal.HostRecords
import dev.cueseek.core.data.internal.TokenCipher
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.DeviceToken
import dev.cueseek.core.model.HostId
import dev.cueseek.core.model.PairedHost
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.cueSeekStore: DataStore<Preferences> by preferencesDataStore(
    name = HostRepository.STORE_NAME,
)

/**
 * Which agents this app is paired with, and their credentials.
 *
 * Keyed by [HostId] throughout (ADR-0008). The M2 interface shows one host, but nothing
 * here assumes that: adding a second is inserting a record. Retrofitting multi-host later
 * would be a navigation rewrite, and carrying it now is one field.
 *
 * Tokens are held under their own key per host rather than inside the host record, so that
 * editing an address does not rewrite a secret, and so a token can be dropped without
 * touching anything else.
 */
class HostRepository internal constructor(
    private val store: DataStore<Preferences>,
    private val cipher: TokenCipher,
) {

    constructor(context: Context) : this(context.applicationContext.cueSeekStore, TokenCipher())

    /**
     * Decrypted tokens, cached in memory for the process's lifetime.
     *
     * [dev.cueseek.core.api.TokenProvider] is consulted from an OkHttp interceptor on
     * OkHttp's thread, for every request, and cannot suspend. Decrypting there would mean
     * blocking on disk and on the Keystore inside the request path.
     */
    private val tokenCache = ConcurrentHashMap<String, DeviceToken>()

    val hosts: Flow<List<PairedHost>> =
        store.data.map { HostRecords.decode(it[KEY_HOSTS]) }.distinctUntilChanged()

    /**
     * The host the UI is currently showing, or `null` before pairing.
     *
     * Falls back to the first record when the selection names a host that no longer
     * exists, which is what happens between removing the selected host and choosing
     * another.
     */
    val selectedHost: Flow<PairedHost?> = store.data.map { prefs ->
        val all = HostRecords.decode(prefs[KEY_HOSTS])
        val selected = prefs[KEY_SELECTED_HOST]
        all.firstOrNull { it.hostId.value == selected } ?: all.firstOrNull()
    }
        // Load-bearing, not tidiness. DataStore re-emits on every write, and this flow
        // feeds a flatMapLatest that opens the event stream - so an unrelated write would
        // tear down a healthy connection and open a new one. That costs 3-4 seconds of
        // data (A7) and, worse, the fresh snapshot resets the freshness timer, which is
        // how a stale stream can be made to look live. Observed on a device before this
        // line existed.
        .distinctUntilChanged()

    suspend fun host(id: HostId): PairedHost? = hosts.first().firstOrNull { it.hostId == id }

    /**
     * Records a pairing, replacing any existing record for the same host.
     *
     * Re-pairing an already-paired host is a normal thing to do — it is how a device
     * recovers a token it can no longer decrypt — so this is an upsert rather than an
     * insert that can collide.
     */
    suspend fun save(host: PairedHost, token: DeviceToken) {
        val sealed = cipher.seal(token.value)
        store.edit { prefs ->
            val existing = HostRecords.decode(prefs[KEY_HOSTS])
            val updated = existing.filterNot { it.hostId == host.hostId } + host
            prefs[KEY_HOSTS] = HostRecords.encode(updated)
            prefs[tokenKey(host.hostId)] = sealed
            prefs[KEY_SELECTED_HOST] = host.hostId.value
        }
        tokenCache[host.hostId.value] = token
    }

    /** Updates where a host is reachable, leaving its identity and credential alone. */
    suspend fun updateAddress(id: HostId, address: AgentAddress) {
        store.edit { prefs ->
            val existing = HostRecords.decode(prefs[KEY_HOSTS])
            if (existing.none { it.hostId == id }) return@edit
            prefs[KEY_HOSTS] = HostRecords.encode(
                existing.map { if (it.hostId == id) it.copy(address = address) else it }
            )
        }
    }

    suspend fun select(id: HostId) {
        store.edit { it[KEY_SELECTED_HOST] = id.value }
    }

    /**
     * Forgets a host locally. This is what "log out" does.
     *
     * It does **not** revoke the device on the agent. Revocation needs the
     * `devices.manage` scope, and the CLI's default grant of `read,service.control` does
     * not include it — so a typical device cannot revoke anything, including itself
     * (`docs/m2-android-api.md` §5). Callers must say so plainly rather than implying the
     * server forgot anything.
     */
    suspend fun forget(id: HostId) {
        store.edit { prefs ->
            val remaining = HostRecords.decode(prefs[KEY_HOSTS]).filterNot { it.hostId == id }
            prefs[KEY_HOSTS] = HostRecords.encode(remaining)
            prefs.remove(tokenKey(id))
            if (prefs[KEY_SELECTED_HOST] == id.value) {
                val next = remaining.firstOrNull()
                if (next == null) prefs.remove(KEY_SELECTED_HOST)
                else prefs[KEY_SELECTED_HOST] = next.hostId.value
            }
        }
        tokenCache.remove(id.value)
    }

    /**
     * The token for a host, or `null` if there is none or it can no longer be decrypted.
     *
     * An undecryptable token is not an error to report but a state to act on: the record
     * is dropped, so the app presents itself as unpaired rather than as paired-but-broken
     * and failing every request with a 401 it cannot explain.
     */
    suspend fun token(id: HostId): DeviceToken? {
        tokenCache[id.value]?.let { return it }

        val sealed = store.data.first()[tokenKey(id)] ?: return null
        val plaintext = cipher.unseal(sealed)
        if (plaintext == null) {
            forget(id)
            return null
        }
        return DeviceToken(plaintext).also { tokenCache[id.value] = it }
    }

    /** Non-suspending read for the interceptor path. Null until [token] has warmed it. */
    internal fun cachedToken(id: HostId): DeviceToken? = tokenCache[id.value]

    companion object {
        internal const val STORE_NAME = "cueseek"

        private val KEY_HOSTS = stringPreferencesKey("hosts")
        private val KEY_SELECTED_HOST = stringPreferencesKey("selected_host")

        private fun tokenKey(id: HostId) = stringPreferencesKey("token.${id.value}")
    }
}
