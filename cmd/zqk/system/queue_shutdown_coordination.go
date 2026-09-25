package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitQueueShutdownEventViaCoordinator bridges queue shutdown events to coordination
// This allows shutdown events to be observed through the coordinator system
func emitQueueShutdownEventViaCoordinator(
	ctx context.Context,
	queueName string,
	eventType string,
	pendingCount int64,
	isCritical bool,
	duration time.Duration,
	err error,
) {
	coordinator := coordination.GetCoordinator()
	if coordinator == nil {
		return // Coordinator not available
	}

	// Determine status based on event type and error
	status := eventStatusComplete
	if err != nil {
		status = eventStatusError
	} else if eventType == "drain_start" {
		status = eventStatusInProg
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationIDQueueShutdown, eventType, status)
	if err == nil {
		eventCtx.WithLevel("debug")
	}
	eventCtx.
		WithEventData(&coordination.EventData{
			LoggingFields: []coordination.LoggingField{
				{Key: eventKeyQueueName, Value: queueName},
				{Key: eventKeyEventType, Value: eventType},
				{Key: eventKeyPendingCount, Value: pendingCount},
				{Key: eventKeyIsCritical, Value: isCritical},
				{Key: eventKeyDurationMS, Value: duration.Milliseconds()},
			},
			AuditMetadata: map[string]any{
				eventKeyEventType:       eventTypeSystemConfigChange,
				eventKeyOperation:       fmt.Sprintf("Queue shutdown: %s - %s", queueName, eventType),
				eventKeySeverity:        severityLow,
				eventKeyQueueName:       queueName,
				eventKeyShutdownEvent:   eventType,
				eventKeyPendingCount:    pendingCount,
				eventKeyIsCritical:      isCritical,
				eventKeyDurationSeconds: duration.Seconds(),
			},
			MetricsData: map[string]any{
				eventKeyQueueName:    queueName,
				eventKeyEventType:    eventType,
				eventKeyPendingCount: pendingCount,
				eventKeyIsCritical:   isCritical,
				eventKeyDurationMS:   duration.Milliseconds(),
				eventKeyHasError:     err != nil,
			},
		}).
		WithChannels(true, true, true, true) // Logging, Audit, Metrics, Operational

	// Add error information if present
	if err != nil {
		eventCtx.EventData.LoggingFields = append(eventCtx.EventData.LoggingFields,
			coordination.LoggingField{Key: eventKeyError, Value: err.Error()})
		eventCtx.EventData.AuditMetadata[eventKeyError] = err.Error()
		eventCtx.EventData.MetricsData[eventKeyError] = true
	}

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("queue_shutdown_coordinator_event", fmt.Sprintf("emitting shutdown event for %s", queueName))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx)
	})
}

// init wires up queue shutdown events with coordinator
func init() {
	storage.SetQueueShutdownEventCallback(emitQueueShutdownEventViaCoordinator)
}
