package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitAsyncRouterEventViaCoordinator emits async router events via the coordination system
// This provides unified event routing for async router lifecycle events
func emitAsyncRouterEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider any,
	workerID string,
	eventType string, // "worker_start", "worker_shutdown", "worker_idle_shutdown"
	status string,
	workerCount int,
	processedCount int,
	failedCount int,
	duration time.Duration,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, systemProfileSystem)

	// Type assert storage provider
	var storageProviderTyped storage.ObjectStorageProvider
	if storageProvider != nil {
		if sp, ok := storageProvider.(storage.ObjectStorageProvider); ok {
			storageProviderTyped = sp
		}
	}

	// Create routers for coordinator
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProviderTyped)

	// Create metrics pipeline for metrics router (if storage provider available)
	var metricsRouter coordination.MetricsRouter
	if storageProviderTyped != nil {
		metricsPipeline := metrics.MetricPipelineForProject(storageProviderTyped, projectRoot)
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
	auditMetadata[eventKeyEventType] = fmt.Sprintf("async_router_%s", eventType)
	auditMetadata[eventKeyOperation] = fmt.Sprintf("Async router %s: %d workers, %d processed, %d failed", eventType, workerCount, processedCount, failedCount)
	auditMetadata[eventKeyWorkerID] = workerID
	auditMetadata[eventKeyWorkerCount] = workerCount
	auditMetadata[eventKeyProcessedCount] = processedCount
	auditMetadata[eventKeyFailedCount] = failedCount
	auditMetadata[eventKeyDurationSeconds] = duration.Seconds()
	auditMetadata[eventKeyOperationType] = operationTypeAsyncRouter
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
		{Key: eventKeyEventType, Value: eventType},
		{Key: eventKeyWorkerID, Value: workerID},
		{Key: eventKeyWorkerCount, Value: workerCount},
		{Key: eventKeyProcessedCount, Value: processedCount},
		{Key: eventKeyFailedCount, Value: failedCount},
		{Key: eventKeyStatus, Value: status},
	}
	if duration > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: eventKeyDurationSeconds, Value: duration.Seconds()})
	}

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[eventKeyEventType] = eventType
	metricsData[eventKeyWorkerID] = workerID
	metricsData[eventKeyWorkerCount] = workerCount
	metricsData[eventKeyProcessedCount] = processedCount
	metricsData[eventKeyFailedCount] = failedCount
	metricsData[eventKeyStatus] = status
	if duration > 0 {
		metricsData[eventKeyDurationSeconds] = duration.Seconds()
		metricsData[eventKeyDurationNS] = duration.Nanoseconds()
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Create event context (enable audit, metrics, and logging; operational for lifecycle events)
	emitOperational := eventType == "worker_start" || eventType == "worker_shutdown" || eventType == "worker_idle_shutdown"
	eventCtx := coordination.NewEventContext(workerID, eventTypeAsyncRouter, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, true, emitOperational) // Logging, audit, metrics; operational for lifecycle

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("async_router_event_emit", fmt.Sprintf("emitting async router event: %s", eventType))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}

func init() {
	// Register async router event callback for coordinator integration
	// This allows the async router to emit events via coordinator without import cycles
	transceiver.SetAsyncRouterEventCallback(emitAsyncRouterEventViaCoordinator)
}
