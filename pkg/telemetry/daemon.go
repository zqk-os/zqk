package telemetry

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// Daemon represents the telemetry aggregation daemon
type Daemon struct {
	logger  logging.Logger
	summary map[string]any
	mu      sync.RWMutex
}

// NewDaemon creates a new Daemon
func NewDaemon(logger logging.Logger) *Daemon {
	if logger == nil {
		logger = logging.GetLoggerFromProfile("system")
	}
	return &Daemon{
		logger:  logger,
		summary: make(map[string]any),
	}
}

// Aggregate runs the aggregation of logs and metrics into unified graph nodes.
func (d *Daemon) Aggregate(ctx context.Context) error {
	logging.Fluent(d.logger).Info("Aggregating logs and metrics from agent tasks into unified graph nodes...").Log()

	mgr := GlobalManager()
	var totalSpans, totalMetrics int
	var errorSpans int

	for _, h := range mgr.GetHooks() {
		if inMem, ok := h.(*InMemoryHook); ok {
			spans := inMem.GetRecentSpans()
			metrics := inMem.GetRecentMetrics()
			totalSpans += len(spans)
			totalMetrics += len(metrics)
			for _, s := range spans {
				if s.Err != "" {
					errorSpans++
				}
			}
		}
	}

	d.mu.Lock()
	d.summary = map[string]any{
		"aggregated_at": time.Now().UTC().Format(time.RFC3339),
		"total_spans":   totalSpans,
		"error_spans":   errorSpans,
		"total_metrics": totalMetrics,
		"status":        "healthy",
	}
	d.mu.Unlock()

	return nil
}

// GetSummary returns the most recent aggregation summary.
func (d *Daemon) GetSummary() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	res := make(map[string]any, len(d.summary))
	for k, v := range d.summary {
		res[k] = v
	}
	return res
}

// RunBackgroundGC starts a blocking loop that periodically runs CompactOldSegments.
// It stops when the provided context is canceled.
func (d *Daemon) RunBackgroundGC(ctx context.Context, roots []string, maxAge time.Duration, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logging.Fluent(d.logger).Info("Starting telemetry background GC loop").Interval(interval.String()).Log()

	for {
		select {
		case <-ctx.Done():
			logging.Fluent(d.logger).Info("Stopping telemetry background GC loop").Log()
			return
		case <-ticker.C:
			count, err := CompactOldSegments(roots, maxAge)
			if err != nil {
				logging.Fluent(d.logger).Error("Telemetry GC failed", err).Log()
			} else if count > 0 {
				logging.Fluent(d.logger).Info("Telemetry GC completed").Int("removed_files", count).Log()
			}
		}
	}
}
