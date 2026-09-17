package tsdb

import (
	"context"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// DataPoint represents a single metric reading.
type DataPoint struct {
	Timestamp time.Time
	Value     float64
}

// TSDB defines the interface for time series database operations.
// TRACK: BLI-REDACTED
type TSDB interface {
	Write(ctx context.Context, metric string, point DataPoint) error
	Read(ctx context.Context, metric string, start, end time.Time) ([]DataPoint, error)
	Close() error
}

// MemoryTSDB provides a simple in-memory implementation of TSDB.
type MemoryTSDB struct {
	mu   sync.RWMutex
	data map[string][]DataPoint
}

// NewMemoryTSDB initializes and returns a new MemoryTSDB.
func NewMemoryTSDB() *MemoryTSDB {
	return &MemoryTSDB{
		data: make(map[string][]DataPoint),
	}
}

// Write adds a new data point for a given metric.
func (db *MemoryTSDB) Write(ctx context.Context, metric string, point DataPoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	db.data[metric] = append(db.data[metric], point)
	return nil
}

// Read retrieves data points for a given metric within the time range [start, end].
func (db *MemoryTSDB) Read(ctx context.Context, metric string, start, end time.Time) ([]DataPoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db.mu.RLock()
	defer db.mu.RUnlock()

	points, ok := db.data[metric]
	if !ok {
		return nil, errfmt.Errorf("metric not found")
	}

	var result []DataPoint
	for _, p := range points {
		if (p.Timestamp.Equal(start) || p.Timestamp.After(start)) && (p.Timestamp.Equal(end) || p.Timestamp.Before(end)) {
			result = append(result, p)
		}
	}
	return result, nil
}

// Close closes the database connection (no-op for memory).
func (db *MemoryTSDB) Close() error {
	return nil
}
