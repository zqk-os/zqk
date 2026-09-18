package telemetry

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// EventBus represents an internal event bus for publishing system events.
type EventBus interface {
	Publish(ctx context.Context, topic string, payload interface{}) error
}

// DriftEvent represents a performance drift detection event.
type DriftEvent struct {
	MetricName   string
	Baseline     float64
	Current      float64
	DeviationPct float64
	DetectedAt   time.Time
	JobID        string
	ProcessID    int
}

// DriftDetector implements the drift detection logic.
type DriftDetector struct {
	eventBus EventBus
	logger   logging.Logger
}

// NewDriftDetector creates a new drift detector.
func NewDriftDetector(bus EventBus, logger logging.Logger) *DriftDetector {
	return &DriftDetector{
		eventBus: bus,
		logger:   logger,
	}
}

// CheckDrift checks if the current metric deviates from the baseline by more than 10%.
// If it does, a DriftEvent is published to the internal event bus.
func (d *DriftDetector) CheckDrift(ctx context.Context, jobID string, processID int, metricName string, baseline, current float64) error {
	if baseline == 0 {
		return nil // Cannot calculate percentage of 0 baseline
	}

	diff := math.Abs(current - baseline)
	deviation := (diff / math.Abs(baseline)) * 100.0

	if deviation > 10.0 {
		event := DriftEvent{
			MetricName:   metricName,
			Baseline:     baseline,
			Current:      current,
			DeviationPct: deviation,
			DetectedAt:   time.Now().UTC(),
			JobID:        jobID,
			ProcessID:    processID,
		}

		if d.logger != nil {
			// Log it using structured logging
			logging.Fluent(d.logger).Warn("Performance drift detected").
				String("metric", metricName).
				String("baseline", fmt.Sprintf("%f", baseline)).
				String("current", fmt.Sprintf("%f", current)).
				String("deviation_pct", fmt.Sprintf("%f", deviation)).
				String("job_id", jobID).
				Int("process_id", processID).
				Log()
		}

		if d.eventBus != nil {
			return d.eventBus.Publish(ctx, "telemetry.drift", event)
		}
	}

	return nil
}
