package instance_builders

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestBaseInstanceBuilder_DefaultStatusForMetrics verifies that metric kinds get
// a lifecycle-valid default status when none is explicitly set.
func TestBaseInstanceBuilder_DefaultStatusForMetrics(t *testing.T) {
	t.Parallel()

	builder := NewBaseInstanceBuilder("base_metric", "1.0.0", []string{"id"}, nil)
	builder.SetID("BAS-test")

	instance, err := builder.Build()
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	status, ok := instance[objects.FieldKeyStatus].(string)
	if !ok {
		t.Fatalf("status field missing or not a string: %#v", instance[objects.FieldKeyStatus])
	}
	if status != "implemented" {
		t.Errorf("default status = %q, want %q", status, "implemented")
	}
}
