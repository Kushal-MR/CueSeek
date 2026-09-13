package dev.cueseek.wear

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import dev.cueseek.core.data.HostRepository
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import androidx.lifecycle.viewModelScope

/** Which screen the app should be on, derived from what is stored rather than remembered. */
sealed interface Root {
    /** Before the store has answered. Distinct from [Pairing] on purpose — see below. */
    data object Deciding : Root
    data object Pairing : Root
    data object Dashboard : Root
}

/**
 * Decides the first screen.
 *
 * Trivial, and it exists because the obvious version was wrong. M5.3b routed on a
 * `remember { mutableStateOf(false) }` set when pairing succeeded, which meant an
 * already-paired watch showed the pairing screen on every launch: the token was in the
 * store, and nothing ever asked.
 *
 * [Deciding] is a separate state rather than defaulting to [Pairing] while the store loads.
 * Defaulting would flash "Pair with an agent" at somebody who paired last week, every single
 * time they raised their wrist — a wrong answer shown confidently for 100ms is still a wrong
 * answer, and it is the same reason nothing in this project renders `unknown` as healthy.
 */
class RootViewModel(app: Application) : AndroidViewModel(app) {

    private val hosts = HostRepository(app)

    val root: StateFlow<Root> = hosts.selectedHost
        .map { if (it == null) Root.Pairing else Root.Dashboard }
        .stateIn(viewModelScope, SharingStarted.Eagerly, Root.Deciding)
}
