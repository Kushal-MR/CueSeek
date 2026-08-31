// Package metrics reads the host machine's own vitals: CPU, memory, storage and
// temperature.
//
// A sibling of the systemd control in the parent package rather than part of it, because
// the two share a subject and nothing else. Control is D-Bus calls that need polkit to
// authorise them; this is reading world-readable files under /proc and /sys and needs no
// privilege at all. Keeping them in one package would put two unrelated mechanisms behind
// one door, and the reason CueSeek's privileges are auditable in a minute is that the
// privileged surface stays small and obvious.
//
// # Nothing here is required
//
// Every value is optional and absence means "could not read". Hardware differs: consumer
// laptops publish sensors that servers do not, virtual machines publish none, and the first
// collection after startup cannot produce CPU utilisation at all because the kernel counts
// cumulatively. Reporting any of those as zero would describe an idle, cold machine that
// never answered.
package metrics

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// DefaultMounts is what gets watched when configuration says nothing.
//
// Just the root filesystem. Guessing further would be wrong more often than right — a
// stranger's box may keep its media on /mnt/media, /srv, a ZFS dataset or nothing at all —
// and a dashboard listing filesystems the operator does not care about is worse than one
// listing the single filesystem everybody has.
var DefaultMounts = []string{"/"}

// maxSensors bounds the thermal list.
//
// Sensible hardware exposes a handful. A board exposing sixty would otherwise put sixty
// objects on a stream frame every ten seconds, which is the same bandwidth argument that
// caps the activity samples — and unlike those, nobody is going to read the sixtieth
// temperature.
const maxSensors = 20

// Collector reads the host's vitals, holding the one piece of state the job needs.
//
// Safe for concurrent use, though in practice one goroutine calls it on a ticker.
type Collector struct {
	mounts []string

	// root prefixes every path read, so tests can point the collector at a directory of
	// fixtures. Empty in production, which leaves the paths absolute.
	root string

	now func() time.Time

	// diskUsage is a field so a test can supply filesystem sizes without a filesystem.
	// It is the one thing here that is a syscall rather than a file read, so it is also
	// the one thing a fixture directory cannot stand in for.
	diskUsage func(path string) (total, free int64, err error)

	mu sync.Mutex

	// previousCPU is the last sample, and the entire reason this type has state.
	//
	// /proc/stat counts since boot, so utilisation is a difference between two readings.
	// Keeping the previous one here is what makes the second collection able to answer a
	// question the first cannot.
	previousCPU *cpuTimes
}

// Option configures a Collector.
type Option func(*Collector)

// WithRoot prefixes the filesystem paths this collector reads. For tests.
func WithRoot(root string) Option { return func(c *Collector) { c.root = root } }

// WithClock replaces the source of CollectedAt. For tests.
func WithClock(now func() time.Time) Option { return func(c *Collector) { c.now = now } }

// WithDiskUsage replaces the filesystem measurement. For tests.
func WithDiskUsage(fn func(path string) (total, free int64, err error)) Option {
	return func(c *Collector) { c.diskUsage = fn }
}

