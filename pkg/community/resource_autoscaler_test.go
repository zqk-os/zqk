package community

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAutoscalingPolicy_Validation(t *testing.T) {
	valid := AutoscalingPolicy{
		MinConcurrency: 2,
		MaxConcurrency: 10,
		QueueThreshold: 5,
		ScaleFactor:    1.5,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid policy, got: %v", err)
	}

	invalidCases := []AutoscalingPolicy{
		{MinConcurrency: 0, MaxConcurrency: 10, QueueThreshold: 5, ScaleFactor: 1.5},
		{MinConcurrency: 5, MaxConcurrency: 4, QueueThreshold: 5, ScaleFactor: 1.5},
		{MinConcurrency: 2, MaxConcurrency: 10, QueueThreshold: 5, ScaleFactor: 1.0},
		{MinConcurrency: 2, MaxConcurrency: 10, QueueThreshold: 5, ScaleFactor: 0.8},
		{MinConcurrency: 2, MaxConcurrency: 10, QueueThreshold: -1, ScaleFactor: 1.5},
	}

	for i, c := range invalidCases {
		if err := c.Validate(); err != ErrInvalidAutoscalingPolicy {
			t.Errorf("case %d: expected ErrInvalidAutoscalingPolicy, got: %v", i, err)
		}
	}
}

func TestResourceAutoscaler_DynamicScaling(t *testing.T) {
	policy := AutoscalingPolicy{
		MinConcurrency: 2,
		MaxConcurrency: 10,
		QueueThreshold: 5,
		ScaleFactor:    2.0,
	}

	scaler, err := NewResourceAutoscaler(policy)
	if err != nil {
		t.Fatalf("failed to create autoscaler: %v", err)
	}

	if initial := scaler.CurrentConcurrency(); initial != 2 {
		t.Errorf("expected initial concurrency 2, got %d", initial)
	}

	// Trigger scale up
	newConc := scaler.AdjustConcurrency(10, 2)
	if newConc != 4 {
		t.Errorf("expected scaled up concurrency 4, got %d", newConc)
	}

	// Scale up again past threshold
	newConc = scaler.AdjustConcurrency(20, 4)
	if newConc != 8 {
		t.Errorf("expected scaled up concurrency 8, got %d", newConc)
	}

	// Scale up capped at MaxConcurrency
	newConc = scaler.AdjustConcurrency(50, 8)
	if newConc != 10 {
		t.Errorf("expected capped concurrency 10, got %d", newConc)
	}

	// Scale down when queue is empty and workers are idle
	newConc = scaler.AdjustConcurrency(0, 1)
	if newConc != 5 {
		t.Errorf("expected scaled down concurrency 5, got %d", newConc)
	}

	// Scale down to minimum
	newConc = scaler.AdjustConcurrency(0, 0)
	if newConc != 2 {
		t.Errorf("expected floor concurrency 2, got %d", newConc)
	}
}

func TestResourceAutoscaler_AdversarialNil(t *testing.T) {
	var nilPolicy *AutoscalingPolicy
	if err := nilPolicy.Validate(); err != ErrInvalidAutoscalingPolicy {
		t.Errorf("expected ErrInvalidAutoscalingPolicy on nil policy, got: %v", err)
	}

	_, err := NewResourceAutoscaler(AutoscalingPolicy{
		MinConcurrency: -1,
	})
	if err != ErrInvalidAutoscalingPolicy {
		t.Errorf("expected ErrInvalidAutoscalingPolicy on constructor, got: %v", err)
	}
}

func TestResourceAutoscaler_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_DYNAMIC_RESOURCE_AUTOSCALING_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0059 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0059 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0059 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
