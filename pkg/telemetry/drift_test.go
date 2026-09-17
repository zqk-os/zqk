package telemetry

import (
	"context"
	"testing"
)

type mockEventBus struct {
	published []interface{}
}

func (m *mockEventBus) Publish(ctx context.Context, topic string, payload interface{}) error {
	m.published = append(m.published, payload)
	return nil
}

func TestDriftDetector_CheckDrift(t *testing.T) {
	bus := &mockEventBus{}
	detector := NewDriftDetector(bus, nil) // passing nil logger
	ctx := context.Background()

	tests := []struct {
		name      string
		metric    string
		baseline  float64
		current   float64
		expectPub bool
	}{
		{
			name:      "no drift exact match",
			metric:    "cpu_usage",
			baseline:  100.0,
			current:   100.0,
			expectPub: false,
		},
		{
			name:      "no drift under 10 percent",
			metric:    "cpu_usage",
			baseline:  100.0,
			current:   105.0,
			expectPub: false,
		},
		{
			name:      "drift exactly 10 percent",
			metric:    "cpu_usage",
			baseline:  100.0,
			current:   110.0,
			expectPub: false,
		},
		{
			name:      "drift over 10 percent positive",
			metric:    "cpu_usage",
			baseline:  100.0,
			current:   111.0,
			expectPub: true,
		},
		{
			name:      "drift over 10 percent negative",
			metric:    "cpu_usage",
			baseline:  100.0,
			current:   85.0,
			expectPub: true,
		},
		{
			name:      "zero baseline handles gracefully",
			metric:    "cpu_usage",
			baseline:  0.0,
			current:   10.0,
			expectPub: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus.published = nil // reset
			err := detector.CheckDrift(ctx, "test-job-id", 12345, tt.metric, tt.baseline, tt.current)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectPub {
				if len(bus.published) == 0 {
					t.Fatalf("expected event to be published, got none")
				}
				ev, ok := bus.published[0].(DriftEvent)
				if !ok {
					t.Fatalf("expected DriftEvent payload")
				}
				if ev.MetricName != tt.metric {
					t.Errorf("expected metric %q, got %q", tt.metric, ev.MetricName)
				}
				if ev.DeviationPct <= 10.0 {
					t.Errorf("expected deviation > 10.0, got %f", ev.DeviationPct)
				}
			} else {
				if len(bus.published) > 0 {
					t.Fatalf("did not expect event to be published")
				}
			}
		})
	}
}
