package dev.cueseek.core.api

import dev.cueseek.core.model.ActionAcceptance
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.Device
import dev.cueseek.core.model.DeviceId
import dev.cueseek.core.model.Pairing
import dev.cueseek.core.model.Platform
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.SystemInfo

/**
 * One agent's REST surface, in domain terms.
 *
 * Every operation returns an [ApiResult] and none of them throws: a rejected token and an
 * unreachable host are both ordinary outcomes on a phone, and neither is a programming
 * error. Nothing generated appears in this signature — that is the boundary ADR-0009 asks
 * for, and it is what makes replacing the generator a swap rather than a rewrite.
 *
 * `GET /v1/stream` is deliberately absent. It is not request/response, and it arrives in
 * P3 as its own type.
 */
interface CueSeekApi {

    /** `GET /v1/system` — scope `read`. */
    suspend fun system(): ApiResult<SystemInfo>

    /**
     * `POST /v1/pair` — the only unauthenticated operation.
     *
     * The code may be given in any form the user typed it: the agent strips anything
     * outside its alphabet and uppercases before matching, so `d8jt-hupv` and `D8JT HUPV`
     * both work.
     */
    suspend fun pair(
        code: String,
        deviceName: String,
        platform: Platform = Platform.Android,
    ): ApiResult<Pairing>

    /** `GET /v1/devices` — scope `read`. Newest first. */
    suspend fun devices(): ApiResult<List<Device>>

    /**
     * `DELETE /v1/devices/{id}` — scope `devices.manage`.
     *
     * A device may revoke itself, but still needs the scope to do it. A device paired with
     * the CLI's default `read,service.control` cannot revoke anything, including itself.
     */
    suspend fun revokeDevice(id: DeviceId): ApiResult<Unit>

    /** `GET /v1/services` — scope `read`. Configuration order. */
    suspend fun services(): ApiResult<List<Service>>

    /** `GET /v1/services/{id}` — scope `read`. */
    suspend fun service(id: String): ApiResult<Service>

    /**
     * `POST /v1/services/{serviceId}/actions/{actionId}` — scope `service.control`.
     *
     * Returns as soon as the agent has accepted the invocation, which is not the same as
     * the action having happened. Keep the returned
     * [ActionInvocationId][dev.cueseek.core.model.ActionInvocationId]: the outcome arrives
     * only as a stream event carrying it, and there is no endpoint to ask again.
     */
    suspend fun invokeAction(serviceId: String, actionId: String): ApiResult<ActionAcceptance>
}
