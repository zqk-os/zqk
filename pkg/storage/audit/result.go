package audit

import "time"

// AggregationResult is the outcome of AggregateAuditEvents.
type AggregationResult struct {
	MetricID        string
	EventCount      int
	MetricsCreated  int
	EventsProcessed []string
	EventsUpdated   int
	WindowStart     time.Time
	WindowEnd       time.Time
}

// EmptyAggregationResult is a successful run that found no events.
func EmptyAggregationResult() *AggregationResult {
	return &AggregationResult{EventsProcessed: []string{}}
}

// CompletedAggregationResult is a successful run that wrote one metric.
func CompletedAggregationResult(metricID string, eventIDs []string, updated int, start, end time.Time) *AggregationResult {
	return &AggregationResult{
		MetricID:        metricID,
		EventCount:      len(eventIDs),
		MetricsCreated:  1,
		EventsProcessed: eventIDs,
		EventsUpdated:   updated,
		WindowStart:     start,
		WindowEnd:       end,
	}
}
