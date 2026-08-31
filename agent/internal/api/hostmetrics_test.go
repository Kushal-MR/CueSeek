package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func sampleMetrics(at time.Time) domain.HostMetrics {
	return domain.HostMetrics{
		CollectedAt:   at,
		UptimeSeconds: ptr(int64(86400)),
		CPU: &domain.CPUMetrics{
			UsagePercent: ptr(float32(23.5)),
			Cores:        ptr(8),
			Load1:        ptr(float32(1.25)),
		},
		Memory: &domain.MemoryMetrics{
			TotalBytes:     ptr(int64(16 << 30)),
			AvailableBytes: ptr(int64(11 << 30)),
			UsedBytes:      ptr(int64(5 << 30)),
		},
		Storage: []domain.StorageMetrics{
			{Mount: "/", Filesystem: "/dev/sda2", TotalBytes: 500 << 30, FreeBytes: 120 << 30},
		},
		Thermal: []domain.ThermalMetrics{
			{Label: "coretemp Package id 0", Celsius: 47.5, HighCelsius: ptr(float32(84))},
		},
	}
}

// ---------------------------------------------------------------- the event

// TestHostMetricsReachTheStream is the property option A exists for.
//
// Metrics travel as their own event rather than as a field on System, which is delivered
// once at connect. Inside System they would have been frozen at the moment the client
// connected — a permanently stale CPU figure sitting under a live indicator, which on this
// console is the worst available failure (ADR-0004 Amendment 3).
func TestHostMetricsReachTheStream(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	frames, _ := openStream(t, env, token)
	nextFrame(t, frames, "the snapshot")

	at := time.Date(2026, 8, 31, 9, 30, 0, 0, time.UTC)
	env.api.PublishHostMetrics(sampleMetrics(at))

	event := nextFrame(t, frames, "a host_updated event")
	if event.name != string(gen.StreamEventTypeHostUpdated) {
		t.Errorf("SSE event name = %q, want host_updated", event.name)
	}
	if event.envelope.Type != gen.StreamEventTypeHostUpdated {
		t.Errorf("envelope type = %q, want host_updated", event.envelope.Type)
	}

	got := event.envelope.HostMetrics
	if got == nil {
		t.Fatal("host_updated carried no metrics")
	}
	if !got.CollectedAt.Equal(at) {
		t.Errorf("CollectedAt = %v, want %v", got.CollectedAt, at)
	}
	if got.Cpu == nil || got.Cpu.UsagePercent == nil || *got.Cpu.UsagePercent != 23.5 {
		t.Errorf("cpu.usage_percent did not survive: %+v", got.Cpu)
	}
	if got.Storage == nil || len(*got.Storage) != 1 || (*got.Storage)[0].Mount != "/" {
		t.Errorf("storage did not survive: %+v", got.Storage)
	}
	if event.envelope.Service != nil {
		t.Error("a host event carried a service payload")
	}
}

// TestSnapshotCarriesLastHostMetrics is what makes a reconnecting client whole. There is no
// replay buffer, so anything the snapshot omits is simply lost until the next tick.
func TestSnapshotCarriesLastHostMetrics(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	at := time.Date(2026, 8, 31, 9, 30, 0, 0, time.UTC)
	env.api.PublishHostMetrics(sampleMetrics(at))

	frames, _ := openStream(t, env, token)
	first := nextFrame(t, frames, "the snapshot")

	if first.envelope.Snapshot == nil {
		t.Fatal("no snapshot")
	}
	got := first.envelope.Snapshot.HostMetrics
	if got == nil {
		t.Fatal("the snapshot omitted host metrics that had already been collected")
	}
	if !got.CollectedAt.Equal(at) {
		t.Errorf("CollectedAt = %v, want %v", got.CollectedAt, at)
	}
}

// TestSnapshotOmitsHostMetricsBeforeFirstCollection covers the first seconds after a
// restart, and the whole life of a platform that cannot read them. Absent, never zero: an
// empty object would claim a machine that was measured and found idle.
func TestSnapshotOmitsHostMetricsBeforeFirstCollection(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	frames, _ := openStream(t, env, token)
	first := nextFrame(t, frames, "the snapshot")

	if first.envelope.Snapshot.HostMetrics != nil {
		t.Errorf("host_metrics = %+v before any collection, want absent",
			first.envelope.Snapshot.HostMetrics)
	}
}

