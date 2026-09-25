package dev.cueseek.wear.power

import dev.cueseek.core.model.Action
import dev.cueseek.core.model.ActionRisk
import dev.cueseek.core.model.Capability
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.NowPlaying
import dev.cueseek.core.model.Scope
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.Transfers
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/**
 * Whether a wrist may end a machine.
 *
 * These are the cases M5.7 exists to decide, and they are worth holding in a test rather
 * than in review: every one of them is a *silent* behaviour. Nothing on screen distinguishes
 * "correctly hidden" from "accidentally dropped", which is precisely the state the watch was
 * already in — carrying the agent's power actions in every snapshot since M5.4 and rendering
 * none of them because nobody had written the screen yet.
 */
class HostPowerAccessTest {

    private val reboot = Action("reboot", "Reboot", ActionRisk.Destructive, "The machine will restart.")
    private val shutdown = Action("poweroff", "Shut down", ActionRisk.Destructive, null)

    // ------------------------------------------------------------------ the gate

    /**
     * The default a watch is paired with, and the one that matters most (ADR-0014). The
     * agent hands these actions to every caller holding `read`, so a client that rendered
     * what it received would offer a control the agent would then refuse.
     */
    @Test
    fun `a watch without the grant is offered nothing, even though the agent sent the actions`() {
        val access = powerAccess(
            scopes = setOf(Scope.Read, Scope.ServiceControl),
            hostActions = listOf(reboot, shutdown),
        )
        assertEquals(PowerAccess.Ungranted, access)
    }

    @Test
    fun `a watch granted host power is offered exactly what the agent offers`() {
        val access = powerAccess(
            scopes = setOf(Scope.Read, Scope.ServiceControl, Scope.HostPower),
            hostActions = listOf(reboot, shutdown),
        )
        assertEquals(PowerAccess.Offered(listOf(reboot, shutdown)), access)
    }

    /**
     * The distinction the agent's design preserves by returning the same list to everybody:
     * "this agent cannot" and "you were not allowed" are two different problems with two
     * different fixes, and a client that collapsed them would send somebody to re-pair a
     * watch over an agent that was never going to offer power at all.
     */
    @Test
    fun `granted but offered nothing is its own answer, not the ungranted one`() {
        val access = powerAccess(
            scopes = setOf(Scope.Read, Scope.HostPower),
            hostActions = emptyList(),
        )
        assertEquals(PowerAccess.NoneOffered, access)
    }

    /** Scopes are not tiers. `host.power` alone is a grant, whatever else is missing. */
    @Test
    fun `host power does not depend on holding any other scope`() {
        val access = powerAccess(setOf(Scope.HostPower), listOf(reboot))
        assertTrue(access is PowerAccess.Offered)
    }

    /**
     * The whole point of the gate. `service.control` restarts a unit; it does not reach the
     * machine, and a watch that treated one as implying the other would be widening a grant
     * the operator chose narrowly.
     */
    @Test
    fun `controlling services does not imply powering the machine`() {
        assertEquals(
            PowerAccess.Ungranted,
            powerAccess(setOf(Scope.Read, Scope.ServiceControl, Scope.DevicesManage), listOf(reboot)),
        )
    }

    @Test
    fun `a device with no scopes at all is offered nothing`() {
        assertEquals(PowerAccess.Ungranted, powerAccess(emptySet(), listOf(reboot)))
    }

    // ------------------------------------------------------------------ what it interrupts

    @Test
    fun `an idle machine says nothing rather than saying it is idle`() {
        assertNull(wearBusySummary(emptyList()))
        assertNull(wearBusySummary(listOf(service(), service())))
    }

    @Test
    fun `playback and transfers are counted across every service`() {
        val services = listOf(
            service(nowPlaying = playing(sessions = 2)),
            service(nowPlaying = playing(sessions = 1)),
            service(transfers = transferring(active = 3)),
        )
        assertEquals("3 playing · 3 transferring", wearBusySummary(services))
    }

