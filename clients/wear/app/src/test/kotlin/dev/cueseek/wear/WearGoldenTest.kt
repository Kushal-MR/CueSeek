package dev.cueseek.wear

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.Color
import app.cash.paparazzi.DeviceConfig
import app.cash.paparazzi.Paparazzi
import androidx.wear.compose.material3.AppScaffold
import dev.cueseek.core.design.LocalStatusColors
import dev.cueseek.core.design.token.CueSeekStatusColors
import dev.cueseek.core.model.Action
import dev.cueseek.core.model.ActionRisk
import dev.cueseek.core.model.Capability
import dev.cueseek.core.model.CpuMetrics
import dev.cueseek.core.model.Health
import dev.cueseek.core.model.HealthReason
import dev.cueseek.core.model.HealthStatus
import dev.cueseek.core.model.HostMetrics
import dev.cueseek.core.model.MemoryMetrics
import dev.cueseek.core.model.NowPlaying
import dev.cueseek.core.model.PlaybackSession
import dev.cueseek.core.model.Scope
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.StorageMetrics
import dev.cueseek.core.model.Tally
import dev.cueseek.core.model.ThermalMetrics
import dev.cueseek.wear.ambient.AmbientScreen
import dev.cueseek.wear.dashboard.ActionUi
import dev.cueseek.wear.dashboard.DashboardContent
import dev.cueseek.wear.dashboard.DashboardUi
import dev.cueseek.wear.detail.ServiceDetailScreen
import dev.cueseek.wear.power.HostPowerScreen
import dev.cueseek.wear.power.PowerAccess
import dev.cueseek.wear.theme.CueSeekWearTheme
import org.junit.Rule
import org.junit.Test
import java.time.Instant
import kotlin.math.pow

/**
 * Golden images of the watch's screens, at two real geometries (M5.15).
 *
 * # Why two sizes, and which two
 *
 * A layout that survives 233dp can break at 192dp, and neither is the phone. So every screen
 * is recorded twice — once per subclass below — and a change that only breaks the small
 * watch fails here rather than on somebody's wrist:
 *
 *  - [SmallRoundGoldenTest]: 192dp, the smallest round Wear screen Android Studio models.
 *  - [Watch2RGoldenTest]: 233dp, the OnePlus Watch 2R every M5 phase was verified on.
 *
 * # Why these states
 *
 * Not "every screen", which would be a suite nobody reads when it fails. These are the states
 * where being wrong is a lie rather than a blemish: attention, stale, empty, failed, a
 * destructive control, the power screen, ambient — plus the greyscale dashboard, which is the
 * claim "colour is not load-bearing" rendered, exactly as `:core:design` does it for the
 * phone. That test found two real defects there that were invisible by eye.
 *
 * Nothing here is animated or clock-driven except the ambient age, whose reading is placed
 * safely inside one minute so it renders "2m ago" whenever the suite runs.
 */
abstract class WearGoldenTest(device: DeviceConfig) {

    @get:Rule
    val paparazzi = Paparazzi(
        deviceConfig = device,
        showSystemUi = false,
        // Recorded on Windows, verified on Linux CI; the same tolerance :core:design uses,
        // for the same reason — sub-pixel text antialiasing differs, nothing a person sees.
        maxPercentDifference = 0.25,
    )

    // Inside the same AppScaffold the app uses, because that is what paints the near-black
    // page: without it the first recording came out on layoutlib's mid-grey, a background no
    // user ever sees. The time text is suppressed, or every golden would change each minute.
    private fun shot(content: @Composable () -> Unit) = paparazzi.snapshot {
        CueSeekWearTheme {
            AppScaffold(timeText = {}) { content() }
        }
    }

    @Test
    fun dashboard_attention() = shot { DashboardContent(ui = loaded(), stale = false) }

    /** The roster itself, which the vitals push below the fold on a real reading. */
    @Test
    fun dashboard_roster() = shot {
        DashboardContent(ui = loaded(metrics = null), stale = false)
    }

    @Test
    fun dashboard_stale() = shot {
        DashboardContent(ui = loaded(metrics = null), stale = true)
    }

    @Test
    fun dashboard_empty() = shot {
        DashboardContent(ui = loaded(services = emptyList()), stale = false)
    }

    @Test
    fun dashboard_failed() = shot {
        DashboardContent(ui = DashboardUi.Failed("Could not reach the agent"), stale = false)
    }

    /** Hue removed from every status colour; shape and word must still tell them apart. */
    @Test
    fun dashboard_roster_greyscale() = shot {
        CompositionLocalProvider(
            LocalStatusColors provides CueSeekStatusColors.Dark.desaturated(),
        ) {
            DashboardContent(ui = loaded(metrics = null), stale = false)
        }
    }

    @Test
    fun detail_actions() = shot {
        ServiceDetailScreen(service = CRON, stale = false)
    }

    @Test
    fun detail_playing() = shot {
        ServiceDetailScreen(service = MEDIA, stale = false)
    }

    @Test
    fun power_busy() = shot {
        HostPowerScreen(
            access = PowerAccess.Offered(POWER),
            services = listOf(MEDIA),
            stale = false,
            action = ActionUi.Idle,
            onInvoke = { _, _ -> },
        )
    }

    @Test
    fun ambient() = shot { AmbientScreen(ui = loaded(), stale = false) }
}

class SmallRoundGoldenTest : WearGoldenTest(DeviceConfig.WEAR_OS_SMALL_ROUND)

