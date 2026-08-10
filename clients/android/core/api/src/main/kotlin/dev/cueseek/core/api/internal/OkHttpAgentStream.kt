package dev.cueseek.core.api.internal

import dev.cueseek.core.api.AgentStream
import dev.cueseek.core.api.StreamFailure
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.ApiError
import dev.cueseek.core.model.StreamEnvelope
import java.io.IOException
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.channels.trySendBlocking
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources

/**
 * [AgentStream] over OkHttp's `EventSource`.
 *
 * Hand-written because it has to be: no OpenAPI generator models `text/event-stream`, and
 * the Go server hand-wrote its side for the same reason (ADR-0004 Amendment 3).
 *
 * OkHttp's `EventSource` does not reconnect on its own, which is what makes it the right
 * primitive here — the reconnect policy belongs to the layer that knows whether the app is
 * in the foreground.
 */
internal class OkHttpAgentStream(
    private val address: AgentAddress,
    private val client: OkHttpClient,
    private val json: Json,
) : AgentStream {

    override fun events(): Flow<StreamEnvelope> = callbackFlow {
        val request = Request.Builder()
            .url(address.baseUrl + STREAM_PATH)
            .header("Accept", "text/event-stream")
            // Proxies and the agent both honour this; the agent already sets
            // X-Accel-Buffering itself, but asking costs nothing.
            .header("Cache-Control", "no-cache")
            .build()

        val listener = object : EventSourceListener() {

            override fun onEvent(
                eventSource: EventSource,
                id: String?,
                type: String?,
                data: String,
            ) {
                val envelope = parseEnvelope(type, data)
                if (envelope != null) {
                    // Blocking send: the server disconnects a client too slow to drain its
                    // 16-event buffer, which is the correct outcome - it reconnects and
                    // gets a fresh snapshot. Dropping events here would instead leave the
                    // client quietly wrong.
                    trySendBlocking(envelope)
                }
            }

            override fun onFailure(
                eventSource: EventSource,
                t: Throwable?,
                response: Response?,
            ) {
                close(StreamFailure(failureToError(t, response)))
            }

            override fun onClosed(eventSource: EventSource) {
                // A clean close from the agent - it is shutting down, or it dropped a slow
                // client. Either way the collector should reconnect, so this is a failure
                // from the caller's point of view rather than a completed stream.
                close(
                    StreamFailure(
                        ApiError.Transport(IOException("the agent closed the stream"))
                    )
                )
            }
        }

        val source = EventSources.createFactory(client).newEventSource(request, listener)
        awaitClose { source.cancel() }
    }

    private fun failureToError(t: Throwable?, response: Response?): ApiError = when {
        // A response means the agent answered and refused: 401 for a dead token, 503 while
        // shutting down. Those need different handling, and the problem document says which.
        response != null -> response.toApiError(json)
        t is IOException -> ApiError.Transport(t)
        t != null -> ApiError.Transport(IOException(t))
        else -> ApiError.Transport(IOException("the stream failed without a reason"))
    }

    /**
     * Turns one frame into an envelope, or `null` if it cannot be understood at all.
     *
     * A frame this build cannot parse is skipped rather than fatal. Failing the connection
     * over one bad frame would produce a reconnect loop against an agent that is otherwise
     * working, and the next snapshot rebuilds state anyway. An event whose *type* is
     * unknown is a different matter and is kept, as
     * [dev.cueseek.core.model.StreamEvent.Unrecognised], so skew stays visible.
     */
    private fun parseEnvelope(type: String?, data: String): StreamEnvelope? = try {
        json.decodeFromString<WireStreamEnvelope>(data).toDomain(type)
    } catch (_: Exception) {
        null
    }

    private companion object {
        const val STREAM_PATH = "v1/stream"
    }
}

private typealias WireStreamEnvelope = dev.cueseek.core.api.wire.StreamEnvelope
