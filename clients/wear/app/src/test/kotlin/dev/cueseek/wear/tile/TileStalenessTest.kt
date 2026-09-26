package dev.cueseek.wear.tile

import dev.cueseek.wear.dashboard.DashboardViewModel
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The tile and the app must agree about when a reading stops being believable.
 *
 * They cannot share the constant directly: the app asks `isStale(observedAt)` in Kotlin at
 * build time, while the tile compiles a **dynamic expression** the renderer evaluates later,
 * and that comparison needs a plain `Int` of seconds. So the number exists twice.
 *
 * Twice is once too many, and this is the guard. A tile that said `Unverified` at 60 seconds
 * while the dashboard still said `Running` — or worse, the other way round — would be the
 * console contradicting itself on the same wrist about the same host, which is the failure
 * the shared-verdict rule exists to prevent.
 */
class TileStalenessTest {

    @Test
    fun `the tile's threshold is the app's threshold`() {
        assertEquals(
            DashboardViewModel.STALE_AFTER.seconds,
            CueSeekTileService.STALE_AFTER_SECONDS.toLong(),
        )
    }
}
