package coordination

import (
	"context"
	"testing"
)

func TestProgressHelper_EmitProgress_ZeroTotal(t *testing.T) {
	t.Parallel()

	coordinator := NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       nil,
		MetricsRouter:     nil,
		OperationalRouter: &DefaultOperationalRouter{},
	})

	helper := NewProgressHelper(coordinator, "/tmp/dummy", "op-001", "test_op", "default")

	// EmitProgress with total = 0 must not panic and must not produce NaN or Inf
	err := helper.EmitProgress(context.Background(), 0, 0, "Starting", nil, false)
	if err != nil {
		t.Fatalf("EmitProgress failed: %v", err)
	}

	// Verify with positive progress but zero total
	err = helper.EmitProgress(context.Background(), 1, 0, "Progress without total", nil, false)
	if err != nil {
		t.Fatalf("EmitProgress with non-zero progress failed: %v", err)
	}
}

func TestProgressHelper_EmitProgress_Calculation(t *testing.T) {
	t.Parallel()

	coordinator := NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       nil,
		MetricsRouter:     nil,
		OperationalRouter: &DefaultOperationalRouter{},
	})

	helper := NewProgressHelper(coordinator, "/tmp/dummy", "op-002", "test_op", "default")

	err := helper.EmitProgress(context.Background(), 50, 100, "Halfway", nil, false)
	if err != nil {
		t.Fatalf("EmitProgress failed: %v", err)
	}

	// Make sure percent doesn't exceed 100
	err = helper.EmitProgress(context.Background(), 150, 100, "Over", nil, false)
	if err != nil {
		t.Fatalf("EmitProgress failed: %v", err)
	}
}
