package dev.cueseek.core.model

/**
 * What a service is *doing*, as distinct from whether it is up.
 *
 * Both types carry aggregate counts and a **bounded sample** of items. The counts are the
 * truth; the list is what a screen can show. A client that rendered `items.size` as the
 * total would quietly understate a busy server the moment the agent's cap bit — which is
 * exactly when the number matters most.
 *
 * Absent (`null` on [Service]) and empty are different facts and must render differently.
 * Null means the agent could not ask; empty means it asked and nothing is happening.
 */

/** Active playback on a media server. */
data class NowPlaying(
    /** The true total, which may exceed [items] size. */
    val sessions: Int,
    /**
     * How many of those the server is transcoding.
     *
     * The operationally significant number on a self-hosted box: direct play is nearly
     * free, and one transcode can saturate the CPU every other service is sharing.
     */
    val transcoding: Int,
    val items: List<PlaybackSession>,
) {
    /** True when the agent asked and found nothing playing. */
    val idle: Boolean get() = sessions == 0
}

data class PlaybackSession(
    val id: String,
    val title: String,
    /** Episode, artist or year. Null when the service offers none — never synthesised. */
    val subtitle: String?,
    val user: String?,
    val client: String?,
    val positionSeconds: Int?,
    val durationSeconds: Int?,
    val paused: Boolean,
    val transcoding: Boolean,
) {
    /**
     * Fraction watched, or null when it cannot be known.
     *
     * Null rather than 0 for live or unbounded content: a progress bar parked at the far
     * left is a claim that the stream just started, which for a live channel is a
     * statement nobody made.
     */
    val progress: Float?
        get() {
            val duration = durationSeconds ?: return null
            val position = positionSeconds ?: return null
            if (duration <= 0) return null
            return (position.toFloat() / duration).coerceIn(0f, 1f)
        }
}

/** In-flight transfers: torrents, usenet downloads, an import queue. */
data class Transfers(
    /** Transfers currently moving data. */
    val active: Int,
    /** Everything the service tracks, including paused, queued and seeding. */
    val total: Int,
    val downloadRateBytes: Long,
    val uploadRateBytes: Long,
    val items: List<TransferItem>,
) {
    /** True when the agent asked and the service is moving nothing. */
    val idle: Boolean get() = active == 0
}

data class TransferItem(
    val id: String,
    val name: String,
    /**
     * The service's own state word, verbatim — `downloading`, `stalledDL`, `seeding`.
     *
     * Deliberately a string rather than an enum. Every download client has its own
     * vocabulary, and the difference between "stalled" and "queued" is precisely what
     * tells an operator whether to care.
     */
    val state: String,
    val progress: Float,
    val sizeBytes: Long?,
    val downloadRateBytes: Long?,
    val etaSeconds: Int?,
)
