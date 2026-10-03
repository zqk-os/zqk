package metrics

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ListMetricAggregator aggregates list-based metric fields
type ListMetricAggregator struct {
	storage storage.ObjectStorageProvider
}

// NewListMetricAggregator creates a new list metric aggregator
func NewListMetricAggregator(storageProvider storage.ObjectStorageProvider) *ListMetricAggregator {
	return &ListMetricAggregator{
		storage: storageProvider,
	}
}

// GetMetricType returns the metric type this aggregator handles
func (a *ListMetricAggregator) GetMetricType() string {
	return MetricTypeList
}

// Aggregate collects and aggregates list metric data
func (a *ListMetricAggregator) Aggregate(ctx context.Context, config *AggregationConfig) (*AggregationResult, error) {
	rawObjects, err := QueryAggregationObjects(ctx, a.storage, config)
	if err != nil {
		return nil, err
	}

	// Collect metric values and count frequencies
	valueFrequency := make(map[string]int)
	objectCount := 0
	totalValues := 0

	for _, obj := range rawObjects {
		if !isInWindow(obj, config.WindowStart, config.WindowEnd) {
			continue
		}

		fieldValue, exists := obj[config.FieldName]
		if !exists {
			continue
		}

		// Extract list values
		listValue, ok := fieldValue.([]any)
		if !ok {
			continue
		}

		// Count value frequencies
		for _, item := range listValue {
			var key string
			switch v := item.(type) {
			case string:
				key = v
			case int, int64, int32:
				key = fmt.Sprintf("%v", v)
			case float64, float32:
				key = fmt.Sprintf("%v", v)
			default:
				key = fmt.Sprintf("%v", v)
			}
			valueFrequency[key]++
			totalValues++
		}
		objectCount++
	}

	// Perform aggregations
	aggregations := make(map[string]any)

	for _, aggFunc := range config.Aggregations {
		switch aggFunc {
		case "count":
			aggregations["count"] = totalValues
		case "unique_count", "distinct_count":
			aggregations["unique_count"] = len(valueFrequency)
		case "frequency", "distribution":
			aggregations[objects.FieldKeyFrequency] = valueFrequency
		case "top_values":
			aggregations["top_values"] = getTopValues(valueFrequency, 10)
		}
	}

	// If no aggregations specified, use defaults
	if len(aggregations) == 0 {
		aggregations["count"] = totalValues
		aggregations["unique_count"] = len(valueFrequency)
		aggregations[objects.FieldKeyFrequency] = valueFrequency
	}

	return FinishAggregation(ctx, config, aggregations, objectCount, a.createMetricObject)
}

// createMetricObject creates a base_metric object using the factory pattern (non-blocking)
func (a *ListMetricAggregator) createMetricObject(
	ctx context.Context,
	config *AggregationConfig,
	aggregations map[string]any,
	objectCount int,
) (string, error) {
	title := fmt.Sprintf("List metric aggregation: %s.%s from %s to %s",
		config.ObjectKind, config.FieldName,
		config.WindowStart.Format("2006-01-02 15:04:05"),
		config.WindowEnd.Format("2006-01-02 15:04:05"))

	objConfig := &MetricObjectConfig{
		Source:         MetricSourcePipeline,
		Sampled:        false,
		AdditionalTags: []string{MetricTypeList},
	}

	return CreateSyncMetricObject(ctx, a.storage, config, title, objConfig, aggregations, objectCount)
}

// getTopValues returns the top N values by frequency
func getTopValues(frequency map[string]int, n int) map[string]int {
	return TopNMap(frequency, n)
}
