package dev.cueseek.wear.tile

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first
import java.time.Instant

/**
 * The last thing the watch actually knew, kept where a tile can reach it.
 *
 * # Why a tile needs its own copy
 *
 * A tile is not the app. The Tiles carousel asks for a layout at moments nobody chose — a
 * swipe, a system refresh, a reboot — and at those moments the app may never have run, its
 * ViewModels do not exist, and there may be no network. Rendering *nothing* in that case
 * would make the tile useless exactly when a glance is what somebody wanted.
 *
 * So the last successful reading is written down. The tile renders it, says how old it is,
 * and stops claiming it is current once it ages out — which is the same rule the dashboard,
 * the detail screen and ambient all follow, applied to the surface where a confident green
 * would do the most damage, because a tile is read in a second without being opened.
 *
 * # What is deliberately not stored
 *
 * The roster, the metrics, the actions — none of it. A tile shows a verdict, a count and an
 * age, so storing more would be keeping a second copy of the agent's state on disk to render
 * three lines. If the tile ever needs more, it should ask the agent, not grow this file.
 */
data class LastReading(
    val verdict: String,
    val healthy: Int,
    val total: Int,
    val observedAt: Instant,
)

private val Context.tileStore by preferencesDataStore(name = "tile_last_reading")

private val VERDICT = stringPreferencesKey("verdict")
private val HEALTHY = intPreferencesKey("healthy")
private val TOTAL = intPreferencesKey("total")
private val OBSERVED_AT = longPreferencesKey("observed_at")

class LastReadingStore(private val context: Context) {

    /** Null when nothing has ever been read — a fresh install whose app has not run. */
    suspend fun read(): LastReading? {
        val prefs = context.tileStore.data.first()
        val verdict = prefs[VERDICT] ?: return null
        val observedAt = prefs[OBSERVED_AT] ?: return null
        return LastReading(
            verdict = verdict,
            healthy = prefs[HEALTHY] ?: 0,
            total = prefs[TOTAL] ?: 0,
            observedAt = Instant.ofEpochSecond(observedAt),
        )
    }

    suspend fun write(reading: LastReading) {
        context.tileStore.edit { prefs ->
            prefs[VERDICT] = reading.verdict
            prefs[HEALTHY] = reading.healthy
            prefs[TOTAL] = reading.total
            prefs[OBSERVED_AT] = reading.observedAt.epochSecond
        }
    }
}
