package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// MockStorageProvider for testing metrics aggregation
type MockStorageProvider struct {
	storage.ObjectStorageProvider
	objects []map[string]any
}

func (m *MockStorageProvider) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return &storage.QueryResult{Objects: m.objects}, nil
}

func TestScalarMetricAggregator_Aggregate(t *testing.T) {
	mockStorage := &MockStorageProvider{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"value":                   10.0,
			},
			{
				objects.FieldKeyCreatedAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"value":                   20.0,
			},
		},
	}

	aggregator := NewScalarMetricAggregator(mockStorage)
	config := &AggregationConfig{
		ObjectKind:   "test_kind",
		FieldName:    "value",
		WindowStart:  time.Now().Add(-2 * time.Hour),
		WindowEnd:    time.Now(),
		Aggregations: []string{"sum", "avg", "count"},
	}

	result, err := aggregator.Aggregate(context.Background(), config)
	if err != nil {
		t.Fatalf("Aggregate failed: %v", err)
	}

	if result.Aggregations["sum"] != 30.0 {
		t.Errorf("Expected sum 30.0, got %v", result.Aggregations["sum"])
	}
	if result.Aggregations["avg"] != 15.0 {
		t.Errorf("Expected avg 15.0, got %v", result.Aggregations["avg"])
	}
	if result.Aggregations["count"] != 2 {
		t.Errorf("Expected count 2, got %v", result.Aggregations["count"])
	}
}
