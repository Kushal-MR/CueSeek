package dev.cueseek.core.model

import java.time.Instant

/**
 * The host machine's own vitals, from the `host_updated` stream event.
 *
 * Not part of [SystemInfo], and the separation is load-bearing rather than tidiness.
 * `SystemInfo` is identity: it arrives once, in the snapshot, and never changes for the life
 * of the agent process. These change every few seconds. Had they been carried there, a CPU
 * figure would have frozen at the moment this client connected and then sat under a live
 * indicator claiming to be current — which is the one thing this client exists not to do.
 *
 * **Every field is nullable and null means "the agent could not read this".** Never zero.
 * Hardware differs: a virtual machine exposes no temperature sensors, a container may see no
 * usable `/proc/stat`, and the first collection after an agent restart cannot compute CPU
 * utilisation at all because the kernel counts cumulatively. Rendering any of those as zero
 * would describe an idle, cold machine that never answered.
 */
data class HostMetrics(
    /** When the agent measured. Staleness is judged from this, never from arrival. */
    val collectedAt: Instant,
    val uptimeSeconds: Long? = null,
    val cpu: CpuMetrics? = null,
    val memory: MemoryMetrics? = null,
    /**
     * Watched filesystems.
     *
     * Null and empty differ, and both cross the wire intact. Null means the agent could not
     * measure any filesystem; empty means it was asked about mounts and none answered — a
     * configured drive that has gone away, which is worth knowing and is not the same as
     * never having asked.
     */
    val storage: List<StorageMetrics>? = null,
    /**
     * Temperature sensors.
     *
     * Null when the agent could not enumerate sensors; empty when it enumerated them and
     * this machine has none. Empty is completely ordinary — most servers and every virtual
     * machine report it — and must not be rendered as a problem.
     */
    val thermal: List<ThermalMetrics>? = null,
) {
    /** True when nothing at all could be read. A payload arrived, but it says nothing. */
    val isEmpty: Boolean
        get() = cpu == null && memory == null &&
            storage.isNullOrEmpty() && thermal.isNullOrEmpty() && uptimeSeconds == null
}

/** Processor load. */
data class CpuMetrics(
    /**
     * Busy percentage across all cores since the agent's previous collection, 0 to 100.
     *
     * Null on the agent's first collection after a restart, which is correct rather than a
     * gap: utilisation exists only as a difference between two samples of a cumulative
     * counter, and there is no honest answer from one.
     */
    val usagePercent: Float? = null,
    val cores: Int? = null,
    /**
     * Load averages. Kept alongside [usagePercent] because they measure different things —
     * load counts processes blocked on disk as well as on CPU, so a machine thrashing its
     * swap reads as near-idle usage and near-unusable load.
     */
    val load1: Float? = null,
    val load5: Float? = null,
    val load15: Float? = null,
) {
    /**
     * Load as a fraction of capacity, or null when either half is unknown.
     *
     * A load of 4 is saturation on a quad-core and half idle on an eight, so the raw number
     * cannot be rendered as a proportion without the core count. Deliberately not clamped:
     * a machine can be loaded beyond its cores, and that is exactly when it matters.
     */
    val loadFraction: Float?
        get() {
            val load = load1 ?: return null
            val count = cores ?: return null
            if (count <= 0) return null
            return load / count
        }
}

/** Physical memory and swap. */
data class MemoryMetrics(
    val totalBytes: Long? = null,
    /** What a new process could actually get, per the kernel's own estimate. */
    val availableBytes: Long? = null,
    val usedBytes: Long? = null,
    val swapTotalBytes: Long? = null,
    val swapUsedBytes: Long? = null,
) {
    /**
     * Used as a fraction of total, or null when either is missing.
     *
     * Derived from the agent's `used`, which is total minus the kernel's *available* rather
     * than minus free. That distinction is why a media server up for a week does not appear
     * to be out of memory: page cache is counted as available, because it is.
     */
    val usedFraction: Float?
        get() {
            val used = usedBytes ?: return null
            val total = totalBytes ?: return null
            if (total <= 0) return null
            return (used.toDouble() / total.toDouble()).toFloat().coerceIn(0f, 1f)
        }
}

/** One watched filesystem. */
data class StorageMetrics(
    /** The path the operator configured, verbatim. What they will recognise. */
    val mount: String,
    val totalBytes: Long,
    /** Space an unprivileged process may actually use — the root reserve is excluded. */
    val freeBytes: Long,
    val filesystem: String? = null,
) {
    val usedBytes: Long get() = (totalBytes - freeBytes).coerceAtLeast(0)

    val usedFraction: Float?
        get() = if (totalBytes <= 0) null
        else (usedBytes.toDouble() / totalBytes.toDouble()).toFloat().coerceIn(0f, 1f)
}

/** One temperature sensor. */
data class ThermalMetrics(
    /**
     * The sensor's own name, verbatim: "coretemp Package id 0", "nvme Composite". Displayed
     * as given — sensor naming is hardware-specific and any shared vocabulary this client
     * imposed would either drop sensors or mislabel them.
     */
    val label: String,
    val celsius: Float,
    /**
     * The temperature the hardware itself calls high, when it publishes one.
     *
     * Present so a reading can be judged without this app hard-coding a threshold for
     * silicon it has never seen. A laptop CPU at 85°C is fine; a drive at 85°C is not.
     */
    val highCelsius: Float? = null,
) {
    /** True when the hardware's own threshold has been reached. Null threshold means unknown, not fine. */
    val isHot: Boolean get() = highCelsius?.let { celsius >= it } ?: false
}
