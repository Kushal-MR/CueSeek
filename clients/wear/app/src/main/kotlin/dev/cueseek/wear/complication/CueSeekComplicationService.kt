package dev.cueseek.wear.complication

import android.app.PendingIntent
import android.content.Intent
import androidx.wear.watchface.complications.data.ComplicationData
import androidx.wear.watchface.complications.data.ComplicationText
import androidx.wear.watchface.complications.data.ComplicationType
import androidx.wear.watchface.complications.data.CountUpTimeReference
import androidx.wear.watchface.complications.data.LongTextComplicationData
import androidx.wear.watchface.complications.data.PlainComplicationText
import androidx.wear.watchface.complications.data.RangedValueComplicationData
import androidx.wear.watchface.complications.data.ShortTextComplicationData
import androidx.wear.watchface.complications.data.TimeDifferenceComplicationText
import androidx.wear.watchface.complications.data.TimeDifferenceStyle
import androidx.wear.watchface.complications.data.TimeRange
import androidx.wear.watchface.complications.datasource.ComplicationRequest
import androidx.wear.watchface.complications.datasource.SuspendingComplicationDataSourceService
import dev.cueseek.wear.MainActivity
import dev.cueseek.wear.tile.LastReading
import dev.cueseek.wear.tile.LastReadingStore
import java.time.Duration
import java.time.Instant
import java.util.concurrent.TimeUnit

/**
 * One slot on somebody else's watch face.
 *
 * # The smallest surface, and the one with least room to be wrong
 *
 * A complication is not a screen CueSeek draws. It hands typed data to a watch face, which
 * owns the layout, the colours, the font and whether a title is shown at all. So the
 * discipline here is **subtraction**: decide the one thing worth a slot, and decline every
 * type that cannot carry it.
 *
 * The one thing is **how many services are healthy out of how many**. Not the verdict
 * sentence — "1 needs attention" does not fit in a slot built for seven characters, and a
 * face free to truncate it would render "1 needs…", which is worse than silence. Not a
 * service name, because a complication that tries to show four services shows none of them.
 *
 * # It reads, and never fetches
 *
 * **This is the difference between a complication and the tile.** `onTileRequest` runs when
 * somebody looks at a tile; a complication is refreshed on the *system's* schedule whether
 * anybody is looking or not. Fetching here would be exactly the background polling ADR-0004
 * forbids and M5.10 was written to prevent — a radio woken on a timer to service a slot
 * nobody has glanced at.
 *
 * So it reads what the app and the tile already wrote and never opens a socket. The cost,
 * stated: a watch whose app has not run has nothing to show, and says so.
 *
 * # Staleness, on a surface that cannot re-evaluate
 *
 * There is no dynamic expression here and no chance to recompute — the face renders whatever
 * was last supplied. Two mechanisms carry the honesty instead:
 *
 *  1. **A live age**, inside the main text — a [TimeDifferenceComplicationText] counting
 *     up from the reading, which the *watch face* ticks by itself: "3/4 5m".
 *  2. **An expiry.** [ComplicationData.validTimeRange] ends an hour on, so the slot empties
 *     rather than sitting on a watch face looking current.
 *
 * **The age was a title first, and two watch faces declined to draw it.** A title is the
 * face's to show or not, and on the Watch 2R neither face tried drew it — so `3/4` sat on
 * the wrist with nothing qualifying it, and in M5.17 it showed `2/4` for a service that had
 * already been started again. The age therefore moved into the main text, which every face
 * draws. The expiry stays at an hour as the backstop: past it, the slot empties rather than
 * showing an age nobody should still be reading.
 *
 * Blanking at 90 seconds was the alternative, and it was rejected because a complication
 * that is empty most of the day teaches its owner to ignore it. A number with its age on it
 * is honest at any age; that is what makes the hour safe.
 */
class CueSeekComplicationService : SuspendingComplicationDataSourceService() {

    override suspend fun onComplicationRequest(request: ComplicationRequest): ComplicationData? {
        val reading = LastReadingStore(applicationContext).read()
            ?: return unavailable(request.complicationType)
        return render(request.complicationType, reading)
    }

    /**
     * What the complication picker shows while somebody is choosing one.
     *
     * Fixed sample values rather than the real reading: a preview is drawn in a list, at a
     * moment when the app may never have run, and putting a live verdict there would assert
     * a state nobody has measured — the same reason the tile's preview is a mark rather than
     * a screenshot.
     */
    override fun getPreviewData(type: ComplicationType): ComplicationData? =
        render(
            type = type,
            reading = LastReading(
                verdict = "Operational",
                healthy = 4,
                total = 4,
                observedAt = Instant.now(),
            ),
            preview = true,
        )

