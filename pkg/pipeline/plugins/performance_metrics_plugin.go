package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// PerformanceMetricsPlugin tracks and reports on pipeline execution efficiency.
type PerformanceMetricsPlugin struct{}

// NewPerformanceMetricsPlugin creates a new PerformanceMetricsPlugin.
func NewPerformanceMetricsPlugin() *PerformanceMetricsPlugin {
	return &PerformanceMetricsPlugin{}
}

// Name returns the unique string identifier for the plugin.
func (p *PerformanceMetricsPlugin) Name() string {
	return "PerformanceMetricsPlugin"
}

// Execute tracks execution timestamps and computes metrics.
func (p *PerformanceMetricsPlugin) Execute(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = make(map[string]any)
	}

	metrics, ok := payload["performance_metrics"].(map[string]any)
	if !ok {
		metrics = make(map[string]any)
	}

	metrics["execution_started_at"] = time.Now().UTC().Format(time.RFC3339)
	metrics["efficiency_score"] = 100.0 // Placeholder for computed efficiency
	metrics[objects.FieldKeyStatus] = "tracked"

	payload["performance_metrics"] = metrics

	return payload, nil
}

// Validate ensures the performance metrics were correctly attached.
func (p *PerformanceMetricsPlugin) Validate(ctx context.Context, output map[string]any) error {
	metrics, ok := output["performance_metrics"].(map[string]any)
	if !ok {
		return fmt.Errorf("performance metrics tracking failed: 'performance_metrics' missing from output payload")
	}

	if _, ok := metrics["execution_started_at"]; !ok {
		return fmt.Errorf("performance metrics tracking failed: 'execution_started_at' missing from metrics")
	}

	return nil
}
