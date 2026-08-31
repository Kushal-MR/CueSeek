package dev.cueseek.core.api.internal

import dev.cueseek.core.model.StreamEnvelope
import dev.cueseek.core.model.StreamEvent
import java.time.OffsetDateTime
import dev.cueseek.core.api.wire.StreamEnvelope as WireEnvelope

/**
 * Generated stream envelope to domain envelope.
 *
 * The envelope's `type` decides which optional payload is populated, and the contract is
 * explicit about the pairing, so a mismatch — `type: service_updated` with no `service` —
 * is a malformed event rather than something to paper over with a default.
 *
 * @param frameType the SSE `event:` field. The agent sends the same value as the
 *   envelope's `type`, so it is used only as a fallback for a frame whose body somehow
 *   lacks one.
 */
internal fun WireEnvelope.toDomain(frameType: String?): StreamEnvelope {
    val kind = type.ifBlank { frameType.orEmpty() }

    val event = when (kind) {
        "snapshot" -> snapshot?.let {
            StreamEvent.Snapshot(
                system = it.system.toDomain(),
                services = it.services.map { service -> service.toDomain() },
                // Nullable all the way through: an agent that has not measured the host
                // yet, or cannot, says nothing rather than sending an empty object.
                hostMetrics = it.hostMetrics?.toDomain(),
            )
        }

        "service_updated" -> service?.let { StreamEvent.ServiceUpdated(it.toDomain()) }

        "host_updated" -> hostMetrics?.let { StreamEvent.HostUpdated(it.toDomain()) }

        "action_progress" -> actionProgress?.let { StreamEvent.ActionOutcome(it.toDomain()) }

        "heartbeat" -> StreamEvent.Heartbeat

        // A type from an agent newer than this build. Kept rather than dropped: a silently
        // discarded event is indistinguishable from a quiet stream, and quiet is the one
        // thing this client must never misread (ADR-0007).
        else -> StreamEvent.Unrecognised(kind)
    } ?: throw IllegalArgumentException("$kind envelope carried no payload")

    return StreamEnvelope(
        seq = seq,
        emittedAt = OffsetDateTime.parse(emittedAt).toInstant(),
        schemaVersion = schemaVersion,
        event = event,
    )
}
