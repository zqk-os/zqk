package system

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/storage"
)

// emitSnapshotExpandEventViaCoordinator emits snapshot expansion events via the coordination system
// This replaces the separate emitSnapshotExpandEvent and emitSnapshotExpandMetrics functions
func emitSnapshotExpandEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	eventData snapshotExpandEventData,
	err error,
	duration time.Duration,
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

	// Convert snapshotExpandEventData to coordination.EventData
	coordEventData := &coordination.EventData{
		LoggingFields: convertLoggingFields(eventData.LoggingFields),
		AuditMetadata: eventData.AuditMetadata,
		MetricsData:   eventData.MetricsData,
	}

	// Build audit metadata with event type and operation
	if coordEventData.AuditMetadata == nil {
		coordEventData.AuditMetadata = make(map[string]any)
	}

	// Set event type and operation based on status
	eventType := fmt.Sprintf("snapshot_expand_%s", status)
	switch status {
	case eventStatusError:
		eventType = "snapshot_expand_error"
	case eventStatusComplete:
		eventType = "snapshot_expand_complete"
	}

	operation := fmt.Sprintf("Snapshot expansion: %s", status)
	if coordEventData.AuditMetadata[eventKeyOperation] == nil {
		coordEventData.AuditMetadata[eventKeyOperation] = operation
	}
	if coordEventData.AuditMetadata[eventKeyEventType] == nil {
		coordEventData.AuditMetadata[eventKeyEventType] = eventType
	}

	// Determine severity
	severity := severityLow
	switch status {
	case eventStatusError:
		severity = severityHigh
	case eventStatusComplete:
		severity = severityMedium
	}
	if coordEventData.AuditMetadata[eventKeySeverity] == nil {
		coordEventData.AuditMetadata[eventKeySeverity] = severity
	}

	// Set target_kind if not set
	if coordEventData.AuditMetadata[eventKeyTargetKind] == nil {
		coordEventData.AuditMetadata[eventKeyTargetKind] = "snapshot"
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationID, operationType, status).
		WithEventData(coordEventData).
		WithContext(ctx)

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("snapshot_expand_event_emit", fmt.Sprintf("emitting snapshot expand event: %s", operationID))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}

// convertLoggingFields converts logging.Field to coordination.LoggingField
func convertLoggingFields(fields []logging.Field) []coordination.LoggingField {
	result := make([]coordination.LoggingField, 0, len(fields))
	for _, field := range fields {
		result = append(result, coordination.LoggingField{
			Key:   field.Key,
			Value: field.Value,
		})
	}
	return result
}
