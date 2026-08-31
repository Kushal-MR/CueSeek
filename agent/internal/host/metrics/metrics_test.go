package metrics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// These run on every platform, and that is the point of the package's layout. The build
// tag covers one syscall and a filesystem; every judgement call — counter differencing,
// which memory figure is honest, how a sensor is named — lives in untagged code and is
// exercised here whether the developer is on Linux or not.

// ---------------------------------------------------------------- CPU

func TestParseCPUTimes(t *testing.T) {
	// Real /proc/stat, trimmed. The per-core lines must be ignored: summing them as well
	// as the aggregate would double every counter.
	const stat = `cpu  100 20 50 800 30 0 5 0 0 0
cpu0 50 10 25 400 15 0 2 0 0 0
cpu1 50 10 25 400 15 0 3 0 0 0
intr 12345
`
	times, ok := parseCPUTimes(strings.NewReader(stat))
	if !ok {
		t.Fatal("parseCPUTimes reported failure on a well-formed file")
	}
	if want := uint64(1005); times.Total != want {
		t.Errorf("Total = %d, want %d", times.Total, want)
	}
	// idle 800 + iowait 30. iowait counts as idle: the CPU was not working, it was
	// waiting on a disk, and calling that busy would report saturation on a machine that
	// is merely slow to read.
	if want := uint64(830); times.Idle != want {
		t.Errorf("Idle = %d, want %d", times.Idle, want)
	}
}

func TestParseCPUTimesRejectsGarbage(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"no cpu line":     "intr 1 2 3\nctxt 99\n",
		"too few columns": "cpu 1 2\n",
		"unparseable":     "cpu 100 20 fifty 800 30\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := parseCPUTimes(strings.NewReader(content)); ok {
				// A partial sum would give a utilisation figure that is wrong rather
				// than missing, which is the worse of the two failures.
				t.Error("reported success on input it cannot trust")
			}
		})
	}
}

func TestUsageBetween(t *testing.T) {
	t.Run("first sample has no answer", func(t *testing.T) {
		// The case most likely to be papered over with a zero. Utilisation exists only
		// as a difference, so one sample is not a quiet machine — it is no measurement.
		if got := usageBetween(nil, cpuTimes{Total: 1000, Idle: 900}); got != nil {
			t.Errorf("usage = %v, want nil on the first sample", *got)
		}
	})

	t.Run("computes the busy fraction", func(t *testing.T) {
		previous := cpuTimes{Total: 1000, Idle: 900}
		got := usageBetween(&previous, cpuTimes{Total: 1200, Idle: 1050})
		if got == nil {
			t.Fatal("usage = nil, want a figure")
		}
		// 200 jiffies elapsed, 150 of them idle: 25% busy.
		if *got != 25 {
			t.Errorf("usage = %v, want 25", *got)
		}
	})

	t.Run("counters going backwards yield nothing", func(t *testing.T) {
		previous := cpuTimes{Total: 5000, Idle: 4000}
		if got := usageBetween(&previous, cpuTimes{Total: 100, Idle: 50}); got != nil {
			t.Errorf("usage = %v, want nil after a wrap", *got)
		}
	})

	t.Run("no elapsed time yields nothing", func(t *testing.T) {
		previous := cpuTimes{Total: 1000, Idle: 900}
		if got := usageBetween(&previous, previous); got != nil {
			t.Errorf("usage = %v, want nil; 0/0 is not zero percent", *got)
		}
	})

	t.Run("idle exceeding total yields nothing", func(t *testing.T) {
		previous := cpuTimes{Total: 1000, Idle: 900}
		if got := usageBetween(&previous, cpuTimes{Total: 1100, Idle: 1100}); got != nil {
			t.Errorf("usage = %v, want nil on impossible counters", *got)
		}
	})
}

func TestParseLoadAvg(t *testing.T) {
	load1, load5, load15 := parseLoadAvg("0.31 0.28 0.24 1/512 20293")
	for name, got := range map[string]*float32{"load1": load1, "load5": load5, "load15": load15} {
		if got == nil {
			t.Fatalf("%s = nil, want a value", name)
		}
	}
	if *load1 != 0.31 {
		t.Errorf("load1 = %v, want 0.31", *load1)
	}

	t.Run("partial line yields what it carried", func(t *testing.T) {
		one, five, fifteen := parseLoadAvg("1.50")
		if one == nil || *one != 1.5 {
			t.Error("load1 was lost from a truncated line")
		}
		if five != nil || fifteen != nil {
			t.Error("absent averages were invented")
		}
	})

	t.Run("garbage yields nothing", func(t *testing.T) {
		if one, _, _ := parseLoadAvg("not a load average"); one != nil {
			t.Errorf("load1 = %v, want nil", *one)
		}
	})
}

