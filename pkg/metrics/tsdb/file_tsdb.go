package tsdb

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TRACK: BLI-1783822950016030000-81da5812 — FileTSDB wires chunked timeseries prototype for embedded telemetry.

const (
	fileTSDBChunkDuration = time.Hour
	fileTSDBValueScale    = 1000.0 // persist float64 as milli-units int64
)

// FileTSDB stores metrics under root using pkg/metrics chunked timeseries encoding.
type FileTSDB struct {
	root string
	mu   sync.Mutex
	// writers keyed by metric name; closed on Close.
	writers map[string]*metrics.TimeSeriesWriter
}

// OpenFileTSDB creates a disk-backed TSDB under root (created if missing).
func OpenFileTSDB(root string) (*FileTSDB, error) {
	if root == "" {
		return nil, errfmt.Errorf("tsdb: file root required")
	}
	if err := fileutil.EnsureDir(root); err != nil {
		return nil, errfmt.Newf("tsdb: ensure root").Wrap(err)
	}
	return &FileTSDB{
		root:    root,
		writers: make(map[string]*metrics.TimeSeriesWriter),
	}, nil
}

// Write appends a point for metric (float values stored as milli-units).
func (db *FileTSDB) Write(ctx context.Context, metric string, point DataPoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if metric == "" {
		return errfmt.Errorf("tsdb: metric name required")
	}
	if point.Timestamp.IsZero() {
		point.Timestamp = time.Now().UTC()
	}
	w, err := db.writerFor(metric)
	if err != nil {
		return err
	}
	scaled := int64(math.Round(point.Value * fileTSDBValueScale))
	return w.Append(metrics.TimeSeriesPoint{Ts: point.Timestamp.UTC(), Value: scaled})
}

// Read returns points in [start, end] for metric (milli-units converted back to float64).
func (db *FileTSDB) Read(ctx context.Context, metric string, start, end time.Time) ([]DataPoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if metric == "" {
		return nil, errfmt.Errorf("tsdb: metric name required")
	}
	// Flush active writer so reads see latest appends.
	db.mu.Lock()
	if w, ok := db.writers[metric]; ok {
		_ = w.Close()
		delete(db.writers, metric)
	}
	db.mu.Unlock()

	cfg := metrics.TimeSeriesConfig{
		ChunkDuration: fileTSDBChunkDuration,
		Dir:           db.root,
		Series:        metric,
	}
	r := metrics.NewTimeSeriesReader(cfg)
	var out []DataPoint
	err := r.Range(start.UTC(), end.UTC(), func(p metrics.TimeSeriesPoint) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		out = append(out, DataPoint{
			Timestamp: p.Ts,
			Value:     float64(p.Value) / fileTSDBValueScale,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errfmt.Errorf("metric not found")
	}
	return out, nil
}

// Close flushes and closes all open series writers.
func (db *FileTSDB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	var first error
	for name, w := range db.writers {
		if err := w.Close(); err != nil && first == nil {
			first = err
		}
		delete(db.writers, name)
	}
	return first
}

func (db *FileTSDB) writerFor(metric string) (*metrics.TimeSeriesWriter, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	if w, ok := db.writers[metric]; ok {
		return w, nil
	}
	cfg := metrics.TimeSeriesConfig{
		ChunkDuration: fileTSDBChunkDuration,
		Dir:           db.root,
		Series:        metric,
	}
	w, err := metrics.NewTimeSeriesWriter(cfg)
	if err != nil {
		return nil, err
	}
	db.writers[metric] = w
	return w, nil
}
