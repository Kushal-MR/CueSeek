package dev.cueseek.wear.tile

import android.content.ComponentName
import androidx.compose.ui.graphics.toArgb
import androidx.concurrent.futures.CallbackToFutureAdapter
import androidx.wear.protolayout.ActionBuilders
import androidx.wear.protolayout.ColorBuilders.argb
import androidx.wear.protolayout.DimensionBuilders.dp
import androidx.wear.protolayout.DimensionBuilders.expand
import androidx.wear.protolayout.DimensionBuilders.sp
import androidx.wear.protolayout.LayoutElementBuilders
import androidx.wear.protolayout.ModifiersBuilders
import androidx.wear.protolayout.TypeBuilders
import androidx.wear.protolayout.expression.DynamicBuilders.DynamicInstant
import androidx.wear.protolayout.expression.DynamicBuilders.DynamicString
import androidx.wear.protolayout.ResourceBuilders
import androidx.wear.protolayout.TimelineBuilders
import androidx.wear.tiles.RequestBuilders
import androidx.wear.tiles.TileBuilders
import androidx.wear.tiles.TileService
import com.google.common.util.concurrent.ListenableFuture
import dev.cueseek.core.api.CueSeekApiFactory
import dev.cueseek.core.data.AgentClients
import dev.cueseek.core.data.HostRepository
import dev.cueseek.core.data.ServicesRepository
import dev.cueseek.core.design.token.CueSeekStatusColors
import dev.cueseek.core.model.ApiResult
import dev.cueseek.core.model.Tally
import dev.cueseek.core.model.verdict
import dev.cueseek.wear.dashboard.DashboardViewModel
import dev.cueseek.wear.dashboard.readingAge
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import java.time.Instant

/**
 * The tile: *is everything fine?*, answered without opening anything.
 *
 * # A third renderer over the same capabilities
 *
 * This shares **no UI code** with either client — not a composable, not a theme, not a
 * modifier. A tile is a ProtoLayout tree handed to the system and drawn by the Tiles
 * carousel in its own process, so nothing from the app can cross into it.
 *
 * What it does share is the part that should be shared: the **verdict** comes from
 * `:core:model`, computed by the same function the phone and the watch dashboard use, and
 * the **palette** comes from `:core:design`. A tile that disagreed with the app on the same
 * host would be the console contradicting itself on the user's own wrist. So the judgement
 * is imported and only the drawing is rewritten — which is exactly the split ADR-0010 states
 * and ADR-0007 predicts.
 *
 * # It fetches, and it falls back
 *
 * `onTileRequest` runs when somebody looks at the tile, so fetching here is polling *while
 * visible* — the rule the app already follows (ADR-0004, M5.10). But a tile is asked for at
 * moments nobody chose, often with no network and sometimes before the app has ever run, so
 * a failed fetch must not produce an empty tile. It falls back to [LastReadingStore] and
 * says how old that is.
 *
 * **The fallback is why the age line exists**, and why it is not decoration. A tile is read
 * in a second without being opened, which makes it the surface where a confident green with
 * nothing behind it would do the most damage.
 */
class CueSeekTileService : TileService() {

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    override fun onTileResourcesRequest(
        requestParams: RequestBuilders.ResourcesRequest,
    ): ListenableFuture<ResourceBuilders.Resources> =
        CallbackToFutureAdapter.getFuture { completer ->
            // No images, no custom fonts. Everything on the tile is text in the system
            // face: a tile carries three short lines, and shipping a font file to render
            // them would cost more than it buys.
            completer.set(ResourceBuilders.Resources.Builder().setVersion(RESOURCES).build())
            "tile-resources"
        }

    override fun onTileRequest(
        requestParams: RequestBuilders.TileRequest,
    ): ListenableFuture<TileBuilders.Tile> =
        CallbackToFutureAdapter.getFuture { completer ->
            scope.launch {
                completer.set(runCatching { buildTile() }.getOrElse { failedTile() })
            }
            "tile-request"
        }

    private suspend fun buildTile(): TileBuilders.Tile {
        val store = LastReadingStore(applicationContext)
        val reading = fetch()?.also { store.write(it) } ?: store.read()

        return TileBuilders.Tile.Builder()
            .setResourcesVersion(RESOURCES)
            // How often the system may ask again on its own. Deliberately long: a tile
            // the operator is not looking at should not be waking a radio, and the age
            // line means an older reading is still an honest one. A swipe to the tile
            // asks immediately regardless, which is the case that matters.
            .setFreshnessIntervalMillis(FRESHNESS_MILLIS)
            .setTileTimeline(
                TimelineBuilders.Timeline.fromLayoutElement(layout(reading)),
            )
            .build()
    }

