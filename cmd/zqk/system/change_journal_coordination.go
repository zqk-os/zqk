package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitChangeJournalEventViaCoordinator emits change journal entry creation events via the coordination system
// This provides unified event routing for change tracking operations
func emitChangeJournalEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	changeType string,
	objectRef string,
	kind string,
	objectID string,
	duration time.Duration,
	err error,
) {
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, systemProfileSystem, true)
	if !ok {
		return
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = fmt.Sprintf("change_journal_%s", status)
	auditMetadata[eventKeyOperation] = fmt.Sprintf("Change journal entry created: %s %s", changeType, objectRef)
	auditMetadata[eventKeyTargetKind] = kind
	auditMetadata[eventKeyTargetID] = objectID
	auditMetadata[eventKeyChangeType] = changeType
	auditMetadata[eventKeyObjectRef] = objectRef
	auditMetadata[eventKeyDurationSeconds] = duration.Seconds()

	// Determine severity
	severity := severityLow
	if status == eventStatusError || err != nil {
		severity = severityHigh
	}
	auditMetadata[eventKeySeverity] = severity

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: eventKeyChangeType, Value: changeType},
		{Key: eventKeyObjectRef, Value: objectRef},
		{Key: eventKeyKind, Value: kind},
		{Key: eventKeyObjectID, Value: objectID},
		{Key: eventKeyStatus, Value: status},
	}
	if duration > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: eventKeyDurationSeconds, Value: duration.Seconds()})
	}

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[eventKeyChangeType] = changeType
	metricsData[eventKeyKind] = kind
	metricsData[eventKeyStatus] = status
	if duration > 0 {
		metricsData[eventKeyDurationSeconds] = duration.Seconds()
		metricsData[eventKeyDurationNS] = duration.Nanoseconds()
	}
	if err != nil {
		metricsData[eventKeyError] = err.Error()
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Create event context (enable audit, metrics, and operational for rollback coordination)
	eventCtx := coordination.NewEventContext(operationID, operationType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, true, true) // Audit, metrics, and operational (for rollback coordination)

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("change_journal_event_emit", fmt.Sprintf("emitting change journal event: %s %s", changeType, objectRef))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
