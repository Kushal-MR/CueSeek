package dev.cueseek.wear.power

import dev.cueseek.core.model.Action
import dev.cueseek.core.model.Scope
import dev.cueseek.core.model.Service

/**
 * Whether a wrist may end a machine, and what that means for the screen.
 *
 * # Why this is not "one paragraph and no code"
 *
 * The M5 plan allowed for that outcome: *if* M5.0 decided the watch does not get
 * `host.power`, M5.7 is documentation. ADR-0014 decided something narrower than that. It
 * made the scope **not a default**, and then wrote the sentence that settles this phase:
 *
 * > An operator who wants it can ask for it by name, exactly as on the phone, and
 * > press-and-hold still applies.
 *
 * "Press-and-hold still applies" is only true if the control exists. So the decision M5.0
 * actually took was *ungranted by default*, not *impossible* — and a watch that silently
 * dropped a grant an operator typed out by name would be quietly overruling them.
 *
 * There is also a fact on the wire that made the no-code reading untenable. The agent
 * returns the power actions to **every** caller holding `read`, on purpose: what the agent
 * *offers* is a property of the agent, and what a device may *do* is a property of its
 * token (`agent/internal/api/hostpower.go`). The watch's snapshot has therefore been
 * carrying reboot and shut down since M5.4, and dropping them on the floor was an accident
 * of nobody having written the screen — not a policy. This makes it a policy.
 *
 * # The three answers, and why hiding is right here and wrong on the phone
 *
 * The phone shows the power items greyed out with a sentence underneath saying the device
 * was never granted permission. That is the correct behaviour *there*: they sit inside a
 * menu somebody opened deliberately, the explanation costs a line nobody was using, and
 * learning why is the point.
 *
 * A watch has no such line. A permanently dead control would occupy the most valuable
 * pixels on a 233dp screen for an explanation that cannot fit next to it, on the device
 * least able to act on it — and the phone in the same pocket already says it properly.
 * So [Ungranted] removes the entry point entirely.
 *
 * [NoneOffered] is the case that justifies the agent's design. A granted watch talking to
 * an agent with no power support gets a screen that says so, rather than an empty one — the
 * difference between "this agent cannot" and "you were not allowed", which are two problems
 * with completely different fixes and which a scope-filtered list could not tell apart.
 */
sealed interface PowerAccess {

    /** No `host.power` in this device's token. The dashboard shows no way in. */
    data object Ungranted : PowerAccess

    /** Granted, but this agent offers nothing — a platform that cannot, or an older build. */
    data object NoneOffered : PowerAccess

    data class Offered(val actions: List<Action>) : PowerAccess
}

/**
 * The gate, from the token's scopes and the agent's offer.
 *
 * Scope first, deliberately. This is user experience and never a control: enforcement is
 * entirely server-side and [dev.cueseek.core.model.ApiError.InsufficientScope] is handled
 * regardless, because a client's idea of its own permissions is a cached claim and the
 * agent's is the fact.
 */
fun powerAccess(scopes: Set<Scope>, hostActions: List<Action>): PowerAccess = when {
    Scope.HostPower !in scopes -> PowerAccess.Ungranted
    hostActions.isEmpty() -> PowerAccess.NoneOffered
    else -> PowerAccess.Offered(hostActions)
}

/**
 * What the machine is in the middle of, as one short phrase, or null when nothing is.
 *
 * The phone says "2 streams playing and 1 transfer running" at the moment somebody is
 * deciding whether to shut a machine down. The **fact** is shared — the counts come from the
 * same activity capabilities, and a watch that disagreed with the phone about the same host
 * would be a console contradicting itself. The **sentence** is not: a wrist gets the counts
 * and nothing else, because the row is about 24 characters wide and every word spent on
 * grammar is one not spent on the number.
 *
 * Null when nothing is happening, so the screen stays quiet rather than reassuring. "Nothing
 * is running" would be a claim about services whose activity the agent could not read
 * either, and this cannot tell those two apart.
 */
/**
 * What the machine's power screen may claim about what it is about to interrupt.
 *
 * Three outcomes, and the middle one is why this is a function rather than an `if`:
 *
 * | | |
 * | --- | --- |
 * | [BusyClaim.Unknown] | the reading has aged out — say so |
 * | [BusyClaim.Busy] | something is running, and here is what |
 * | [BusyClaim.Quiet] | asked, and nothing is running — say nothing |
 *
 * **Stale is not the same as quiet, and collapsing them is the bug worth a test.** Both
 * would render as an empty line, and on a screen whose buttons end a machine, "nothing is
 * running" and "I have no idea what is running" are the two sentences that must never be
 * confused. The first invites the button; the second should give pause.
 */
sealed interface BusyClaim {
    /** The reading is too old to claim anything from. */
    data object Unknown : BusyClaim

    /** Nothing is running, as of a reading recent enough to believe. */
    data object Quiet : BusyClaim

    data class Busy(val summary: String) : BusyClaim
}

fun busyClaim(services: List<Service>, stale: Boolean): BusyClaim = when {
    stale -> BusyClaim.Unknown
    else -> wearBusySummary(services)?.let(BusyClaim::Busy) ?: BusyClaim.Quiet
}

fun wearBusySummary(services: List<Service>): String? {
    val playing = services.sumOf { it.nowPlaying?.sessions ?: 0 }
    val transferring = services.sumOf { it.transfers?.active ?: 0 }

    return buildList {
        if (playing > 0) add("$playing playing")
        if (transferring > 0) add("$transferring transferring")
    }.takeIf { it.isNotEmpty() }?.joinToString(" · ")
}