// New prepares a collector for the given mount points.
func New(mounts []string, opts ...Option) *Collector {
	if len(mounts) == 0 {
		mounts = DefaultMounts
	}
	c := &Collector{
		mounts:    append([]string(nil), mounts...),
		now:       func() time.Time { return time.Now().UTC() },
		diskUsage: diskUsage,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Collect reads everything available right now.
//
// Cannot fail, and deliberately so. Every individual failure — an unreadable file, a mount
// that has gone away, a machine with no sensors — leaves that field absent and the rest of
// the collection intact, because a missing temperature is no reason to withhold the memory
// figure. Whether the platform can answer at all is a separate question, decided once by
// [Supported] rather than re-asked on every tick.
func (c *Collector) Collect() domain.HostMetrics {
	return domain.HostMetrics{
		CollectedAt:   c.now(),
		UptimeSeconds: parseUptime(c.readFile("/proc/uptime")),
		CPU:           c.cpu(),
		Memory:        c.memory(),
		Storage:       c.storage(),
		Thermal:       c.thermal(),
	}
}

// primeDelay is how long Run waits between taking its baseline CPU sample and publishing
// its first collection.
//
// It exists so the very first payload a client sees carries a real utilisation figure
// instead of an absent one. Without it the field would be missing for a whole interval,
// which is honest but means opening the app within ten seconds of an agent restart shows a
// machine with no CPU reading. A second is long enough to difference two samples and short
// enough that nothing waits on it noticeably.
const primeDelay = time.Second

// Run collects on a ticker until the context is cancelled, handing each collection to
// publish.
//
// Its own goroutine and its own interval, deliberately not part of the adapter poller.
// That loop is one goroutine per service, each with a request timeout and a nudge channel,
// and none of those apply here: there is no adapter, no upstream to time out against, and
// no health to report. Folding the host into it would have meant teaching a per-service
// loop about something that is not a service.
func (c *Collector) Run(ctx context.Context, interval time.Duration, publish func(domain.HostMetrics)) {
	// Baseline first, so the first published collection can already report utilisation.
	c.cpuUsage()

	select {
	case <-ctx.Done():
		return
	case <-time.After(primeDelay):
	}
	publish(c.Collect())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			publish(c.Collect())
		}
	}
}

func (c *Collector) path(p string) string {
	if c.root == "" {
		return p
	}
	return filepath.Join(c.root, filepath.FromSlash(p))
}

// readFile returns a file's contents, or "" if it could not be read. The distinction
// between "missing" and "empty" is made by the parsers, all of which treat both as absent.
func (c *Collector) readFile(p string) string {
	content, err := os.ReadFile(c.path(p))
	if err != nil {
		return ""
	}
	return string(content)
}

// cpu samples utilisation and load.
//
// Returns nil only when nothing at all was readable. A machine that yields load averages
// but no /proc/stat still reports the load, because half an answer is more use than none
// and the shape says which half is missing.
func (c *Collector) cpu() *domain.CPUMetrics {
	usage := c.cpuUsage()
	load1, load5, load15 := parseLoadAvg(c.readFile("/proc/loadavg"))
	cores := c.cores()

	if usage == nil && load1 == nil && cores == nil {
		return nil
	}
	return &domain.CPUMetrics{
		UsagePercent: usage,
		Cores:        cores,
		Load1:        load1,
		Load5:        load5,
		Load15:       load15,
	}
}

