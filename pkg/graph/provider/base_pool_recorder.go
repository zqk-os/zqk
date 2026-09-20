package provider

import (
	"time"

	"github.com/zqk-os/zqk/pkg/observability"
)

// getPoolMetricsRecorder gets a metrics recorder for pool operations
// This is in a separate file to help manage import cycles
func (p *BasePool) getPoolMetricsRecorder() observability.Recorder {
	if p.recorder == nil {
		p.recorder = observability.GetNoOpRecorder()
	}
	if recorder, ok := p.recorder.(observability.Recorder); ok {
		return recorder
	}
	return observability.GetNoOpRecorder()
}

// buildPoolWaitMetric builds a metric for pool wait operations
func buildPoolWaitMetric(duration time.Duration) observability.Builder {
	return observability.NewBuilder("pool_wait").
		WithDuration(duration).
		WithTags("graph", "provider", "pool", "wait")
}

// buildConnectionAcquiredMetric builds a metric for connection acquisition
func buildConnectionAcquiredMetric(duration time.Duration) observability.Builder {
	return observability.NewBuilder("connection_acquired").
		WithDuration(duration).
		WithTags("graph", "provider", "pool", "connection", "acquired")
}

// buildConnectionReleasedMetric builds a metric for connection release
func buildConnectionReleasedMetric() observability.Builder {
	return observability.NewBuilder("connection_released").
		WithTags("graph", "provider", "pool", "connection", "released")
}

// buildPoolSizeChangeMetric builds a metric for pool size changes
func buildPoolSizeChangeMetric(active, idle, maxSize int) observability.Builder {
	return observability.NewBuilder("pool_size_change").
		WithField("active", active).
		WithField("idle", idle).
		WithField("max_size", maxSize).
		WithTags("graph", "provider", "pool", "size_change")
}
