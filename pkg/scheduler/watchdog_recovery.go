package scheduler

import (
	"context"
	"path/filepath"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/lockhealth"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// WatchdogInterceptor monitors worker heartbeats and orchestrates recovery for stalled daemons/workers.
type WatchdogInterceptor struct {
	tracker            *HeartbeatTracker
	projectRoot        string
	sweepInterval      time.Duration
	LockSweepThreshold time.Duration
	OnStalledWorker    func(status WorkerStatus)
	logger             logging.Logger
}

// NewWatchdogInterceptor creates an initialized WatchdogInterceptor.
func NewWatchdogInterceptor(tracker *HeartbeatTracker, projectRoot string, sweepInterval time.Duration) *WatchdogInterceptor {
	if tracker == nil {
		tracker = GetGlobalHeartbeatTracker()
	}
	if sweepInterval <= 0 {
		sweepInterval = 5 * time.Second
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	return &WatchdogInterceptor{
		tracker:            tracker,
		projectRoot:        projectRoot,
		sweepInterval:      sweepInterval,
		LockSweepThreshold: 5 * time.Minute,
		logger:             logger,
	}
}

// Start launches the watchdog recovery loop until the context is cancelled.
func (w *WatchdogInterceptor) Start(ctx context.Context) {
	ticker := time.NewTicker(w.sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.SweepAndRecover(ctx)
		}
	}
}

// SweepAndRecover inspects all registered workers for missed heartbeats and initiates recovery.
func (w *WatchdogInterceptor) SweepAndRecover(ctx context.Context) {
	staleWorkers := w.tracker.GetStaleWorkers(time.Now())

	for _, worker := range staleWorkers {
		// Log stalled worker detection
		if w.logger != nil {
			SLog(w.logger).Warn("watchdog_worker_stalled").
				String("worker_id", worker.ID).
				String("worker_kind", worker.Meta.Kind).
				String("stale_duration", time.Since(worker.LastPingAt).String()).
				Log()
		}

		// Mark worker stalled in tracker
		w.tracker.MarkStalled(worker.ID)

		// Abort worker context if cancelFunc is registered
		if worker.Meta.CancelFunc != nil {
			worker.Meta.CancelFunc()
		}

		// Invoke external notification callback
		if w.OnStalledWorker != nil {
			w.OnStalledWorker(worker)
		}
	}

	// Trigger stale lock sweep if a lock directory exists under projectRoot
	if w.projectRoot != "" {
		lockDir := filepath.Join(w.projectRoot, ".zqk", "locks")
		info, err := fileutil.Stat(lockDir)
		if err == nil && info.IsDir() {
			threshold := w.LockSweepThreshold
			if threshold <= 0 {
				threshold = 5 * time.Minute
			}
			report, err := lockhealth.Sweep(lockDir, threshold)
			if err == nil && report.Stats.RemovedStale > 0 {
				if w.logger != nil {
					SLog(w.logger).Info("watchdog_stale_locks_drained").
						Int("removed_count", report.Stats.RemovedStale).
						String("lock_dir", lockDir).
						Log()
				}
			}
		}
	}
}
