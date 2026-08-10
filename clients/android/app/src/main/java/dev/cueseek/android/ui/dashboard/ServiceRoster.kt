package dev.cueseek.android.ui.dashboard

import androidx.compose.animation.animateColorAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.LocalIndication
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.cueseek.android.ui.capability.observedPhrase
import dev.cueseek.core.design.icon.CueSeekIcons
import dev.cueseek.core.design.status.StatusMark
import dev.cueseek.core.design.status.statusStyle
import dev.cueseek.core.design.token.CueSeekShapes
import dev.cueseek.core.design.token.CueSeekSpacing
import dev.cueseek.core.design.token.CueSeekType
import dev.cueseek.core.model.PairedHost
import dev.cueseek.core.model.Service
import java.time.Instant

/**
 * Every service, in one container.
 *
 * One surface holding many rows rather than a card per service. A card each pays for
 * padding twice and caps a phone at three visible services; this fits eight or nine and
 * still reads as a deliberate object.
 *
 * Depth differs by theme on purpose. Light gets a soft shadow because it has a ground to
 * cast onto; dark gets none, because a shadow on a dark page is invisible and the tonal
 * step is what does the work there.
 */
@Composable
fun ServiceRoster(
    services: List<Service>,
    host: PairedHost,
    stale: Boolean,
    now: Instant,
    onServiceClick: (Service) -> Unit,
    modifier: Modifier = Modifier,
) {
    val dark = isSystemInDarkTheme()

    Surface(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = CueSeekSpacing.rosterInset)
            .then(
                if (dark || stale) Modifier
                else Modifier.shadow(2.dp, CueSeekShapes.Roster, clip = false)
            ),
        shape = CueSeekShapes.Roster,
        // Stale removes the fill entirely: the surface de-materialises rather than merely
        // changing colour, which is what makes "unverified" feel like a loss of substance.
        color = if (stale) {
            MaterialTheme.colorScheme.surface
        } else if (dark) {
            MaterialTheme.colorScheme.surfaceContainer
        } else {
            MaterialTheme.colorScheme.surfaceContainerLowest
        },
        border = if (stale) {
            androidx.compose.foundation.BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant)
        } else {
            null
        },
    ) {
        Column {
            services.forEachIndexed { index, service ->
                if (index > 0) {
                    HorizontalDivider(
                        // Dark drops divider contrast almost to nothing and separates by
                        // tone and extra row height instead.
                        color = MaterialTheme.colorScheme.outlineVariant.copy(
                            alpha = if (dark) 0.35f else 1f,
                        ),
                        modifier = Modifier.padding(start = CueSeekSpacing.textStart),
                    )
                }
                ServiceRow(
                    service = service,
                    host = host,
                    stale = stale,
                    now = now,
                    onClick = { onServiceClick(service) },
                )
            }
        }
    }
}

@Composable
private fun ServiceRow(
    service: Service,
    host: PairedHost,
    stale: Boolean,
    now: Instant,
    onClick: () -> Unit,
) {
    val dark = isSystemInDarkTheme()
    val style = statusStyle(service.health.status, stale)
    val interaction = remember { MutableInteractionSource() }

    val supporting = when {
        stale -> "Last verified " + service.health.observedAt.toString()
            .substringAfter('T').take(8)

        service.health.reasons.isNotEmpty() -> service.health.reasons.first().message
        else -> style.label
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(
                interactionSource = interaction,
                indication = LocalIndication.current,
                onClick = onClick,
                onClickLabel = "Open ${service.name}",
            )
            .padding(
                start = CueSeekSpacing.rowHorizontal,
                end = 4.dp,
                top = if (dark) CueSeekSpacing.rowVerticalDark else CueSeekSpacing.rowVertical,
                bottom = if (dark) CueSeekSpacing.rowVerticalDark else CueSeekSpacing.rowVertical,
            )
            .clearAndSetSemantics {
                contentDescription = "${service.name}, ${style.label}"
            },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        StatusMark(status = service.health.status, stale = stale)
        Spacer(Modifier.width(CueSeekSpacing.markGap))

        Column(Modifier.weight(1f)) {
            Text(
                service.name,
                style = MaterialTheme.typography.titleMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                supporting,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }

        Text(
            if (stale) "—" else observedPhrase(service.health.observedAt, now).removeSuffix(" ago"),
            style = CueSeekType.Data.Small,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.defaultMinSize(minWidth = 24.dp),
        )

        // Reserved even when absent, so the age column stays true down the list. A service
        // with no `control` capability has nothing to press, and pretending otherwise would
        // produce a 403 the user could not have avoided.
        Spacer(Modifier.width(4.dp))
        if (service.actions.isNotEmpty() && host.canControlServices()) {
            Icon(
                CueSeekIcons.Restart,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(18.dp).padding(end = 0.dp),
            )
            Spacer(Modifier.width(12.dp))
        } else {
            Spacer(Modifier.width(30.dp))
        }
    }
}
