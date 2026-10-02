package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
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
	var ok bool
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, profile, false)
	if !ok {
		return
	}

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

	// Create event context (enable audit and logging channels)
	eventCtx := coordination.NewEventContext(operationID, fmt.Sprintf("service_%s", operation), status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, false, false) // Audit and logging, no metrics/operational

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	if duration > 0 {
		eventCtx = eventCtx.WithDuration(duration)
	}

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("service_event_emitter", "emitting service event")
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
