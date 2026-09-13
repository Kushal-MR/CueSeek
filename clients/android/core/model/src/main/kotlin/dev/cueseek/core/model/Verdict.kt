package dev.cueseek.core.model

/**
 * What CueSeek thinks of a host, as a judgement rather than a rendering.
 *
 * # Why this is not in a UI module
 *
 * It was, until M5.4a: `verdict`, `hostConcern`, the thresholds and `Tally` lived inside the
 * phone app's dashboard package, three of them `internal`. That was fine while there was one
 * client, and stopped being fine the moment there were two.
 *
 * Layout is per form factor — ADR-0007 is explicit that the same capability is rendered
 * completely differently on a phone and a watch, and M5.3b already gave the watch its own
 * error copy on exactly that reasoning. **This is not that.** "Is everything fine?" is one
 * question about one machine, and two clients that answered it differently from identical
 * data would not be a design difference; they would be a console contradicting itself. A
 * watch saying `Operational` while the phone in your pocket says `Disk almost full` is worse
 * than either being wrong alone.
 *
 * So the judgement is shared and the drawing is not.
 */

/** How many services sit in each status. */
data class Tally(
    val healthy: Int = 0,
    val degraded: Int = 0,
    val unreachable: Int = 0,
    val unknown: Int = 0,
) {
    val total: Int get() = healthy + degraded + unreachable + unknown
    val needingAttention: Int get() = degraded + unreachable

    companion object {
        fun of(services: List<Service>): Tally {
            var h = 0
            var d = 0
            var u = 0
            var k = 0
            services.forEach {
                when (it.health.status) {
                    HealthStatus.Healthy -> h++
                    HealthStatus.Degraded -> d++
                    HealthStatus.Unreachable -> u++
                    HealthStatus.Unknown -> k++
                }
            }
            return Tally(h, d, u, k)
        }
    }
}

/**
 * The point at which a resource is worth mentioning, and the point at which it is urgent.
 *
 * Shared by the headline and by whatever draws the bar, so the two can never disagree about
 * whether something is wrong. They disagreed once, in a different form: a disk sat at 97%
 * with its rule drawn red while the headline above it read "All good".
 */
const val PRESSURE: Float = 0.85f

/** @see PRESSURE */
const val CRITICAL: Float = 0.95f

/** The fullest of the reported filesystems, or null when none can be judged. */
fun fullest(storage: List<StorageMetrics>?): StorageMetrics? =
    storage?.filter { it.usedFraction != null }?.maxByOrNull { it.usedFraction ?: 0f }

/**
 * What the machine itself is complaining about, or null when it is not.
 *
 * Temperature is judged against the sensor's own stated limit, never a number invented here.
 * CPU is deliberately absent: a processor at 100% is a transcode doing its job, and a
 * console that announced it would be crying wolf every time somebody watched a film.
 */
fun hostConcern(metrics: HostMetrics?): String? {
    if (metrics == null) return null

    val disk = fullest(metrics.storage)?.usedFraction
    val memory = metrics.memory?.usedFraction

    // Critical first, across both resources, so the worse of the two wins rather than
    // whichever happens to be checked first.
    if (disk != null && disk >= CRITICAL) return "Disk almost full"
    if (memory != null && memory >= CRITICAL) return "Memory almost full"

    if (metrics.thermal?.any { it.isHot } == true) return "Running hot"

    if (disk != null && disk >= PRESSURE) return "Disk filling up"
    if (memory != null && memory >= PRESSURE) return "Memory under pressure"

    return null
}

/**
 * The verdict, in the user's terms rather than the system's.
 *
 * Ordered by how definite the problem is, not by subject. Services needing attention come
 * first because that is what the console is for. **Host pressure comes next, ahead of
 * unknown services**, because a filesystem at 97% is a fact somebody has to act on while
 * `unknown` is the absence of a fact.
 *
 * It stops at the headline. The tally and the roster stay about services, because a machine
 * is not one of its own services and counting it as one would make "2 healthy" mean two
 * different kinds of thing at once (ADR-0004 Amendment 4).
 */
fun verdict(state: AgentState, tally: Tally): String = verdict(
    stale = state.freshness.isStale,
    services = state.services,
    hostMetrics = state.hostMetrics,
    tally = tally,
)

/**
 * The same judgement, over the three things it actually depends on.
 *
 * [AgentState] is shaped by the phone's event stream — it carries a stream status, action
 * outcomes, a refreshing flag. A watch polls and holds none of that (ADR-0004: nothing
 * background-critical may depend on SSE), so requiring one would have meant either
 * fabricating a stream state on the watch or writing a second verdict. Both are worse than
 * naming the three inputs.
 */
fun verdict(
    stale: Boolean,
    services: List<Service>,
    hostMetrics: HostMetrics?,
    tally: Tally,
): String {
    if (stale) return "Unverified"
    if (services.isEmpty()) return "No services"

    val attention = tally.needingAttention
    return when {
        attention == 1 -> "1 needs attention"
        attention > 1 -> "$attention need attention"
        else -> hostConcern(hostMetrics)
            ?: if (tally.unknown > 0) "${tally.unknown} unknown" else "Operational"
    }
}
