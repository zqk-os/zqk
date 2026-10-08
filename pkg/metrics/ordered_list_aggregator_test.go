package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestOrderedListMetricAggregator_Aggregate(t *testing.T) {
	now := time.Now()
	mockStorage := &MockStorageProvider{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
				"steps":                   []any{"login", "browse", "checkout"},
			},
			{
				objects.FieldKeyCreatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				"steps":                   []any{"login", "browse", "logout"},
			},
			{
				// Outside window
				objects.FieldKeyCreatedAt: now.Add(-5 * time.Hour).Format(time.RFC3339),
				"steps":                   []any{"idle"},
			},
		},
	}

	agg := NewOrderedListMetricAggregator(mockStorage)
	require.Equal(t, MetricTypeOrderedList, agg.GetMetricType())

	cfg := &AggregationConfig{
		ObjectKind:   "test_kind",
		FieldName:    "steps",
		WindowStart:  now.Add(-1 * time.Hour),
		WindowEnd:    now.Add(1 * time.Hour),
		Aggregations: []string{"sequence_count", "avg_sequence_length", "transitions", "common_sequences"},
	}

	res, err := agg.Aggregate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 2, res.Aggregations["sequence_count"])
	require.Equal(t, 3.0, res.Aggregations["avg_sequence_length"])

	transitions, ok := res.Aggregations[objects.FieldKeyTransitions].(map[string]int)
	require.True(t, ok)
	require.Equal(t, 2, transitions["login->browse"])
	require.Equal(t, 1, transitions["browse->checkout"])
	require.Equal(t, 1, transitions["browse->logout"])
}

func TestOrderedListMetricAggregator_DefaultAndEdgeCases(t *testing.T) {
	now := time.Now()
	mockStorage := &MockStorageProvider{
		objects: []map[string]any{
			{
				objects.FieldKeyCreatedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
				"steps":                   []any{"start"},
			},
		},
	}

	agg := NewOrderedListMetricAggregator(mockStorage)
	cfg := &AggregationConfig{
		ObjectKind:  "test_kind",
		FieldName:   "steps",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now.Add(1 * time.Hour),
	}

	res, err := agg.Aggregate(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, 1, res.Aggregations["sequence_count"])
	require.Equal(t, 1.0, res.Aggregations["avg_sequence_length"])

	// Test averageSequenceLength empty
	require.Equal(t, 0.0, averageSequenceLength(nil))
}