    /** One kind of activity is one clause. The separator only earns its width with two. */
    @Test
    fun `a single kind of activity does not carry a separator`() {
        assertEquals("1 playing", wearBusySummary(listOf(service(nowPlaying = playing(1)))))
        assertEquals("2 transferring", wearBusySummary(listOf(service(transfers = transferring(2)))))
    }

    /**
     * A capability the agent could not read is absent, not zero — the rule the whole project
     * turns on. It must not add to a count that somebody is about to weigh a shutdown
     * against, and it must not make the line appear at all on its own.
     */
    @Test
    fun `a service whose activity could not be read contributes nothing`() {
        assertNull(wearBusySummary(listOf(service(nowPlaying = null, transfers = null))))
        assertEquals(
            "1 playing",
            wearBusySummary(listOf(service(nowPlaying = playing(1)), service())),
        )
    }

    /**
     * Asked and nothing is happening, which is different from not having asked. Both produce
     * no line here, and that is right — the line is about what would be interrupted.
     */
    @Test
    fun `an answered zero reads the same as silence`() {
        assertNull(wearBusySummary(listOf(service(nowPlaying = playing(0), transfers = transferring(0)))))
    }

    /**
     * Transfers are counted by `active`, not by `total`. A stack of finished torrents is not
     * something a shutdown interrupts, and saying it was would make the warning cry wolf on
     * every machine anybody actually uses.
     */
    @Test
    fun `finished transfers are not counted as interrupted work`() {
        val settled = Transfers(
            active = 0,
            total = 40,
            downloadRateBytes = 0,
            uploadRateBytes = 0,
            items = emptyList(),
        )
        assertNull(wearBusySummary(listOf(service(transfers = settled))))
    }

    // ------------------------------------------------------------------ the claim (M5.9)

    /**
     * The distinction this whole type exists for. Both render as an empty line, and on a
     * screen whose buttons end a machine, *"nothing is running"* and *"I have no idea what
     * is running"* are the two sentences that must never be confused.
     */
    @Test
    fun `a stale reading is not the same as a quiet machine`() {
        val idle = listOf(service())
        assertEquals(BusyClaim.Quiet, busyClaim(idle, stale = false))
        assertEquals(BusyClaim.Unknown, busyClaim(idle, stale = true))
    }

    /** Staleness withdraws the claim rather than qualifying it. A count from a dead reading is not a count. */
    @Test
    fun `a stale reading withdraws a busy summary it would otherwise have made`() {
        val busy = listOf(service(nowPlaying = playing(2)))
        assertEquals(BusyClaim.Busy("2 playing"), busyClaim(busy, stale = false))
        assertEquals(BusyClaim.Unknown, busyClaim(busy, stale = true))
    }

    @Test
    fun `a fresh reading reports what is running`() {
        val mixed = listOf(service(nowPlaying = playing(1)), service(transfers = transferring(2)))
        assertEquals(BusyClaim.Busy("1 playing · 2 transferring"), busyClaim(mixed, stale = false))
    }

    /** No services at all is quiet, not unknown — the agent answered, and the answer was nothing. */
    @Test
    fun `an empty roster is quiet rather than unknown`() {
        assertEquals(BusyClaim.Quiet, busyClaim(emptyList(), stale = false))
    }

    // ------------------------------------------------------------------ fixtures

    private fun service(
        nowPlaying: NowPlaying? = null,
        transfers: Transfers? = null,
    ) = Service(
        id = "anything",
        name = "Anything",
        capabilities = listOf(Capability(id = "health", label = "Health")),
        health = Health(
            status = HealthStatus.Healthy,
            reachable = true,
            reportedStatus = null,
            reasons = emptyList(),
            observedAt = Instant.EPOCH,
        ),
        actions = emptyList(),
        nowPlaying = nowPlaying,
        transfers = transfers,
    )

    private fun playing(sessions: Int) = NowPlaying(
        sessions = sessions,
        transcoding = 0,
        items = emptyList(),
    )

    private fun transferring(active: Int) = Transfers(
        active = active,
        total = active,
        downloadRateBytes = 0,
        uploadRateBytes = 0,
        items = emptyList(),
    )
}
