package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitServiceOperationEventViaCoordinator emits service operation events via the coordination system
// This provides unified event routing for service lifecycle operations (start, stop, status)
func emitServiceOperationEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operation string,
	serviceName string,
	status string,
	err error,
	duration time.Duration,
	profile string, // CLI context profile for logging format
) {
	withSystemCoordinator(ctx, projectRoot, storageProvider, profile, false, func(ctx context.Context, coordinator *coordination.Coordinator) {
		// Build audit metadata
		auditMetadata := make(map[string]any)
		auditMetadata[eventKeyEventType] = fmt.Sprintf("service_%s", operation)
		auditMetadata[eventKeyOperation] = fmt.Sprintf("Service %s: %s", operation, serviceName)
		auditMetadata[eventKeyTargetKind] = targetKindService
		auditMetadata[eventKeyTargetID] = serviceName
		auditMetadata[eventKeyServiceName] = serviceName
		auditMetadata[eventKeySeverity] = severityLow
		if status == eventStatusError || err != nil {
			auditMetadata[eventKeySeverity] = severityHigh
		} else if status == eventStatusComplete {
			auditMetadata[eventKeySeverity] = severityMedium
		}

		// Build logging fields
		loggingFields := []coordination.LoggingField{
			{Key: targetKindService, Value: serviceName},
			{Key: eventKeyOperation, Value: operation},
			{Key: eventKeyStatus, Value: status},
		}
		if duration > 0 {
			loggingFields = append(loggingFields, coordination.LoggingField{Key: eventKeyDurationSeconds, Value: duration.Seconds()})
		}

		// Create event data
		eventData := &coordination.EventData{
			LoggingFields: loggingFields,
			AuditMetadata: auditMetadata,
			MetricsData:   nil, // Service operations don't create metrics
		}

		// Create operation ID
		operationID := fmt.Sprintf("service_%s_%s_%d", operation, serviceName, time.Now().Unix())
		eventCtx := buildEventContext(ctx, operationID, fmt.Sprintf("service_%s", operation), status, eventData, duration, err, true, true, false, false)
		emitAsyncCoordinationEvent(ctx, coordinator, "service_event_emitter", "emitting service event", eventCtx)
	})
}
