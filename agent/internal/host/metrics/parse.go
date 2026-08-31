package metrics

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// The parsers.
//
// Deliberately separated from the file reads, and not behind a build tag. Every judgement
// call in this package is in here — how a counter difference becomes a percentage, which
// memory number is the honest one, what an unparseable line means — and none of it should
// only be testable on the deployment platform. The build-tagged file does IO and calls
// these; that split is what lets the arithmetic be tested on a developer machine that is
// not Linux.

// cpuTimes is one sample of the kernel's cumulative CPU counters, in jiffies.
//
// Absolute values are meaningless on their own: they count since boot, so a single sample
// says only that the machine has been on. Only the difference between two is information.
type cpuTimes struct {
	// Total is every counter summed, including idle.
	Total uint64

	// Idle is idle plus iowait.
	//
	// iowait is counted as idle on purpose. It is time the CPU spent with nothing to run
	// because a process was blocked on disk — the processor was not working, and calling
	// it busy would report a saturated CPU on a machine that is merely waiting for a slow
	// drive. That case is common on exactly this kind of box, where a torrent client and
	// a media server share one disk. The waiting shows up in load average instead, which
	// is one of the reasons both numbers are reported.
	Idle uint64
}

// parseCPUTimes reads the aggregate `cpu` line from /proc/stat.
//
// Returns ok=false rather than an error for anything unexpected. There is nothing a caller
// could do differently, and the honest outcome is the same either way: no CPU sample this
// time, so no utilisation figure, so the field goes absent.
func parseCPUTimes(r io.Reader) (cpuTimes, bool) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// The aggregate line is "cpu" exactly; "cpu0", "cpu1" are per-core and skipped.
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}

		var times cpuTimes
		for i, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				// A malformed column makes the whole sample untrustworthy: a partial sum
				// would produce a utilisation figure that is wrong rather than missing.
				return cpuTimes{}, false
			}
			times.Total += value
			// Columns are user, nice, system, idle, iowait, ... in a fixed order.
			if i == 3 || i == 4 {
				times.Idle += value
			}
		}
		return times, true
	}
	return cpuTimes{}, false
}

// usageBetween turns two samples into a percentage, or nil when it cannot.
//
// Nil in three cases, all of which are real:
//
//   - No previous sample. The first collection after startup has nothing to difference
//     against, so there is no answer to give.
//   - No elapsed jiffies. Two samples taken inside one kernel tick differ by nothing, and
//     0/0 is not zero percent.
//   - The counters went backwards. They are monotonic in normal operation, so this means a
//     wrap or a rebooted view of the world, and the difference is meaningless.
func usageBetween(previous *cpuTimes, current cpuTimes) *float32 {
	if previous == nil || current.Total <= previous.Total || current.Idle < previous.Idle {
		return nil
	}
	total := current.Total - previous.Total
	idle := current.Idle - previous.Idle
	if idle > total {
		return nil
	}

	usage := float32(total-idle) / float32(total) * 100
	return clampPercent(usage)
}

func clampPercent(value float32) *float32 {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return &value
}

// parseLoadAvg reads /proc/loadavg: "0.31 0.28 0.24 1/512 20293".
//
// Partial success is allowed. Three averages parse independently, so a truncated line still
// yields what it did carry rather than nothing.
func parseLoadAvg(content string) (load1, load5, load15 *float32) {
	fields := strings.Fields(content)
	loads := [...]**float32{&load1, &load5, &load15}
	for i, target := range loads {
		if i >= len(fields) {
			break
		}
		if value, err := strconv.ParseFloat(fields[i], 32); err == nil && value >= 0 {
			parsed := float32(value)
			*target = &parsed
		}
	}
	return load1, load5, load15
}

// parseUptime reads /proc/uptime: "350735.47 234388.90". Nil when unreadable.
func parseUptime(content string) *int64 {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return nil
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || seconds < 0 {
		return nil
	}
	whole := int64(seconds)
	return &whole
}

// parseMemInfo reads /proc/meminfo.
//
// Returns nil when the file carried nothing recognisable, so an unreadable source stays
// absent rather than arriving as a machine with zero bytes of memory.
func parseMemInfo(r io.Reader) *domain.MemoryMetrics {
	// Every value in this file is in kB despite the kernel labelling it kB and meaning
	// KiB. The factor is 1024, which is the convention every tool that reads this file
	// follows, and getting it wrong understates memory by 2.4%.
	const kib = 1024

	values := make(map[string]int64, 8)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		key, raw, found := strings.Cut(scanner.Text(), ":")
		if !found {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			continue
		}
		if value, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
			values[key] = value * kib
		}
	}
	if len(values) == 0 {
		return nil
	}

	memory := &domain.MemoryMetrics{
		TotalBytes:     lookup(values, "MemTotal"),
		AvailableBytes: lookup(values, "MemAvailable"),
		SwapTotalBytes: lookup(values, "SwapTotal"),
	}

	// Used is derived rather than read, and only when both inputs exist. MemAvailable is
	// the kernel's own estimate of what a new process could obtain, which already accounts
	// for reclaimable page cache — the reason a long-running media server looks alarming to
	// anything that subtracts MemFree instead.
	if memory.TotalBytes != nil && memory.AvailableBytes != nil {
		used := *memory.TotalBytes - *memory.AvailableBytes
		if used < 0 {
			used = 0
		}
		memory.UsedBytes = &used
	}

	// Swap has no "available" equivalent, so used is total minus free.
	if free := lookup(values, "SwapFree"); memory.SwapTotalBytes != nil && free != nil {
		used := *memory.SwapTotalBytes - *free
		if used < 0 {
			used = 0
		}
		memory.SwapUsedBytes = &used
	}

	return memory
}

func lookup(values map[string]int64, key string) *int64 {
	value, ok := values[key]
	if !ok {
		return nil
	}
	return &value
}

// parseTemperature converts a hwmon millidegree reading to celsius.
//
// Nil for anything unparseable and for the sentinel zero that some drivers publish for a
// sensor they cannot actually read. Zero degrees is a physically possible temperature, so
// this discards a real reading in the rare case a machine is at exactly freezing — a much
// smaller cost than reporting a fictional 0°C on every boot for a sensor that is not there.
func parseTemperature(content string) *float32 {
	raw, err := strconv.ParseInt(strings.TrimSpace(content), 10, 64)
	if err != nil || raw == 0 {
		return nil
	}
	celsius := float32(raw) / 1000
	return &celsius
}

// sensorLabel composes the name a client will display.
//
// Two parts because neither alone identifies a sensor: the chip name says what hardware it
// is ("coretemp", "nvme") and the label says which sensor on it ("Package id 0",
// "Composite"). A list of four readings all called "temp1" would be useless.
func sensorLabel(chip, label string) string {
	chip, label = strings.TrimSpace(chip), strings.TrimSpace(label)
	switch {
	case chip == "" && label == "":
		return ""
	case chip == "":
		return label
	case label == "":
		return chip
	default:
		return chip + " " + label
	}
}
