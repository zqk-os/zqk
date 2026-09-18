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

// OnStart emits start event via coordinator
func (c *CoordinatorOperationCallback) OnStart(operationID string, metadata map[string]any) {
	if c.projectRoot == emptyValue || c.storageProvider == nil {
		return // Best effort - skip if not available
	}

	// Create coordinator
	auditRouter := NewStorageAuditRouter(c.projectRoot, c.storageProvider)
	coordinator := NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil, // Start events don't create metrics
		OperationalRouter: &DefaultOperationalRouter{},
	})

	// Build event data
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

	// Add metadata fields (audit map merge + structured logging keys)
	maps.Copy(eventData.AuditMetadata, metadata)
	for k, v := range metadata {
		eventData.LoggingFields = append(eventData.LoggingFields, LoggingField{Key: k, Value: v})
	}

	eventCtx := NewEventContext(operationID, c.operationType, "start").
		WithEventData(eventData).
		WithContext(c.ctx).
		WithChannels(true, true, false, true) // Audit, logging, no metrics, operational

	// Emit via storage-backed coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("operation_callback_start", fmt.Sprintf("emitting start event for %s", operationID)).
		StartSimple(func() {
			_ = coordinator.Emit(c.ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})

	// Also emit to global coordinator so CLI subscribers can see it
	// Emit synchronously to ensure CLI subscribers receive it immediately
	globalCoordinator := GetCoordinator()
	if globalCoordinator != nil {
		// Only operational channel for global coordinator
		opEventCtx := NewEventContext(operationID, c.operationType, "start").
			WithEventData(eventData).
			WithContext(c.ctx).
			WithChannels(false, false, false, true) // Operational only

		// Emit synchronously for CLI subscribers
		if syncCoordinator, ok := globalCoordinator.(*Coordinator); ok {
			_ = syncCoordinator.EmitOperationalSync(c.ctx, opEventCtx) //nolint:errcheck // Synchronous for CLI
		} else {
			// Fallback to regular Emit if not a Coordinator instance
			_ = globalCoordinator.Emit(c.ctx, opEventCtx) //nolint:errcheck // Best-effort
		}
	}
}

// OnProgress emits progress event via coordinator
func (c *CoordinatorOperationCallback) OnProgress(operationID string, progress int, total int, message string) {
	if c.projectRoot == emptyValue || c.storageProvider == nil {
		return
	}

	auditRouter := NewStorageAuditRouter(c.projectRoot, c.storageProvider)
	coordinator := NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil,
		OperationalRouter: &DefaultOperationalRouter{},
	})

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
		WithChannels(true, true, false, true) // Audit, logging, no metrics, operational

	goroutinelabels.NewGoroutine("operation_callback_progress", fmt.Sprintf("emitting progress event for %s", operationID)).
		StartSimple(func() {
			_ = coordinator.Emit(c.ctx, eventCtx) //nolint:errcheck
		})

	// Also emit to global coordinator so CLI subscribers can see it
	// Emit synchronously to ensure CLI subscribers receive it immediately
	globalCoordinator := GetCoordinator()
	if globalCoordinator != nil {
		opEventCtx := NewEventContext(operationID, c.operationType, "progress").
			WithEventData(eventData).
			WithContext(c.ctx).
			WithChannels(false, false, false, true) // Operational only

		// Emit synchronously for CLI subscribers
		if syncCoordinator, ok := globalCoordinator.(*Coordinator); ok {
			_ = syncCoordinator.EmitOperationalSync(c.ctx, opEventCtx) //nolint:errcheck // Synchronous for CLI
		} else {
			// Fallback to regular Emit if not a Coordinator instance
			_ = globalCoordinator.Emit(c.ctx, opEventCtx) //nolint:errcheck // Best-effort
		}
	}
}

// OnComplete emits completion event via coordinator
func (c *CoordinatorOperationCallback) OnComplete(operationID string, result any, duration time.Duration) {
	if c.projectRoot == emptyValue || c.storageProvider == nil {
		return
	}

	auditRouter := NewStorageAuditRouter(c.projectRoot, c.storageProvider)
	coordinator := NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil,
		OperationalRouter: &DefaultOperationalRouter{},
	})

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
		WithChannels(true, true, false, true) // Audit, logging, no metrics, operational

	goroutinelabels.NewGoroutine("operation_callback_complete", fmt.Sprintf("emitting completion event for %s", operationID)).
		StartSimple(func() {
			_ = coordinator.Emit(c.ctx, eventCtx) //nolint:errcheck
		})

	// Also emit to global coordinator so CLI subscribers can see it
	// Emit synchronously to ensure CLI subscribers receive it immediately
	globalCoordinator := GetCoordinator()
	if globalCoordinator != nil {
		opEventCtx := NewEventContext(operationID, c.operationType, "complete").
			WithEventData(eventData).
			WithContext(c.ctx).
			WithDuration(duration).
			WithChannels(false, false, false, true) // Operational only

		// Emit synchronously for CLI subscribers
		if syncCoordinator, ok := globalCoordinator.(*Coordinator); ok {
			_ = syncCoordinator.EmitOperationalSync(c.ctx, opEventCtx) //nolint:errcheck // Synchronous for CLI
		} else {
			// Fallback to regular Emit if not a Coordinator instance
			_ = globalCoordinator.Emit(c.ctx, opEventCtx) //nolint:errcheck // Best-effort
		}
	}
}

// OnError emits error event via coordinator
func (c *CoordinatorOperationCallback) OnError(operationID string, err error) {
	if c.projectRoot == emptyValue || c.storageProvider == nil {
		return
	}

	auditRouter := NewStorageAuditRouter(c.projectRoot, c.storageProvider)
	coordinator := NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil,
		OperationalRouter: &DefaultOperationalRouter{},
	})

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

	goroutinelabels.NewGoroutine("operation_callback_error", fmt.Sprintf("emitting error event for %s", operationID)).
		StartSimple(func() {
			_ = coordinator.Emit(c.ctx, eventCtx) //nolint:errcheck
		})
}

// OnCancel emits cancellation event via coordinator
func (c *CoordinatorOperationCallback) OnCancel(operationID string, reason string) {
	if c.projectRoot == emptyValue || c.storageProvider == nil {
		return
	}

	auditRouter := NewStorageAuditRouter(c.projectRoot, c.storageProvider)
	coordinator := NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil,
		OperationalRouter: &DefaultOperationalRouter{},
	})

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

	goroutinelabels.NewGoroutine("operation_callback_cancel", fmt.Sprintf("emitting cancellation event for %s", operationID)).
		StartSimple(func() {
			_ = coordinator.Emit(c.ctx, eventCtx) //nolint:errcheck
		})
}
