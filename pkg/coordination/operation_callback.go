package coordination

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

const (
	logFieldOperationID   = "operation_id"
	logFieldOperationType = "operation_type"
	logFieldStatus        = "status"
	logFieldError         = "error"

	statusStartedValue   = "started"
	statusCompletedValue = "completed"
	statusErrorValue     = "error"
	statusCancelledValue = "cancelled"

	operationCallbackAuditEventTypeSystemConfigChange = "system_config_change"
)

// CoordinatorOperationCallback implements OperationCallback using coordinator
// This provides standard callback/notify pattern for all operations
// Located in pkg/coordination to avoid import cycles (can import both concurrency and storage)
type CoordinatorOperationCallback struct {
	ctx             context.Context
	projectRoot     string
	storageProvider storage.ObjectStorageProvider
	profile         string
	operationType   string
}

// NewCoordinatorOperationCallback creates a new coordinator-based operation callback
func NewCoordinatorOperationCallback(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationType string,
	profile string,
) concurrency.OperationCallback {
	return &CoordinatorOperationCallback{
		ctx:             ctx,
		projectRoot:     projectRoot,
		storageProvider: storageProvider,
		profile:         profile,
		operationType:   operationType,
	}
}

func (c *CoordinatorOperationCallback) getStorageCoordinator() *Coordinator {
	if c.projectRoot == emptyValue || c.storageProvider == nil {
		return nil
	}
	auditRouter := NewStorageAuditRouter(c.projectRoot, c.storageProvider)
	return NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil,
		OperationalRouter: &DefaultOperationalRouter{},
	})
}

type callbackEmitSpec struct {
	label       string
	operationID string
	eventType   string
	eventCtx    *EventContext
	emitGlobal  bool
	duration    time.Duration
}

func (c *CoordinatorOperationCallback) dispatchEvent(spec callbackEmitSpec) {
	coordinator := c.getStorageCoordinator()
	if coordinator == nil {
		return
	}

	goroutinelabels.NewGoroutine(spec.label, fmt.Sprintf("emitting %s event for %s", spec.eventType, spec.operationID)).
		StartSimple(func() {
			if emitErr := coordinator.Emit(c.ctx, spec.eventCtx); emitErr != nil {
				// best effort async emission
			}
		})

	if !spec.emitGlobal {
		return
	}

	globalCoordinator := GetCoordinator()
	if globalCoordinator == nil {
		return
	}

	opEventCtx := NewEventContext(spec.operationID, c.operationType, spec.eventType).
		WithEventData(spec.eventCtx.EventData).
		WithContext(c.ctx).
		WithChannels(false, false, false, true)

	if spec.duration > 0 {
		opEventCtx = opEventCtx.WithDuration(spec.duration)
	}

	if syncCoordinator, ok := globalCoordinator.(*Coordinator); ok {
		if syncErr := syncCoordinator.EmitOperationalSync(c.ctx, opEventCtx); syncErr != nil {
			// best effort sync emission
		}
	} else {
		if globalErr := globalCoordinator.Emit(c.ctx, opEventCtx); globalErr != nil {
			// best effort global emission
		}
	}
}

// OnStart emits start event via coordinator
func (c *CoordinatorOperationCallback) OnStart(operationID string, metadata map[string]any) {
	eventData := &EventData{
		LoggingFields: []LoggingField{
			{Key: logFieldOperationID, Value: operationID},
			{Key: logFieldOperationType, Value: c.operationType},
			{Key: logFieldStatus, Value: statusStartedValue},
		},
		AuditMetadata: map[string]any{
			objects.FieldKeyEventType:   operationCallbackAuditEventTypeSystemConfigChange,
			objects.FieldKeyOperation:   fmt.Sprintf("%s started", c.operationType),
			objects.FieldKeyOperationID: operationID,
			"operation_type":            c.operationType,
			objects.FieldKeySeverity:    "low",
		},
		MetricsData: nil,
	}

	maps.Copy(eventData.AuditMetadata, metadata)
	for k, v := range metadata {
		eventData.LoggingFields = append(eventData.LoggingFields, LoggingField{Key: k, Value: v})
	}

	eventCtx := NewEventContext(operationID, c.operationType, "start").
		WithEventData(eventData).
		WithContext(c.ctx).
		WithChannels(true, true, false, true)

	c.dispatchEvent(callbackEmitSpec{
		label:       "operation_callback_start",
		operationID: operationID,
		eventType:   "start",
		eventCtx:    eventCtx,
		emitGlobal:  true,
	})
}

