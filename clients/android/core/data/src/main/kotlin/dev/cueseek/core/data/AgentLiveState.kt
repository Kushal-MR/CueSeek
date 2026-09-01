package dev.cueseek.core.data

import dev.cueseek.core.api.AgentStream
import dev.cueseek.core.api.StreamFailure
import dev.cueseek.core.model.ActionProgress
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.Freshness
import dev.cueseek.core.model.Action
import dev.cueseek.core.model.HostActionOutcome
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.StreamEnvelope
import dev.cueseek.core.model.StreamEvent
import dev.cueseek.core.model.StreamStatus
import dev.cueseek.core.model.SystemInfo
import dev.cueseek.core.model.degradedToUnknown
import java.io.IOException
import java.util.concurrent.atomic.AtomicReference
import java.time.Duration
import java.time.Instant
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.flow.conflate
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeoutOrNull

/**
 * Live state for one agent, and the reason this client can be trusted.
 *
 * Two independent things run here. One holds a stream open and applies what arrives. The
 * other watches a **clock** and marks the data stale when nothing has arrived for a while,
 * regardless of what the connection claims about itself.
 *
 * The second one exists because of A7. With the screen off, the stream does not
 * disconnect — it freezes silently for up to 168 seconds on cellular while continuing to
 * report itself connected, then delivers everything in a burst on wake. A client that
 * trusted the transport would show a green dot for a service that died three minutes
 * earlier. So: **never trust the connection state** (`docs/m2-android-api.md` §8).
 *
 * Deliberately lifecycle-agnostic. This is a cold flow; nothing runs until something
 * collects, and everything stops when collection stops. Whether that should be happening —
 * whether the app is in the foreground at all — is a question for the UI layer, which
 * scopes collection with `repeatOnLifecycle`. The stream is a foreground affordance and
 * nothing background-critical may depend on it (ADR-0004 Amendment 2).
 */
