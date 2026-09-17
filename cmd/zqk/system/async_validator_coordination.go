package system

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// emitAsyncValidatorEventViaCoordinator emits async validator events via the coordination system
// This provides unified event routing for validation errors, warnings, and metrics
func emitAsyncValidatorEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	eventType string, // "semaphore_full", "validation_timeout", "file_read_error", "worker_stuck", "metrics"
	objectID string,
	message string,
	fields map[string]any,
	severity string, // "low", "medium", "high", "critical"
	profile string, // CLI context profile for logging format
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

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

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "operation_id", Value: operationID},
		{Key: "operation_type", Value: eventTypeAsyncValidation},
		{Key: eventKeyEventType, Value: eventType},
	}
	if objectID != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "object_id", Value: objectID})
	}
	if message != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "message", Value: message})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: k, Value: v})
	}

	// Build audit metadata (only for important events, not metrics)
	auditMetadata := make(map[string]any)
	shouldAudit := eventType != "metrics" && severity != severityLow
	if shouldAudit {
		auditMetadata[eventKeyEventType] = eventTypeSystemConfigChange
		if message != emptyValue {
			auditMetadata[eventKeyOperation] = fmt.Sprintf("Async validation %s: %s", eventType, message)
		} else {
			auditMetadata[eventKeyOperation] = fmt.Sprintf("Async validation %s", eventType)
		}
		if severity == emptyValue {
			severity = severityMedium
		}
		auditMetadata[eventKeySeverity] = severity
		auditMetadata[eventKeyTargetKind] = eventTypeAsyncValidation
		if objectID != emptyValue {
			auditMetadata[eventKeyObjectID] = objectID
		}
		// Add custom fields
		mergeMetadata(auditMetadata, fields)
	}

	// Build metrics data (always include for metrics tracking)
	metricsData := make(map[string]any)
	metricsData[eventKeyOperation] = eventTypeAsyncValidation
	metricsData[eventKeyEventType] = eventType
	if objectID != emptyValue {
		metricsData[eventKeyObjectID] = objectID
	}
	if message != emptyValue {
		metricsData["message"] = message
	}
	if severity != emptyValue {
		metricsData[eventKeySeverity] = severity
	}
	// Add custom fields
	mergeMetadata(metricsData, fields)

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Determine status based on event type
	status := eventStatusInProg
	switch eventType {
	case "validation_timeout", "file_read_error", "worker_stuck":
		status = eventStatusError
	case "semaphore_full":
		status = eventStatusWarning
	case "metrics":
		status = eventStatusComplete
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationID, eventTypeAsyncValidation, status).
		WithEventData(eventData).
		WithContext(ctx)

	// Enable channels based on event type
	// Metrics: logging + metrics only (high frequency)
	// Errors/warnings: all channels enabled
	if shouldAudit {
		eventCtx = eventCtx.WithChannels(true, true, true, true) // All channels for errors/warnings
	} else {
		eventCtx = eventCtx.WithChannels(true, false, true, true) // Logging, metrics, operational (no audit for metrics)
	}

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("async_validator_metrics_emit", "emitting async validator metrics")
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
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
