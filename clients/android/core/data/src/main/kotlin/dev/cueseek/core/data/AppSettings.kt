package dev.cueseek.core.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map

/**
 * Which theme the user asked for.
 *
 * [System] is the default and is not the same as "light" — it means *follow the device*,
 * which is what most people expect and what changes under them at sunset.
 */
enum class ThemeChoice {
    System,
    Light,
    Dark;

    companion object {
        /** Unknown stored values fall back rather than crashing an app that cannot start. */
        fun fromStored(value: String?): ThemeChoice =
            entries.firstOrNull { it.name == value } ?: System
    }
}

/**
 * Preferences that are about this app rather than about any host.
 *
 * Shares [cueSeekStore] with [HostRepository]: one file, distinct keys. A second DataStore
 * would mean a second file, a second write lock and a second thing to exclude from backup,
 * for four bytes of preference.
 */
class SettingsRepository internal constructor(
    private val store: DataStore<Preferences>,
) {

    constructor(context: Context) : this(context.applicationContext.cueSeekStore)

    val theme: Flow<ThemeChoice> = store.data
        .map { ThemeChoice.fromStored(it[KEY_THEME]) }
        // The theme drives the whole composition; re-emitting an equal value would
        // recompose every screen for nothing.
        .distinctUntilChanged()

    suspend fun setTheme(choice: ThemeChoice) {
        store.edit { it[KEY_THEME] = choice.name }
    }

    private companion object {
        val KEY_THEME = stringPreferencesKey("theme")
    }
}
