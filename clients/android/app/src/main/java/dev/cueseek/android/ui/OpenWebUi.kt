package dev.cueseek.android.ui

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import androidx.core.net.toUri
import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.Service
import dev.cueseek.core.model.urlFor

/**
 * Where a tap on the body of a service row goes.
 *
 * A type rather than a nullable string so the fallback is a destination with a name. The
 * row is never inert: a service with no configured interface still has health, reasons and
 * capabilities worth reading, and showing them is a better answer to a tap than nothing.
 */
sealed interface RowDestination {
    data class WebUi(val url: String) : RowDestination
    data object Details : RowDestination
}

/**
 * Decides the destination without touching Android.
 *
 * Pure, and therefore testable: this is the one piece of the row interaction that can be
 * got wrong silently, since both outcomes look like "the tap did something".
 */
fun rowDestination(service: Service, address: AgentAddress): RowDestination {
    val url = service.webUi?.urlFor(address) ?: return RowDestination.Details
    return RowDestination.WebUi(url)
}

/**
 * Hands a service's own interface to whatever the user browses with.
 *
 * A plain `ACTION_VIEW`, with no package filtering and no attempt to notice that a native
 * app for this service is installed. Detecting one would mean this module knowing which
 * services have apps — a per-service branch in a client whose whole design rests on not
 * having any (ADR-0005) — and it would mean querying the installed package list to make a
 * decision the user has already made in their default-app settings. Android resolves this
 * intent against those settings, so a user who wants Jellyfin's app to open Jellyfin links
 * already gets it, and this code stays ignorant of which service it just opened.
 *
 * Returns `false` when nothing handled it, which is the caller's cue to do something
 * useful instead of leaving the tap unanswered.
 */
fun openWebUi(context: Context, url: String): Boolean {
    val intent = Intent(Intent.ACTION_VIEW, url.toUri())
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    return try {
        context.startActivity(intent)
        true
    } catch (_: ActivityNotFoundException) {
        // A device with no browser and no matching app. Rare, but the alternative to
        // catching it is a crash on a tap.
        false
    }
}
