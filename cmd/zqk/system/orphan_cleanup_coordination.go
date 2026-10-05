package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
)

// emitOrphanCleanupEventViaCoordinator emits orphan cleanup events via the coordination system
// This provides unified event routing for orphan cleanup operations
func emitOrphanCleanupEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProviderCas caspkg.CASFacade,
	operationID string,
	operationType string,
	status string,
	batchSize int,
	successCount int,
	failureCount int,
	duration time.Duration,
	failedFiles []string,
) {
	storageProvider := storageProviderCas.(storage.ObjectStorageProvider)
	withSystemCoordinator(ctx, projectRoot, storageProvider, systemProfileSystem, true, func(ctx context.Context, coordinator *coordination.Coordinator) {
		// Build audit metadata
		auditMetadata := make(map[string]any)
		// Map operation type and status to valid audit event types
		// Valid event types: orphan_cleanup_start, orphan_cleanup_complete, orphan_cleanup_error
		eventType := resolveOrphanCleanupEventType(status, operationType, failureCount)
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

		auditMetadata[eventKeySeverity] = determineFailureSeverity(status, failureCount, batchSize)

		loggingFields, metricsData := buildWorkerLoggingAndMetrics(
			operationType, status,
			coordination.LoggingField{Key: eventKeyBatchSize, Value: batchSize},
			coordination.LoggingField{Key: eventKeySuccessCount, Value: successCount},
			coordination.LoggingField{Key: eventKeyFailureCount, Value: failureCount},
		)
		if len(failedFiles) > 0 {
			metricsData[eventKeyFailedFileCount] = len(failedFiles)
		}

		emitOperational := operationType == "worker_started" || operationType == "worker_stopped"
		emitWorkerLifecycleCoordinationEvent(
			ctx, coordinator, operationID, "orphan_cleanup", operationType, status,
			auditMetadata, loggingFields, metricsData, duration, emitOperational,
			"orphan_cleanup_event_emit", fmt.Sprintf("emitting orphan cleanup event: %s", operationType),
		)
	})
}

func resolveOrphanCleanupEventType(status, operationType string, failureCount int) string {
	switch {
	case status == eventStatusError || failureCount > 0:
		return "orphan_cleanup_error"
	case status == eventStatusStart || operationType == "worker_started":
		return "orphan_cleanup_start"
	default:
		return "orphan_cleanup_complete"
	}
}
