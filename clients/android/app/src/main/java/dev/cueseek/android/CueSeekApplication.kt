package dev.cueseek.android

import android.app.Application
import dev.cueseek.android.wear.AgentAddressPublisher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob

/**
 * Owns the [AppContainer] for the process's lifetime.
 *
 * The container holds a connection pool and a decrypted-token cache, both of which are
 * meant to outlive any one screen and neither of which should be rebuilt on rotation.
 */
class CueSeekApplication : Application() {

    lateinit var container: AppContainer
        private set

    /**
     * Process-lifetime scope, for work that belongs to the app rather than to a screen.
     *
     * Only the watch handoff uses it. A `SupervisorJob` so a failure there cannot cancel
     * anything else that later shares it.
     */
    private val appScope = CoroutineScope(SupervisorJob())

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)

        // Mirrors the selected agent's address to a paired watch, if there is one. Sends
        // an address and never a credential (ADR-0014). Silent when there is no watch,
        // which is the ordinary case.
        AgentAddressPublisher(this, container.hosts).start(appScope)
    }
}
