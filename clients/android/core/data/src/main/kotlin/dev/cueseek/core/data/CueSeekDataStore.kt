package dev.cueseek.core.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.preferencesDataStore

/**
 * The app's one DataStore.
 *
 * Declared here rather than beside [HostRepository] because more than one repository reads
 * it now, and DataStore permits exactly one instance per file — a second delegate pointed
 * at the same name throws at runtime, not at compile time.
 */
internal val Context.cueSeekStore: DataStore<Preferences> by preferencesDataStore(
    name = HostRepository.STORE_NAME,
)
