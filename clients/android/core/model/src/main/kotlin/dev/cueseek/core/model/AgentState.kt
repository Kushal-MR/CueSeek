package dev.cueseek.core.model

import java.time.Instant

/**
 * Everything a screen needs to know about one agent right now.
 *
 * [services] is **already degraded when [freshness] is stale**. That is deliberate: the
 * requirement to render `unknown` rather than the last known values is the one this client
 * is most likely to get wrong, and leaving it to each screen means every new screen is
 * another chance to show stale green. A screen that ignores [freshness] entirely still
 * renders honestly.
 *
 * The observed timestamps are preserved through degradation, so a screen can still say
 * "last seen three minutes ago" — staleness is rendered from `observed_at`, never from
 * arrival time.
 */
data class AgentState(
    val host: PairedHost,
    val system: SystemInfo?,
    val services: List<Service>,
    val status: StreamStatus,
    val freshness: Freshness,
    /**
     * The machine's own vitals, or null when there are none to show.
     *
     * Null in three cases that a screen treats identically and should: nothing has arrived
     * yet, the agent cannot measure this host, and — the one worth naming — [freshness] has
     * gone stale. Services degrade to `unknown` in that situation and keep their
     * timestamps, because "it was healthy three minutes ago" is still useful. A CPU
     * percentage from three minutes ago is not: it changes every few seconds, there is no
     * last-known value worth preserving, and leaving one on screen under a live-looking
     * layout would be exactly the confident wrongness this client is built to avoid.
     */
    val hostMetrics: HostMetrics? = null,
    /**
     * Power actions the agent offers for the machine.
     *
     * Empty on a platform that cannot perform them, and **not** filtered by what this
     * device may do — that is [PairedHost.scopes]' job, and the two answer different
     * questions. Knowing the agent offers a reboot while knowing this device was not
     * granted `host.power` is what lets a screen say "not permitted on this device"
     * rather than showing nothing and leaving the user to guess which it was.
     */
    val hostActions: List<Action> = emptyList(),
    /**
     * Terminal action outcomes seen on the current connection, newest last, bounded.
     *
     * The stream is the only delivery mechanism — there is no endpoint to ask an action
     * how it went — so an outcome that arrives while nothing is collecting is simply lost.
     * A reconnect delivers a fresh snapshot, not missed events.
     */
    val actionOutcomes: List<ActionProgress> = emptyList(),
    /**
     * Host power actions that **failed**, newest last, bounded.
     *
     * There is no success list and there cannot be: a reboot that worked ends the stream
     * that would have reported it. Anything in here means the machine is still running.
     */
    val hostActionFailures: List<HostActionOutcome> = emptyList(),
    /**
     * Whether a manual refresh is in flight *right now*.
     *
     * Purely a statement about a request this client has outstanding, and deliberately not
     * connected to [freshness]. Asking is not the same as being answered: the data becomes
     * fresh when the agent replies, and a refresh that fails leaves the freshness clock
     * exactly where it was. A screen that showed green because the user pulled would be
     * the precise failure this client exists to avoid.
     */
    val refreshing: Boolean = false,
) {
    fun outcomeOf(actionId: ActionInvocationId): ActionProgress? =
        actionOutcomes.lastOrNull { it.actionId == actionId }

    /**
     * True when the agent implements a contract version this build was not written for.
     *
     * Reported honestly in both directions rather than silently mis-rendering (ADR-0007).
     */
    fun apiVersionSkew(expected: String): Boolean =
        system != null && system.apiVersion != expected

    companion object {
        /** How many outcomes to keep. Enough to survive a burst after a Doze thaw. */
        const val MAX_ACTION_OUTCOMES: Int = 16

        fun initial(host: PairedHost) = AgentState(
            host = host,
            system = null,
            services = emptyList(),
            status = StreamStatus.Idle,
            // Nothing has arrived yet, so nothing is known yet. Starting at Fresh would
            // mean an empty service list briefly looked like a healthy empty server.
            freshness = Freshness.Stale(lastEventAt = null),
        )
    }
}

/**
 * Rewrites health as `unknown` while preserving when it was observed.
 *
 * `status` becomes [HealthStatus.Unknown] and a client-generated reason is prepended.
 * `reachable`, `reported_status` and `observed_at` are left alone: they are timestamped
 * statements about a moment that did happen, and discarding them would remove the
 * information a user needs to judge how bad the silence is.
 *
 * The reason code is `client_stale`, not the agent's `stale`, so that nobody reading a bug
 * report has to work out which side generated it.
 */
fun Service.degradedToUnknown(): Service = copy(
    health = health.copy(
        status = HealthStatus.Unknown,
        reasons = listOf(
            HealthReason(
                code = "client_stale",
                message = "No update from the agent recently; this may be out of date.",
            )
        ) + health.reasons,
    )
)
