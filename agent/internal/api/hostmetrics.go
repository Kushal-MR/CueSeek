package api

import (
	"context"

	"github.com/Kushal-MR/CueSeek/agent/internal/api/gen"
	"github.com/Kushal-MR/CueSeek/agent/internal/domain"
)

// The host metrics surface.
//
// Metrics arrive here from a collector on its own ticker and leave as `host_updated`
// stream events, reusing the same hub every service update goes through. They are
// deliberately not part of the adapter cache and not a pseudo-service: a machine has no
// health status, no actions and no owner, and modelling it as one would have put the
// computer into the dashboard's own tally of healthy services (ADR-0005's scope rule).
//
// Why an event at all, rather than a field on System: System is identity, delivered once
// at connect and unchanged for the life of the process. Metrics change continuously, so
// putting them there would have frozen a CPU figure at the moment the client connected —
// on a console whose whole premise is that staleness must be visible, a permanently stale
// number under a live indicator is the worst available outcome (ADR-0004 Amendment 3).

// PublishHostMetrics records a collection and tells every connected client.
//
// Storing before publishing, on purpose: a client that connects between the two must get
// the new values in its snapshot rather than the previous ones followed by nothing.
func (s *Server) PublishHostMetrics(metrics domain.HostMetrics) {
	s.hostMetricsMu.Lock()
	s.hostMetrics = &metrics
	s.hostMetricsMu.Unlock()

	view := toGenHostMetrics(&metrics)
	s.hub.publish(streamEvent{
		typ:         gen.StreamEventTypeHostUpdated,
		emittedAt:   s.now(),
		hostMetrics: view,
	})
}

// GetHostMetrics serves the last collection.
//
// 204 rather than an error or an empty object when nothing has been collected. The request
// succeeded and there is genuinely nothing to report — an agent seconds into its life,
// metrics switched off, or a platform that cannot read them — and a zero-filled body would
// describe a machine measured and found idle, which is a different and untrue claim.
func (s *Server) GetHostMetrics(
	_ context.Context, _ gen.GetHostMetricsRequestObject,
) (gen.GetHostMetricsResponseObject, error) {
	metrics := s.currentHostMetrics()
	if metrics == nil {
		return gen.GetHostMetrics204Response{}, nil
	}
	return gen.GetHostMetrics200JSONResponse(*metrics), nil
}

// currentHostMetrics returns the last collection, or nil if none has happened.
func (s *Server) currentHostMetrics() *gen.HostMetrics {
	s.hostMetricsMu.RLock()
	defer s.hostMetricsMu.RUnlock()
	return toGenHostMetrics(s.hostMetrics)
}

// toGenHostMetrics converts to the wire shape, preserving every absence.
//
// The mapping is mechanical except for the slices, where nil and empty mean different
// things and both have to survive: a nil Storage means the agent could not measure any
// filesystem, while an empty one means it tried and none answered. Marshalling nil as `[]`
// would collapse the two and tell a client the machine has no filesystems.
func toGenHostMetrics(metrics *domain.HostMetrics) *gen.HostMetrics {
	if metrics == nil {
		return nil
	}
	out := &gen.HostMetrics{
		CollectedAt:   metrics.CollectedAt,
		UptimeSeconds: metrics.UptimeSeconds,
	}

	if cpu := metrics.CPU; cpu != nil {
		out.Cpu = &gen.CpuMetrics{
			UsagePercent: cpu.UsagePercent,
			Cores:        cpu.Cores,
			Load1:        cpu.Load1,
			Load5:        cpu.Load5,
			Load15:       cpu.Load15,
		}
	}

	if memory := metrics.Memory; memory != nil {
		out.Memory = &gen.MemoryMetrics{
			TotalBytes:     memory.TotalBytes,
			AvailableBytes: memory.AvailableBytes,
			UsedBytes:      memory.UsedBytes,
			SwapTotalBytes: memory.SwapTotalBytes,
			SwapUsedBytes:  memory.SwapUsedBytes,
		}
	}

	if metrics.Storage != nil {
		storage := make([]gen.StorageMetrics, 0, len(metrics.Storage))
		for _, filesystem := range metrics.Storage {
			storage = append(storage, gen.StorageMetrics{
				Mount:      filesystem.Mount,
				Filesystem: optionalString(filesystem.Filesystem),
				TotalBytes: filesystem.TotalBytes,
				FreeBytes:  filesystem.FreeBytes,
			})
		}
		out.Storage = &storage
	}

	if metrics.Thermal != nil {
		thermal := make([]gen.ThermalMetrics, 0, len(metrics.Thermal))
		for _, sensor := range metrics.Thermal {
			thermal = append(thermal, gen.ThermalMetrics{
				Label:       sensor.Label,
				Celsius:     sensor.Celsius,
				HighCelsius: sensor.HighCelsius,
			})
		}
		out.Thermal = &thermal
	}

	return out
}
