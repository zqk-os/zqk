package scheduler

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// emitJobExecutionEventViaCoordinator emits scheduler job execution events via the coordination system
// This provides unified event routing for job lifecycle operations (started, completed, failed)
func emitJobExecutionEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	eventType string, // "scheduler_job_started", "scheduler_job_completed", "scheduler_job_failed"
	jobID, jobType, category string,
	success bool,
	duration time.Duration,
	jobErr error,
) {
	if projectRoot == emptyValue || projectRoot == "." {
		// Best effort - skip if no project root
		return
	}

	// Use "system" profile for scheduler operations (no CLI context available)
	// Embed LoggingContext in context so coordinator logging respects profile
	loggingCtx := pkgctx.NewSystemLoggingContext()
	ctx = pkgctx.WithLoggingContext(ctx, loggingCtx)

	// Create routers directly (we call them synchronously, not via coordinator.Emit which spawns goroutines)
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)

	// Build operation description
	operation := fmt.Sprintf("Scheduler job %s (%s) %s", jobID, jobType, eventType)
	if eventType == "scheduler_job_completed" && success {
		operation = fmt.Sprintf("Scheduler job %s (%s) completed successfully in %s", jobID, jobType, duration)
	} else if eventType == "scheduler_job_failed" && jobErr != nil {
		operation = fmt.Sprintf("Scheduler job %s (%s) failed: %v", jobID, jobType, jobErr)
	}

	// Determine severity
	severity := "low"
	if !success {
		severity = "medium" // Job failures are medium severity
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[objects.FieldKeyEventType] = eventType
	auditMetadata[objects.FieldKeyOperation] = operation
	auditMetadata[objects.FieldKeyTargetKind] = objects.KindSchedulerJob
	auditMetadata[objects.FieldKeyTargetID] = jobID
	auditMetadata[KeyJobID] = jobID
	auditMetadata[KeyJobType] = jobType
	auditMetadata[KeyCategory] = category
	auditMetadata[objects.FieldKeyDurationSeconds] = duration.Seconds()
	auditMetadata[KeySuccess] = success
	auditMetadata[KeySeverity] = severity
	auditMetadata[objects.FieldKeySource] = "scheduler"
	if jobErr != nil {
		auditMetadata[KeyError] = jobErr.Error()
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: KeyJobID, Value: jobID},
		{Key: KeyJobType, Value: jobType},
		{Key: KeyCategory, Value: category},
		{Key: KeyEventType, Value: eventType},
	}
	if duration > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "duration_seconds", Value: duration.Seconds()})
	}
	if !success && jobErr != nil {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: KeyError, Value: jobErr.Error()})
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Job execution events don't create metrics (metrics tracked separately)
	}

	// Determine status
	status := "complete"
	if !success {
		status = "error"
	} else if eventType == "scheduler_job_started" {
		status = "start"
	}

	// Create operation ID
	operationID := fmt.Sprintf("scheduler_job_%s_%s_%d", eventType, jobID, time.Now().UnixNano())

	// Create event context (enable audit and logging channels)
	eventCtx := coordination.NewEventContext(operationID, "scheduler_job_execution", status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, false, false) // Audit and logging, no metrics/operational

	if jobErr != nil {
		eventCtx = eventCtx.WithError(jobErr)
	}

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Update activity cache and emit synchronously in the caller (worker goroutine).
	// Previously coordinator.Emit() spawned 1-2 goroutines per event × 3 events per job = 3-6 goroutines per job,
	// which caused unbounded thread growth under load. Call routers directly to avoid goroutine spawning.
	if projectRoot != emptyValue {
		cache := GetGlobalActivityCache()
		if cache.GetMetadata() == nil {
			_ = cache.LoadCache(projectRoot) //nolint:errcheck // Best effort
		}
		cache.UpdateEvent(jobID, eventType, duration, jobErr)
		_ = cache.SaveCache(projectRoot) //nolint:errcheck // Best effort
	}

	// Call routers directly (synchronous) instead of coordinator.Emit() which spawns goroutines.
	// This prevents unbounded goroutine growth: 3 events/job × 1-2 goroutines/event = 3-6 goroutines/job.
	// Routers handle their own async I/O if needed (e.g. audit router uses storage which may buffer).
	loggingRouter := &coordination.DefaultLoggingRouter{}
	_ = loggingRouter.Emit(ctx, eventCtx) //nolint:errcheck // Best effort
	_ = auditRouter.Emit(ctx, eventCtx)   //nolint:errcheck // Best effort
}
