package dev.cueseek.core.design.token

import androidx.compose.foundation.shape.CornerSize
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Shapes
import androidx.compose.ui.unit.dp

/**
 * CueSeek's shape grammar.
 *
 * Roundness alternates by level rather than being applied everywhere, so that shape
 * carries hierarchy instead of just friendliness:
 *
 * | Level | Shape | Why |
 * |---|---|---|
 * | Screen | none | it is the page |
 * | Header | **no container at all** | flat and quiet; type alone carries it |
 * | Tally rule | full | reduced to a rule; the only fully-round strip |
 * | Roster | extra large, 28dp | one soft mass holding the work |
 * | Rows | none | quiet inside the container |
 * | Status mark | full | a closed circle means settled |
 * | Actions | full | standard M3 |
 *
 * The header having no surface is the load-bearing decision. It is what makes the roster
 * read as the one substantial object on screen, and it is why adding a summary card would
 * undo the hierarchy rather than reinforce it.
 */
object CueSeekShapes {

    val Shapes = Shapes(
        extraSmall = RoundedCornerShape(4.dp),
        small = RoundedCornerShape(8.dp),
        medium = RoundedCornerShape(12.dp),
        large = RoundedCornerShape(16.dp),
        extraLarge = RoundedCornerShape(28.dp),
    )

    /** The service roster. One container, many rows. */
    val Roster = RoundedCornerShape(28.dp)

    /** The status mark and the beat dot. A closed circle means we have an answer. */
    val Mark = RoundedCornerShape(percent = 50)

    /** Segments of the tally rule. */
    val TallySegment = RoundedCornerShape(percent = 50)

    /**
     * Rows are square on purpose.
     *
     * A rounded row inside a rounded container produces two competing radii and reads as a
     * card that failed to separate. The container is the shape; rows are its contents.
     */
    val Row = RoundedCornerShape(CornerSize(0.dp))
}
