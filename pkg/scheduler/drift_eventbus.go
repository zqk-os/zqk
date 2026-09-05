package scheduler

import (
	"context"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/telemetry"
)

// CoordinationEventBusAdapter implements telemetry.EventBus and bridges to CoordinationChannel.
type CoordinationEventBusAdapter struct {
	cc CoordinationChannelInterface
}

// NewCoordinationEventBusAdapter creates a new CoordinationEventBusAdapter.
func NewCoordinationEventBusAdapter(cc CoordinationChannelInterface) *CoordinationEventBusAdapter {
	return &CoordinationEventBusAdapter{cc: cc}
}

// Publish translates a telemetry event to a CoordinationChannel event.
func (a *CoordinationEventBusAdapter) Publish(ctx context.Context, topic string, payload interface{}) error {
	if topic == "telemetry.drift" {
		if de, ok := payload.(telemetry.DriftEvent); ok {
			ev := Event{
				Type:      "drift_detected",
				Timestamp: de.DetectedAt,
				JobID:     de.JobID,
				ProcessID: de.ProcessID,
				Metadata: map[string]any{
					objects.FieldKeyMetric: de.MetricName,
					"baseline":             de.Baseline,
					"current":              de.Current,
					"deviation_pct":        de.DeviationPct,
				},
			}
			return a.cc.PublishEvent(ev)
		}
	}
	return nil
}
