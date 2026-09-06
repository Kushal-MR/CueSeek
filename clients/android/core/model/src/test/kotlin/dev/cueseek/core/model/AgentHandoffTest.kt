package dev.cueseek.core.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AgentHandoffTest {

    @Test
    fun `round trips an address`() {
        val address = AgentAddress("100.92.18.125", 7777)
        assertEquals(address, AgentHandoff.decode(AgentHandoff.encode(address)))
    }

    @Test
    fun `encodes the format ADR-0014 committed to`() {
        assertEquals(
            "cueseek://pair?host=100.92.18.125&port=7777",
            AgentHandoff.encode(AgentAddress("100.92.18.125", 7777)),
        )
    }

    @Test
    fun `round trips a hostname and a non-default port`() {
        val address = AgentAddress("cueseek-vm.local", 8080)
        assertEquals(address, AgentHandoff.decode(AgentHandoff.encode(address)))
    }

    /**
     * The rule ADR-0014 turns on, enforced by the parser rather than by a sentence.
     *
     * A pairing code is one redemption away from being a credential, and the whole point of
     * the handoff is that no credential crosses between devices. If some future change ever
     * appends `code`, the watch declines the whole payload rather than quietly using the
     * address and ignoring the rest — because the interesting failure is not "bad address",
     * it is "somebody started sending secrets over this channel".
     */
    @Test
    fun `refuses a URI carrying a pairing code`() {
        assertNull(
            AgentHandoff.decode("cueseek://pair?host=100.92.18.125&port=7777&code=J2GX-PC2U"),
        )
        // Order must not matter: a parser that only checked the last parameter would pass
        // the case above and fail this one.
        assertNull(
            AgentHandoff.decode("cueseek://pair?code=J2GX-PC2U&host=100.92.18.125&port=7777"),
        )
    }

    @Test
    fun `refuses anything that is not this URI`() {
        listOf(
            "",
            "   ",
            "https://cueseek.dev/pair?host=1.2.3.4&port=7777",
            "cueseek://other?host=1.2.3.4&port=7777",
            "cueseek://pair?port=7777",
            "cueseek://pair?host=1.2.3.4",
            "cueseek://pair?host=&port=7777",
            "cueseek://pair?host=1.2.3.4&port=notanumber",
            "cueseek://pair?host=1.2.3.4&port=0",
            "cueseek://pair?host=1.2.3.4&port=65536",
            "cueseek://pair?host=1.2.3.4&port=-1",
        ).forEach { assertNull("should have refused: $it", AgentHandoff.decode(it)) }
    }

    @Test
    fun `tolerates surrounding whitespace`() {
        assertEquals(
            AgentAddress("1.2.3.4", 7777),
            AgentHandoff.decode("  cueseek://pair?host=1.2.3.4&port=7777\n"),
        )
    }
}
