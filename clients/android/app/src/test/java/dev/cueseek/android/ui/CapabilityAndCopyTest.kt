package dev.cueseek.android.ui

import dev.cueseek.android.ui.capability.CapabilityRegistry
import dev.cueseek.core.model.ApiError
import java.io.File
import java.io.IOException
import java.time.Instant
import java.time.ZoneId
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class CapabilityAndCopyTest {

    // ------------------------------------------------------------------ capabilities

    @Test
    fun `the capabilities the agent emits today have renderers`() {
        assertNotNull(CapabilityRegistry.rendererFor("health"))
        assertNotNull(CapabilityRegistry.rendererFor("control"))
        assertNotNull(CapabilityRegistry.rendererFor("web_ui"))
        assertNotNull(CapabilityRegistry.rendererFor("now_playing"))
        assertNotNull(CapabilityRegistry.rendererFor("transfers"))
    }

    @Test
    fun `a capability this build predates has no renderer, and that is a normal path`() {
        // `now_playing` and `transfers` landed in M3.5, so the placeholders for them are
        // gone. `immich_jobs` and `sonarr_queue` do not exist at all, and stand in for the
        // permanent case: a client meets capabilities that postdate it for the whole life
        // of the project, and must fall to the honest fallback rather than crash (ADR-0007).
        assertNull(CapabilityRegistry.rendererFor("immich_jobs"))
        assertNull(CapabilityRegistry.rendererFor("sonarr_queue"))
        assertNull(CapabilityRegistry.rendererFor(""))
    }

    // ------------------------------------------------------------------ the ADR-0005 guard

    /**
     * The rule ADR-0005 and ADR-0007 both rest on, enforced rather than reviewed.
     *
     * `when (service.id)` in the UI discards capability discovery: adding qBittorrent would
     * mean editing the dashboard instead of adding a renderer, and every other client would
     * need the same edit. `clients/android/README.md` calls it review-blocking; review is
     * exactly the mechanism that misses a one-line regression six months from now.
     */
    @Test
    fun `no UI code branches on service identity`() {
        val uiRoot = File("src/main/java/dev/cueseek/android/ui")
        if (!uiRoot.exists()) throw IOException("cannot find ${uiRoot.absolutePath}")

        val offenders = mutableListOf<String>()
        val banned = listOf(
            Regex("""when\s*\(\s*\w*[Ss]ervice\.id"""),
            Regex("""when\s*\(\s*serviceId"""),
            Regex("""\bservice\.id\s*==\s*"""),
            Regex("""\bserviceId\s*==\s*""""),
        )

        uiRoot.walkTopDown().filter { it.extension == "kt" }.forEach { file ->
            file.readLines().forEachIndexed { i, line ->
                val trimmed = line.trim()
                // Comments are skipped, or this test fails on the doc comment that
                // explains it. Scanning prose for banned code is how a guard cries wolf.
                val isComment = trimmed.startsWith("//") || trimmed.startsWith("*") ||
                    trimmed.startsWith("/*")
                if (!isComment && banned.any { it.containsMatchIn(line) }) {
                    offenders += "${file.name}:${i + 1}: $trimmed"
                }
            }
        }

        assertTrue(
            "UI must look capabilities up by id, never branch on which service it is:\n" +
                offenders.joinToString("\n"),
            offenders.isEmpty(),
        )
    }

    // ------------------------------------------------------------------ error copy

    @Test
    fun `every failure says what happened and what to do`() {
        val cases = listOf(
            ApiError.Unauthorized(tokenWasSent = true, detail = null),
            ApiError.Unauthorized(tokenWasSent = false, detail = null),
            ApiError.InsufficientScope(null),
            ApiError.InvalidPairingCode(null),
            ApiError.RateLimited(null),
            ApiError.NotFound(null),
            ApiError.ActionInProgress(null),
            ApiError.ActionUnavailable("polkit refused"),
            ApiError.BadRequest(null),
            ApiError.Internal(null),
            ApiError.NotImplemented(null),
            ApiError.Unrecognised("x", "y", 418, null),
            ApiError.Transport(IOException("down")),
            ApiError.Malformed(IOException("bad")),
        )

        cases.forEach { error ->
            val copy = error.explain()
            assertTrue("${error::class.simpleName} has no title", copy.title.isNotBlank())
            assertTrue("${error::class.simpleName} has no body", copy.body.isNotBlank())
            // "Something went wrong" is how an app teaches its user it cannot be trusted
            // to know what happened.
            assertFalse(
                "${error::class.simpleName} is vague",
                copy.title.contains("went wrong", ignoreCase = true),
            )
        }
    }

    @Test
    fun `a rejected pairing code never claims to know why`() {
        val copy = ApiError.InvalidPairingCode(null).explain()
        // Unknown, expired and already-redeemed are merged by the agent on purpose:
        // saying "expired" would reveal the code was once real.
        listOf("expired", "already", "unknown", "used").forEach { word ->
            assertFalse(
                "copy must not claim which kind of rejection it was: contains '$word'",
                (copy.title + copy.body).contains(word, ignoreCase = true),
            )
        }
    }

    @Test
    fun `an unavailable action carries the agent's own words`() {
        val detail = "polkit refused RestartUnit for jellyfin.service"
        val copy = ApiError.ActionUnavailable(detail).explain()
        // The agent's detail names the rule or the missing unit; our paraphrase would be
        // strictly less useful to whoever has to fix it.
        assertEquals(detail, copy.detail)
    }

    @Test
    fun `the two 401s give different advice`() {
        val dead = ApiError.Unauthorized(tokenWasSent = true, detail = null).explain()
        val absent = ApiError.Unauthorized(tokenWasSent = false, detail = null).explain()
        assertFalse("these must not read the same", dead.title == absent.title)
    }

    // ------------------------------------------------------------------ timestamps

    @Test
    fun `age is rendered from when it was observed`() {
        val observed = Instant.parse("2026-08-10T06:00:00Z")
        assertEquals("4s ago", observedPhrase(observed, observed.plusSeconds(4)))
        assertEquals("3m ago", observedPhrase(observed, observed.plusSeconds(200)))
        assertTrue(observedPhrase(observed, observed.plusSeconds(7200)).startsWith("at "))
    }

    @Test
    fun `absolute times are shown in the reader's timezone, not UTC`() {
        // The bug this replaces: the app sliced observed_at's ISO string and printed
        // 04:14:32 to someone whose phone said 09:45. A zone is passed explicitly here
        // because CI runs in UTC, where the bug is invisible.
        val observed = Instant.parse("2026-08-10T06:00:00Z")
        val ist = ZoneId.of("Asia/Kolkata")

        assertEquals("11:30:00", localClock(observed, ist))
        assertEquals("06:00:00", localClock(observed, ZoneId.of("UTC")))
        assertEquals(
            "at 11:30:00",
            observedPhrase(observed, observed.plusSeconds(7200), ist),
        )
    }
}
