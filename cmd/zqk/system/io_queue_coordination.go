package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitIOQueueStateChangeEventViaCoordinator emits I/O queue state change events via the coordination system.
func emitIOQueueStateChangeEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	changeType string,
) {
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, systemProfileSystem, true)
	if !ok {
		return
	}

	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = fmt.Sprintf("io_queue_state_%s", changeType)
	auditMetadata[eventKeyOperation] = fmt.Sprintf("I/O queue state change: %s", changeType)
	auditMetadata[eventKeyChangeType] = changeType
	auditMetadata[eventKeySeverity] = severityLow

	loggingFields := []coordination.LoggingField{
		{Key: eventKeyChangeType, Value: changeType},
	}

	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   map[string]any{eventKeyChangeType: changeType},
	}

	operationID := fmt.Sprintf("io_queue_state_%s_%d", changeType, time.Now().UnixNano())

	eventCtx := coordination.NewEventContext(operationID, "io_queue_state_change", changeType).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, true, false)

	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("io_queue_state_change_emit", fmt.Sprintf("emitting I/O queue state change %s", changeType))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