    /**
     * One snapshot, or null.
     *
     * Bounded by a timeout because a tile request is not a screen: the system is waiting to
     * draw, and a watch on a network that will never answer must not hold that open. When it
     * expires the stored reading is rendered with its age, which is a better tile than a
     * spinner and an honest one.
     */
    private suspend fun fetch(): LastReading? = withTimeoutOrNull(FETCH_TIMEOUT_MILLIS) {
        val hosts = HostRepository(applicationContext)
        val host = hosts.selectedHost.first() ?: return@withTimeoutOrNull null
        val services = ServicesRepository(
            AgentClients(hosts, CueSeekApiFactory.sharedHttp()),
        )

        when (val result = services.snapshot(host)) {
            is ApiResult.Failure -> null
            is ApiResult.Success -> {
                val tally = Tally.of(result.value.services)
                LastReading(
                    verdict = verdict(
                        stale = false,
                        services = result.value.services,
                        hostMetrics = result.value.hostMetrics,
                        tally = tally,
                    ),
                    healthy = tally.healthy,
                    total = tally.total,
                    observedAt = Instant.now(),
                )
            }
        }
    }

    private fun layout(reading: LastReading?): LayoutElementBuilders.LayoutElement {
        val colors = CueSeekStatusColors.Dark
        val onSurface = argb(0xFFE3E3DD.toInt())
        val variant = argb(colors.unknown.toArgb())

        val column = LayoutElementBuilders.Column.Builder()
            .setWidth(expand())
            .setHorizontalAlignment(LayoutElementBuilders.HORIZONTAL_ALIGN_CENTER)

        if (reading == null) {
            // Never read anything: a fresh install whose app has not run, or a watch that
            // was never paired. Says which, rather than rendering an empty tile that looks
            // like the tile itself is broken.
            column.addContent(text("CueSeek", 14f, variant))
            column.addContent(spacer(4))
            column.addContent(text("Not paired", 20f, onSurface))
        } else {
            // Elapsed time evaluated **by the renderer**, not by this method.
            //
            // This is the correction that M5.11's hardware test forced. The Tiles carousel
            // caches the layout and only calls back on its own schedule — fifteen minutes
            // here — so an age baked in at build time makes the tile claim "now" for
            // fifteen minutes after the agent has died. That is precisely the stale-green
            // failure this project legislates against, on the one surface read in a second
            // without being opened.
            //
            // A platform time source fixes it without a single extra request: the renderer
            // recomputes these strings itself while the tile sits there.
            val since = DynamicInstant.withSecondsPrecision(reading.observedAt)
                .durationUntil(DynamicInstant.platformTimeWithSecondsPrecision())
            val seconds = since.toIntSeconds()
            val minutes = since.toIntMinutes()

            val staleNow = seconds.gt(STALE_AFTER_SECONDS)

            column.addContent(
                dynamicText(
                    // Static value first: it is what a renderer too old for dynamic
                    // expressions will draw, so it has to be true on its own.
                    fallback = if (DashboardViewModel.isStale(reading.observedAt)) {
                        "Unverified"
                    } else {
                        reading.verdict
                    },
                    dynamic = DynamicString.onCondition(staleNow)
                        .use(DynamicString.constant("Unverified"))
                        .elseUse(DynamicString.constant(reading.verdict)),
                    sizeSp = 20f,
                    colour = onSurface,
                ),
            )
            column.addContent(spacer(4))

            if (reading.total > 0) {
                column.addContent(
                    text("${reading.healthy}/${reading.total} healthy", 13f, variant),
                )
                column.addContent(spacer(2))
            }

            column.addContent(
                dynamicText(
                    fallback = readingAge(reading.observedAt),
                    dynamic = DynamicString.onCondition(seconds.lt(60))
                        .use(DynamicString.constant("now"))
                        .elseUse(
                            DynamicString.onCondition(minutes.lt(60))
                                .use(minutes.format().concat(DynamicString.constant("m ago")))
                                .elseUse(
                                    since.toIntHours().format()
                                        .concat(DynamicString.constant("h ago")),
                                ),
                        ),
                    sizeSp = 13f,
                    colour = variant,
                ),
            )
        }

        return LayoutElementBuilders.Box.Builder()
            .setWidth(expand())
            .setHeight(expand())
            .setModifiers(
                ModifiersBuilders.Modifiers.Builder()
                    .setClickable(
                        ModifiersBuilders.Clickable.Builder()
                            .setId("open")
                            .setOnClick(
                                ActionBuilders.LaunchAction.Builder()
                                    .setAndroidActivity(
                                        ActionBuilders.AndroidActivity.Builder()
                                            .setPackageName(packageName)
                                            .setClassName(MAIN_ACTIVITY)
                                            .build(),
                                    )
                                    .build(),
                            )
                            .build(),
                    )
                    .setSemantics(
                        // The tile is one control, so it gets one label — and it is the
                        // state, not the app's name. A screen reader announcing "CueSeek"
                        // would have said nothing a glance did not already.
                        ModifiersBuilders.Semantics.Builder()
                            .setContentDescription(describe(reading))
                            .build(),
                    )
                    .build(),
            )
            .addContent(column.build())
            .build()
    }

