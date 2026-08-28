package dev.cueseek.android.ui.dashboard

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.LocalIndication
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.cueseek.android.ui.localClock
import dev.cueseek.android.ui.observedPhrase
import dev.cueseek.core.design.status.StatusMark
import dev.cueseek.core.design.status.statusStyle
import dev.cueseek.core.design.token.CueSeekMotion
import dev.cueseek.core.design.token.CueSeekShapes
import dev.cueseek.core.design.token.CueSeekSpacing
import dev.cueseek.core.design.token.CueSeekType
import dev.cueseek.core.model.Action
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
    onInvoke: (Service, Action) -> Unit,
    onOpenWebUi: (Service) -> Unit,
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
            BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant)
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
                    onInvoke = { action -> onInvoke(service, action) },
                    onOpenWebUi = { onOpenWebUi(service) },
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
    onInvoke: (Action) -> Unit,
    onOpenWebUi: () -> Unit,
) {
    val dark = isSystemInDarkTheme()
    val style = statusStyle(service.health.status, stale)
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()

    // Priority, worst news first. A degraded service leads with its reason even while
    // three things are playing — the problem is what the operator is here for, and
    // activity is what they get when there is no problem to report.
    val supporting = when {
        stale -> "Last verified " + localClock(service.health.observedAt)

        service.health.reasons.isNotEmpty() -> service.health.reasons.first().message
        else -> activityLine(service) ?: style.label
    }

    // The row's own press tint, under the ripple. It exists so the two targets read as two
    // objects: pressing the body shades the body and leaves the trailing button alone,
    // which is the only way a user learns, without being told, that the button is separate.
    val pressTint by animateColorAsState(
        targetValue = if (pressed) {
            MaterialTheme.colorScheme.onSurface.copy(alpha = if (dark) 0.06f else 0.04f)
        } else {
            Color.Transparent
        },
        animationSpec = tween(CueSeekMotion.DurationExit),
        label = "rowPress",
    )

    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            modifier = Modifier
                .weight(1f)
                .clickable(
                    interactionSource = interaction,
                    indication = LocalIndication.current,
                    onClick = onClick,
                    onClickLabel = openLabel(service),
                )
                .background(pressTint)
                .padding(
                    start = CueSeekSpacing.rowHorizontal,
                    end = 4.dp,
                    top = if (dark) CueSeekSpacing.rowVerticalDark else CueSeekSpacing.rowVertical,
                    bottom = if (dark) CueSeekSpacing.rowVerticalDark else CueSeekSpacing.rowVertical,
                )
                // Merged rather than cleared. The row used to `clearAndSetSemantics`,
                // which also erases the clickable's own action node — the row read
                // correctly to TalkBack but had nothing left to activate. Merging keeps
                // one node per row *and* keeps the action, so `onClickLabel` above is what
                // gets announced.
                .semantics(mergeDescendants = true) {
                    // The supporting line, not the status label. The two were the same
                    // until activity arrived on the row, and then they diverged: the
                    // screen said "1 playing" while this still said "Healthy". A screen
                    // reader that describes a different row from the one on display is
                    // worse than one that describes less.
                    contentDescription = "${service.name}, $supporting"
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
        }

        // Width is reserved whether or not there is a menu, so the age column stays true
        // down the list. A service the agent offers nothing for has nothing to press, and
        // an enabled button that could only ever 403 is worse than no button.
        // Always rendered now. It used to appear only for a service with actions, which
        // was right when the menu held nothing else; it now also carries the only route to
        // the detail sheet, and a service with no actions still has health worth reading.
        ServiceActionMenu(
            service = service,
            host = host,
            onInvoke = onInvoke,
            onOpenWebUi = onOpenWebUi,
            modifier = Modifier.padding(end = CueSeekSpacing.menuInset),
        )
    }
}

/**
 * What the body tap will do, said before it happens.
 *
 * One destination now, and always the same one. The body used to open the service's own
 * web interface, which put the thing that leaves the app on the largest, easiest target
 * and buried the service's own state behind a menu. On a phone that is backwards: the
 * reason to open CueSeek at all is to see what a service is doing, and the browser is
 * where you go afterwards if the answer warrants it.
 */
private fun openLabel(service: Service): String = "Show ${service.name} details"

/** The trailing slot, at Material's minimum touch target plus the row's own end inset. */
private val TrailingSlotWidth = 48.dp + 4.dp