/** 466px at xhdpi: 233dp, round. Read off the device in M5.1. */
class Watch2RGoldenTest : WearGoldenTest(
    DeviceConfig.WEAR_OS_SMALL_ROUND.copy(screenWidth = 466, screenHeight = 466),
)

// ---------------------------------------------------------------------------- fixtures

private val NOW: Instant = Instant.now()

private fun health(status: HealthStatus, reported: String? = null, reasons: List<HealthReason> = emptyList()) =
    Health(
        status = status,
        reachable = status != HealthStatus.Unreachable,
        reportedStatus = reported,
        reasons = reasons,
        observedAt = NOW,
    )

private val CONTROL = listOf(Capability("health", "Health"), Capability("control", "Control"))

private val CRON = Service(
    id = "cron",
    name = "Cron",
    capabilities = CONTROL,
    health = health(HealthStatus.Healthy, reported = "active (running)"),
    actions = listOf(
        Action("restart", "Restart Cron", ActionRisk.Safe, null),
        Action("stop", "Stop Cron", ActionRisk.Destructive, null),
    ),
)

private val MEDIA = Service(
    id = "media",
    name = "Jellyfin",
    capabilities = CONTROL + Capability("now_playing", "Now playing"),
    health = health(HealthStatus.Healthy, reported = "Healthy"),
    actions = listOf(
        Action(
            "restart",
            "Restart Jellyfin",
            ActionRisk.Disruptive,
            "Anything currently playing will be interrupted.",
        ),
    ),
    nowPlaying = NowPlaying(
        sessions = 2,
        transcoding = 1,
        items = listOf(
            PlaybackSession(
                id = "s1",
                title = "The Long Way Home",
                subtitle = "S02E04",
                user = "sam",
                client = "Android TV",
                positionSeconds = 1_260,
                durationSeconds = 2_700,
                paused = false,
                transcoding = true,
            ),
            PlaybackSession(
                id = "s2",
                title = "Another film",
                subtitle = null,
                user = null,
                client = null,
                positionSeconds = null,
                durationSeconds = null,
                paused = true,
                transcoding = false,
            ),
        ),
    ),
)

private val SERVICES = listOf(
    MEDIA,
    Service(
        id = "downloads",
        name = "qBittorrent",
        capabilities = CONTROL,
        health = health(
            HealthStatus.Degraded,
            reported = "firewalled",
            reasons = listOf(HealthReason("firewalled", "No incoming connections.")),
        ),
        actions = emptyList(),
    ),
    CRON,
    Service(
        id = "vpn",
        name = "WireGuard",
        capabilities = CONTROL,
        health = health(HealthStatus.Unknown),
        actions = emptyList(),
    ),
)

private val POWER = listOf(
    Action("reboot", "Reboot", ActionRisk.Destructive, "The machine restarts and comes back."),
    Action("shutdown", "Shut down", ActionRisk.Destructive, "It stays off until someone turns it on."),
)

private val METRICS = HostMetrics(
    collectedAt = NOW,
    cpu = CpuMetrics(usagePercent = 34f),
    memory = MemoryMetrics(totalBytes = 16_000_000_000, usedBytes = 13_300_000_000),
    storage = listOf(StorageMetrics(mount = "/", totalBytes = 1_000_000_000_000, freeBytes = 80_000_000_000)),
    thermal = listOf(ThermalMetrics(label = "Package id 0", celsius = 58f)),
)

private fun loaded(
    services: List<Service> = SERVICES,
    metrics: HostMetrics? = METRICS,
): DashboardUi.Loaded {
    val tally = Tally.of(services)
    return DashboardUi.Loaded(
        hostname = "hp-server",
        verdict = if (tally.needingAttention > 0) "${tally.needingAttention} needs attention" else "Operational",
        tally = tally,
        services = services,
        metrics = metrics,
        hostActions = POWER,
        scopes = setOf(Scope.Read, Scope.ServiceControl, Scope.HostPower),
        // Mid-way through a minute, so the ambient age reads "2m ago" on any run.
        observedAt = NOW.minusSeconds(150),
        stale = false,
    )
}

// ---------------------------------------------------------------------------- greyscale

/** Each colour replaced by the grey of identical relative luminance, as `:core:design` does. */
private fun CueSeekStatusColors.desaturated() = CueSeekStatusColors(
    healthy = healthy.grey(),
    healthyContainer = healthyContainer.grey(),
    degraded = degraded.grey(),
    degradedContainer = degradedContainer.grey(),
    unreachable = unreachable.grey(),
    unreachableContainer = unreachableContainer.grey(),
    unknown = unknown.grey(),
    unknownOutline = unknownOutline.grey(),
    beat = beat.grey(),
    tallyOnHealthy = tallyOnHealthy.grey(),
    tallyOnDegraded = tallyOnDegraded.grey(),
    tallyOnUnreachable = tallyOnUnreachable.grey(),
)

private fun Color.grey(): Color {
    if (alpha == 0f) return this
    fun lin(v: Float): Double {
        val d = v.toDouble()
        return if (d <= 0.03928) d / 12.92 else ((d + 0.055) / 1.055).pow(2.4)
    }
    val y = 0.2126 * lin(red) + 0.7152 * lin(green) + 0.0722 * lin(blue)
    val s = if (y <= 0.0031308) y * 12.92 else 1.055 * y.pow(1 / 2.4) - 0.055
    return Color(s.toFloat(), s.toFloat(), s.toFloat(), alpha)
}
