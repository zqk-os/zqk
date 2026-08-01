package system

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/storage"
)

// emitOrphanCleanupEventViaCoordinator emits orphan cleanup events via the coordination system
// This provides unified event routing for orphan cleanup operations
func emitOrphanCleanupEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	batchSize int,
	successCount int,
	failureCount int,
	duration time.Duration,
	failedFiles []string,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, systemProfileSystem)

	// Create routers for coordinator
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)

	// Create metrics pipeline for metrics router (if storage provider available)
	var metricsRouter coordination.MetricsRouter
	if storageProvider != nil {
		metricsPipeline := metrics.MetricPipelineForProject(storageProvider, projectRoot)
		metricsRouter = coordination.NewMetricPipelineRouter(metricsPipeline)
	}

	// Create coordinator with routers
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     metricsRouter,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build audit metadata
	auditMetadata := make(map[string]any)
	// Map operation type and status to valid audit event types
	// Valid event types: orphan_cleanup_start, orphan_cleanup_complete, orphan_cleanup_error
	var eventType string
	if status == eventStatusError || failureCount > 0 {
		eventType = "orphan_cleanup_error"
	} else if status == eventStatusStart || operationType == "worker_started" {
		eventType = "orphan_cleanup_start"
	} else {
		// Default to complete for batch operations and other statuses
		eventType = "orphan_cleanup_complete"
	}
	auditMetadata[eventKeyEventType] = eventType
	auditMetadata[eventKeyOperation] = fmt.Sprintf("Orphan cleanup %s: %d files processed (%d succeeded, %d failed)", operationType, batchSize, successCount, failureCount)
	auditMetadata[eventKeyBatchSize] = batchSize
	auditMetadata[eventKeySuccessCount] = successCount
	auditMetadata[eventKeyFailureCount] = failureCount
	auditMetadata[eventKeyDurationSeconds] = duration.Seconds()
	auditMetadata[eventKeyOperationType] = "orphan_cleanup"
	auditMetadata[eventKeySource] = sourceBackgroundWorker

	if len(failedFiles) > 0 {
		// Include first few failed files for debugging (don't include all to avoid huge events)
		maxFailedFiles := 5
		if len(failedFiles) > maxFailedFiles {
			auditMetadata["failed_files"] = failedFiles[:maxFailedFiles]
			auditMetadata["failed_files_truncated"] = true
			auditMetadata["total_failed_files"] = len(failedFiles)
		} else {
			auditMetadata["failed_files"] = failedFiles
		}
	}

	// Determine severity
	severity := severityLow
	if status == eventStatusError || failureCount > 0 {
		if failureCount > batchSize/2 {
			severity = severityHigh
		} else {
			severity = severityMedium
		}
	}
	auditMetadata[eventKeySeverity] = severity

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: eventKeyOperationType, Value: operationType},
		{Key: eventKeyBatchSize, Value: batchSize},
		{Key: eventKeySuccessCount, Value: successCount},
		{Key: eventKeyFailureCount, Value: failureCount},
		{Key: eventKeyStatus, Value: status},
	}
	if duration > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: eventKeyDurationSeconds, Value: duration.Seconds()})
	}

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[eventKeyOperationType] = operationType
	metricsData[eventKeyBatchSize] = batchSize
	metricsData[eventKeySuccessCount] = successCount
	metricsData[eventKeyFailureCount] = failureCount
	metricsData[eventKeyStatus] = status
	if duration > 0 {
		metricsData[eventKeyDurationSeconds] = duration.Seconds()
		metricsData[eventKeyDurationNS] = duration.Nanoseconds()
	}
	if len(failedFiles) > 0 {
		metricsData[eventKeyFailedFileCount] = len(failedFiles)
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Create event context (enable audit, metrics, and logging; operational for lifecycle events)
	emitOperational := operationType == "worker_started" || operationType == "worker_stopped"
	eventCtx := coordination.NewEventContext(operationID, "orphan_cleanup", status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, true, emitOperational) // Logging, audit, metrics; operational for lifecycle

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("orphan_cleanup_event_emit", fmt.Sprintf("emitting orphan cleanup event: %s", operationType))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