class AgentLiveState(
    private val streams: suspend (PairedHost) -> AgentStream?,
    /**
     * Fetches the agent's current state on demand, for a manual refresh.
     *
     * Deliberately the same [StreamEvent.Snapshot] the stream's first event carries, so a
     * pull and a reconnect converge on one application path rather than two that have to
     * be kept honest separately.
     */
    private val snapshots: suspend (PairedHost) -> ApiResult<StreamEvent.Snapshot> = {
        ApiResult.Failure(ApiError.Transport(IOException("no refresh path is configured")))
    },
    /**
     * Asks the agent to observe its services now.
     *
     * Distinct from [snapshots], and the distinction is the entire feature. [snapshots]
     * re-reads what the agent already knows; this makes it look again. A refresh that
     * only did the former would answer "has this changed?" with the state from before the
     * change (ADR-0003 Amendment 1).
     */
    private val nudge: suspend (PairedHost) -> ApiResult<Unit> = { ApiResult.Success(Unit) },
    /**
     * How long to give the agent to observe before reading the result.
     *
     * Not a guess at how long a poll takes so much as a bound on how long the indicator
     * may spin. If the agent is slower than this, the answer still arrives — over the
     * stream, a moment later, through the ordinary path.
     */
    private val observeWindow: Duration = Duration.ofMillis(1_200),
    private val now: () -> Instant = Instant::now,
    /**
     * How long silence is tolerated before the data is disbelieved.
     *
     * Two heartbeat intervals. One would make an ordinary scheduling hiccup look like a
     * dead agent; much more, and the console spends longer being confidently wrong.
     */
    private val staleAfter: Duration = Duration.ofSeconds(30),
    /** How often silence is re-checked. Cheap, and bounds how late a staleness flip is. */
    private val checkInterval: Duration = Duration.ofSeconds(1),
    /**
     * How long a stream may deliver nothing before it is treated as dead and replaced.
     *
     * Defaults to [staleAfter] on purpose: one number with one meaning — "silence this long
     * means the connection is gone" — rather than two thresholds that can drift apart and
     * leave the screen saying one thing while the transport believes another.
     *
     * This exists because the stream's read timeout is deliberately infinite. A quiet
     * agent is normal and must not be disconnected, so OkHttp is told to wait forever;
     * the cost is that a *frozen* socket also waits forever. A7 measured exactly that —
     * the connection reports itself open and delivers nothing — and the client applied the
     * lesson to rendering while still trusting the transport to notice its own death. It
     * does not. Heartbeats arrive every 15s, so silence past this bound is not a quiet
     * system; it is a dead one.
     */
    private val silenceTimeout: Duration = staleAfter,
    private val backoff: StreamBackoff = StreamBackoff(),
) {

    /**
     * @param refreshes manual refresh requests. Each emission means "ask the agent now".
     *   Requests arriving while one is already in flight are dropped rather than queued —
     *   see the collector below for why that is the correct answer and not a shortcut.
     */
    fun stateFor(
        host: PairedHost,
        refreshes: Flow<Unit> = emptyFlow(),
    ): Flow<AgentState> = channelFlow {
        val mutex = Mutex()
        val refreshLock = Mutex()
        var internal = Internal()

        suspend fun publish(transform: (Internal) -> Internal) {
            val rendered = mutex.withLock {
                internal = transform(internal)
                internal.render(host, now(), staleAfter)
            }
            send(rendered)
        }

        publish { it }

        // The watchdog. It never talks to the network; its only input is the clock, which
        // is the point - a frozen stream produces no events to react to, so the absence of
        // events has to be what drives this.
        launch {
            while (isActive) {
                delay(checkInterval.toMillis())
                publish { it }
            }
        }

        // A refresh is also the user saying "try now". If the stream is sitting out a
        // backoff, that wait should end. Conflated: several pulls during one wait are one
        // instruction, and only the fact that one arrived matters.
        val reconnectNow = Channel<Unit>(Channel.CONFLATED)

        launch {
            // Conflated, so a burst of pulls during one request collapses to a single
            // follow-up instead of a queue of identical ones.
            refreshes.conflate().collect {
                reconnectNow.trySend(Unit)

                // Single-flight, and the reason it is a drop rather than a queue: every
                // request would fetch the same cached state from the agent, so running a
                // second one behind the first produces an identical answer later. The
                // caller wants current state, not a count of how many times it was asked.
                if (!refreshLock.tryLock()) return@collect
                try {
                    publish { it.copy(refreshing = true) }

                    // Ask the agent to look, then give it a moment to do so before
                    // reading. A failed nudge is not fatal: the read below still proves
                    // reachability and still refuses to invent freshness, so an agent too
                    // old to know this endpoint degrades to the previous behaviour rather
                    // than to an error.
                    if (nudge(host) is ApiResult.Success) {
                        delay(observeWindow.toMillis())
                    }

                    when (val result = snapshots(host)) {
                        // Applied through the ordinary event path, so the freshness clock
                        // advances for the same reason it always does: the agent answered.
                        is ApiResult.Success -> publish {
                            it.applyEvent(result.value, now()).copy(refreshing = false)
                        }

                        // The clock is untouched. A refresh that failed is evidence of
                        // nothing, and moving the timestamp here would let a pull make a
                        // dead agent look alive — the one lie this client must not tell.
                        is ApiResult.Failure -> publish { it.copy(refreshing = false) }
                    }
                } finally {
                    refreshLock.unlock()
                }
            }
        }

        var attempt = 0
        while (isActive) {
            publish { it.copy(status = StreamStatus.Connecting) }

            val stream = streams(host)
            if (stream == null) {
                publish {
                    it.copy(
                        status = StreamStatus.Stopped(
                            ApiError.Unauthorized(
                                tokenWasSent = false,
                                detail = "This device holds no usable token for ${host.hostname}.",
                            )
                        )
                    )
                }
                return@channelFlow
            }

            var delivered = false
            val failure = collectUntilFailure(stream) { envelope ->
                delivered = true
                publish { it.apply(envelope, now()) }
            }

            if (failure == null) {
                // Cancelled from outside; nothing to report and nothing to retry.
                return@channelFlow
            }

            // A rejected token cannot be fixed by trying again, and retrying would mean an
            // endless loop of 401s against an agent that has already given its answer.
            if (failure is ApiError.Unauthorized && failure.tokenWasSent) {
                publish { it.copy(status = StreamStatus.Stopped(failure)) }
                return@channelFlow
            }

            // A connection that delivered anything was a working connection, so the next
            // reconnect starts from the bottom of the backoff rather than wherever an
            // earlier bad patch left it. Without this the counter only ever climbs, and a
            // stream that has dropped a few times over a long session sits at the 15s cap
            // for the rest of that session — slowest exactly when it has been most reliable.
            if (delivered) attempt = 0
            attempt += 1
            publish { it.copy(status = StreamStatus.Retrying(failure, attempt)) }
            // Whichever comes first: the backoff elapsing, or the user asking. A signal
            // left over from a refresh that happened while connected simply means the next
            // reconnect is prompt, which is the behaviour a user who just pulled expects.
            withTimeoutOrNull(backoff.delayFor(attempt).toMillis()) { reconnectNow.receive() }
        }

        awaitClose { }
    }.distinctUntilChanged()

    /**
     * Collects [stream] until it fails **or goes silent**, returning why.
     *
     * A gap in `seq` is turned into a failure on purpose. The sequence detects loss within
     * one connection, and there is no way to ask for what was missed — so the only correct
     * response is to reconnect, which by contract yields a fresh snapshot.
     *
     * The silence half is the one that took a real phone to find. The stream's read timeout
     * is infinite by design — a quiet agent must not be disconnected — so a socket that has
     * died without saying so is indistinguishable from a socket with nothing to say, and
     * OkHttp will wait on it for as long as the process lives. The freshness watchdog saw
     * this and said "Unverified"; nothing acted on it, so the screen sat there being
     * correct and useless until something else forced a reconnect.
     *
     * Two coroutines and whichever finishes first wins: one collects, one watches the
     * clock. That mirrors the split in [stateFor] itself, and for the same reason — the
     * absence of events cannot be detected by waiting for an event.
     */
    private suspend fun collectUntilFailure(
        stream: AgentStream,
        onEnvelope: suspend (StreamEnvelope) -> Unit,
    ): ApiError? = coroutineScope {
        // Written by the collector, read by the watcher, so it crosses threads.
        val lastActivity = AtomicReference(now())
        val outcome = CompletableDeferred<ApiError>()

        val collector = launch {
            try {
                var previousSeq: Long? = null
                stream.events().collect { envelope ->
                    lastActivity.set(now())
                    val expected = previousSeq?.plus(1)
                    // seq restarts at 0 on every connection, so a snapshot is never a gap.
                    if (expected != null && envelope.seq != expected && envelope.seq != 0L) {
                        throw StreamFailure(
                            ApiError.Transport(
                                IOException("event sequence gap: expected $expected, got ${envelope.seq}")
                            )
                        )
                    }
                    previousSeq = envelope.seq
                    onEnvelope(envelope)
                }
                // A stream that completes without failing is still a stream that stopped.
                outcome.complete(ApiError.Transport(IOException("the stream ended")))
            } catch (e: StreamFailure) {
                outcome.complete(e.error)
            }
        }

        val silenceWatch = launch {
            while (isActive) {
                delay(checkInterval.toMillis())
                val silent = Duration.between(lastActivity.get(), now())
                if (silent >= silenceTimeout) {
                    outcome.complete(
                        ApiError.Transport(
                            IOException(
                                "no events for ${silent.seconds}s on an open connection; " +
                                    "treating it as dead"
                            )
                        )
                    )
                    return@launch
                }
            }
        }

        val failure = outcome.await()
        // Whichever one lost is torn down here. Cancelling the collector is what actually
        // closes the dead socket; leaving it would leak a connection per reconnect.
        collector.cancel()
        silenceWatch.cancel()
        failure
    }

    /** State held between events, before staleness is applied. */
    private data class Internal(
        val system: SystemInfo? = null,
        val services: List<Service> = emptyList(),
        val hostMetrics: HostMetrics? = null,
        val hostActions: List<Action> = emptyList(),
        val status: StreamStatus = StreamStatus.Idle,
        val lastEventAt: Instant? = null,
        val outcomes: List<ActionProgress> = emptyList(),
        val hostFailures: List<HostActionOutcome> = emptyList(),
        /** A request this client has outstanding. Says nothing about the data's age. */
        val refreshing: Boolean = false,
    ) {

        /**
         * Applies an event that arrived over the stream.
         *
         * The status update lives here rather than in [applyEvent] because it is the one
         * thing only the stream may claim. A manual refresh proves the agent answered an
         * HTTP request; it proves nothing about whether the stream is open, and a
         * successful pull during a reconnect must leave "Reconnecting" on screen.
         */
        fun apply(envelope: StreamEnvelope, receivedAt: Instant): Internal =
            applyEvent(envelope.event, receivedAt).copy(
                status = StreamStatus.Open(
                    since = (status as? StreamStatus.Open)?.since ?: receivedAt,
                ),
            )

        fun applyEvent(event: StreamEvent, receivedAt: Instant): Internal {
            // Every event of any kind counts, heartbeats included. Their whole purpose is
            // to make silence unambiguous, so they must reset the same timer.
            val base = copy(lastEventAt = receivedAt)

            return when (event) {
                is StreamEvent.Snapshot -> base.copy(
                    system = event.system,
                    // Replace wholesale. Merging assumes the old and new describe the same
                    // moment, and after a reconnect they do not.
                    services = event.services,
                    // Including when it is null. An agent that has restarted and not yet
                    // measured the host must clear what this client is holding, not keep
                    // showing figures from before the restart.
                    hostMetrics = event.hostMetrics,
                    hostActions = event.hostActions,
                )

                is StreamEvent.ServiceUpdated -> base.copy(
                    services = services.replaceById(event.service),
                )

                is StreamEvent.HostUpdated -> base.copy(hostMetrics = event.metrics)

                // A power action that failed, which means the machine is still here. Kept
                // alongside service outcomes so one screen can surface either without
                // caring which kind it was.
                is StreamEvent.HostActionFailed -> base.copy(
                    hostFailures = (hostFailures + event.outcome)
                        .takeLast(AgentState.MAX_ACTION_OUTCOMES),
                )

                is StreamEvent.ActionOutcome -> base.copy(
                    outcomes = (outcomes + event.progress)
                        .takeLast(AgentState.MAX_ACTION_OUTCOMES),
                )

                StreamEvent.Heartbeat -> base

                // Counts as traffic - the agent is alive and talking - but says nothing
                // this build can act on.
                is StreamEvent.Unrecognised -> base
            }
        }

        fun render(host: PairedHost, at: Instant, staleAfter: Duration): AgentState {
            val stale = lastEventAt == null ||
                Duration.between(lastEventAt, at) > staleAfter

            return AgentState(
                host = host,
                system = system,
                services = if (stale) services.map { it.degradedToUnknown() } else services,
                // Dropped rather than degraded. A service keeps its timestamps through
                // staleness because "healthy three minutes ago" is still worth saying; a
                // CPU percentage from three minutes ago is worth nothing, and showing one
                // would put a live-looking number under a stale banner.
                hostMetrics = if (stale) null else hostMetrics,
                // Not dropped when stale, unlike the metrics. What the agent *offers* does
                // not expire the way a CPU reading does, and blanking the menu the moment
                // a phone's screen slept would remove the buttons at the exact moment
                // somebody went looking for them.
                hostActions = hostActions,
                hostActionFailures = hostFailures,
                status = status,
                freshness = if (stale) Freshness.Stale(lastEventAt) else Freshness.Fresh,
                actionOutcomes = outcomes,
                refreshing = refreshing,
            )
        }
    }
}

private fun List<Service>.replaceById(service: Service): List<Service> {
    val index = indexOfFirst { it.id == service.id }
    return if (index < 0) this + service else toMutableList().apply { this[index] = service }
}

/**
 * How long to wait before reconnecting.
 *
 * Exponential, capped, and without jitter. Jitter solves a thundering-herd problem that a
 * fleet of clients has; this is one phone talking to one server on a private network, and
 * a deterministic schedule is worth more than a spread here because it can be tested.
 *
 * A7 measured a reconnect at 3-4 seconds, so the early steps are deliberately shorter than
 * that: the common case is a transient stall on a phone radio, and waiting longer than the
 * reconnect itself takes would be the slower path back to correct data.
 */
class StreamBackoff(
    private val initial: Duration = Duration.ofSeconds(1),
    private val max: Duration = Duration.ofSeconds(15),
) {
    fun delayFor(attempt: Int): Duration {
        if (attempt <= 1) return initial
        val scaled = initial.multipliedBy(1L shl (attempt - 1).coerceAtMost(SHIFT_CEILING))
        return if (scaled > max) max else scaled
    }

    private companion object {
        /** Beyond this the shift overflows before the cap ever applies. */
        const val SHIFT_CEILING = 16
    }
}