func TestParseUptime(t *testing.T) {
	got := parseUptime("350735.47 234388.90")
	if got == nil || *got != 350735 {
		t.Errorf("uptime = %v, want 350735", got)
	}
	if parseUptime("") != nil || parseUptime("nonsense") != nil {
		t.Error("an unreadable uptime became a number")
	}
}

// ---------------------------------------------------------------- memory

func TestParseMemInfo(t *testing.T) {
	const meminfo = `MemTotal:       16311512 kB
MemFree:          321044 kB
MemAvailable:   12004328 kB
Buffers:          158920 kB
Cached:          9812004 kB
SwapTotal:       2097148 kB
SwapFree:        2097148 kB
`
	memory := parseMemInfo(strings.NewReader(meminfo))
	if memory == nil {
		t.Fatal("memory = nil, want values")
	}

	const kib = 1024
	if want := int64(16311512 * kib); *memory.TotalBytes != want {
		t.Errorf("TotalBytes = %d, want %d", *memory.TotalBytes, want)
	}
	if want := int64(12004328 * kib); *memory.AvailableBytes != want {
		t.Errorf("AvailableBytes = %d, want %d", *memory.AvailableBytes, want)
	}

	// The whole point of preferring MemAvailable. This machine has 9.4 GiB of page cache
	// and only 313 MiB genuinely free; used must come out at 4.1 GiB, not 15.7.
	if want := int64((16311512 - 12004328) * kib); *memory.UsedBytes != want {
		t.Errorf("UsedBytes = %d, want %d — cache was counted as consumed",
			*memory.UsedBytes, want)
	}
	if *memory.SwapUsedBytes != 0 {
		t.Errorf("SwapUsedBytes = %d, want 0", *memory.SwapUsedBytes)
	}
}

func TestParseMemInfoPartial(t *testing.T) {
	// A cgroup or an unusual kernel may not publish MemAvailable. What it did publish
	// must survive, and what it did not must stay absent rather than be derived from a
	// worse proxy.
	memory := parseMemInfo(strings.NewReader("MemTotal:  1000 kB\nMemFree:  400 kB\n"))
	if memory == nil || memory.TotalBytes == nil {
		t.Fatal("TotalBytes was lost")
	}
	if memory.AvailableBytes != nil {
		t.Error("AvailableBytes was invented from MemFree")
	}
	if memory.UsedBytes != nil {
		t.Error("UsedBytes was computed without an available figure")
	}
}

func TestParseMemInfoEmpty(t *testing.T) {
	if got := parseMemInfo(strings.NewReader("")); got != nil {
		t.Error("an unreadable meminfo became a machine with zero bytes of memory")
	}
}

// ---------------------------------------------------------------- sensors

func TestParseTemperature(t *testing.T) {
	got := parseTemperature("47500\n")
	if got == nil || *got != 47.5 {
		t.Errorf("celsius = %v, want 47.5", got)
	}
	// Drivers publish 0 for a sensor they cannot read. Reporting a fictional 0°C on
	// every boot is worse than losing a genuine reading at exactly freezing.
	if parseTemperature("0") != nil {
		t.Error("a zero sentinel was reported as a real reading")
	}
	if parseTemperature("") != nil || parseTemperature("warm") != nil {
		t.Error("an unparseable reading became a temperature")
	}
}

