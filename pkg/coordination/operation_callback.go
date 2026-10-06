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

type callbackEventConfig struct {
	label         string
	eventType     string
	action        string
	severity      string
	loggingFields []LoggingField
	auditMetadata map[string]any
	emitGlobal    bool
	duration      time.Duration
	err           error
}

func (c *CoordinatorOperationCallback) emitCallbackEvent(operationID string, cfg callbackEventConfig) {
	loggingFields := make([]LoggingField, 0, 1+len(cfg.loggingFields))
	loggingFields = append(loggingFields, LoggingField{Key: logFieldOperationID, Value: operationID})
	loggingFields = append(loggingFields, cfg.loggingFields...)

	auditMetadata := map[string]any{
		objects.FieldKeyEventType:   operationCallbackAuditEventTypeSystemConfigChange,
		objects.FieldKeyOperation:   fmt.Sprintf("%s %s", c.operationType, cfg.action),
		objects.FieldKeyOperationID: operationID,
		objects.FieldKeySeverity:    cfg.severity,
	}
	for k, v := range cfg.auditMetadata {
		auditMetadata[k] = v
	}

	eventData := &EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   nil,
	}

	eventCtx := NewEventContext(operationID, c.operationType, cfg.eventType).
		WithEventData(eventData).
		WithContext(c.ctx).
		WithChannels(true, true, false, cfg.emitGlobal)

	if cfg.duration > 0 {
		eventCtx = eventCtx.WithDuration(cfg.duration)
	}
	if cfg.err != nil {
		eventCtx = eventCtx.WithError(cfg.err)
	}

	c.dispatchEvent(callbackEmitSpec{
		label:       fmt.Sprintf("operation_callback_%s", cfg.label),
		operationID: operationID,
		eventType:   cfg.eventType,
		eventCtx:    eventCtx,
		emitGlobal:  cfg.emitGlobal,
		duration:    cfg.duration,
	})
}

// OnStart emits start event via coordinator
func (c *CoordinatorOperationCallback) OnStart(operationID string, metadata map[string]any) {
	loggingFields := []LoggingField{
		{Key: logFieldOperationType, Value: c.operationType},
		{Key: logFieldStatus, Value: statusStartedValue},
	}
	for k, v := range metadata {
		loggingFields = append(loggingFields, LoggingField{Key: k, Value: v})
	}

	auditMeta := map[string]any{
		"operation_type": c.operationType,
	}
	maps.Copy(auditMeta, metadata)

	c.emitCallbackEvent(operationID, callbackEventConfig{
		label:         "start",
		eventType:     "start",
		action:        "started",
		severity:      "low",
		loggingFields: loggingFields,
		auditMetadata: auditMeta,
		emitGlobal:    true,
	})
}

// OnProgress emits progress event via coordinator
func (c *CoordinatorOperationCallback) OnProgress(operationID string, progress int, total int, message string) {
	percent := 0.0
	if total > 0 {
		percent = float64(progress) / float64(total) * 100.0
	}

	c.emitCallbackEvent(operationID, callbackEventConfig{
		label:     "progress",
		eventType: "progress",
		action:    "progress",
		severity:  "low",
		loggingFields: []LoggingField{
			{Key: "progress", Value: progress},
			{Key: "total", Value: total},
			{Key: "percent", Value: percent},
			{Key: "message", Value: message},
		},
		auditMetadata: map[string]any{
			"progress": progress,
			"total":    total,
			"percent":  percent,
		},
		emitGlobal: true,
	})
}

// OnComplete emits completion event via coordinator
func (c *CoordinatorOperationCallback) OnComplete(operationID string, result any, duration time.Duration) {
	c.emitCallbackEvent(operationID, callbackEventConfig{
		label:     "complete",
		eventType: "complete",
		action:    "completed",
		severity:  "low",
		loggingFields: []LoggingField{
			{Key: logFieldStatus, Value: statusCompletedValue},
			{Key: objects.FieldKeyDurationSeconds, Value: duration.Seconds()},
		},
		auditMetadata: map[string]any{
			objects.FieldKeyDurationSeconds: duration.Seconds(),
		},
		emitGlobal: true,
		duration:   duration,
	})
}

// OnError emits error event via coordinator
func (c *CoordinatorOperationCallback) OnError(operationID string, err error) {
	c.emitCallbackEvent(operationID, callbackEventConfig{
		label:     "error",
		eventType: "error",
		action:    "failed",
		severity:  "high",
		loggingFields: []LoggingField{
			{Key: logFieldStatus, Value: statusErrorValue},
			{Key: logFieldError, Value: err.Error()},
		},
		auditMetadata: map[string]any{
			"error": err.Error(),
		},
		emitGlobal: false,
		err:        err,
	})
}

// OnCancel emits cancellation event via coordinator
func (c *CoordinatorOperationCallback) OnCancel(operationID string, reason string) {
	c.emitCallbackEvent(operationID, callbackEventConfig{
		label:     "cancel",
		eventType: "cancelled",
		action:    "cancelled",
		severity:  "medium",
		loggingFields: []LoggingField{
			{Key: logFieldStatus, Value: statusCancelledValue},
			{Key: "reason", Value: reason},
		},
		auditMetadata: map[string]any{
			objects.FieldKeyReason: reason,
		},
		emitGlobal: false,
	})
}
