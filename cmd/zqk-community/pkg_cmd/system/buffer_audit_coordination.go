package system

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

// emitBufferedAuditEventViaCoordinator emits buffered audit events via the coordination system
func emitBufferedAuditEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	buffered *BufferedEvent,
	profile string,
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

	// Create coordinator with routers (only audit for buffered events)
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil, // Buffered events don't need metrics router
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

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

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("buffer_audit_event_emitter", "emitting buffer audit event").
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
}
