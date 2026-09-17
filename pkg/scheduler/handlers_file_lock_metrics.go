package scheduler

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// Log events for metrics_collection (file lock) handler (POL-CODE-007 stable keys).
const (
	LogEventMetricsCollectionJobStart     = JobTypeMetricsCollection + "_job_start"
	LogEventMetricsCollectionProcessing   = JobTypeMetricsCollection + "_processing"
	LogEventMetricsCollectionFailed       = JobTypeMetricsCollection + "_failed"
	LogEventMetricsCollectionTimedOut     = JobTypeMetricsCollection + "_timed_out"
	LogEventMetricsCollectionJobCompleted = JobTypeMetricsCollection + "_job_completed"
)

// FileLockMetricsCollectionHandler collects file lock metrics
type FileLockMetricsCollectionHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// NewFileLockMetricsCollectionHandler creates a new file lock metrics collection handler
// NewFileLockMetricsCollectionHandler creates a new file lock metrics collection handler
func NewFileLockMetricsCollectionHandler(storage storagepkg.ObjectStorageProvider) JobHandler {
	return &FileLockMetricsCollectionHandler{
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute collects file lock metrics and creates a metric object
func (h *FileLockMetricsCollectionHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunMetricsCollectionViaPipeline(ctx, h, job)
}

// executeMetricsCollectionCore collects file lock metrics and creates a metric object.
// Called from RunMetricsCollectionViaPipeline NORMALIZE stage.
func (h *FileLockMetricsCollectionHandler) executeMetricsCollectionCore(ctx context.Context, job *ScheduledJob) error {
	SLog(h.logger).Info(LogEventMetricsCollectionJobStart).
		JobID(job.ID).
		Log()

	// Get collection window from job environment variables or use defaults
	// Default: collect metrics from last 6 hours
	windowDuration := 6 * time.Hour

	if job.EnvironmentVariables != nil {
		if windowStr, ok := job.EnvironmentVariables[EnvKeyCollectionWindow]; ok && windowStr != emptyValue {
			if duration, err := time.ParseDuration(windowStr); err == nil {
				windowDuration = duration
			}
		}
	}

	// Calculate time window
	windowEnd := time.Now().UTC()
	windowStart := windowEnd.Add(-windowDuration)

	SLog(h.logger).Info(LogEventMetricsCollectionProcessing).
		String("start", windowStart.Format(time.RFC3339)).
		String("end", windowEnd.Format(time.RFC3339)).
		String("window", windowDuration.String()).
		Log()

	// Get contexts
	secCtx := pkgctx.NewSystemSecurityContext()

	// Use synchronous collector directly for reliability
	// The async collector may have issues with worker initialization or callback handling
	collector := storagepkg.NewFileLockMetricsCollector(h.storage)

	// Collect metrics with context timeout handling
	// Use a channel to detect if collection is blocking
	done := make(chan struct {
		metricID string
		err      error
	}, 1)

	goroutinelabels.NewGoroutine("scheduler_file_lock_metrics_collector", "collecting file lock metrics").
		StartWithContext(ctx, func(ctx context.Context) error {
			metricID, err := collector.CollectAndReset(ctx, secCtx, windowStart, windowEnd)
			select {
			case done <- struct {
				metricID string
				err      error
			}{metricID, err}:
			case <-ctx.Done():
				// Context cancelled, skip sending result
			}
			return nil
		})

	// Wait for completion or context cancellation
	var metricID string
	select {
	case result := <-done:
		if result.err != nil {
			SLog(h.logger).Error(LogEventMetricsCollectionFailed, result.err).
				JobID(job.ID).
				Log()
			return errfmt.Newf("metrics collection failed").Wrap(result.err)
		}
		metricID = result.metricID
	case <-ctx.Done():
		// Context was cancelled or timed out
		if ctx.Err() == context.DeadlineExceeded {
			SLog(h.logger).Error(LogEventMetricsCollectionTimedOut,
				errfmt.Errorf("collection exceeded max_runtime_seconds (%d) - may be stuck waiting for locks or I/O", job.MaxRuntimeSeconds)).
				JobID(job.ID).
				String("job_type", job.JobType).
				Int("max_runtime_seconds", job.MaxRuntimeSeconds).
				Log()
			return errfmt.Errorf("metrics collection timed out after %d seconds", job.MaxRuntimeSeconds)
		}
		return ctx.Err()
	}

	if metricID != emptyValue {
		SLog(h.logger).Info(LogEventMetricsCollectionJobCompleted).
			JobID(job.ID).
			MetricID(metricID).
			Log()
	}

	return nil
}
