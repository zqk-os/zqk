package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
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
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, systemProfileSystem, true)
	if !ok {
		return
	}

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

	// Determine severity
	severity := severityLow
	if status == eventStatusError || failedCount > 0 {
		if failedCount > processedCount/2 {
			severity = severityHigh
		} else {
			severity = severityMedium
		}
	}
	auditMetadata[eventKeySeverity] = severity

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: eventKeyOperationType, Value: operationType},
		{Key: eventKeyWorkerCount, Value: workerCount},
		{Key: eventKeyProcessedCount, Value: processedCount},
		{Key: eventKeyFailedCount, Value: failedCount},
		{Key: eventKeyStatus, Value: status},
	}
	// Build metrics data
	metricsData := map[string]any{
		eventKeyOperationType:  operationType,
		eventKeyWorkerCount:    workerCount,
		eventKeyProcessedCount: processedCount,
		eventKeyFailedCount:    failedCount,
		eventKeyStatus:         status,
	}

	eventData := buildCoordinationEventData(loggingFields, auditMetadata, metricsData, duration, nil)

	// Create event context (enable audit, metrics, and logging; operational for lifecycle events)
	emitOperational := operationType == "worker_start" || operationType == "worker_shutdown" || operationType == "worker_idle_shutdown"
	eventCtx := coordination.NewEventContext(operationID, eventTypeOperationExecutor, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, true, emitOperational) // Logging, audit, metrics; operational for lifecycle

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("operation_executor_event_emit", fmt.Sprintf("emitting operation executor event: %s", operationType))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}

func init() {
	// Register operation executor event callback for coordinator integration
	// This allows the operation executor to emit events via coordinator without import cycles
	storage.SetOperationExecutorEventCallback(emitOperationExecutorEventViaCoordinator)
}
