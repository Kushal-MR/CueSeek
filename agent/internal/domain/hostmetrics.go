package domain

import "time"

// Host metrics: the machine's own vitals, as distinct from any service running on it.
//
// These are not a service capability and are deliberately not modelled as one. A service
// has health, actions and an owner; the host has none of those in the same sense, and
// making it a pseudo-service would have put the machine in the dashboard's tally — so
// "two of three healthy" would have started counting the computer as one of its own
// services (ADR-0005's scope rule).
//
// # Optional means optional
//
// Every field below except CollectedAt is a pointer or a nilable slice, and nil means
// "could not read" rather than zero. This is the same rule the activity capabilities
// established in M3.5, and it matters more here: hardware genuinely differs. A virtual
// machine exposes no temperature sensors, a container may see no usable /proc/stat, and
// the first collection after startup cannot compute CPU utilisation at all. Every one of
// those is an honest "unknown", and rendering any of them as zero would report an idle,
// cold machine that never answered the question.

// HostMetrics is one collection of the host's vitals.
type HostMetrics struct {
	// CollectedAt is when these were read. Never zero — a metric whose age is unknown
	// cannot be judged stale, and everything else here is worthless once it is.
	CollectedAt time.Time

	// UptimeSeconds is how long the host has been up. Nil when unreadable.
	UptimeSeconds *int64

	CPU    *CPUMetrics
	Memory *MemoryMetrics

	// Storage and Thermal distinguish nil from empty, and the distinction is carried all
	// the way to the wire. Nil means the agent could not look; an empty slice means it
	// looked and found nothing — an ordinary answer for thermals on consumer hardware,
	// and a meaningful one for storage when a configured mount has gone away.
	Storage []StorageMetrics
	Thermal []ThermalMetrics
}

// CPUMetrics is processor load.
type CPUMetrics struct {
	// UsagePercent is busy time across all cores over the interval since the previous
	// collection, 0 to 100.
	//
	// Nil on the first collection after the agent starts, and that is correct rather
	// than a gap to be filled. The kernel reports cumulative counters, so utilisation
	// exists only as a difference between two samples; with one sample there is no
	// answer, and zero would claim an idle machine.
	UsagePercent *float32

	// Cores is logical cores, which is what Load1 has to be read against — a load of 4
	// is saturation on a quad-core and half idle on an eight.
	Cores *int

	// Load averages, nil when /proc/loadavg is unreadable.
	//
	// Kept alongside UsagePercent rather than instead of it because they measure
	// different things: load counts processes blocked on disk as well as on CPU, so a
	// box thrashing its swap sits near zero usage and near-unusable load. Either number
	// alone would mislead in a case the other catches.
	Load1  *float32
	Load5  *float32
	Load15 *float32
}

// MemoryMetrics is physical memory and swap.
type MemoryMetrics struct {
	TotalBytes *int64

	// AvailableBytes is the kernel's own estimate of what a new process could get, and
	// is not TotalBytes minus some notion of "used".
	//
	// This is the number that matters, and computing it any other way is the classic way
	// to make a healthy Linux box look like it is out of memory: the page cache is
	// counted as consumed even though it is reclaimable on demand. A media server that
	// has been up for a week will have cached tens of gigabytes and be perfectly fine.
	AvailableBytes *int64

	// UsedBytes is Total minus Available, present only when both are.
	UsedBytes *int64

	SwapTotalBytes *int64
	SwapUsedBytes  *int64
}

// StorageMetrics is one watched filesystem.
type StorageMetrics struct {
	// Mount is the path the operator configured, verbatim. Not resolved or canonicalised:
	// it is what they will recognise on screen.
	Mount string

	// Filesystem is the backing device where known. Display only, and empty when the
	// platform does not offer it cheaply.
	Filesystem string

	TotalBytes int64

	// FreeBytes is what an unprivileged process may actually use, which is deliberately
	// not the raw free count — that includes the root reserve, typically five percent,
	// which the operator cannot spend and should not be told they can.
	FreeBytes int64
}

// ThermalMetrics is one temperature sensor.
type ThermalMetrics struct {
	// Label is the sensor's own name, verbatim: "coretemp Package id 0", "nvme
	// Composite", "acpitz". Not mapped onto a shared vocabulary, for the same reason a
	// transfer's State is not — sensor naming is hardware-specific, and any shared
	// vocabulary would either drop sensors or mislabel them.
	Label string

	Celsius float32

	// HighCelsius is the threshold the hardware itself calls high, when it publishes one.
	// Carried so a client can say whether a reading is bad without hard-coding a number
	// for silicon it has never seen.
	HighCelsius *float32
}