    /** What a screen reader hears. Spelled out, because "3/4" is not a sentence. */
    private fun describe(reading: LastReading?): String = when {
        reading == null -> "CueSeek. Not paired."
        DashboardViewModel.isStale(reading.observedAt) ->
            "CueSeek. Unverified, last read ${readingAge(reading.observedAt)}."
        reading.total > 0 ->
            "CueSeek. ${reading.verdict}. ${reading.healthy} of ${reading.total} services healthy."
        else -> "CueSeek. ${reading.verdict}."
    }

    private fun failedTile(): TileBuilders.Tile = TileBuilders.Tile.Builder()
        .setResourcesVersion(RESOURCES)
        .setFreshnessIntervalMillis(FRESHNESS_MILLIS)
        .setTileTimeline(TimelineBuilders.Timeline.fromLayoutElement(layout(null)))
        .build()

    private fun text(
        text: String,
        sizeSp: Float,
        colour: androidx.wear.protolayout.ColorBuilders.ColorProp,
    ): LayoutElementBuilders.LayoutElement =
        LayoutElementBuilders.Text.Builder()
            .setText(text)
            .setMaxLines(2)
            .setFontStyle(
                LayoutElementBuilders.FontStyle.Builder()
                    .setSize(sp(sizeSp))
                    .setColor(colour)
                    .build(),
            )
            .build()

    /**
     * Text the renderer keeps up to date, with a static value that is true on its own.
     *
     * The fallback is not a formality. A renderer without dynamic-expression support draws
     * it verbatim and never updates it, so it must be correct at the moment the tile was
     * built rather than a placeholder.
     */
    private fun dynamicText(
        fallback: String,
        dynamic: DynamicString,
        sizeSp: Float,
        colour: androidx.wear.protolayout.ColorBuilders.ColorProp,
    ): LayoutElementBuilders.LayoutElement =
        LayoutElementBuilders.Text.Builder()
            .setText(
                TypeBuilders.StringProp.Builder(fallback)
                    .setDynamicValue(dynamic)
                    .build(),
            )
            .setMaxLines(2)
            .setFontStyle(
                LayoutElementBuilders.FontStyle.Builder()
                    .setSize(sp(sizeSp))
                    .setColor(colour)
                    .build(),
            )
            .build()

    private fun spacer(height: Int): LayoutElementBuilders.LayoutElement =
        LayoutElementBuilders.Spacer.Builder().setHeight(dp(height.toFloat())).build()

    companion object {
        private const val RESOURCES = "1"
        private const val MAIN_ACTIVITY = "dev.cueseek.wear.MainActivity"

        /**
         * Fifteen minutes, and the reasoning rather than the number is the point: this is
         * how often the **system** may refresh a tile nobody is looking at. A swipe to the
         * tile asks immediately and is unaffected. Shorter would wake the radio to correct
         * a display nobody is reading, which is the thing M5.10 exists to prevent.
         */
        private const val FRESHNESS_MILLIS = 15L * 60L * 1000L

        /** The system is waiting to draw. It does not wait long, so neither does this. */
        private const val FETCH_TIMEOUT_MILLIS = 5_000L

        /**
         * The same 90 seconds `DashboardViewModel.STALE_AFTER` uses, restated here as an
         * Int because a dynamic expression compares against one. Kept in sync by the test
         * that asserts they agree rather than by anyone remembering.
         */
        const val STALE_AFTER_SECONDS = 90

        /** Asks the system to re-render, for the app to call after a successful poll. */
        fun requestUpdate(context: android.content.Context) {
            getUpdater(context).requestUpdate(CueSeekTileService::class.java)
        }

        private fun getUpdater(context: android.content.Context) =
            androidx.wear.tiles.TileService.getUpdater(context)

        @Suppress("unused")
        private fun component(context: android.content.Context) =
            ComponentName(context, CueSeekTileService::class.java)
    }
}
