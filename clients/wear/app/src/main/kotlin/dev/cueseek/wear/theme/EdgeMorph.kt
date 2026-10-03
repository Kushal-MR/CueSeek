package dev.cueseek.wear.theme

import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.wear.compose.foundation.lazy.TransformingLazyColumnItemScope
import androidx.wear.compose.material3.lazy.TransformationSpec
import androidx.wear.compose.material3.lazy.transformedHeight

/**
 * Shrink and fade a list item as it reaches the curved top or bottom of the screen, instead of
 * letting the circle cut it off.
 *
 * Wear's own list treatment, applied to CueSeek's custom rows. Until M5.17 nothing used it,
 * so an item scrolled to the edge kept its full width and the bezel clipped it — a vitals bar
 * running off the glass, a service row with half its status mark gone. On a wrist that read
 * as a phone layout that did not fit, which is exactly what Kushal said it looked like.
 * Items now narrow and fade into the curve and come back to full size in the middle, which
 * is where a round screen has room for them.
 *
 * Driven by scroll position, not time, so there is nothing here for the reduced-motion
 * preference to turn off.
 */
fun Modifier.morphAtEdges(
    scope: TransformingLazyColumnItemScope,
    spec: TransformationSpec,
): Modifier = with(scope) {
    this@morphAtEdges
        .transformedHeight(scope, spec)
        .graphicsLayer {
            with(spec) {
                applyContainerTransformation(scrollProgress)
                applyContentTransformation(scrollProgress)
            }
        }
}
