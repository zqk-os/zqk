package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ScalarMetricAggregator aggregates scalar (numeric) metric fields
type ScalarMetricAggregator struct {
	storage storage.ObjectStorageProvider
}

// NewScalarMetricAggregator creates a new scalar metric aggregator
func NewScalarMetricAggregator(storageProvider storage.ObjectStorageProvider) *ScalarMetricAggregator {
	return &ScalarMetricAggregator{
		storage: storageProvider,
	}
}

// GetMetricType returns the metric type this aggregator handles
func (a *ScalarMetricAggregator) GetMetricType() string {
	return MetricTypeScalar
}

// Aggregate collects and aggregates scalar metric data
func (a *ScalarMetricAggregator) Aggregate(ctx context.Context, config *AggregationConfig) (*AggregationResult, error) {
	rawObjects, err := QueryAggregationObjects(ctx, a.storage, config)
	if err != nil {
		return nil, err
	}

	// Collect metric values
	var values []float64
	objectCount := 0

	for _, obj := range rawObjects {
		// Check if object is within time window (if timestamps available)
		if !isInWindow(obj, config.WindowStart, config.WindowEnd) {
			continue
		}

		// Extract field value
		fieldValue, exists := obj[config.FieldName]
		if !exists {
			continue
		}

		// Convert to float64
		var floatValue float64
		switch v := fieldValue.(type) {
		case float64:
			floatValue = v
		case float32:
			floatValue = float64(v)
		case int:
			floatValue = float64(v)
		case int64:
			floatValue = float64(v)
		case int32:
			floatValue = float64(v)
		default:
			continue // Skip non-numeric values
		}

		values = append(values, floatValue)
		objectCount++
	}

	// Perform aggregations
	aggregations := make(map[string]any)

	for _, aggFunc := range config.Aggregations {
		switch aggFunc {
		case "sum":
			aggregations["sum"] = sum(values)
		case "avg", "average", "mean":
			aggregations["avg"] = average(values)
		case "min", "minimum":
			aggregations["min"] = minFloatSlice(values)
		case "max", "maximum":
			aggregations["max"] = maxFloatSlice(values)
		case "count":
			aggregations["count"] = len(values)
		}
	}

	// If no aggregations specified, use defaults
	if len(aggregations) == 0 {
		aggregations["sum"] = sum(values)
		aggregations["avg"] = average(values)
		aggregations["min"] = minFloatSlice(values)
		aggregations["max"] = maxFloatSlice(values)
		aggregations["count"] = len(values)
	}

	// Create base_metric object
	metricID, err := a.createMetricObject(ctx, config, aggregations, objectCount)
	if err != nil {
		return nil, errfmt.Newf("failed to create metric object").Wrap(err)
	}

	return BuildAggregationResult(config, metricID, aggregations, objectCount), nil
}

// createMetricObject creates a base_metric object using the factory pattern (non-blocking)
func (a *ScalarMetricAggregator) createMetricObject(
	ctx context.Context,
	config *AggregationConfig,
	aggregations map[string]any,
	objectCount int,
) (string, error) {
	title := fmt.Sprintf("Scalar metric aggregation: %s.%s from %s to %s",
		config.ObjectKind, config.FieldName,
		config.WindowStart.Format("2006-01-02 15:04:05"),
		config.WindowEnd.Format("2006-01-02 15:04:05"))

	objConfig := &MetricObjectConfig{
		Source:         MetricSourcePipeline,
		Sampled:        false,
		AdditionalTags: []string{MetricTypeScalar},
	}

	return CreateSyncMetricObject(ctx, a.storage, config, title, objConfig, aggregations, objectCount)
}

// Helper functions for aggregations
func sum(values []float64) float64 {
	var total float64
	for _, v := range values {
		total += v
	}
	return total
}

func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return sum(values) / float64(len(values))
}

func minFloatSlice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minVal := values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
	}
	return minVal
}

func maxFloatSlice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxVal := values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

// isInWindow checks if an object is within the time window
func isInWindow(obj map[string]any, windowStart, windowEnd time.Time) bool {
	// Check created_at or updated_at
	if createdAt, ok := obj[objects.FieldKeyCreatedAt].(string); ok {
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			if t.Before(windowStart) || t.After(windowEnd) {
				return false
			}
		}
	}
	// Also check updated_at
	if updatedAt, ok := obj[objects.FieldKeyUpdatedAt].(string); ok {
		if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
			if t.Before(windowStart) || t.After(windowEnd) {
				return false
			}
		}
	}
	return true
}
