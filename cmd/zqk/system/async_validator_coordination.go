package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck/asynccheck"
)

// emitAsyncValidatorEventViaCoordinator emits async validator events via the coordination system
func emitAsyncValidatorEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	eventType string,
	objectID string,
	message string,
	fields map[string]any,
	severity string,
	profile string,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	asynccheck.EmitAsyncValidatorEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		eventType, objectID, message, fields, severity, profile,
	)
}

// emitValidationMetricsViaCoordinator emits validation performance metrics via coordinator
func emitValidationMetricsViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	queueSize int,
	completed int,
	failed int,
	throughput float64,
	activeGoroutines int,
	profile string,
) {
	fields := map[string]any{
		"queue_size":           queueSize,
		"completed":            completed,
		objects.FieldKeyFailed: failed,
		"throughput_per_sec":   throughput,
		"active_goroutines":    activeGoroutines,
	}

	emitAsyncValidatorEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"metrics", "", fmt.Sprintf("Queue: %d, Completed: %d, Failed: %d, Throughput: %.2f/sec", queueSize, completed, failed, throughput),
		fields, "low", profile,
	)
}

// emitSemaphoreFullWarningViaCoordinator emits a warning when semaphore is full
func emitSemaphoreFullWarningViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	objectID string,
	queueSize int,
	semaphoreCapacity int,
	profile string,
) {
	fields := map[string]any{
		"queue_size":         queueSize,
		"semaphore_capacity": semaphoreCapacity,
		"object_id":          objectID,
	}

	emitAsyncValidatorEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"semaphore_full", objectID,
		fmt.Sprintf("Semaphore full (capacity: %d), validation goroutines may be stuck", semaphoreCapacity),
		fields, "high", profile,
	)
}

// emitValidationTimeoutViaCoordinator emits a timeout event for validation
func emitValidationTimeoutViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	objectID string,
	timeoutDuration time.Duration,
	profile string,
) {
	fields := map[string]any{
		"object_id":        objectID,
		"timeout_duration": timeoutDuration.String(),
	}

	emitAsyncValidatorEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"validation_timeout", objectID,
		fmt.Sprintf("Validation timeout after %v for %s", timeoutDuration, objectID),
		fields, "high", profile,
	)
}

// emitFileReadErrorViaCoordinator emits an error event for file read failures
func emitFileReadErrorViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	objectID string,
	filePath string,
	err error,
	profile string,
) {
	fields := map[string]any{
		eventKeyObjectID:         objectID,
		objects.FieldKeyFilePath: filePath,
		eventKeyError:            err.Error(),
	}

	emitAsyncValidatorEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"file_read_error", objectID,
		fmt.Sprintf("Failed to read file: %s", filePath),
		fields, "medium", profile,
	)
}

// emitWorkerStuckWarningViaCoordinator emits a warning when workers appear stuck
func emitWorkerStuckWarningViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	workerID int,
	state string,
	stuckDuration time.Duration,
	profile string,
) {
	fields := map[string]any{
		"worker_id":      workerID,
		"worker_state":   state,
		"stuck_duration": stuckDuration.String(),
	}

	emitAsyncValidatorEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"worker_stuck", "",
		fmt.Sprintf("Worker %d stuck in state '%s' for %v", workerID, state, stuckDuration),
		fields, "high", profile,
	)
}
