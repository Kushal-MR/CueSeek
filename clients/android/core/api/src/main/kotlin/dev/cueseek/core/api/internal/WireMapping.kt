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
import dev.cueseek.core.model.CpuMetrics
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthReason
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.HostId
import dev.cueseek.core.model.HostActionAcceptance
import dev.cueseek.core.model.HostActionOutcome
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.MemoryMetrics
import dev.cueseek.core.model.NowPlaying
import dev.cueseek.core.model.PlaybackSession
import dev.cueseek.core.model.Pairing
import dev.cueseek.core.model.Platform
import dev.cueseek.core.model.Scope
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.StorageMetrics
import dev.cueseek.core.model.ThermalMetrics
import dev.cueseek.core.model.WebUi
import dev.cueseek.core.model.SystemInfo
import dev.cueseek.core.model.TransferItem
import dev.cueseek.core.model.Transfers
import java.time.Instant
import java.time.OffsetDateTime
import dev.cueseek.core.api.wire.Action as WireAction
import dev.cueseek.core.api.wire.ActionAccepted as WireActionAccepted
import dev.cueseek.core.api.wire.ActionProgress as WireActionProgress
import dev.cueseek.core.api.wire.Capability as WireCapability
import dev.cueseek.core.api.wire.CpuMetrics as WireCpuMetrics
import dev.cueseek.core.api.wire.Device as WireDevice
import dev.cueseek.core.api.wire.Health as WireHealth
import dev.cueseek.core.api.wire.HealthReason as WireHealthReason
import dev.cueseek.core.api.wire.HostActionAccepted as WireHostActionAccepted
import dev.cueseek.core.api.wire.HostActionProgress as WireHostActionProgress
import dev.cueseek.core.api.wire.HostMetrics as WireHostMetrics
import dev.cueseek.core.api.wire.MemoryMetrics as WireMemoryMetrics
import dev.cueseek.core.api.wire.PairResponse as WirePairResponse
import dev.cueseek.core.api.wire.NowPlaying as WireNowPlaying
import dev.cueseek.core.api.wire.PlaybackSession as WirePlaybackSession
import dev.cueseek.core.api.wire.Service as WireService
import dev.cueseek.core.api.wire.StorageMetrics as WireStorageMetrics
import dev.cueseek.core.api.wire.ThermalMetrics as WireThermalMetrics
import dev.cueseek.core.api.wire.TransferItem as WireTransferItem
import dev.cueseek.core.api.wire.Transfers as WireTransfers
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
    nowPlaying = nowPlaying?.toDomain(),
    transfers = transfers?.toDomain(),
)

/**
 * The activity payloads.
 *
 * Nullability is carried through rather than defaulted. A wire `null` means the agent
 * never managed to observe this, and turning it into an empty list here would tell every
 * screen that nothing is playing — a claim the agent did not make.
 *
 * `state` stays a string for the same reason it is one in the contract: every download
 * client has its own vocabulary, and an enum would either lose values or throw on them.
 */
internal fun WireNowPlaying.toDomain() = NowPlaying(
    sessions = sessions,
    transcoding = transcoding,
    items = items.map { it.toDomain() },
)

internal fun WirePlaybackSession.toDomain() = PlaybackSession(
    id = id,
    title = title,
    subtitle = subtitle,
    user = user,
    client = client,
    positionSeconds = positionSeconds,
    durationSeconds = durationSeconds,
    paused = paused,
    transcoding = transcoding,
)

internal fun WireTransfers.toDomain() = Transfers(
    active = active,
    total = total,
    downloadRateBytes = downloadRateBytes,
    uploadRateBytes = uploadRateBytes,
    items = items.map { it.toDomain() },
)

/**
 * The host's vitals.
 *
 * Nullability is carried through exactly as it arrived, including the difference between a
 * null list and an empty one. Null storage means the agent could not measure any filesystem;
 * an empty list means it was asked about mounts and none answered. Defaulting either to
 * `emptyList()` here would collapse a real distinction into a screen that says the same
 * thing for both.
 */
internal fun WireHostMetrics.toDomain() = HostMetrics(
    collectedAt = parseTimestamp(collectedAt),
    uptimeSeconds = uptimeSeconds,
    cpu = cpu?.toDomain(),
    memory = memory?.toDomain(),
    storage = storage?.map { it.toDomain() },
    thermal = thermal?.map { it.toDomain() },
)

internal fun WireCpuMetrics.toDomain() = CpuMetrics(
    usagePercent = usagePercent,
    cores = cores,
    load1 = load1,
    load5 = load5,
    load15 = load15,
)

internal fun WireMemoryMetrics.toDomain() = MemoryMetrics(
    totalBytes = totalBytes,
    availableBytes = availableBytes,
    usedBytes = usedBytes,
    swapTotalBytes = swapTotalBytes,
    swapUsedBytes = swapUsedBytes,
)

internal fun WireStorageMetrics.toDomain() = StorageMetrics(
    mount = mount,
    totalBytes = totalBytes,
    freeBytes = freeBytes,
    filesystem = filesystem,
)

internal fun WireThermalMetrics.toDomain() = ThermalMetrics(
    label = label,
    celsius = celsius,
    highCelsius = highCelsius,
)

internal fun WireTransferItem.toDomain() = TransferItem(
    id = id,
    name = name,
    state = state,
    progress = progress,
    sizeBytes = sizeBytes,
    downloadRateBytes = downloadRateBytes,
    etaSeconds = etaSeconds,
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

internal fun WireHostActionAccepted.toDomain() = HostActionAcceptance(
    actionId = ActionInvocationId(actionId),
    action = action,
    status = ActionStatus.fromWire(status),
    acceptedAt = parseTimestamp(acceptedAt),
)

internal fun WireHostActionProgress.toDomain() = HostActionOutcome(
    actionId = ActionInvocationId(actionId),
    action = action,
    status = ActionStatus.fromWire(status),
    at = parseTimestamp(at),
    error = error,
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
