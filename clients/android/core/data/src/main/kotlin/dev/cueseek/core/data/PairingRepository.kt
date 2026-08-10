package dev.cueseek.core.data

import dev.cueseek.core.api.CueSeekApiFactory
import dev.cueseek.core.api.TokenProvider
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Platform
import java.time.Clock

/**
 * Turns a pairing code into a stored, usable host.
 *
 * Pairing is two calls, not one, and the order is forced by the contract:
 *
 *  1. `POST /v1/pair` redeems the code and returns the device and its token. It does
 *     **not** return `host_id` — the agent has no reason to, since the caller asked about
 *     a code, not about a host.
 *  2. `GET /v1/system`, authenticated with the token just issued, supplies `host_id`, the
 *     stable key everything is filed under (ADR-0008).
 *
 * Nothing is persisted until both succeed. A record keyed by an address rather than a host
 * id would be a second kind of key in a data model that has exactly one, and every
 * repository would have to know which kind it was holding.
 */
class PairingRepository(
    private val hosts: HostRepository,
    private val http: CueSeekApiFactory.AgentHttp = CueSeekApiFactory.sharedHttp(),
    private val clock: Clock = Clock.systemUTC(),
) {

    /**
     * Redeems [code] against the agent at [address] and stores the result.
     *
     * [code] may be given exactly as the user typed it. The agent strips anything outside
     * its alphabet and uppercases before matching, so `d8jt hupv` and `D8JT-HUPV` are the
     * same code and the client does not need to normalise.
     *
     * On failure nothing is written. The failure is returned as-is: an
     * [dev.cueseek.core.model.ApiError.InvalidPairingCode] must not be reworded into a
     * claim about *why* the code was rejected, because the agent merges unknown, expired
     * and already-redeemed on purpose.
     */
    suspend fun pair(
        address: AgentAddress,
        code: String,
        deviceName: String,
    ): ApiResult<PairedHost> {
        val unauthenticated = CueSeekApiFactory.create(
            address = address,
            tokens = TokenProvider.None,
            http = http,
        )

        val pairing = when (val result = unauthenticated.pair(code, deviceName, Platform.Android)) {
            is ApiResult.Failure -> return result
            is ApiResult.Success -> result.value
        }

        // The token is issued exactly once and is not recoverable. From here on, failing
        // without either storing or explicitly discarding it would strand the user with a
        // consumed code and no credential.
        val authenticated = CueSeekApiFactory.create(
            address = address,
            tokens = TokenProvider { pairing.token },
            http = http,
        )

        val system = when (val result = authenticated.system()) {
            is ApiResult.Failure -> return result
            is ApiResult.Success -> result.value
        }

        val host = PairedHost(
            hostId = system.hostId,
            address = address,
            hostname = system.hostname,
            agentVersion = system.agentVersion,
            apiVersion = system.apiVersion,
            deviceId = pairing.device.id,
            deviceName = pairing.device.name,
            scopes = pairing.device.scopes,
            pairedAt = clock.instant(),
        )

        hosts.save(host, pairing.token)
        return ApiResult.Success(host)
    }
}
