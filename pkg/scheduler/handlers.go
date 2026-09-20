package scheduler

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// JobHandler defines the interface for job execution
type JobHandler interface {
	Execute(ctx context.Context, job *ScheduledJob) error
}

// CachePrewarmHandler moved to handlers_cache_prewarm.go

// LifecycleCheckHandler runs lifecycle checks
type LifecycleCheckHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// NewLifecycleCheckHandler creates a new lifecycle check handler
// NewLifecycleCheckHandler creates a new lifecycle check handler
func NewLifecycleCheckHandler(storage storagepkg.ObjectStorageProvider) LifecycleCheckHandlerInterface {
	return &LifecycleCheckHandler{
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute runs lifecycle checks
func (h *LifecycleCheckHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunLifecycleCheckViaPipeline(ctx, h, job)
}

// executeLifecycleCheckCore runs lifecycle checks (called from RunLifecycleCheckViaPipeline NORMALIZE stage).
func (h *LifecycleCheckHandler) executeLifecycleCheckCore(ctx context.Context, job *ScheduledJob) error {
	LifecycleCheckLog(h.logger).Info(LogEventLifecycleCheckStarted).
		JobID(job.ID).
		Log()

	// This would trigger lifecycle evaluation for all objects
	// Implementation would call lifecycle evaluation logic
	// For now, placeholder
	LifecycleCheckLog(h.logger).Info(LogEventLifecycleCheckCompleted).
		JobID(job.ID).
		Log()

	return nil
}

// Aggregation handlers moved to handlers_aggregation.go
// Cleanup handler moved to handlers_cleanup.go

// NoOpHandler is a placeholder handler for unimplemented job types
type NoOpHandler struct {
	logger logging.Logger
}

// NewNoOpHandler creates a new no-op handler
// NewNoOpHandler creates a new no-op handler
func NewNoOpHandler() JobHandler {
	return &NoOpHandler{
		logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute does nothing
func (h *NoOpHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	NoOpHandlerLog(h.logger).Warn(LogEventNoOpHandlerExecuted).
		JobID(job.ID).
		SchedulerJobType(job.JobType).
		Log()
	if isJobTypeIntentionalNoHandler(job.JobType) {
		return nil
	}
	return errfmt.Errorf("job type %s not yet implemented", job.JobType)
}
