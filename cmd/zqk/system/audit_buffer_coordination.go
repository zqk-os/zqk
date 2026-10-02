package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
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
	// Build audit metadata
	auditMetadata := map[string]any{
		eventKeyEventType:                 fmt.Sprintf("audit_buffer_flush_%s", status),
		eventKeyOperation:                 fmt.Sprintf("Audit buffer flush: %d %s events for %s", eventCount, eventType, targetKind),
		eventKeyTargetKind:                targetKind,
		"group_key":                       groupKey,
		"event_type_aggregated":           eventType,
		objects.FieldKeyEventCount:        eventCount,
		objects.FieldKeyAggregationWindow: aggregationWindow,
		eventKeyDurationSeconds:           duration.Seconds(),
		eventKeySeverity:                  severityForStatusOrError(status, err),
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "group_key", Value: groupKey},
		{Key: eventKeyEventType, Value: eventType},
		{Key: eventKeyTargetKind, Value: targetKind},
		{Key: "event_count", Value: eventCount},
		{Key: eventKeyStatus, Value: status},
	}

	// Build metrics data
	metricsData := map[string]any{
		"group_key":                groupKey,
		eventKeyEventType:          eventType,
		eventKeyTargetKind:         targetKind,
		objects.FieldKeyEventCount: eventCount,
		eventKeyStatus:             status,
	}

	emitSystemEventViaCoordinator(
		ctx,
		projectRoot,
		storageProvider,
		operationID,
		operationType,
		status,
		duration,
		err,
		loggingFields,
		auditMetadata,
		metricsData,
		false,
		"audit_buffer_event_emit",
		fmt.Sprintf("emitting audit buffer flush event: %s", groupKey),
	)
}
