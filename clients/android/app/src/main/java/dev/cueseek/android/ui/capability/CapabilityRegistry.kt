package dev.cueseek.android.ui.capability

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.cueseek.android.ui.observedPhrase
import dev.cueseek.core.design.status.StatusMark
import dev.cueseek.core.design.token.CueSeekType
import dev.cueseek.core.model.Capability
import dev.cueseek.core.model.Service

/** Renders one capability of one service. */
typealias CapabilityContent = @Composable (service: Service, stale: Boolean) -> Unit

/**
 * Capability id to renderer.
 *
 * This map is the whole point of ADR-0007 and ADR-0005. The agent says *what* a service
 * can do; the client decides what that looks like here, and looks it up by id.
 *
 * **Branching on service id is a review-blocking defect.** `when (service.id)` would throw
 * away capability discovery entirely: adding qBittorrent would mean editing the dashboard
 * instead of adding a renderer, and a Wear build would need the same edit again. There is a
 * test that greps for it.
 *
 * An id with no renderer is not an error. A client will meet capabilities that postdate it
 * for the whole life of the project — permanently, not transitionally — so the fallback is
 * a first-class path, not a safety net.
 */
object CapabilityRegistry {

    private val renderers: Map<String, CapabilityContent> = mapOf(
        "health" to { service, stale -> HealthCapability(service, stale) },
        "control" to { service, _ -> ControlCapability(service) },
        "web_ui" to { service, _ -> WebUiCapability(service) },
        "now_playing" to { service, stale -> NowPlayingCapability(service, stale) },
        "transfers" to { service, stale -> TransfersCapability(service, stale) },
    )

    fun rendererFor(id: String): CapabilityContent? = renderers[id]

    /** Ids this build can draw. Exposed for tests, not for branching. */
    val known: Set<String> get() = renderers.keys
}

/**
 * Renders a capability, or says honestly that it cannot.
 *
 * The unknown case shows the agent's own [Capability.label] — which is exactly why the
 * contract carries one — so an old client meeting `immich_jobs` renders "Immich Jobs ·
 * update CueSeek to view this" rather than an empty box or a crash.
 */
@Composable
fun CapabilitySection(
    capability: Capability,
    service: Service,
    stale: Boolean,
    modifier: Modifier = Modifier,
) {
    val renderer = CapabilityRegistry.rendererFor(capability.id)

    Column(modifier = modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            capability.label,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (renderer != null) {
            renderer(service, stale)
        } else {
            UnrenderableCapability()
        }
    }
}

@Composable
private fun UnrenderableCapability() {
    Text(
        "Update CueSeek to view this",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun HealthCapability(service: Service, stale: Boolean) {
    val health = service.health
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            StatusMark(status = health.status, stale = stale)
            Column {
                Text(
                    if (stable(stale)) health.status.name else "Unverified",
                    style = MaterialTheme.typography.titleMedium,
                )
                Text(
                    // Rendered from observed_at, never from arrival time: the agent serves
                    // cached state by design, so when it arrived says nothing about how
                    // current it is.
                    observedPhrase(health.observedAt),
                    style = CueSeekType.Data.Small,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        if (health.reachable) {
            Text(
                "The agent reached it at that moment.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        health.reportedStatus?.let {
            Text(
                "It reports: $it",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        health.reasons.forEach { reason ->
            Text(
                "${reason.code} — ${reason.message}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 2.dp),
            )
        }
    }
}

@Composable
private fun ControlCapability(service: Service) {
    Text(
        if (service.actions.isEmpty()) {
            "No actions available."
        } else {
            service.actions.joinToString { it.label }
        },
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

private fun stable(stale: Boolean) = !stale

/**
 * The web interface, described rather than linked.
 *
 * It had no renderer until M3.5, which meant the sheet told the reader to "update CueSeek
 * to view this" for a capability the app has fully supported since M3.2 — the one message
 * the fallback must never show for something that works. Nobody saw it because the sheet
 * itself was unreachable for any service with a web_ui; adding the Details route exposed it.
 *
 * No link here on purpose. The ⋮ menu already opens it, and a second tappable route to the
 * same place would be one more thing to keep in step for no gain. Saying where it is and
 * how to reach it is the useful half.
 */
@Composable
private fun WebUiCapability(service: Service) {
    val webUi = service.webUi
    Text(
        if (webUi == null) {
            // Advertised without a payload. The agent should not do this, so say so
            // plainly rather than rendering an empty section.
            "Advertised, but the agent sent no address."
        } else {
            "Served over ${webUi.scheme} on port ${webUi.port}. " +
                "Open it from the row's ⋮ menu."
        },
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}