    private fun render(
        type: ComplicationType,
        reading: LastReading,
        preview: Boolean = false,
    ): ComplicationData? {
        // "3/4". The whole payload, and the reason the other types are declined.
        val count = "${reading.healthy}/${reading.total}"
        val description = PlainComplicationText.Builder(
            "CueSeek: ${reading.verdict}. ${reading.healthy} of ${reading.total} services healthy.",
        ).build()

        // The count and its age as one text: "3/4 now", "3/4 5m", "3/4 2h". `^1` is where
        // the watch face puts the time difference, which it ticks by itself — no fetch, no
        // wake, nothing from this service after the one write.
        //
        // **This was a title until M5.17, and the wrist proved a title is not enough.** The
        // face owns whether a title is drawn, and both faces tried on the Watch 2R drew
        // only the count. During the TalkBack test the slot read `2/4` — a reading taken
        // while cron was stopped — after cron was running again, with nothing to say it was
        // old. The main text is the one field every face draws, so the age lives there.
        //
        // A preview has no real age, so it gets a fixed word rather than a counter that
        // would read "0m" forever in a picker.
        fun counted(template: String): ComplicationText = if (preview) {
            PlainComplicationText.Builder(template.replace("^1", "now")).build()
        } else {
            TimeDifferenceComplicationText.Builder(
                TimeDifferenceStyle.SHORT_SINGLE_UNIT,
                CountUpTimeReference(reading.observedAt),
            )
                .setMinimumTimeUnit(TimeUnit.MINUTES)
                .setDisplayAsNow(true)
                .setText(template)
                .build()
        }

        val validity = TimeRange.between(
            reading.observedAt,
            reading.observedAt.plus(EXPIRES_AFTER),
        )

        return when (type) {
            ComplicationType.SHORT_TEXT -> ShortTextComplicationData.Builder(
                text = counted("$count ^1"),
                contentDescription = description,
            )
                .setTapAction(openApp())
                .setValidTimeRange(validity)
                .build()

            // The only type that can draw the ratio as a shape rather than as characters,
            // which is the whole reason it is supported: on a face that renders an arc,
            // "most things are fine" becomes readable without reading.
            ComplicationType.RANGED_VALUE -> RangedValueComplicationData.Builder(
                value = reading.healthy.toFloat(),
                min = 0f,
                max = reading.total.coerceAtLeast(1).toFloat(),
                contentDescription = description,
            )
                .setText(counted("$count ^1"))
                .setTapAction(openApp())
                .setValidTimeRange(validity)
                .build()

            // Room for the sentence, so it gets the sentence.
            ComplicationType.LONG_TEXT -> LongTextComplicationData.Builder(
                text = counted("$count healthy · ^1"),
                contentDescription = description,
            )
                .setTapAction(openApp())
                .setValidTimeRange(validity)
                .build()

            // Declined, deliberately. Image-only slots can carry a mark but not a state, and
            // a CueSeek logo sitting on a watch face saying nothing about the machine would
            // be decoration pretending to be instrumentation. Returning null makes the
            // complication unavailable for that slot rather than filling it badly.
            else -> null
        }
    }

    /**
     * Nothing has ever been read: a fresh install whose app has not run.
     *
     * Says so rather than rendering a zero. `0/0` would be a claim about a machine nobody
     * has asked about yet — the same `absent ≠ zero` rule the agent applies to a sensor it
     * could not read.
     */
    private fun unavailable(type: ComplicationType): ComplicationData? {
        val text = PlainComplicationText.Builder("--").build()
        val description = PlainComplicationText.Builder("CueSeek: no reading yet.").build()

        return when (type) {
            ComplicationType.SHORT_TEXT -> ShortTextComplicationData.Builder(text, description)
                .setTapAction(openApp())
                .build()

            ComplicationType.LONG_TEXT -> LongTextComplicationData.Builder(
                PlainComplicationText.Builder("No reading yet").build(),
                description,
            )
                .setTapAction(openApp())
                .build()

            // A ranged value has no honest empty: any number sits somewhere on the arc, and
            // zero would draw an empty gauge that looks like a measured nothing.
            else -> null
        }
    }

    private fun openApp(): PendingIntent = PendingIntent.getActivity(
        this,
        0,
        Intent(this, MainActivity::class.java),
        PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
    )

    private companion object {
        /**
         * One hour, after which the slot empties rather than showing something old.
         *
         * **This was twelve hours, and the watch corrected it.** The design leaned on the
         * live age in [TimeDifferenceComplicationText] to keep an ageing number honest —
         * and then the first face it was added to **did not draw the title at all**, which
         * is entirely the face's right. `3/4` sat on the wrist with nothing qualifying it.
         *
         * So the expiry is carrying weight it was not designed to carry, and twelve hours
         * was sized for a slot that showed its own age. An hour is the compromise: long
         * enough to survive an ordinary gap between glances, short enough that an
         * unqualified number can never be half a day old.
         *
         * It is still not the 90-second staleness threshold, and deliberately not — a slot
         * that blanked every 90 seconds would be empty most of the day, and a complication
         * its owner has learned to ignore is worse than one that is an hour behind.
         *
         * **Since M5.17 it is a backstop again.** The age moved into the main text, which
         * every face draws, so the number is never unqualified; the hour now only bounds how
         * long a visibly old reading stays on the face.
         */
        val EXPIRES_AFTER: Duration = Duration.ofHours(1)
    }
}
