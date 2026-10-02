package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
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
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, profile, true)
	if !ok {
		return
	}

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
	eventCtx := buildEventContext(ctx, operationID, operationType, status, coordEventData, duration, err, true, true, true, false)
	emitAsyncCoordinationEvent(ctx, coordinator, "snapshot_expand_event_emit", fmt.Sprintf("emitting snapshot expand event: %s", operationID), eventCtx)
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
