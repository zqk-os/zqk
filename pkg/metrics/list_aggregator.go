package metrics

import (
	"context"
	"fmt"
	"sort"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
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
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Build filter
	filter := storage.ListFilter{
		Kind:    config.ObjectKind,
		Filters: config.Filters,
		Limit:   0,
	}

	// Query objects
	result, err := a.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to list objects").Wrap(err)
	}

	// Collect metric values and count frequencies
	valueFrequency := make(map[string]int)
	objectCount := 0
	totalValues := 0

	for _, obj := range result.Objects {
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

	// Create base_metric object
	metricID, err := a.createMetricObject(ctx, config, aggregations, objectCount)
	if err != nil {
		return nil, errfmt.Newf("failed to create metric object").Wrap(err)
	}

	return &AggregationResult{
		MetricID:        metricID,
		ObjectKind:      config.ObjectKind,
		FieldName:       config.FieldName,
		MetricType:      config.MetricType,
		WindowStart:     config.WindowStart,
		WindowEnd:       config.WindowEnd,
		Aggregations:    aggregations,
		ObjectCount:     objectCount,
		CollectionCount: 1,
	}, nil
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

	// Use factory for metric creation
	factory := NewMetricFactory(a.storage)

	// Create metric using factory (non-blocking async)
	metricIDChan := make(chan string, 1)
	errChan := make(chan error, 1)

	factory.CreateMetricFromConfigAsync(
		ctx,
		config,
		title,
		objectCount,
		objConfig,
		aggregations,
		func(metricID string, err error) {
			if err != nil {
				errChan <- err
				return
			}
			metricIDChan <- metricID
		},
	)

	// Wait for result with timeout
	select {
	case id := <-metricIDChan:
		return id, nil
	case err := <-errChan:
		return "", err
	case <-time.After(30 * time.Second):
		return "", errfmt.Errorf("metric creation timeout after 30s")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// getTopValues returns the top N values by frequency
func getTopValues(frequency map[string]int, n int) map[string]int {
	type kv struct {
		key   string
		value int
	}
	pairs := make([]kv, 0, len(frequency))
	for k, v := range frequency {
		pairs = append(pairs, kv{k, v})
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].value > pairs[j].value
	})

	topN := make(map[string]int)
	for i := 0; i < n && i < len(pairs); i++ {
		topN[pairs[i].key] = pairs[i].value
	}

	return topN
}
