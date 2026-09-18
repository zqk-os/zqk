package metrics

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// StatusHistoryMetricAggregator aggregates status history metric fields
type StatusHistoryMetricAggregator struct {
	storage storage.ObjectStorageProvider
}

// NewStatusHistoryMetricAggregator creates a new status history metric aggregator
func NewStatusHistoryMetricAggregator(storageProvider storage.ObjectStorageProvider) *StatusHistoryMetricAggregator {
	return &StatusHistoryMetricAggregator{
		storage: storageProvider,
	}
}

// GetMetricType returns the metric type this aggregator handles
func (a *StatusHistoryMetricAggregator) GetMetricType() string {
	return MetricTypeStatusHistory
}

// Aggregate collects and aggregates status history metric data
func (a *StatusHistoryMetricAggregator) Aggregate(ctx context.Context, config *AggregationConfig) (*AggregationResult, error) {
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

	// Collect status histories and analyze
	statusHistories := make([][]string, 0)
	stateDurations := make(map[string]float64) // state -> total duration
	stateCounts := make(map[string]int)        // state -> count
	transitionCounts := make(map[string]int)   // "from->to" -> count
	objectCount := 0

	for _, obj := range result.Objects {
		if !isInWindow(obj, config.WindowStart, config.WindowEnd) {
			continue
		}

		fieldValue, exists := obj[config.FieldName]
		if !exists {
			continue
		}

		// Extract status history (ordered list)
		listValue, ok := fieldValue.([]any)
		if !ok {
			continue
		}

		// Convert to string sequence
		history := make([]string, 0, len(listValue))
		for _, item := range listValue {
			history = append(history, fmt.Sprintf("%v", item))
		}

		if len(history) > 0 {
			statusHistories = append(statusHistories, history)

			// Track state counts
			for _, state := range history {
				stateCounts[state]++
			}

			// Track transitions
			for i := 0; i < len(history)-1; i++ {
				transition := fmt.Sprintf("%s->%s", history[i], history[i+1])
				transitionCounts[transition]++
			}

			// Calculate state durations (simplified - assumes equal time per state)
			// In production, would parse timestamps from status history entries
			for _, state := range history {
				stateDurations[state] += 1.0 // Placeholder - would use actual timestamps
			}
		}

		objectCount++
	}

	// Perform aggregations
	aggregations := make(map[string]any)

	for _, aggFunc := range config.Aggregations {
		switch aggFunc {
		case "state_distribution":
			aggregations["state_distribution"] = stateCounts
		case "state_durations":
			aggregations["state_durations"] = stateDurations
		case "transitions", "transition_frequency":
			aggregations[objects.FieldKeyTransitions] = transitionCounts
		case "common_transitions":
			aggregations["common_transitions"] = getTopTransitions(transitionCounts, 10)
		case "avg_history_length":
			aggregations["avg_history_length"] = averageSequenceLength(statusHistories)
		}
	}

	// If no aggregations specified, use defaults
	if len(aggregations) == 0 {
		aggregations["state_distribution"] = stateCounts
		aggregations[objects.FieldKeyTransitions] = transitionCounts
		aggregations["avg_history_length"] = averageSequenceLength(statusHistories)
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
func (a *StatusHistoryMetricAggregator) createMetricObject(
	ctx context.Context,
	config *AggregationConfig,
	aggregations map[string]any,
	objectCount int,
) (string, error) {
	title := fmt.Sprintf("Status history metric aggregation: %s.%s from %s to %s",
		config.ObjectKind, config.FieldName,
		config.WindowStart.Format("2006-01-02 15:04:05"),
		config.WindowEnd.Format("2006-01-02 15:04:05"))

	objConfig := &MetricObjectConfig{
		Source:         MetricSourcePipeline,
		Sampled:        false,
		AdditionalTags: []string{MetricTypeStatusHistory},
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

// getTopTransitions returns the top N transitions by frequency
func getTopTransitions(transitions map[string]int, n int) map[string]int {
	type kv struct {
		key   string
		value int
	}
	var pairs []kv
	for k, v := range transitions {
		pairs = append(pairs, kv{k, v})
	}

	// Sort by value (descending)
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].value > pairs[i].value {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}

	// Take top N
	topN := make(map[string]int)
	for i := 0; i < n && i < len(pairs); i++ {
		topN[pairs[i].key] = pairs[i].value
	}

	return topN
}
