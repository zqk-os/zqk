package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitAuditBufferFlushEventViaCoordinator emits audit buffer flush events via the coordination system
// This provides unified event routing for audit buffer flush operations
func emitAuditBufferFlushEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	groupKey string,
	eventType string,
	targetKind string,
	eventCount int,
	aggregationWindow string,
	duration time.Duration,
	err error,
) {
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, systemProfileSystem, true)
	if !ok {
		return
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = fmt.Sprintf("audit_buffer_flush_%s", status)
	auditMetadata[eventKeyOperation] = fmt.Sprintf("Audit buffer flush: %d %s events for %s", eventCount, eventType, targetKind)
	auditMetadata[eventKeyTargetKind] = targetKind
	auditMetadata["group_key"] = groupKey
	auditMetadata["event_type_aggregated"] = eventType
	auditMetadata[objects.FieldKeyEventCount] = eventCount
	auditMetadata[objects.FieldKeyAggregationWindow] = aggregationWindow
	auditMetadata[eventKeyDurationSeconds] = duration.Seconds()

	// Determine severity
	severity := severityLow
	if status == eventStatusError || err != nil {
		severity = severityHigh
	}
	auditMetadata[eventKeySeverity] = severity

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "group_key", Value: groupKey},
		{Key: eventKeyEventType, Value: eventType},
		{Key: eventKeyTargetKind, Value: targetKind},
		{Key: "event_count", Value: eventCount},
		{Key: eventKeyStatus, Value: status},
	}
	if duration > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: eventKeyDurationSeconds, Value: duration.Seconds()})
	}

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData["group_key"] = groupKey
	metricsData[eventKeyEventType] = eventType
	metricsData[eventKeyTargetKind] = targetKind
	metricsData[objects.FieldKeyEventCount] = eventCount
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

	// Create event context (enable audit and metrics, minimal logging)
	eventCtx := coordination.NewEventContext(operationID, operationType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, true, false) // Audit and metrics, no logging/operational

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("audit_buffer_event_emit", fmt.Sprintf("emitting audit buffer flush event: %s", groupKey))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
