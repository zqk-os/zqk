package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitBufferedAuditEventViaCoordinator emits buffered audit events via the coordination system
func emitBufferedAuditEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	buffered *BufferedEvent,
	profile string,
) {
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, profile, false)
	if !ok {
		return
	}

	// Build audit metadata from buffered event
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = buffered.EventType
	auditMetadata[eventKeyOperation] = buffered.Operation
	auditMetadata[eventKeySeverity] = buffered.Severity
	auditMetadata[eventKeyTargetKind] = buffered.TargetKind
	if buffered.TargetID != emptyValue {
		auditMetadata[eventKeyTargetID] = buffered.TargetID
	}
	if buffered.TargetPath != emptyValue {
		auditMetadata[objects.FieldKeyTargetPath] = buffered.TargetPath
	}
	if buffered.OriginalValue != emptyValue {
		auditMetadata[objects.FieldKeyOriginalValue] = buffered.OriginalValue
	}
	if buffered.NewValue != emptyValue {
		auditMetadata[objects.FieldKeyNewValue] = buffered.NewValue
	}
	if buffered.RecoveryMethod != emptyValue {
		auditMetadata[objects.FieldKeyRecoveryMethod] = buffered.RecoveryMethod
	}
	mergeMetadata(auditMetadata, buffered.Metadata)
	// Add occurrence information
	if len(buffered.Occurrences) > 1 {
		auditMetadata[objects.FieldKeyOccurrenceCount] = len(buffered.Occurrences)
		var occurrenceTimestamps []string
		for _, t := range buffered.Occurrences {
			occurrenceTimestamps = append(occurrenceTimestamps, t.Format("2006-01-02T15:04:05Z"))
		}
		auditMetadata[objects.FieldKeyOccurrenceTimestamps] = occurrenceTimestamps
	}
	// Add timestamps
	auditMetadata[objects.FieldKeyCreatedAt] = buffered.FirstOccurrence.Format("2006-01-02T15:04:05Z")
	auditMetadata[objects.FieldKeyUpdatedAt] = buffered.LastOccurrence.Format("2006-01-02T15:04:05Z")

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: nil, // Buffered events don't need logging
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Buffered events don't create metrics
	}

	// Create operation ID from buffered event key
	operationID := fmt.Sprintf("buffered_%s_%d", buffered.Key, time.Now().Unix())

	// Create event context (only audit channel enabled)
	eventCtx := coordination.NewEventContext(operationID, buffered.EventType, eventStatusComplete).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, false, false) // Only audit, no logging/metrics/operational

	emitAsyncCoordinationEvent(ctx, coordinator, "buffer_audit_event_emitter", "emitting buffer audit event", eventCtx)
}
