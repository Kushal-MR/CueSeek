package dev.cueseek.wear.dashboard

import dev.cueseek.core.model.Capability
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.NowPlaying
import dev.cueseek.core.model.PlaybackSession
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.TransferItem
import dev.cueseek.core.model.Transfers
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.io.IOException
import java.time.Instant

class WearCapabilityTest {

    // ------------------------------------------------------------------ the ADR-0005 guard

    /**
     * The Wear sibling of the phone's guard, and the reason M5.4b is not just a layout.
     *
     * `when (service.id)` anywhere in the UI discards capability discovery: adding an adapter
     * would mean editing this client instead of adding a renderer, and **every** client would
     * need the same edit. M3 proved the opposite — qBittorrent reached the phone with no
     * client release at all — and that property only survives while nothing branches on
     * identity.
     *
     * A second client is exactly where this rule decays, because the shortcut is cheapest in
     * the file nobody has reviewed yet. So the phone's test gets a twin rather than the
     * phone's test being assumed to cover both.
     */
    @Test
    fun `no watch UI branches on service identity`() {
        val uiRoot = File("src/main/kotlin/dev/cueseek/wear")
        if (!uiRoot.exists()) throw IOException("cannot find ${uiRoot.absolutePath}")

        val banned = listOf(
            Regex("""when\s*\(\s*\w*[Ss]ervice\.id"""),
            Regex("""when\s*\(\s*serviceId"""),
            Regex("""\bservice\.id\s*==\s*"""),
            Regex("""\bserviceId\s*==\s*""""),
        )

        val offenders = mutableListOf<String>()
        uiRoot.walkTopDown().filter { it.extension == "kt" }.forEach { file ->
            file.readLines().forEachIndexed { i, line ->
                val trimmed = line.trim()
                // Comments are skipped, or this fails on the doc comment explaining it.
                // Scanning prose for banned code is how a guard cries wolf.
                val isComment = trimmed.startsWith("//") || trimmed.startsWith("*") ||
                    trimmed.startsWith("/*")
                if (!isComment && banned.any { it.containsMatchIn(line) }) {
                    offenders += "${file.name}:${i + 1}: $trimmed"
                }
            }
        }

        assertTrue(
            "Watch UI must render from capabilities, never from which service it is:\n" +
                offenders.joinToString("\n"),
            offenders.isEmpty(),
        )
    }

    // ------------------------------------------------------------------ the activity line

    @Test
    fun `a service with no activity capability says nothing`() {
        assertNull(wearActivityLine(service()))
    }

    /**
     * Idle is silence, not "0 playing".
     *
     * A row that reports an absence spends its only line on a non-event, and the status mark
     * beside it already covers "everything is fine". The phone makes the same choice; if
     * these two ever diverge it is a defect rather than a form-factor difference.
     */
    @Test
    fun `an idle service says nothing`() {
        assertNull(wearActivityLine(service(nowPlaying = playing(sessions = 0, transcoding = 0))))
        assertNull(wearActivityLine(service(transfers = moving(active = 0, total = 0))))
    }

    @Test
    fun `playback reports sessions`() {
        assertEquals(
            "2 playing",
            wearActivityLine(service(nowPlaying = playing(sessions = 2, transcoding = 0))),
        )
    }

    /** Transcoding is the number that explains a hot machine, so it is named when nonzero. */
    @Test
    fun `transcoding is named only when it is happening`() {
        assertEquals(
            "2 playing +1 tc",
            wearActivityLine(service(nowPlaying = playing(sessions = 2, transcoding = 1))),
        )
    }

    @Test
    fun `transfers report what is moving`() {
        assertEquals(
            "3 active",
            wearActivityLine(service(transfers = moving(active = 3, total = 12))),
        )
    }

    /**
     * A media server that also moves files is a real configuration, and choosing one of the
     * two to show would be arbitrary.
     */
    @Test
    fun `a service doing both gets both`() {
        assertEquals(
            "1 playing · 2 active",
            wearActivityLine(
                service(
                    nowPlaying = playing(sessions = 1, transcoding = 0),
                    transfers = moving(active = 2, total = 2),
                ),
            ),
        )
    }

    /**
     * The whole reason this is not the phone's line, asserted as the decision rather than as
     * a character count.
     *
     * A first version of this test picked 24 characters out of the air and failed on a line
     * that fits perfectly well — 27 characters of 10sp proportional type in roughly 187dp of
     * row. The number was invented, so the test was testing the number.
     *
     * What is actually being decided is *what the watch leaves out*: the phone renders this
     * same state as `3 of 12 active · ↓ 4.2 MB/s`, carrying a total and a transfer rate that
     * do not survive the width here. Those two omissions are the design; a length bound is
     * only a sanity rail around it.
     */
    @Test
    fun `the watch line drops what the phone can afford and the watch cannot`() {
        val busy = service(
            nowPlaying = playing(sessions = 3, transcoding = 2),
            transfers = moving(active = 12, total = 40),
        )
        val line = wearActivityLine(busy)!!

        // The fixture supplies a 4.2 MB/s download rate precisely so this can assert it is
        // absent rather than assume it.
        assertFalse("a transfer rate reached the watch row: \"$line\"", line.contains("/s"))
        assertFalse("a transfer total reached the watch row: \"$line\"", line.contains(" of "))

        // A rail, not the claim. Comfortably above what fits, so it catches a future line
        // that runs away without failing on one that merely grew a word.
        assertTrue("unexpectedly long: \"$line\" (${line.length} chars)", line.length <= 32)
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

    private fun playing(sessions: Int, transcoding: Int) = NowPlaying(
        sessions = sessions,
        transcoding = transcoding,
        items = List(sessions) {
            PlaybackSession(
                id = "s$it",
                title = "Title $it",
                subtitle = null,
                user = null,
                client = null,
                positionSeconds = null,
                durationSeconds = null,
                paused = false,
                transcoding = it < transcoding,
            )
        },
    )

    private fun moving(active: Int, total: Int) = Transfers(
        active = active,
        total = total,
        // Non-zero rates on purpose: the phone would render "↓ 4.2 MB/s" from these, and the
        // watch deliberately does not. If that ever changes, the width test catches it.
        downloadRateBytes = 4_200_000,
        uploadRateBytes = 100_000,
        items = List(total) {
            TransferItem(
                id = "t$it",
                name = "Item $it",
                state = "downloading",
                progress = 0.5f,
                sizeBytes = null,
                downloadRateBytes = null,
                etaSeconds = null,
            )
        },
    )
}
