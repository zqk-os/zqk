package metrics

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
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

	// Collect sequences and analyze patterns
	sequences := make([][]string, 0)
	objectCount := 0
	transitionCounts := make(map[string]int) // "from->to" -> count

	for _, obj := range result.Objects {
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
