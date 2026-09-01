package dev.cueseek.android

import android.content.Context
import dev.cueseek.core.api.CueSeekApiFactory
import dev.cueseek.core.data.AgentClients
import dev.cueseek.core.data.AgentLiveState
import dev.cueseek.core.data.HostRepository
import dev.cueseek.core.data.PairingRepository
import dev.cueseek.core.data.ServicesRepository
import dev.cueseek.core.data.SettingsRepository

/**
 * Everything the app is built from, constructed once.
 *
 * Manual construction rather than Hilt (ADR-0013). Three screens and one data layer do not
 * need an annotation processor, and with plain constructors the swap stays cheap — the
 * moment it earns its keep is M5, when the Wear client assembles these same repositories
 * and the wiring exists twice.
 *
 * One [CueSeekApiFactory.AgentHttp] is shared by everything, so every host and, from P3,
 * the event stream reuse a single connection pool.
 */
class AppContainer(context: Context) {

    private val http = CueSeekApiFactory.sharedHttp()

    val hosts: HostRepository = HostRepository(context)

    /** Appearance and anything else that is about the app rather than a host. */
    val settings: SettingsRepository = SettingsRepository(context)

    private val clients = AgentClients(hosts, http)

    val pairing: PairingRepository = PairingRepository(hosts, http)

    val services: ServicesRepository = ServicesRepository(clients)

    /**
     * Live state, as a cold flow.
     *
     * Nothing here starts a connection. The UI layer scopes collection to the lifecycle,
     * which is what keeps the stream a foreground affordance: a held SSE connection does
     * not survive Doze, so nothing background-critical may depend on it (ADR-0004
     * Amendment 2).
     */
    val live: AgentLiveState = AgentLiveState(
        streams = clients::streamFor,
        // The manual-refresh path is the ordinary read path. Nothing about a pull deserves
        // its own request logic, and giving it one would be a second place for the
        // credential handling, the error mapping and the shape of a snapshot to drift.
        snapshots = services::snapshot,
        nudge = services::requestRefresh,
    )
}
