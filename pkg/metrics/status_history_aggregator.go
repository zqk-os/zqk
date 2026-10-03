package metrics

import (
	"context"
	"fmt"

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
	rawObjects, err := QueryAggregationObjects(ctx, a.storage, config)
	if err != nil {
		return nil, err
	}

	// Collect status histories and analyze
	statusHistories := make([][]string, 0)
	stateDurations := make(map[string]float64) // state -> total duration
	stateCounts := make(map[string]int)        // state -> count
	transitionCounts := make(map[string]int)   // "from->to" -> count
	objectCount := 0

	for _, obj := range rawObjects {
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

	return BuildAggregationResult(config, metricID, aggregations, objectCount), nil
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

	return CreateSyncMetricObject(ctx, a.storage, config, title, objConfig, aggregations, objectCount)
}

// getTopTransitions returns the top N transitions by frequency
func getTopTransitions(transitions map[string]int, n int) map[string]int {
	return TopNMap(transitions, n)
}
