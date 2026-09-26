package dev.cueseek.wear.dashboard

import android.app.Application
import android.content.ComponentName
import androidx.wear.watchface.complications.datasource.ComplicationDataSourceUpdateRequester
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import dev.cueseek.core.api.CueSeekApiFactory
import dev.cueseek.core.data.AgentClients
import dev.cueseek.core.data.HostRepository
import dev.cueseek.core.data.ServicesRepository
import dev.cueseek.core.model.Action
import dev.cueseek.core.model.ActionStatus
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Scope
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.Tally
import dev.cueseek.core.model.verdict
import dev.cueseek.wear.complication.CueSeekComplicationService
import dev.cueseek.wear.tile.CueSeekTileService
import dev.cueseek.wear.tile.LastReading
import dev.cueseek.wear.tile.LastReadingStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import java.time.Duration
import java.time.Instant

/** What an invoked action is doing, as far as a polling client can honestly say. */
sealed interface ActionUi {
    data object Idle : ActionUi
    data class Working(val label: String) : ActionUi

    /** The agent took the request. Not the same as it having finished — see [DashboardViewModel.invoke]. */
    data class Accepted(val label: String) : ActionUi
    data class Failed(val label: String, val message: String) : ActionUi
}

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
        /**
         * What the agent offers for the machine itself.
         *
         * Carried since M5.4 in the snapshot and dropped here until M5.7, which is the
         * accident that phase turned into a decision. The agent returns these to every
         * caller holding `read` on purpose, so their presence says nothing about whether
         * this device may use them — [scopes] does. See `powerAccess`.
         */
        val hostActions: List<Action>,
        /** This device's grants, as issued at pairing. User experience only; the agent enforces. */
        val scopes: Set<Scope>,
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

    private val _action = MutableStateFlow<ActionUi>(ActionUi.Idle)
    val action: StateFlow<ActionUi> = _action.asStateFlow()

    fun clearAction() { _action.value = ActionUi.Idle }

    /**
     * Ask the agent to do something to a service.
     *
     * # Why this says "asked", not "done"
     *
     * The agent answers an invocation with an *acceptance* — the job is enqueued, not
     * finished — and the terminal outcome arrives as a stream event. The phone holds that
     * stream and can therefore report success or failure honestly. This watch does not, by
     * decision rather than omission (ADR-0004: nothing background-critical may depend on
     * SSE, and a watch radio is the case that rule was written for).
     *
     * So the watch reports what it actually knows: the agent accepted the request, and here
     * is the service's state a moment later. Claiming "Restarted" from an acceptance would be
     * asserting something nobody observed — the same class of thing as rendering stale green.
     */
    fun invoke(serviceId: String, actionId: String, label: String) {
        viewModelScope.launch {
            val host: PairedHost = hosts.selectedHost.first() ?: return@launch
            _action.value = ActionUi.Working(label)

            when (val result = services.invokeAction(host, serviceId, actionId)) {
                is ApiResult.Failure -> _action.value =
                    ActionUi.Failed(label, shortMessage(result.error))

                is ApiResult.Success -> {
                    // The agent may reject an action it cannot perform without failing the
                    // call -- an unlisted unit, a missing polkit grant. Status carries that.
                    if (result.value.status == ActionStatus.Failed) {
                        _action.value = ActionUi.Failed(label, "The agent refused it")
                        return@launch
                    }

                    // Give systemd a moment, then look. The delay is the honest part: the
                    // watch is observing a result rather than being told one, so it has to
                    // wait long enough for there to be something to see.
                    _action.value = ActionUi.Accepted(label)
                    kotlinx.coroutines.delay(2_000)
                    refresh()
                }
            }
        }
    }

    /**
     * Ask the machine to reboot or shut down.
     *
     * # Why this is not [invoke] with a different endpoint
     *
     * [invoke] ends by waiting two seconds and re-reading the agent, because observing the
     * result is the honest way for a polling client to report one. Doing that here would be
     * a defect rather than a refinement: a power action that **worked** takes the agent down
     * with the machine, so the refresh would fail, and the screen would report "Could not
     * reach the agent" at the exact moment everything had gone right. The success case would
     * be the one that looked broken.
     *
     * So this stops at the acceptance and says so. Silence afterwards is the good outcome;
     * for a reboot the dashboard's next poll finds the agent again a minute later, and for a
     * shut down it never does — which is also correct, and is what `stale` is for.
     *
     * The failure that *can* be reported is the agent refusing outright — an unsupported
     * platform, a missing polkit grant, or a token without `host.power` on a device whose
     * cached scopes said otherwise. That last one is why the gate in `powerAccess` is
     * described as user experience: this call handles a `403` regardless of what the screen
     * believed.
     */
    fun invokePower(actionId: String, label: String) {
        viewModelScope.launch {
            val host: PairedHost = hosts.selectedHost.first() ?: return@launch
            _action.value = ActionUi.Working(label)

            _action.value = when (val result = services.invokeHostAction(host, actionId)) {
                is ApiResult.Failure -> ActionUi.Failed(label, shortMessage(result.error))
                is ApiResult.Success ->
                    if (result.value.status == ActionStatus.Failed) {
                        ActionUi.Failed(label, "The agent refused it")
                    } else {
                        ActionUi.Accepted(label)
                    }
            }
        }
    }

    /**
     * Hands the reading to the tile, and asks the system to redraw it.
     *
     * Failures are swallowed deliberately. The tile is a secondary surface, and a store
     * write or a carousel that declined an update must never take down the screen the
     * operator is actually looking at.
     */
    private suspend fun publishToTile(verdict: String, tally: Tally) {
        runCatching {
            LastReadingStore(getApplication()).write(
                LastReading(
                    verdict = verdict,
                    healthy = tally.healthy,
                    total = tally.total,
                    observedAt = Instant.now(),
                ),
            )
            CueSeekTileService.requestUpdate(getApplication())

            // The complication is pushed rather than polled — its manifest asks the system
            // never to wake it on a timer, precisely so that a slot nobody has glanced at
            // costs nothing. This is the push (M5.12).
            ComplicationDataSourceUpdateRequester
                .create(
                    context = getApplication(),
                    complicationDataSourceComponent = ComponentName(
                        getApplication(),
                        CueSeekComplicationService::class.java,
                    ),
                )
                .requestUpdateAll()
        }
    }

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

                    // The tile learns from the app for free. Every successful poll here is
                    // a reading the tile would otherwise have had to fetch for itself, so
                    // handing it over means the common case — open the app, glance at the
                    // tile later — costs no second round trip (M5.11).
                    val verdictNow = verdict(
                        stale = false,
                        services = snapshot.services,
                        hostMetrics = snapshot.hostMetrics,
                        tally = tally,
                    )
                    publishToTile(verdictNow, tally)

                    _ui.value = DashboardUi.Loaded(
                        hostname = snapshot.system.hostname,
                        verdict = verdictNow,
                        tally = tally,
                        services = snapshot.services,
                        metrics = snapshot.hostMetrics,
                        hostActions = snapshot.hostActions,
                        scopes = host.scopes,
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

        fun shortMessage(error: dev.cueseek.core.model.ApiError): String = when (error) {
            is dev.cueseek.core.model.ApiError.Transport -> "Could not reach the agent"
            is dev.cueseek.core.model.ApiError.Unauthorized -> "The agent refused this device"
            is dev.cueseek.core.model.ApiError.InsufficientScope -> "Not allowed with these scopes"
            is dev.cueseek.core.model.ApiError.ActionUnavailable -> "The agent cannot do that"
            is dev.cueseek.core.model.ApiError.ActionInProgress -> "Already running"
            else -> "Could not load"
        }
    }
}
