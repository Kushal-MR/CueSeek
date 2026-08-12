package dev.cueseek.core.api.internal

import dev.cueseek.core.model.Action
import dev.cueseek.core.model.ActionAcceptance
import dev.cueseek.core.model.ActionInvocationId
import dev.cueseek.core.model.ActionProgress
import dev.cueseek.core.model.ActionRisk
import dev.cueseek.core.model.ActionStatus
import dev.cueseek.core.model.Capability
import dev.cueseek.core.model.Device
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.DeviceToken
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthReason
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.HostId
import dev.cueseek.core.model.Pairing
import dev.cueseek.core.model.Platform
import dev.cueseek.core.model.Scope
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.WebUi
import dev.cueseek.core.model.SystemInfo
import java.time.Instant
import java.time.OffsetDateTime
import dev.cueseek.core.api.wire.Action as WireAction
import dev.cueseek.core.api.wire.ActionAccepted as WireActionAccepted
import dev.cueseek.core.api.wire.ActionProgress as WireActionProgress
import dev.cueseek.core.api.wire.Capability as WireCapability
import dev.cueseek.core.api.wire.Device as WireDevice
import dev.cueseek.core.api.wire.Health as WireHealth
import dev.cueseek.core.api.wire.HealthReason as WireHealthReason
import dev.cueseek.core.api.wire.PairResponse as WirePairResponse
import dev.cueseek.core.api.wire.Service as WireService
import dev.cueseek.core.api.wire.WebUI as WireWebUI
import dev.cueseek.core.api.wire.System as WireSystem

/**
 * Generated wire types to domain types.
 *
 * This file is the entire reason `:core:api` can change generators without anything else
 * moving, and it is where every leniency lives. Enum-valued fields cross the wire as
 * strings — the generator's enums throw on values they predate, which would fail a whole
 * response over one field a newer agent added — so each is widened here into a domain type
 * that has somewhere to put the unrecognised case.
 *
 * A malformed timestamp throws, deliberately. There is no sensible substitute for "when
 * was this observed", and guessing one would produce a UI that renders staleness from a
 * fabricated clock. The caller turns it into [ApiError.Malformed]
 * [dev.cueseek.core.model.ApiError.Malformed].
 */
internal fun WireSystem.toDomain() = SystemInfo(
    hostId = HostId(hostId),
    hostname = hostname,
    agentVersion = agentVersion,
    apiVersion = apiVersion,
    startedAt = parseTimestamp(startedAt),
)

internal fun WireDevice.toDomain() = Device(
    id = DeviceId(id),
    name = name,
    platform = Platform.fromWire(platform),
    scopes = scopes.mapNotNull(Scope::fromWire).toSet(),
    createdAt = parseTimestamp(createdAt),
    lastSeenAt = lastSeenAt?.let(::parseTimestamp),
)

internal fun WirePairResponse.toDomain() = Pairing(
    device = device.toDomain(),
    token = DeviceToken(token),
)

internal fun WireHealthReason.toDomain() = HealthReason(code = code, message = message)

internal fun WireHealth.toDomain() = Health(
    status = HealthStatus.fromWire(status),
    reachable = reachable,
    reportedStatus = reportedStatus,
    reasons = reasons.map { it.toDomain() },
    observedAt = parseTimestamp(observedAt),
)

internal fun WireCapability.toDomain() = Capability(id = id, label = label)

internal fun WireAction.toDomain() = Action(
    id = id,
    label = label,
    risk = ActionRisk.fromWire(risk),
    description = description,
)

internal fun WireService.toDomain() = Service(
    id = id,
    name = name,
    capabilities = capabilities.map { it.toDomain() },
    health = health.toDomain(),
    actions = actions.map { it.toDomain() },
    webUi = webUi?.toDomain(),
)

/**
 * `scheme` is one of the few fields the contract models as a closed enum rather than as an
 * open string like `risk`. That is defensible here and nowhere else: the set of schemes a
 * client is willing to hand to a browser intent is not a vocabulary the agent gets to
 * extend. It flattens back to a string so the domain never depends on generated types, and
 * [dev.cueseek.core.model.urlFor] validates it again regardless.
 */
internal fun WireWebUI.toDomain() = WebUi(
    scheme = scheme.value,
    port = port,
    path = path ?: "/",
)

internal fun WireActionAccepted.toDomain() = ActionAcceptance(
    actionId = ActionInvocationId(actionId),
    serviceId = serviceId,
    action = action,
    status = ActionStatus.fromWire(status),
    acceptedAt = parseTimestamp(acceptedAt),
)

internal fun WireActionProgress.toDomain() = ActionProgress(
    actionId = ActionInvocationId(actionId),
    serviceId = serviceId,
    action = action,
    status = ActionStatus.fromWire(status),
    at = parseTimestamp(at),
    error = error,
)

/**
 * Parses an RFC 3339 timestamp.
 *
 * `OffsetDateTime` rather than `Instant.parse` because the contract promises RFC 3339,
 * which permits a numeric offset, while `Instant.parse` historically insisted on `Z`. The
 * agent only emits `Z` today; accepting `+05:30` costs nothing and removes a failure that
 * would be very hard to reproduce.
 */
private fun parseTimestamp(value: String): Instant = OffsetDateTime.parse(value).toInstant()
