package dev.cueseek.core.data

import dev.cueseek.core.api.AgentStream
import dev.cueseek.core.api.StreamFailure
import dev.cueseek.core.model.ActionProgress
import dev.cueseek.core.model.AgentState
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.Freshness
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.StreamEnvelope
import dev.cueseek.core.model.StreamEvent
import dev.cueseek.core.model.StreamStatus
import dev.cueseek.core.model.SystemInfo
import dev.cueseek.core.model.degradedToUnknown
import java.io.IOException
import java.time.Duration
import java.time.Instant
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

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
    private val backoff: StreamBackoff = StreamBackoff(),
) {

    fun stateFor(host: PairedHost): Flow<AgentState> = channelFlow {
        val mutex = Mutex()
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

            val failure = collectUntilFailure(stream) { envelope ->
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

            attempt += 1
            publish { it.copy(status = StreamStatus.Retrying(failure, attempt)) }
            delay(backoff.delayFor(attempt).toMillis())
        }

        awaitClose { }
    }.distinctUntilChanged()

    /**
     * Collects [stream] until it fails, returning why, or `null` if cancelled.
     *
     * A gap in `seq` is turned into a failure on purpose. The sequence detects loss within
     * one connection, and there is no way to ask for what was missed — so the only correct
     * response is to reconnect, which by contract yields a fresh snapshot.
     */
    private suspend fun collectUntilFailure(
        stream: AgentStream,
        onEnvelope: suspend (StreamEnvelope) -> Unit,
    ): ApiError? = try {
        var previousSeq: Long? = null
        stream.events().collect { envelope ->
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
        ApiError.Transport(IOException("the stream ended"))
    } catch (e: StreamFailure) {
        e.error
    }

    /** State held between events, before staleness is applied. */
    private data class Internal(
        val system: SystemInfo? = null,
        val services: List<Service> = emptyList(),
        val status: StreamStatus = StreamStatus.Idle,
        val lastEventAt: Instant? = null,
        val outcomes: List<ActionProgress> = emptyList(),
    ) {

        fun apply(envelope: StreamEnvelope, receivedAt: Instant): Internal {
            // Every event of any kind counts, heartbeats included. Their whole purpose is
            // to make silence unambiguous, so they must reset the same timer.
            val base = copy(
                lastEventAt = receivedAt,
                status = StreamStatus.Open(since = (status as? StreamStatus.Open)?.since ?: receivedAt),
            )

            return when (val event = envelope.event) {
                is StreamEvent.Snapshot -> base.copy(
                    system = event.system,
                    // Replace wholesale. Merging assumes the old and new describe the same
                    // moment, and after a reconnect they do not.
                    services = event.services,
                )

                is StreamEvent.ServiceUpdated -> base.copy(
                    services = services.replaceById(event.service),
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
                status = status,
                freshness = if (stale) Freshness.Stale(lastEventAt) else Freshness.Fresh,
                actionOutcomes = outcomes,
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
