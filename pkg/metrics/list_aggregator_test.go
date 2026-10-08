package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestListMetricAggregator_Aggregate(t *testing.T) {
	now := time.Now()
	mockStorage := &MockStorageProvider{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
				"tags":                    []any{"go", "cli", "backend"},
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				"tags":                    []any{"go", "kernel"},
			},
			{
				// Outside window
				objects.FieldKeyCreatedAt: now.Add(-5 * time.Hour).Format(time.RFC3339),
				"tags":                    []any{"legacy"},
			},
		},
	}

	agg := NewListMetricAggregator(mockStorage)
	require.Equal(t, MetricTypeList, agg.GetMetricType())

	cfg := &AggregationConfig{
		ObjectKind:   "test_kind",
		FieldName:    "tags",
		WindowStart:  now.Add(-1 * time.Hour),
		WindowEnd:    now.Add(1 * time.Hour),
		Aggregations: []string{"count", "unique_count", "frequency", "top_values"},
	}

	res, err := agg.Aggregate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 2, res.ObjectCount)
	require.Equal(t, 5, res.Aggregations["count"])
	require.Equal(t, 4, res.Aggregations["unique_count"])

	freq, ok := res.Aggregations[objects.FieldKeyFrequency].(map[string]int)
	require.True(t, ok)
	require.Equal(t, 2, freq["go"])
	require.Equal(t, 1, freq["cli"])
	require.Equal(t, 1, freq["kernel"])
}

func TestListMetricAggregator_DefaultAggregations(t *testing.T) {
	now := time.Now()
	mockStorage := &MockStorageProvider{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
				"numbers":                 []any{1, 2, 2.5, "item"},
			},
		},
	}

	agg := NewListMetricAggregator(mockStorage)
	cfg := &AggregationConfig{
		ObjectKind:  "test_kind",
		FieldName:   "numbers",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now.Add(1 * time.Hour),
	}

	res, err := agg.Aggregate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 4, res.Aggregations["count"])
	require.Equal(t, 4, res.Aggregations["unique_count"])
}