// cpuUsage differences this sample against the previous one and stores it for the next.
//
// The store happens whether or not a percentage could be produced, which is what makes the
// first collection's nil self-correcting: it records the baseline that lets the second one
// answer.
func (c *Collector) cpuUsage() *float32 {
	file, err := os.Open(c.path("/proc/stat"))
	if err != nil {
		return nil
	}
	defer file.Close()

	current, ok := parseCPUTimes(file)
	if !ok {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	usage := usageBetween(c.previousCPU, current)
	c.previousCPU = &current
	return usage
}

// cores counts logical CPUs from the per-core lines in /proc/stat.
//
// Counted from the same file rather than taken from runtime.NumCPU, which reports the
// count visible to this process. Those differ under a CPU affinity mask or in a container,
// and the number a load average must be read against is the machine's, not the agent's.
func (c *Collector) cores() *int {
	content := c.readFile("/proc/stat")
	if content == "" {
		return nil
	}
	count := 0
	for _, line := range strings.Split(content, "\n") {
		name, _, _ := strings.Cut(line, " ")
		if len(name) > 3 && strings.HasPrefix(name, "cpu") {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return &count
}

func (c *Collector) memory() *domain.MemoryMetrics {
	file, err := os.Open(c.path("/proc/meminfo"))
	if err != nil {
		return nil
	}
	defer file.Close()
	return parseMemInfo(file)
}

// storage measures each configured mount.
//
// Returns a slice whenever mounts are configured, even if every one of them failed. That
// empty slice is information — it says the agent asked about filesystems and none answered,
// which is a different fact from this platform not doing storage at all.
func (c *Collector) storage() []domain.StorageMetrics {
	if len(c.mounts) == 0 {
		return nil
	}
	devices := c.mountDevices()

	out := make([]domain.StorageMetrics, 0, len(c.mounts))
	for _, mount := range c.mounts {
		total, free, err := c.diskUsage(c.path(mount))
		if err != nil {
			// A mount that has gone away, or was never there. Skipped rather than
			// reported as zero bytes, which would read as a full disk.
			continue
		}
		out = append(out, domain.StorageMetrics{
			Mount:      mount,
			Filesystem: devices[mount],
			TotalBytes: total,
			FreeBytes:  free,
		})
	}
	return out
}

// mountDevices maps mount point to backing device, for display only.
//
// Best effort by design: an unreadable /proc/self/mounts costs a label, not a measurement,
// so it returns an empty map rather than failing the collection.
func (c *Collector) mountDevices() map[string]string {
	content := c.readFile("/proc/self/mounts")
	devices := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// Fields are device, mount point, type, options. The mount point is escaped —
		// a space in a path arrives as \040 — and is unescaped so a lookup by the
		// operator's configured path matches.
		devices[unescapeMount(fields[1])] = fields[0]
	}
	return devices
}

// unescapeMount reverses the octal escaping the kernel applies to mount paths.
func unescapeMount(path string) string {
	if !strings.Contains(path, `\`) {
		return path
	}
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(path)
}

// thermal enumerates hwmon sensors.
//
// Returns nil when the hwmon tree could not be walked at all, and an empty slice when it
// was walked and held nothing usable. That second case is completely ordinary — plenty of
// hardware and every virtual machine reports no temperatures — and it is not a failure to
// be logged or a gap to be filled.
func (c *Collector) thermal() []domain.ThermalMetrics {
	chips, err := filepath.Glob(c.path("/sys/class/hwmon/hwmon*"))
	if err != nil || chips == nil {
		return nil
	}

	out := make([]domain.ThermalMetrics, 0, len(chips))
	for _, chip := range chips {
		name := strings.TrimSpace(readRaw(filepath.Join(chip, "name")))

		inputs, err := filepath.Glob(filepath.Join(chip, "temp*_input"))
		if err != nil {
			continue
		}
		for _, input := range inputs {
			celsius := parseTemperature(readRaw(input))
			if celsius == nil {
				continue
			}
			prefix := strings.TrimSuffix(input, "_input")
			label := sensorLabel(name, readRaw(prefix+"_label"))
			if label == "" {
				// Nothing identifies this sensor, so a reading from it cannot be
				// labelled honestly. Better dropped than shown as an anonymous number.
				continue
			}
			out = append(out, domain.ThermalMetrics{
				Label:       label,
				Celsius:     *celsius,
				HighCelsius: highThreshold(prefix),
			})
		}
	}

	// Sorted by label, not by temperature. Ordering by the reading would make the list
	// reshuffle between collections as fans spin up — the same instability that made
	// M3.5's transfer ordering wrong — and a list that reorders under a thumb every ten
	// seconds is worse than one in a duller but predictable order.
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	if len(out) > maxSensors {
		out = out[:maxSensors]
	}
	return out
}

// highThreshold prefers the driver's "high" over its "critical".
//
// Critical is the temperature at which hardware protects itself by throttling or cutting
// power; high is the one a human should act on. Showing critical as the threshold would
// mean a machine sat at 95°C looked fine right up until it shut down.
func highThreshold(prefix string) *float32 {
	for _, suffix := range []string{"_max", "_crit"} {
		if value := parseTemperature(readRaw(prefix + suffix)); value != nil {
			return value
		}
	}
	return nil
}

func readRaw(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}
