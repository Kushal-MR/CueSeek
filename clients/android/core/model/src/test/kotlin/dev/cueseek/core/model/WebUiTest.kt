package dev.cueseek.core.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The composition rule, and everything it refuses.
 *
 * These are the tests that matter most in the module, because [urlFor]'s output is handed
 * to an `ACTION_VIEW` intent. A defect here is not a rendering glitch; it is the app
 * opening somewhere the operator never configured.
 */
class WebUiTest {

    private val tailnet = AgentAddress("100.83.12.4", 7777)
    private val jellyfin = WebUi(scheme = "http", port = 8096, path = "/")

    @Test
    fun `takes the host from the pairing and the port from the service`() {
        // The agent listens on 7777 and Jellyfin on 8096. Taking both ports from one side
        // is the obvious mistake, and it would produce a URL that reaches CueSeek's own
        // API and renders as JSON in a browser.
        assertEquals("http://100.83.12.4:8096/", jellyfin.urlFor(tailnet))
    }

    @Test
    fun `the same configuration follows the address the client actually used`() {
        // One config, two networks. This is the property that makes the no-origin design
        // practical rather than merely safe: nothing about the LAN case is configured.
        assertEquals("http://192.168.1.20:8096/", jellyfin.urlFor(AgentAddress("192.168.1.20", 7777)))
        assertEquals("http://home.example:8096/", jellyfin.urlFor(AgentAddress("home.example", 9999)))
    }

    @Test
    fun `honours a non-root path and https`() {
        val proxied = WebUi(scheme = "https", port = 443, path = "/media/web/index.html")
        assertEquals("https://100.83.12.4:443/media/web/index.html", proxied.urlFor(tailnet))
    }

    @Test
    fun `an empty path means the root`() {
        assertEquals("http://100.83.12.4:8096/", jellyfin.copy(path = "").urlFor(tailnet))
    }

    @Test
    fun `accepts a scheme the agent did not normalise`() {
        assertEquals("http://100.83.12.4:8096/", jellyfin.copy(scheme = "HTTP").urlFor(tailnet))
    }

    @Test
    fun `refuses a scheme that is not http or https`() {
        // The one that matters: a `javascript:` URL handed to a browser is code execution.
        assertNull(jellyfin.copy(scheme = "javascript").urlFor(tailnet))
        assertNull(jellyfin.copy(scheme = "file").urlFor(tailnet))
        assertNull(jellyfin.copy(scheme = "intent").urlFor(tailnet))
        assertNull(jellyfin.copy(scheme = "").urlFor(tailnet))
    }

    @Test
    fun `refuses a port outside the valid range`() {
        assertNull(jellyfin.copy(port = 0).urlFor(tailnet))
        assertNull(jellyfin.copy(port = -1).urlFor(tailnet))
        assertNull(jellyfin.copy(port = 65536).urlFor(tailnet))
    }

    @Test
    fun `refuses a path that could replace the host`() {
        // "//evil.example/x" appended to "http:" is protocol-relative and silently changes
        // origin. This is the whole reason the path is validated rather than concatenated.
        assertNull(jellyfin.copy(path = "//evil.example/x").urlFor(tailnet))
        assertNull(jellyfin.copy(path = "https://evil.example").urlFor(tailnet))
        assertNull(jellyfin.copy(path = "relative").urlFor(tailnet))
    }

    @Test
    fun `an at sign inside the path is not an authority and stays allowed`() {
        // Recorded so the rule above is understood as narrow. Once the authority is
        // already present, "@" is an ordinary path character; rejecting it would break
        // real reverse-proxy prefixes for no security gain.
        assertEquals(
            "http://100.83.12.4:8096/x@y",
            jellyfin.copy(path = "/x@y").urlFor(tailnet),
        )
    }

    @Test
    fun `refuses a paired address that is not a bare host`() {
        assertNull(jellyfin.urlFor(AgentAddress("", 7777)))
        assertNull(jellyfin.urlFor(AgentAddress("host/../x", 7777)))
        assertNull(jellyfin.urlFor(AgentAddress("user@evil.example", 7777)))
    }
}
