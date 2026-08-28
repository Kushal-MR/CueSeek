package dev.cueseek.android.ui.dashboard

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.cueseek.android.ui.capability.CapabilitySection
import dev.cueseek.core.model.Service

/**
 * One service, in detail.
 *
 * A sheet rather than a screen: inspecting a service is a glance deeper, not a journey,
 * and predictive back handles dismissal for free. Capabilities render through the registry,
 * so this composable never learns what a Jellyfin is.
 *
 * Detail only, deliberately. The row's trailing menu is now the single place a service is
 * operated on, and it is reachable from the same row that opens this sheet. Repeating the
 * buttons here put the same two verbs on screen three times — once in the `control`
 * capability's own summary, once as buttons, once in the menu — and a second entry point
 * to a destructive action is a second thing to keep gated correctly forever.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ServiceSheet(
    service: Service,
    stale: Boolean,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                // Scrollable, because the content is no longer bounded. Health and control
                // fitted on any screen; a transfers list of ten items does not, and without
                // this the sheet simply clipped at whatever the screen height allowed — the
                // eleventh item, the "and N more" line and everything after it were
                // unreachable rather than merely below the fold.
                .verticalScroll(rememberScrollState())
                .padding(start = 24.dp, end = 24.dp, bottom = 32.dp),
            verticalArrangement = Arrangement.spacedBy(20.dp),
        ) {
            Text(service.name, style = MaterialTheme.typography.headlineSmall)

            service.capabilities.forEach { capability ->
                CapabilitySection(capability = capability, service = service, stale = stale)
            }

        }
    }
}
