package dev.cueseek.core.api

import dev.cueseek.core.model.ActionRisk
import dev.cueseek.core.model.ActionStatus
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.Platform
import dev.cueseek.core.model.Scope
import org.junit.Assert.assertEquals
import org.junit.Test
import dev.cueseek.core.api.wire.ActionRisk as WireActionRisk
import dev.cueseek.core.api.wire.ActionStatus as WireActionStatus
import dev.cueseek.core.api.wire.HealthStatus as WireHealthStatus
import dev.cueseek.core.api.wire.Platform as WirePlatform
import dev.cueseek.core.api.wire.Scope as WireScope
import dev.cueseek.core.api.wire.StreamEventType as WireStreamEventType

/**
 * The domain's closed sets against the contract's.
 *
 * Enum-valued fields deliberately cross the wire as strings — the generated enums throw on
 * values they predate, and one unrecognised `risk` must not fail an entire response. That
 * decision has a cost: nothing would otherwise notice a value being *added* to the
 * contract, because the field is a `String` either way and the domain quietly maps it to
 * its unrecognised case.
 *
 * These tests are what pays that cost. The generated enum files are not referenced by any
 * production code; they exist so that a contract change shows up in the drift diff, and so
 * that this test fails when the agent grows a value the domain has no case for.
 *
 * A failure here is not a bug. It is the contract having moved, and the fix is to add the
 * case and decide what it should look like.
 */
class ContractEnumsTest {

    private fun <T : Enum<T>> wireValues(entries: List<T>, value: (T) -> String): Set<String> =
        entries.map(value).toSet()

    @Test
    fun `health statuses match the contract exactly`() {
        assertEquals(
            wireValues(WireHealthStatus.entries, WireHealthStatus::value),
            HealthStatus.entries.map { it.wire }.toSet(),
        )
    }

    @Test
    fun `action risks match the contract, plus the unrecognised case`() {
        assertEquals(
            wireValues(WireActionRisk.entries, WireActionRisk::value),
            ActionRisk.entries.filter { it != ActionRisk.Unrecognised }.map { it.wire }.toSet(),
        )
    }

    @Test
    fun `action statuses match the contract, plus the unrecognised case`() {
        assertEquals(
            wireValues(WireActionStatus.entries, WireActionStatus::value),
            ActionStatus.entries.filter { it != ActionStatus.Unrecognised }.map { it.wire }.toSet(),
        )
    }

    @Test
    fun `platforms match the contract exactly`() {
        assertEquals(
            wireValues(WirePlatform.entries, WirePlatform::value),
            Platform.entries.map { it.wire }.toSet(),
        )
    }

    @Test
    fun `scopes match the contract exactly`() {
        assertEquals(
            wireValues(WireScope.entries, WireScope::value),
            Scope.entries.map { it.wire }.toSet(),
        )
    }

    @Test
    fun `stream event types are the six the client has to handle`() {
        // `host_updated` joined in M3.6, `host_action_progress` in M3.7. They are listed
        // here rather than tolerated silently because the mapper turns an unknown type into
        // `Unrecognised` — correct for an agent newer than this build, and a bug for a type
        // this build was written for. This assertion is what tells the two apart.
        assertEquals(
            setOf(
                "snapshot",
                "service_updated",
                "host_updated",
                "action_progress",
                "host_action_progress",
                "heartbeat",
            ),
            wireValues(WireStreamEventType.entries, WireStreamEventType::value),
        )
    }
}
