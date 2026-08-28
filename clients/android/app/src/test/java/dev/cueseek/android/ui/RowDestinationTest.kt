package dev.cueseek.android.ui

import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.WebUi
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Where "Open web interface" sends the user.
 *
 * This resolved the row's *body* tap until the interaction was reversed after use on a
 * phone: the body now inspects the service and the browser moved into the ⋮ menu. The
 * resolution itself is unchanged and still the piece that can be wrong silently, since a
 * refused URL and a working one both end in "the tap did something".
 *
 * The fallback still matters. A menu entry that resolved to nothing would be a dead item,
 * so an unusable URL lands on the detail sheet — the same place the body tap goes.
 */
class RowDestinationTest {

    private val address = AgentAddress("100.83.12.4", 7777)

    private fun service(webUi: WebUi?) = Service(
        id = "jellyfin",
        name = "Jellyfin",
        capabilities = emptyList(),
        health = Health(
            status = HealthStatus.Healthy,
            reachable = true,
            observedAt = Instant.EPOCH,
            reasons = emptyList(),
            reportedStatus = null,
        ),
        actions = emptyList(),
        webUi = webUi,
    )

    @Test
    fun `a configured interface opens in the browser at the paired host`() {
        val to = rowDestination(service(WebUi("http", 8096, "/")), address)
        assertEquals(RowDestination.WebUi("http://100.83.12.4:8096/"), to)
    }

    @Test
    fun `a service with no interface falls back to details rather than doing nothing`() {
        assertEquals(RowDestination.Details, rowDestination(service(null), address))
    }

    @Test
    fun `an interface this client will not open falls back the same way`() {
        // The fallback covers rejection as well as absence. A row that went inert on a
        // malformed value would be indistinguishable from a broken app.
        assertEquals(
            RowDestination.Details,
            rowDestination(service(WebUi("javascript", 8096, "/")), address),
        )
    }

    @Test
    fun `the destination never carries an origin the agent supplied`() {
        // The security property, asserted where it is consumed rather than only where it
        // is composed: whatever the agent sends, the host in the resulting URL is the one
        // this device paired with.
        val hostile = WebUi(scheme = "http", port = 8096, path = "/redirect")
        val to = rowDestination(service(hostile), address) as RowDestination.WebUi
        assertTrue(to.url, to.url.startsWith("http://100.83.12.4:"))
    }
}
