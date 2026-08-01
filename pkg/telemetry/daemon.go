package telemetry

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

// Daemon represents the telemetry aggregation daemon
type Daemon struct {
	logger logging.Logger
}

// NewDaemon creates a new Daemon
func NewDaemon(logger logging.Logger) *Daemon {
	return &Daemon{
		logger: logger,
	}
}

// Aggregate runs the aggregation of logs and metrics into unified graph nodes.
func (d *Daemon) Aggregate(ctx context.Context) error {
	logging.Fluent(d.logger).Info("Aggregating logs and metrics from agent tasks into unified graph nodes...").Log()
	// Integration with Graph DB goes here
	return nil
}

// RunBackgroundGC starts a blocking loop that periodically runs CompactOldSegments.
// It stops when the provided context is canceled.
func (d *Daemon) RunBackgroundGC(ctx context.Context, roots []string, maxAge time.Duration, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logging.Fluent(d.logger).Info("Starting telemetry background GC loop").String("interval", interval.String()).Log()

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
