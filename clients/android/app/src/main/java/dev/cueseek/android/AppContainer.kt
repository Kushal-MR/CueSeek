package dev.cueseek.android

import android.content.Context
import dev.cueseek.core.api.CueSeekApiFactory
import dev.cueseek.core.data.AgentClients
import dev.cueseek.core.data.HostRepository
import dev.cueseek.core.data.PairingRepository
import dev.cueseek.core.data.ServicesRepository

/**
 * Everything the app is built from, constructed once.
 *
 * Manual construction rather than Hilt (ADR-0013). Three screens and one data layer do not
 * need an annotation processor, and with plain constructors the swap stays cheap — the
 * moment it earns its keep is M4, when the Wear client assembles these same repositories
 * and the wiring exists twice.
 *
 * One [CueSeekApiFactory.AgentHttp] is shared by everything, so every host and, from P3,
 * the event stream reuse a single connection pool.
 */
class AppContainer(context: Context) {

    private val http = CueSeekApiFactory.sharedHttp()

    val hosts: HostRepository = HostRepository(context)

    private val clients = AgentClients(hosts, http)

    val pairing: PairingRepository = PairingRepository(hosts, http)

    val services: ServicesRepository = ServicesRepository(clients)
}
