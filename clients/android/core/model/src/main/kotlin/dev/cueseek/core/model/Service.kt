package dev.cueseek.core.model

/**
 * A capability a service supports.
 *
 * [id] is deliberately not an enum. Capabilities arrive with agent updates, and a client
 * will routinely meet ids that did not exist when it was built — permanently, not
 * transitionally (ADR-0007). [label] exists so that such a client can render
 * "Immich Jobs — update CueSeek to view this" instead of an empty box.
 *
 * Clients look capabilities up in a map of id to renderer. Branching on service id is a
 * review-blocking defect (ADR-0005).
 */
data class Capability(
    val id: String,
    val label: String,
)

/**
 * How much care an action warrants.
 *
 * Clients gate confirmation on this **without knowing what the action does**, which is
 * what lets a new action ship to an existing client with an appropriate prompt already
 * attached.
 */
enum class ActionRisk(val wire: String) {
    /** Read-only or trivially reversible. */
    Safe("safe"),

    /** Interrupts the service, which comes back on its own. */
    Disruptive("disruptive"),

    /** May lose data, or need physical access to undo. */
    Destructive("destructive"),

    /**
     * A risk level this build has never heard of.
     *
     * Treated as at least as dangerous as [Destructive]. The alternative — defaulting to
     * safe — means a future action of unknown consequence is invoked with no prompt by
     * every client that predates it.
     */
    Unrecognised(""),
    ;

    /** Whether invoking this action should ask first. */
    val requiresConfirmation: Boolean
        get() = this != Safe

    /**
     * Whether the confirmation should be emphatic — a step-up rather than a dialog.
     *
     * The policy lives here rather than in a screen so that it is decided once. A second
     * screen deciding it differently is how a destructive action acquires a casual prompt.
     */
    val requiresEmphaticConfirmation: Boolean
        get() = this == Destructive || this == Unrecognised

    companion object {
        fun fromWire(value: String): ActionRisk =
            entries.firstOrNull { it != Unrecognised && it.wire == value } ?: Unrecognised
    }
}

/**
 * An action descriptor.
 *
 * Actions are **data, not enum cases**. Hardcoding a list of known actions in a client
 * discards capability discovery entirely (ADR-0005).
 */
data class Action(
    val id: String,
    val label: String,
    val risk: ActionRisk,
    /** Longer explanation, shown in a confirmation dialog. */
    val description: String?,
)

/** A managed service: what it can do, and how it is. */
data class Service(
    val id: String,
    val name: String,
    val capabilities: List<Capability>,
    val health: Health,
    /** Empty when the service does not implement the `control` capability. */
    val actions: List<Action>,
    /** Absent unless the service advertises the `web_ui` capability. */
    val webUi: WebUi? = null,
)
