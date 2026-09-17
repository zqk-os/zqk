package system

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/storage/id_generation"
)

// emitIDQueueEventViaCoordinator emits ID queue manager lifecycle events via coordinator
// This bridges the storage layer's callback pattern with the coordination system
func emitIDQueueEventViaCoordinator(
	ctx context.Context,
	operationType string,
	eventType string,
	status string,
	err error,
) {
	coordinator := coordination.GetCoordinator()
	if coordinator == nil {
		return // Coordinator not available
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationIDIDQueueManager, operationType, status).
		WithEventData(&coordination.EventData{
			LoggingFields: []coordination.LoggingField{
				{Key: eventKeyEventType, Value: eventType},
				{Key: eventKeyStatus, Value: status},
			},
			AuditMetadata: map[string]any{
				eventKeyEventType:     eventTypeSystemConfigChange, // Use valid enum value
				eventKeyOperation:     "ID queue manager: " + eventType,
				eventKeySeverity:      severityLow,
				eventKeyOperationType: operationType,
			},
			MetricsData: map[string]any{
				eventKeyOperationType: operationType,
				eventKeyEventType:     eventType,
				eventKeyStatus:        status,
			},
		}).
		WithChannels(true, true, true, false) // Logging, Audit, Metrics, but not Operational

	// Add error information if present
	if err != nil {
		eventCtx.EventData.LoggingFields = append(eventCtx.EventData.LoggingFields,
			coordination.LoggingField{Key: eventKeyError, Value: err.Error()})
		eventCtx.EventData.AuditMetadata[eventKeyError] = err.Error()
		eventCtx.EventData.MetricsData[eventKeyError] = true
	}

	// Emit via coordinator
	_ = coordinator.Emit(ctx, eventCtx)
}

// init wires up the ID queue manager with coordinator and shutdown coordinator
func init() {
	id_generation.SetIDQueueEventCallback(emitIDQueueEventViaCoordinator)

	// Register ID Generation Queue Manager with shutdown coordinator
	// This ensures it participates in graceful shutdown
	// Note: init() runs before command context exists, so we use system context
	// This is acceptable for package initialization
	coordinator := storage.GetGlobalShutdownCoordinator()
	queueManager := id_generation.GetGlobalQueueManager(pkgctx.NewSystemContext())
	coordinator.RegisterQueue(queueManager)
}
