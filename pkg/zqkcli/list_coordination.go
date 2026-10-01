package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

const (
	internalListEventType             = "internal_list"
	internalListStatusError           = "error"
	internalListStatusWarning         = "warning"
	internalListLogFieldOperation     = "operation"
	internalListLogFieldOperationID   = "operation_id"
	internalListLogFieldKind          = "kind"
	internalListLogFieldObjectCount   = "object_count"
	internalListLogFieldError         = "error"
	internalListEmitterName           = "internal_list_event_emitter"
	internalListEmitterPurpose        = "emitting internal list event"
	internalListOperationPrefix       = "internal_list_%s_%d"
	internalListOutputOperationPrefix = "output_%s"
	internalListOutputPrefix          = "internal_list_output_%s_%d"
)

// emitInternalListEventViaCoordinator emits internal list operation events via coordinator
func emitInternalListEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	_ storage.ObjectStorageProvider,
	operationID string,
	operation string, // "list", "list_all", "list_kind"
	status string, // "complete", "error", "warning"
	kind string,
	objectCount int,
	err error,
	profile string,
) {
	if projectRoot == emptyValue || projectRoot == "." {
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = cli.CreateContextWithLoggingProfile(ctx, profile)

	// Create coordinator with routers
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       nil, // List operations don't create audit events
		MetricsRouter:     nil,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: internalListLogFieldOperation, Value: operation},
		{Key: internalListLogFieldOperationID, Value: operationID},
	}
	if kind != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: internalListLogFieldKind, Value: kind})
	}
	if objectCount >= 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: internalListLogFieldObjectCount, Value: objectCount})
	}
	if err != nil {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: internalListLogFieldError, Value: err.Error()})
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil, // List operations don't create audit events
		MetricsData:   nil,
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationID, internalListEventType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, false) // Logging only

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine(internalListEmitterName, internalListEmitterPurpose).
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
}

// emitInternalListErrorViaCoordinator emits error events for internal list operations
func emitInternalListErrorViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operation string,
	_ string,
	err error,
	profile string,
) {
	operationID := fmt.Sprintf(internalListOperationPrefix, operation, time.Now().UnixNano())
	emitInternalListEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, operation, internalListStatusError, "", -1, err, profile)
}

// emitInternalListOutputErrorViaCoordinator emits error events for output formatting errors
func emitInternalListOutputErrorViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	format string, // "json", "yaml"
	err error,
	profile string,
) {
	operationID := fmt.Sprintf(internalListOutputPrefix, format, time.Now().UnixNano())
	emitInternalListEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, fmt.Sprintf(internalListOutputOperationPrefix, format), internalListStatusError, "", -1, err, profile)
}
