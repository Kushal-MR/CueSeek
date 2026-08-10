package dev.cueseek.android

import android.app.Application

/**
 * Owns the [AppContainer] for the process's lifetime.
 *
 * The container holds a connection pool and a decrypted-token cache, both of which are
 * meant to outlive any one screen and neither of which should be rebuilt on rotation.
 */
class CueSeekApplication : Application() {

    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
    }
}
