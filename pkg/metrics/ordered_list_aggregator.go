package metrics

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// OrderedListMetricAggregator aggregates ordered list-based metric fields
type OrderedListMetricAggregator struct {
	storage storage.ObjectStorageProvider
}

// NewOrderedListMetricAggregator creates a new ordered list metric aggregator
func NewOrderedListMetricAggregator(storageProvider storage.ObjectStorageProvider) *OrderedListMetricAggregator {
	return &OrderedListMetricAggregator{
		storage: storageProvider,
	}
}

// GetMetricType returns the metric type this aggregator handles
func (a *OrderedListMetricAggregator) GetMetricType() string {
	return MetricTypeOrderedList
}

// Aggregate collects and aggregates ordered list metric data
func (a *OrderedListMetricAggregator) Aggregate(ctx context.Context, config *AggregationConfig) (*AggregationResult, error) {
	rawObjects, err := QueryAggregationObjects(ctx, a.storage, config)
	if err != nil {
		return nil, err
	}

	// Collect sequences and analyze patterns
	sequences := make([][]string, 0)
	objectCount := 0
	transitionCounts := make(map[string]int) // "from->to" -> count

	for _, obj := range rawObjects {
		if !isInWindow(obj, config.WindowStart, config.WindowEnd) {
			continue
		}

		fieldValue, exists := obj[config.FieldName]
		if !exists {
			continue
		}

		// Extract ordered list values
		listValue, ok := fieldValue.([]any)
		if !ok {
			continue
		}

		// Convert to string sequence
		sequence := make([]string, 0, len(listValue))
		for _, item := range listValue {
			sequence = append(sequence, fmt.Sprintf("%v", item))
		}

		if len(sequence) > 0 {
			sequences = append(sequences, sequence)

			// Track transitions
			for i := 0; i < len(sequence)-1; i++ {
				transition := fmt.Sprintf("%s->%s", sequence[i], sequence[i+1])
				transitionCounts[transition]++
			}
		}

		objectCount++
	}

	// Perform aggregations
	aggregations := make(map[string]any)

	for _, aggFunc := range config.Aggregations {
		switch aggFunc {
		case "sequence_count":
			aggregations["sequence_count"] = len(sequences)
		case "avg_sequence_length":
			aggregations["avg_sequence_length"] = averageSequenceLength(sequences)
		case "transitions", "transition_frequency":
			aggregations[objects.FieldKeyTransitions] = transitionCounts
		case "common_sequences":
			aggregations["common_sequences"] = findCommonSequences(sequences, 5)
		}
	}

	// If no aggregations specified, use defaults
	if len(aggregations) == 0 {
		aggregations["sequence_count"] = len(sequences)
		aggregations["avg_sequence_length"] = averageSequenceLength(sequences)
		aggregations[objects.FieldKeyTransitions] = transitionCounts
	}

	// Create base_metric object
	metricID, err := a.createMetricObject(ctx, config, aggregations, objectCount)
	if err != nil {
		return nil, errfmt.Newf("failed to create metric object").Wrap(err)
	}

	return BuildAggregationResult(config, metricID, aggregations, objectCount), nil
}

// createMetricObject creates a base_metric object using the factory pattern (non-blocking)
func (a *OrderedListMetricAggregator) createMetricObject(
	ctx context.Context,
	config *AggregationConfig,
	aggregations map[string]any,
	objectCount int,
) (string, error) {
	title := fmt.Sprintf("Ordered list metric aggregation: %s.%s from %s to %s",
		config.ObjectKind, config.FieldName,
		config.WindowStart.Format("2006-01-02 15:04:05"),
		config.WindowEnd.Format("2006-01-02 15:04:05"))

	objConfig := &MetricObjectConfig{
		Source:         MetricSourcePipeline,
		Sampled:        false,
		AdditionalTags: []string{MetricTypeOrderedList},
	}

	return CreateSyncMetricObject(ctx, a.storage, config, title, objConfig, aggregations, objectCount)
}

// averageSequenceLength calculates average length of sequences
func averageSequenceLength(sequences [][]string) float64 {
	if len(sequences) == 0 {
		return 0
	}
	total := 0
	for _, seq := range sequences {
		total += len(seq)
	}
	return float64(total) / float64(len(sequences))
}

// findCommonSequences finds common sequence patterns
func findCommonSequences(sequences [][]string, minLength int) map[string]int {
	sequenceCounts := make(map[string]int)

	for _, seq := range sequences {
		if len(seq) >= minLength {
			// Extract subsequences
			for i := 0; i <= len(seq)-minLength; i++ {
				subseq := seq[i : i+minLength]
				key := fmt.Sprintf("%v", subseq)
				sequenceCounts[key]++
			}
		}
	}

	// Return top sequences
	topSequences := make(map[string]int)
	for key, count := range sequenceCounts {
		if count > 1 { // Only sequences that appear more than once
			topSequences[key] = count
		}
	}

	return topSequences
}
