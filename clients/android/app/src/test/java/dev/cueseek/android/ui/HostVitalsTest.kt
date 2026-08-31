package dev.cueseek.android.ui

import dev.cueseek.android.ui.dashboard.chipOf
import dev.cueseek.android.ui.dashboard.fullest
import dev.cueseek.android.ui.dashboard.trimZero
import dev.cueseek.android.ui.dashboard.uptimePhrase
import dev.cueseek.core.model.CpuMetrics
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.MemoryMetrics
import dev.cueseek.core.model.StorageMetrics
import dev.cueseek.core.model.ThermalMetrics
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The judgement calls behind the vitals strip, tested away from Compose.
 *
 * Everything here decides what a reader is told, and each one has a wrong answer that would
 * look plausible on screen: a proportion computed without a denominator, a temperature
 * judged against a number this app invented, a machine rendered as idle because nothing was
 * measured.
 */
class HostVitalsTest {

    @Test
    fun `uptime coarsens as it grows`() {
        assertEquals("42m", uptimePhrase(42 * 60))
        assertEquals("5h", uptimePhrase(5 * 3600))
        // Below two days, hours still read better than "1d" — a server up 30 hours is a
        // server that rebooted yesterday, and that is the interesting fact.
        assertEquals("30h", uptimePhrase(30 * 3600))
        assertEquals("9d", uptimePhrase(9 * 86_400))
    }

    @Test
    fun `load reads without trailing zeroes`() {
        assertEquals("1.2", trimZero(1.24f))
        assertEquals("3", trimZero(3.0f))
        assertEquals("0", trimZero(0.02f))
    }

    @Test
    fun `the fullest filesystem is the one worth showing`() {
        val root = StorageMetrics(mount = "/", totalBytes = 100, freeBytes = 50)
        val media = StorageMetrics(mount = "/mnt/media", totalBytes = 1000, freeBytes = 40)

        assertEquals(media, fullest(listOf(root, media)))
        assertNull("no filesystems means nothing to show", fullest(null))
        assertNull(fullest(emptyList()))
    }

    @Test
    fun `a filesystem with no size cannot be judged`() {
        // A mount reporting zero total is not a full disk, it is an unmeasurable one.
        val unmeasurable = StorageMetrics(mount = "/proc", totalBytes = 0, freeBytes = 0)
        assertNull(unmeasurable.usedFraction)
        assertNull("an unmeasurable mount must not win the comparison", fullest(listOf(unmeasurable)))
    }

    @Test
    fun `load is a fraction only when the core count is known`() {
        assertEquals(0.5f, CpuMetrics(load1 = 4f, cores = 8).loadFraction)
        // A load of 4 is saturation on a quad-core and half idle on an eight. Without the
        // denominator there is no proportion, and guessing one would mislead either way.
        assertNull(CpuMetrics(load1 = 4f).loadFraction)
        assertNull(CpuMetrics(cores = 8).loadFraction)
    }

    @Test
    fun `memory pressure comes from used over total`() {
        val memory = MemoryMetrics(totalBytes = 1000, availableBytes = 250, usedBytes = 750)
        assertEquals(0.75f, memory.usedFraction)
        assertNull("a partial payload cannot produce a proportion", MemoryMetrics(usedBytes = 750).usedFraction)
    }

    @Test
    fun `a sensor is hot only against its own threshold`() {
        // The threshold comes from the hardware, so a laptop CPU and an NVMe drive are each
        // judged by their own limits rather than by a number this app made up.
        assertTrue(ThermalMetrics("coretemp", celsius = 90f, highCelsius = 84f).isHot)
        assertFalse(ThermalMetrics("coretemp", celsius = 70f, highCelsius = 84f).isHot)
        // No threshold means unknown, and unknown is not an alarm.
        assertFalse(ThermalMetrics("acpitz", celsius = 90f).isHot)
    }

    @Test
    fun `a payload with nothing in it knows it is empty`() {
        // What decides whether the strip is drawn at all. An agent that answered with only a
        // timestamp has measured nothing, and three empty meters would claim otherwise.
        assertTrue(HostMetrics(collectedAt = Instant.EPOCH).isEmpty)
        assertTrue(HostMetrics(collectedAt = Instant.EPOCH, thermal = emptyList()).isEmpty)
        assertFalse(HostMetrics(collectedAt = Instant.EPOCH, uptimeSeconds = 10).isEmpty)
    }

    @Test
    fun `a sensor is shown by its chip, not its full name`() {
        // Found on hardware: the footnote is four short facts on one line, and a full
        // sensor label pushed it onto two, stranding "Core 0" by itself. Which sensor is
        // hottest changes minute to minute, so the layout broke and healed on its own.
        assertEquals("coretemp", chipOf("coretemp Core 0"))
        assertEquals("coretemp", chipOf("coretemp Package id 0"))
        assertEquals("nvme", chipOf("nvme Composite"))
        // Already one word on this machine's acpitz, and it must stay intact.
        assertEquals("acpitz", chipOf("acpitz"))
        assertEquals("", chipOf("   "))
    }
}