func TestSensorLabel(t *testing.T) {
	cases := []struct{ chip, label, want string }{
		{"coretemp", "Package id 0", "coretemp Package id 0"},
		{"nvme", "Composite", "nvme Composite"},
		{"acpitz", "", "acpitz"},
		{"", "Composite", "Composite"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := sensorLabel(c.chip, c.label); got != c.want {
			t.Errorf("sensorLabel(%q, %q) = %q, want %q", c.chip, c.label, got, c.want)
		}
	}
}

// ---------------------------------------------------------------- collection

// fakeRoot writes a fixture tree the collector can be pointed at.
func fakeRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestCollectReadsEverything(t *testing.T) {
	root := fakeRoot(t, map[string]string{
		"proc/stat":        "cpu  100 20 50 800 30 0 0 0 0 0\ncpu0 50 10 25 400 15 0 0 0 0 0\ncpu1 50 10 25 400 15 0 0 0 0 0\n",
		"proc/loadavg":     "0.50 0.40 0.30 2/512 9999\n",
		"proc/uptime":      "12345.67 98765.43\n",
		"proc/meminfo":     "MemTotal: 1000 kB\nMemAvailable: 400 kB\nSwapTotal: 200 kB\nSwapFree: 150 kB\n",
		"proc/self/mounts": "/dev/sda2 / ext4 rw,relatime 0 0\n",

		"sys/class/hwmon/hwmon0/name":        "coretemp\n",
		"sys/class/hwmon/hwmon0/temp1_input": "47500\n",
		"sys/class/hwmon/hwmon0/temp1_label": "Package id 0\n",
		"sys/class/hwmon/hwmon0/temp1_max":   "84000\n",
		"sys/class/hwmon/hwmon0/temp1_crit":  "100000\n",
	})

	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	c := New([]string{"/"},
		WithRoot(root),
		WithClock(fixedClock(at)),
		WithDiskUsage(func(string) (int64, int64, error) { return 500, 120, nil }),
	)

	got := c.Collect()

	if !got.CollectedAt.Equal(at) {
		t.Errorf("CollectedAt = %v, want %v", got.CollectedAt, at)
	}
	if got.UptimeSeconds == nil || *got.UptimeSeconds != 12345 {
		t.Errorf("UptimeSeconds = %v, want 12345", got.UptimeSeconds)
	}

	if got.CPU == nil {
		t.Fatal("CPU = nil")
	}
	// First collection: a baseline exists but nothing to difference it against.
	if got.CPU.UsagePercent != nil {
		t.Errorf("UsagePercent = %v on the first collection, want nil", *got.CPU.UsagePercent)
	}
	if got.CPU.Cores == nil || *got.CPU.Cores != 2 {
		t.Errorf("Cores = %v, want 2", got.CPU.Cores)
	}
	if got.CPU.Load1 == nil || *got.CPU.Load1 != 0.5 {
		t.Errorf("Load1 = %v, want 0.5", got.CPU.Load1)
	}

	if got.Memory == nil || got.Memory.UsedBytes == nil {
		t.Fatal("Memory was not read")
	}
	if want := int64(600 * 1024); *got.Memory.UsedBytes != want {
		t.Errorf("UsedBytes = %d, want %d", *got.Memory.UsedBytes, want)
	}

	if len(got.Storage) != 1 {
		t.Fatalf("Storage = %v, want one filesystem", got.Storage)
	}
	if got.Storage[0].Filesystem != "/dev/sda2" {
		t.Errorf("Filesystem = %q, want /dev/sda2", got.Storage[0].Filesystem)
	}
	if got.Storage[0].FreeBytes != 120 {
		t.Errorf("FreeBytes = %d, want 120", got.Storage[0].FreeBytes)
	}

	if len(got.Thermal) != 1 {
		t.Fatalf("Thermal = %v, want one sensor", got.Thermal)
	}
	sensor := got.Thermal[0]
	if sensor.Label != "coretemp Package id 0" {
		t.Errorf("Label = %q", sensor.Label)
	}
	if sensor.Celsius != 47.5 {
		t.Errorf("Celsius = %v, want 47.5", sensor.Celsius)
	}
	// _max is preferred over _crit: critical is where the hardware protects itself, and
	// showing it would make a machine sat at 95°C look fine until it shut down.
	if sensor.HighCelsius == nil || *sensor.HighCelsius != 84 {
		t.Errorf("HighCelsius = %v, want 84", sensor.HighCelsius)
	}
}

func TestCollectSecondPassReportsUsage(t *testing.T) {
	root := fakeRoot(t, map[string]string{
		"proc/stat": "cpu  100 0 100 800 0 0 0 0 0 0\ncpu0 100 0 100 800 0 0 0 0 0 0\n",
	})
	c := New(nil, WithRoot(root), WithDiskUsage(func(string) (int64, int64, error) {
		return 0, 0, errors.New("no filesystem in this test")
	}))

	if first := c.Collect(); first.CPU.UsagePercent != nil {
		t.Fatal("the first collection produced a utilisation figure it cannot have")
	}

	// 400 more jiffies, 200 of them idle: 50% busy since the previous sample.
	os.WriteFile(filepath.Join(root, "proc", "stat"),
		[]byte("cpu  200 0 200 1000 0 0 0 0 0 0\ncpu0 200 0 200 1000 0 0 0 0 0 0\n"), 0o644)

	second := c.Collect()
	if second.CPU.UsagePercent == nil {
		t.Fatal("UsagePercent = nil on the second collection, want a figure")
	}
	if *second.CPU.UsagePercent != 50 {
		t.Errorf("UsagePercent = %v, want 50", *second.CPU.UsagePercent)
	}
}

func TestAbsenceIsNotZero(t *testing.T) {
	// A machine that publishes none of this. Every field must go missing rather than
	// arrive as a cold, idle, empty computer — the failure this whole payload is shaped
	// to avoid.
	c := New(nil,
		WithRoot(t.TempDir()),
		WithDiskUsage(func(string) (int64, int64, error) {
			return 0, 0, errors.New("no such mount")
		}),
	)
	got := c.Collect()

	if got.CPU != nil {
		t.Errorf("CPU = %+v, want nil", got.CPU)
	}
	if got.Memory != nil {
		t.Errorf("Memory = %+v, want nil", got.Memory)
	}
	if got.UptimeSeconds != nil {
		t.Errorf("UptimeSeconds = %v, want nil", *got.UptimeSeconds)
	}
	if got.Thermal != nil {
		t.Errorf("Thermal = %v, want nil — the sensor tree could not be walked", got.Thermal)
	}
	if got.CollectedAt.IsZero() {
		t.Error("CollectedAt is zero; a metric without a clock cannot be judged stale")
	}
}

func TestStorageEmptyIsNotAbsent(t *testing.T) {
	// The distinction the wire has to carry. A configured mount that failed to measure
	// means "we asked and nothing answered", which is not the same as never asking.
	c := New([]string{"/media"},
		WithRoot(t.TempDir()),
		WithDiskUsage(func(string) (int64, int64, error) {
			return 0, 0, errors.New("gone away")
		}),
	)
	got := c.Collect()

	if got.Storage == nil {
		t.Fatal("Storage = nil, want an empty slice: a mount was configured and asked about")
	}
	if len(got.Storage) != 0 {
		t.Errorf("Storage = %v, want empty", got.Storage)
	}
}

func TestThermalEmptyWhenNoSensors(t *testing.T) {
	// hwmon exists and holds nothing usable. Ordinary on servers and universal in virtual
	// machines, and it means something different from being unable to look.
	root := fakeRoot(t, map[string]string{"sys/class/hwmon/hwmon0/name": "nothing\n"})
	got := New(nil, WithRoot(root)).Collect()

	if got.Thermal == nil {
		t.Fatal("Thermal = nil, want an empty slice: the tree was walked")
	}
	if len(got.Thermal) != 0 {
		t.Errorf("Thermal = %v, want empty", got.Thermal)
	}
}

func TestThermalSortedByLabel(t *testing.T) {
	// Not by temperature, deliberately. Ordering by the reading would reshuffle the list
	// between collections as fans spin up, and rows that move under a thumb every ten
	// seconds are worse than rows in a duller order.
	root := fakeRoot(t, map[string]string{
		"sys/class/hwmon/hwmon0/name":        "nvme\n",
		"sys/class/hwmon/hwmon0/temp1_input": "38000\n",
		"sys/class/hwmon/hwmon0/temp1_label": "Composite\n",
		"sys/class/hwmon/hwmon1/name":        "acpitz\n",
		"sys/class/hwmon/hwmon1/temp1_input": "62000\n",
	})
	got := New(nil, WithRoot(root)).Collect()

	if len(got.Thermal) != 2 {
		t.Fatalf("Thermal = %v, want two sensors", got.Thermal)
	}
	if got.Thermal[0].Label != "acpitz" {
		t.Errorf("first = %q, want acpitz — sorted by label, not by heat",
			got.Thermal[0].Label)
	}
}

func TestMountDeviceUnescaping(t *testing.T) {
	// The kernel escapes spaces in mount paths as \040. Without unescaping, a lookup by
	// the operator's configured path silently misses and the filesystem label vanishes.
	root := fakeRoot(t, map[string]string{
		"proc/self/mounts": "/dev/sdb1 /mnt/my\\040drive ext4 rw 0 0\n",
	})
	c := New([]string{"/mnt/my drive"},
		WithRoot(root),
		WithDiskUsage(func(string) (int64, int64, error) { return 10, 5, nil }),
	)
	got := c.Collect()

	if len(got.Storage) != 1 || got.Storage[0].Filesystem != "/dev/sdb1" {
		t.Errorf("Storage = %+v, want the device resolved through the escaped path",
			got.Storage)
	}
}

func TestRunPublishesUntilCancelled(t *testing.T) {
	root := fakeRoot(t, map[string]string{"proc/uptime": "42.0 0.0\n"})
	c := New(nil, WithRoot(root))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	published := make(chan domain.HostMetrics, 4)
	go c.Run(ctx, 20*time.Millisecond, func(m domain.HostMetrics) { published <- m })

	// The first arrives after the priming delay, which exists so the opening payload
	// already carries a utilisation figure rather than an absent one.
	select {
	case got := <-published:
		if got.UptimeSeconds == nil || *got.UptimeSeconds != 42 {
			t.Errorf("first collection = %v, want uptime 42", got.UptimeSeconds)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was published")
	}

	select {
	case <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("the ticker stopped after one collection")
	}
}

func TestRunStopsOnCancelDuringPriming(t *testing.T) {
	// Cancelled inside the priming wait, which is the window a restart is most likely to
	// land in. It must return rather than sit out the delay and publish once anyway.
	c := New(nil, WithRoot(t.TempDir()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		c.Run(ctx, time.Hour, func(domain.HostMetrics) {
			t.Error("published after the context was cancelled")
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return on a cancelled context")
	}
}
