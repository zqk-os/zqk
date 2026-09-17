package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

// Re-using MockStorageProvider defined in scalar_aggregator_test.go

func TestStatusHistoryMetricAggregator_Aggregate(t *testing.T) {
	mockStorage := &MockStorageProvider{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt:     time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				objects.FieldKeyStatusHistory: []any{"planned", "in_progress", "complete"},
			},
			{
				objects.FieldKeyCreatedAt:     time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				objects.FieldKeyStatusHistory: []any{"planned", "in_progress"},
			},
		},
	}

	aggregator := NewStatusHistoryMetricAggregator(mockStorage)
	config := &AggregationConfig{
		ObjectKind:   "test_kind",
		FieldName:    "status_history",
		WindowStart:  time.Now().Add(-2 * time.Hour),
		WindowEnd:    time.Now(),
		Aggregations: []string{"state_distribution", "transitions"},
	}

	result, err := aggregator.Aggregate(context.Background(), config)
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}

	// Verify state_distribution
	dist := result.Aggregations["state_distribution"].(map[string]int)
	if dist["planned"] != 2 || dist["in_progress"] != 2 || dist["complete"] != 1 {
		t.Errorf("Unexpected distribution: %v", dist)
	}

	// Verify transitions
	trans := result.Aggregations[objects.FieldKeyTransitions].(map[string]int)
	if trans["planned->in_progress"] != 2 || trans["in_progress->complete"] != 1 {
		t.Errorf("Unexpected transitions: %v", trans)
	}
}
