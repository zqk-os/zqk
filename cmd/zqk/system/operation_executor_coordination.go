package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitOperationExecutorEventViaCoordinator emits operation executor events via the coordination system
// This provides unified event routing for operation executor lifecycle events
func emitOperationExecutorEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	workerCount int,
	processedCount int,
	failedCount int,
	duration time.Duration,
) {
	withSystemCoordinator(ctx, projectRoot, storageProvider, systemProfileSystem, true, func(ctx context.Context, coordinator *coordination.Coordinator) {
		// Build audit metadata
		auditMetadata := make(map[string]any)
		auditMetadata[eventKeyEventType] = fmt.Sprintf("operation_executor_%s", operationType)
		auditMetadata[eventKeyOperation] = fmt.Sprintf("Operation executor %s: %d workers, %d processed, %d failed", operationType, workerCount, processedCount, failedCount)
		auditMetadata[eventKeyWorkerCount] = workerCount
		auditMetadata[eventKeyProcessedCount] = processedCount
		auditMetadata[eventKeyFailedCount] = failedCount
		auditMetadata[eventKeyDurationSeconds] = duration.Seconds()
		auditMetadata[eventKeyOperationType] = operationTypeOperationExecutor
		auditMetadata[eventKeySource] = sourceBackgroundWorker
		auditMetadata[eventKeySeverity] = determineFailureSeverity(status, failedCount, processedCount)

		loggingFields, metricsData := buildWorkerLoggingAndMetrics(
			operationType, status,
			coordination.LoggingField{Key: eventKeyWorkerCount, Value: workerCount},
			coordination.LoggingField{Key: eventKeyProcessedCount, Value: processedCount},
			coordination.LoggingField{Key: eventKeyFailedCount, Value: failedCount},
		)

		emitOperational := operationType == "worker_start" || operationType == "worker_shutdown" || operationType == "worker_idle_shutdown"
		emitWorkerLifecycleCoordinationEvent(
			ctx, coordinator, operationID, eventTypeOperationExecutor, operationType, status,
			auditMetadata, loggingFields, metricsData, duration, emitOperational,
			"operation_executor_event_emit", fmt.Sprintf("emitting operation executor event: %s", operationType),
		)
	})
}

func init() {
	// Register operation executor event callback for coordinator integration
	// This allows the operation executor to emit events via coordinator without import cycles
	storage.SetOperationExecutorEventCallback(emitOperationExecutorEventViaCoordinator)
}
