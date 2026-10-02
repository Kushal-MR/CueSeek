package dev.cueseek.wear.theme

import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.calculateEndPadding
import androidx.compose.foundation.layout.calculateStartPadding
import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalLayoutDirection
import androidx.compose.ui.unit.dp

/**
 * The scaffold's padding, with enough room at the bottom for the last item to reach the
 * middle of the screen.
 *
 * # Why a screen that ends in a button needs this
 *
 * A round screen is widest at its middle and narrowest at its bottom. The scaffold's default
 * bottom padding only clears the bezel, so a list that ends in a control lets it scroll as
 * far as the lower curve and no further — and that is where "Stop Cron — hold" sat, clipped
 * at both corners, on the Watch 2R. Kushal found it on his wrist (M5.17): the one control
 * that most needs to be seen whole was the one the screen could not show whole.
 *
 * So the bottom padding is sized from the screen rather than fixed: 40% of its height, which
 * lets a 48dp button scroll up to roughly the centre on both the 192dp and 233dp goldens.
 * Applied to the screens that end in actions, not to the dashboard, whose last control is an
 * `EdgeButton` the scaffold already places.
 */
@Composable
fun PaddingValues.withRoomToCentre(): PaddingValues {
    val direction = LocalLayoutDirection.current
    val screenHeight = LocalConfiguration.current.screenHeightDp.dp
    return PaddingValues(
        start = calculateStartPadding(direction),
        top = calculateTopPadding(),
        end = calculateEndPadding(direction),
        bottom = maxOf(calculateBottomPadding(), screenHeight * 0.4f),
    )
}
