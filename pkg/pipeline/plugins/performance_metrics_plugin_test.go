package plugins

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestPerformanceMetricsPlugin_Execute(t *testing.T) {
	plugin := NewPerformanceMetricsPlugin()

	if plugin.Name() != "PerformanceMetricsPlugin" {
		t.Errorf("Expected Name() to return 'PerformanceMetricsPlugin', got %s", plugin.Name())
	}

	ctx := context.Background()
	payload := make(map[string]any)

	result, err := plugin.Execute(ctx, payload)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	metrics, ok := result["performance_metrics"].(map[string]any)
	if !ok {
		t.Fatalf("Expected 'performance_metrics' to be a map")
	}

	if metrics[objects.FieldKeyStatus] != "tracked" {
		t.Errorf("Expected status to be 'tracked', got %v", metrics[objects.FieldKeyStatus])
	}

	err = plugin.Validate(ctx, result)
	if err != nil {
		t.Errorf("Validate failed on valid output: %v", err)
	}
}

func TestPerformanceMetricsPlugin_Validate_Fails(t *testing.T) {
	plugin := NewPerformanceMetricsPlugin()
	ctx := context.Background()

	// An empty payload should fail validation
	err := plugin.Validate(ctx, make(map[string]any))
	if err == nil {
		t.Errorf("Expected validation to fail for empty payload")
	}

	// Payload with missing execution_started_at should fail
	invalidPayload := map[string]any{
		"performance_metrics": map[string]any{
			objects.FieldKeyStatus: "tracked",
		},
	}
	err = plugin.Validate(ctx, invalidPayload)
	if err == nil {
		t.Errorf("Expected validation to fail for payload without execution_started_at")
	}
}