// ---------------------------------------------------------------- the wire

// TestAbsentFieldsAreOmittedOnTheWire is the M3.5 rule applied to a payload where it is
// easier to get wrong: hardware genuinely differs, so most machines will omit something.
func TestAbsentFieldsAreOmittedOnTheWire(t *testing.T) {
	at := time.Date(2026, 8, 31, 9, 30, 0, 0, time.UTC)
	// A machine that answered the clock and nothing else.
	view := toGenHostMetrics(&domain.HostMetrics{CollectedAt: at})

	body, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"cpu", "memory", "storage", "thermal", "uptime_seconds"} {
		if _, present := raw[key]; present {
			t.Errorf("%q is present as %s; an unread value must be absent, not zero",
				key, raw[key])
		}
	}
	if _, present := raw["collected_at"]; !present {
		t.Error("collected_at is missing; without it nothing here can be judged stale")
	}
}

// TestEmptySlicesSurviveAsEmpty is the other half, and the one a naive mapping loses. An
// empty thermal list means "this machine has no sensors", which every virtual machine will
// report and which is not the same as "the sensors could not be read".
func TestEmptySlicesSurviveAsEmpty(t *testing.T) {
	at := time.Date(2026, 8, 31, 9, 30, 0, 0, time.UTC)
	view := toGenHostMetrics(&domain.HostMetrics{
		CollectedAt: at,
		Storage:     []domain.StorageMetrics{},
		Thermal:     []domain.ThermalMetrics{},
	})

	body, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw struct {
		Storage *[]json.RawMessage `json:"storage"`
		Thermal *[]json.RawMessage `json:"thermal"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw.Storage == nil || raw.Thermal == nil {
		t.Fatalf("an empty list was encoded as absent: %s", body)
	}
	if len(*raw.Storage) != 0 || len(*raw.Thermal) != 0 {
		t.Errorf("lists gained entries: %s", body)
	}
}

func TestNilMetricsConvertToNil(t *testing.T) {
	if got := toGenHostMetrics(nil); got != nil {
		t.Errorf("toGenHostMetrics(nil) = %+v, want nil", got)
	}
}

// ---------------------------------------------------------------- the endpoint

// TestGetHostMetricsServesTheLastCollection is the read path a manual refresh uses. Without
// it a pull would compose a snapshot with no metrics in it and blank values the agent is
// still holding.
func TestGetHostMetricsServesTheLastCollection(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	at := time.Date(2026, 8, 31, 9, 30, 0, 0, time.UTC)
	env.api.PublishHostMetrics(sampleMetrics(at))

	resp := env.do(t, http.MethodGet, "/v1/host/metrics", token, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got gen.HostMetrics
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.CollectedAt.Equal(at) {
		t.Errorf("CollectedAt = %v, want %v", got.CollectedAt, at)
	}
	if got.Memory == nil || got.Memory.UsedBytes == nil {
		t.Error("memory did not survive the endpoint")
	}
}

// TestGetHostMetricsBeforeAnyCollection covers the first seconds of an agent's life and the
// whole life of a machine that cannot be measured. 204, not an empty object: the request
// succeeded and there is nothing to report, which is not the same as a machine found idle.
func TestGetHostMetricsBeforeAnyCollection(t *testing.T) {
	env := newTestEnv(t)
	token := env.pairDevice(t, "Phone", domain.ScopeRead)

	resp := env.do(t, http.MethodGet, "/v1/host/metrics", token, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

// TestGetHostMetricsRequiresAToken keeps the endpoint inside the same authorisation table
// as everything else, derived from x-required-scope rather than restated here.
func TestGetHostMetricsRequiresAToken(t *testing.T) {
	env := newTestEnv(t)

	resp := env.do(t, http.MethodGet, "/v1/host/metrics", "", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}