// OnProgress emits progress event via coordinator
func (c *CoordinatorOperationCallback) OnProgress(operationID string, progress int, total int, message string) {
	percent := 0.0
	if total > 0 {
		percent = float64(progress) / float64(total) * 100.0
	}

	eventData := &EventData{
		LoggingFields: []LoggingField{
			{Key: "operation_id", Value: operationID},
			{Key: "progress", Value: progress},
			{Key: "total", Value: total},
			{Key: "percent", Value: percent},
			{Key: "message", Value: message},
		},
		AuditMetadata: map[string]any{
			objects.FieldKeyEventType:   "system_config_change",
			objects.FieldKeyOperation:   fmt.Sprintf("%s progress", c.operationType),
			objects.FieldKeyOperationID: operationID,
			"progress":                  progress,
			"total":                     total,
			"percent":                   percent,
			objects.FieldKeySeverity:    "low",
		},
		MetricsData: nil,
	}

	eventCtx := NewEventContext(operationID, c.operationType, "progress").
		WithEventData(eventData).
		WithContext(c.ctx).
		WithChannels(true, true, false, true)

	c.dispatchEvent(callbackEmitSpec{
		label:       "operation_callback_progress",
		operationID: operationID,
		eventType:   "progress",
		eventCtx:    eventCtx,
		emitGlobal:  true,
	})
}

// OnComplete emits completion event via coordinator
func (c *CoordinatorOperationCallback) OnComplete(operationID string, result any, duration time.Duration) {
	eventData := &EventData{
		LoggingFields: []LoggingField{
			{Key: logFieldOperationID, Value: operationID},
			{Key: logFieldStatus, Value: statusCompletedValue},
			{Key: objects.FieldKeyDurationSeconds, Value: duration.Seconds()},
		},
		AuditMetadata: map[string]any{
			objects.FieldKeyEventType:       operationCallbackAuditEventTypeSystemConfigChange,
			objects.FieldKeyOperation:       fmt.Sprintf("%s completed", c.operationType),
			objects.FieldKeyOperationID:     operationID,
			objects.FieldKeyDurationSeconds: duration.Seconds(),
			objects.FieldKeySeverity:        "low",
		},
		MetricsData: nil,
	}

	eventCtx := NewEventContext(operationID, c.operationType, "complete").
		WithEventData(eventData).
		WithContext(c.ctx).
		WithDuration(duration).
		WithChannels(true, true, false, true)

	c.dispatchEvent(callbackEmitSpec{
		label:       "operation_callback_complete",
		operationID: operationID,
		eventType:   "complete",
		eventCtx:    eventCtx,
		emitGlobal:  true,
		duration:    duration,
	})
}

// OnError emits error event via coordinator
func (c *CoordinatorOperationCallback) OnError(operationID string, err error) {
	eventData := &EventData{
		LoggingFields: []LoggingField{
			{Key: logFieldOperationID, Value: operationID},
			{Key: logFieldStatus, Value: statusErrorValue},
			{Key: logFieldError, Value: err.Error()},
		},
		AuditMetadata: map[string]any{
			objects.FieldKeyEventType:   operationCallbackAuditEventTypeSystemConfigChange,
			objects.FieldKeyOperation:   fmt.Sprintf("%s failed", c.operationType),
			objects.FieldKeyOperationID: operationID,
			"error":                     err.Error(),
			objects.FieldKeySeverity:    "high",
		},
		MetricsData: nil,
	}

	eventCtx := NewEventContext(operationID, c.operationType, "error").
		WithEventData(eventData).
		WithContext(c.ctx).
		WithError(err).
		WithChannels(true, true, false, false)

	c.dispatchEvent(callbackEmitSpec{
		label:       "operation_callback_error",
		operationID: operationID,
		eventType:   "error",
		eventCtx:    eventCtx,
		emitGlobal:  false,
	})
}

// OnCancel emits cancellation event via coordinator
func (c *CoordinatorOperationCallback) OnCancel(operationID string, reason string) {
	eventData := &EventData{
		LoggingFields: []LoggingField{
			{Key: logFieldOperationID, Value: operationID},
			{Key: logFieldStatus, Value: statusCancelledValue},
			{Key: "reason", Value: reason},
		},
		AuditMetadata: map[string]any{
			objects.FieldKeyEventType:   operationCallbackAuditEventTypeSystemConfigChange,
			objects.FieldKeyOperation:   fmt.Sprintf("%s cancelled", c.operationType),
			objects.FieldKeyOperationID: operationID,
			objects.FieldKeyReason:      reason,
			objects.FieldKeySeverity:    "medium",
		},
		MetricsData: nil,
	}

	eventCtx := NewEventContext(operationID, c.operationType, "cancelled").
		WithEventData(eventData).
		WithContext(c.ctx).
		WithChannels(true, true, false, false)

	c.dispatchEvent(callbackEmitSpec{
		label:       "operation_callback_cancel",
		operationID: operationID,
		eventType:   "cancelled",
		eventCtx:    eventCtx,
		emitGlobal:  false,
	})
}
