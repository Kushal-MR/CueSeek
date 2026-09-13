package dev.cueseek.wear.dashboard

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import dev.cueseek.core.api.CueSeekApiFactory
import dev.cueseek.core.data.AgentClients
import dev.cueseek.core.data.HostRepository
import dev.cueseek.core.data.ServicesRepository
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.Tally
import dev.cueseek.core.model.verdict
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import java.time.Duration
import java.time.Instant

sealed interface DashboardUi {
    data object Loading : DashboardUi

    /** Paired to nothing. The app should be showing pairing, not this. */
    data object Unpaired : DashboardUi

    data class Loaded(
        val hostname: String,
        val verdict: String,
        val tally: Tally,
        val services: List<Service>,
        val metrics: HostMetrics?,
        val observedAt: Instant,
        /** True once the reading is old enough that it should not be presented as fact. */
        val stale: Boolean,
    ) : DashboardUi

    data class Failed(val message: String) : DashboardUi
}

/**
 * The watch's dashboard, polled.
 *
 * # Polling, not streaming
 *
 * ADR-0004 Amendment 2: nothing background-critical may depend on SSE, and a watch radio is
 * the case that rule was written for. `ServicesRepository.snapshot()` fetches system,
 * services, metrics and actions in parallel in one round trip — which is exactly the shape a
 * glanceable surface wants and exactly what the phone's held stream is not.
 *
 * There is no timer here either. [refresh] is called when the screen appears, and that is
 * the whole schedule: a watch screen is on for a few seconds at a time, and a poll loop
 * running behind a dark panel would spend battery producing readings nobody sees. M5.10
 * revisits this with ambient behaviour measured rather than assumed.
 *
 * # Staleness is computed from a clock, never from the connection
 *
 * The phone's rule, inherited deliberately: a client that trusted "connected" to mean
 * "current" would show confident green while the agent was unreachable. Here the reading
 * carries the instant it was taken, and anything older than [STALE_AFTER] stops being
 * presented as fact — including while a refresh is in flight.
 */
class DashboardViewModel(app: Application) : AndroidViewModel(app) {

    private val hosts = HostRepository(app)
    private val services = ServicesRepository(
        AgentClients(hosts, CueSeekApiFactory.sharedHttp()),
    )

    private val _ui = MutableStateFlow<DashboardUi>(DashboardUi.Loading)
    val ui: StateFlow<DashboardUi> = _ui.asStateFlow()

    fun refresh() {
        viewModelScope.launch {
            val host: PairedHost? = hosts.selectedHost.first()
            if (host == null) {
                _ui.value = DashboardUi.Unpaired
                return@launch
            }

            when (val result = services.snapshot(host)) {
                is ApiResult.Failure -> {
                    // A failed poll does not erase a good reading. It goes stale on its own
                    // clock instead, which is the honest outcome: "this is what I last saw,
                    // and I could not confirm it" beats both a blank screen and a green one.
                    if (_ui.value !is DashboardUi.Loaded) {
                        _ui.value = DashboardUi.Failed(shortMessage(result.error))
                    }
                }

                is ApiResult.Success -> {
                    val snapshot = result.value
                    val tally = Tally.of(snapshot.services)
                    _ui.value = DashboardUi.Loaded(
                        hostname = snapshot.system.hostname,
                        verdict = verdict(
                            stale = false,
                            services = snapshot.services,
                            hostMetrics = snapshot.hostMetrics,
                            tally = tally,
                        ),
                        tally = tally,
                        services = snapshot.services,
                        metrics = snapshot.hostMetrics,
                        observedAt = Instant.now(),
                        stale = false,
                    )
                }
            }
        }
    }

    companion object {
        /**
         * Three times the agent's default poll interval, matching the phone.
         *
         * Not a number chosen here: the agent polls each service every 30s by default, so a
         * client that called anything older than one interval stale would flag a reading
         * that was simply the most recent one that exists.
         */
        val STALE_AFTER: Duration = Duration.ofSeconds(90)

        fun isStale(observedAt: Instant, now: Instant = Instant.now()): Boolean =
            Duration.between(observedAt, now) > STALE_AFTER

        private fun shortMessage(error: dev.cueseek.core.model.ApiError): String = when (error) {
            is dev.cueseek.core.model.ApiError.Transport -> "Could not reach the agent"
            is dev.cueseek.core.model.ApiError.Unauthorized -> "The agent refused this device"
            is dev.cueseek.core.model.ApiError.InsufficientScope -> "Not allowed with these scopes"
            else -> "Could not load"
        }
    }
}
