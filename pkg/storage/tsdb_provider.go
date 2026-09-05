package storage

import (
	"context"
	"time"
)

// TSDBPoint represents a single time-series data point.
type TSDBPoint struct {
	Measurement string            // Measurement name (e.g., "scheduler_events", "cpu_usage")
	Tags        map[string]string // Indexed tags for querying
	Fields      map[string]any    // Unindexed fields/values
	Timestamp   time.Time         // Point in time
}

// TSDBQuery represents a query against the TSDB.
type TSDBQuery struct {
	Measurement   string            // Measurement name to query
	Tags          map[string]string // Tag filters
	StartTime     time.Time         // Start of time range
	EndTime       time.Time         // End of time range
	GroupBy       []string          // Tags or time buckets to group by
	Aggregation   string            // e.g., "count", "sum", "mean"
	Field         string            // Field to aggregate
	RPNExpression string            // RPN expression for filtering/aggregating (e.g., "val 10 >")
}

// TSDBQueryResult represents the result of a TSDB query.
type TSDBQueryResult struct {
	Points []TSDBPoint
}

// TSDBProvider defines the interface for time-series metrics and audit storage.
type TSDBProvider interface {
	// Initialize prepares the TSDB connection/storage
	Initialize(ctx context.Context) error

	// WritePoint writes a single data point
	WritePoint(ctx context.Context, point TSDBPoint) error

	// WriteBatch writes multiple data points efficiently
	WriteBatch(ctx context.Context, points []TSDBPoint) error

	// Query executes a query against the time-series data
	Query(ctx context.Context, query TSDBQuery) (*TSDBQueryResult, error)

	// Close gracefully shuts down the provider
	Close(ctx context.Context) error
}
