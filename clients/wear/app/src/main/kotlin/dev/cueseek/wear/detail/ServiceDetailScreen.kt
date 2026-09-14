package dev.cueseek.wear.detail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.wear.compose.foundation.lazy.TransformingLazyColumn
import androidx.wear.compose.foundation.lazy.rememberTransformingLazyColumnState
import androidx.wear.compose.material3.LinearProgressIndicator
import androidx.wear.compose.material3.MaterialTheme
import androidx.wear.compose.material3.ScreenScaffold
import androidx.wear.compose.material3.Text
import dev.cueseek.core.design.CueSeekStatus
import dev.cueseek.core.design.status.statusStyle
import dev.cueseek.core.model.Service
import dev.cueseek.wear.theme.WearType

/**
 * One service, full height.
 *
 * # What a wrist gets that a row cannot
 *
 * The roster row says a service is healthy and roughly what it is doing. This says **why**:
 * the agent's own word for its state, the reasons behind it, and one concrete thing it is
 * working on — the session or the transfer, not a list of them.
 *
 * # What is deliberately absent
 *
 * No list. The phone shows every session and every torrent because a thumb can flick through
 * twelve rows in a second; twelve rotations of a crown is not the same interaction. One item
 * stands for the rest, and the count says how many — see [focusedSession] and
 * [focusedTransfer] for which one, and why choosing beats scrolling here.
 *
 * No actions either: those are M5.6, and a destructive control that appeared before its
 * confirmation was designed would be the wrong thing to ship first.
 *
 * Nothing here asks which service it is.
 */
@Composable
fun ServiceDetailScreen(service: Service, stale: Boolean) {
    val listState = rememberTransformingLazyColumnState()
    val style = statusStyle(service.health.status, stale)

    ScreenScaffold(scrollState = listState) { contentPadding ->
        TransformingLazyColumn(
            state = listState,
            contentPadding = contentPadding,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            item {
                Column(
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(2.dp),
                    modifier = Modifier.padding(bottom = 6.dp),
                ) {
                    Text(
                        text = service.name,
                        style = MaterialTheme.typography.titleMedium,
                        textAlign = TextAlign.Center,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Text(
                        text = style.label,
                        style = MaterialTheme.typography.bodySmall,
                        color = style.content,
                        textAlign = TextAlign.Center,
                    )
                    // The agent's own word, when it has one. Verbatim and unmapped: a
                    // client showing "firewalled" is showing qBittorrent's word, not a
                    // paraphrase of it.
                    service.health.reportedStatus?.takeIf { it.isNotBlank() }?.let {
                        Text(
                            text = it,
                            style = WearType.DataSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            textAlign = TextAlign.Center,
                        )
                    }
                }
            }

            // Reasons. A reason is not necessarily a problem — a healthy service can carry
            // `pending_restart` — so these are not styled as errors.
            items(
                count = service.health.reasons.size,
                key = { service.health.reasons[it].code },
            ) { index ->
                Text(
                    text = service.health.reasons[index].message,
                    style = MaterialTheme.typography.bodyExtraSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(bottom = 4.dp),
                )
            }

            focusedSession(service.nowPlaying)?.let { session ->
                item {
                    Section(
                        label = "Playing",
                        count = service.nowPlaying?.sessions ?: 0,
                    ) {
                        Text(
                            text = session.title,
                            style = MaterialTheme.typography.bodyMedium,
                            maxLines = 2,
                            overflow = TextOverflow.Ellipsis,
                        )
                        // Subtitle, user and client are each absent whenever the service did
                        // not supply them — never synthesised into something plausible.
                        listOfNotNull(session.subtitle, session.user, session.client)
                            .takeIf { it.isNotEmpty() }
                            ?.let {
                                Text(
                                    text = it.joinToString(" · "),
                                    style = MaterialTheme.typography.bodyExtraSmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                )
                            }
                        playbackClock(session.positionSeconds, session.durationSeconds)?.let {
                            Text(
                                text = if (session.paused) "$it · paused" else it,
                                style = WearType.DataSmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
            }

            focusedTransfer(service.transfers)?.let { transfer ->
                item {
                    Section(
                        label = "Transferring",
                        count = service.transfers?.active ?: 0,
                    ) {
                        Text(
                            text = transfer.name,
                            style = MaterialTheme.typography.bodyMedium,
                            maxLines = 2,
                            overflow = TextOverflow.Ellipsis,
                        )
                        LinearProgressIndicator(
                            progress = { transfer.progress.coerceIn(0f, 1f) },
                            modifier = Modifier.fillMaxWidth(),
                        )
                        Text(
                            text = "${(transfer.progress * 100).toInt()}% · ${transfer.state}",
                            style = WearType.DataSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}

/**
 * @param count how many items this one stands for. Shown only when there is more than one,
 *   because "1 of 1" is a longer way of saying nothing — and shown at all because a screen
 *   that silently hid four sessions would be lying by omission.
 */
@Composable
private fun Section(label: String, count: Int, content: @Composable () -> Unit) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 6.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        Text(
            text = if (count > 1) "$label · $count" else label,
            style = MaterialTheme.typography.labelSmall,
            color = CueSeekStatus.colors.beat,
        )
        content()
    }
}
