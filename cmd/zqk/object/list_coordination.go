package object

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
	objectListEventType             = "object_list"
	objectListStatusError           = "error"
	objectListStatusDebug           = "debug"
	objectListLogFieldOperation     = "operation"
	objectListLogFieldOperationID   = "operation_id"
	objectListLogFieldKind          = "kind"
	objectListLogFieldObjectCount   = "object_count"
	objectListLogFieldGroupCount    = "group_count"
	objectListLogFieldError         = "error"
	objectListEmitterName           = "object_list_event_emitter"
	objectListEmitterPurpose        = "emitting object list event"
	objectListOperationPrefix       = "object_list_%s_%d"
	objectListOutputOperationPrefix = "output_%s"
	objectListOutputPrefix          = "object_list_output_%s_%d"
)

// emitObjectListEventViaCoordinator emits object list operation events via coordinator
func emitObjectListEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	_ storage.ObjectStorageProvider,
	operationID string,
	operation string, // "list", "list_all", "list_kind"
	status string, // "complete", "error", "warning", "debug"
	kind string,
	objectCount int,
	groupCount int,
	err error,
	profile string,
	fields map[string]any,
) {
	ctx, coordinator, ok := cli.InitListCoordination(ctx, projectRoot, profile)
	if !ok {
		return
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: objectListLogFieldOperation, Value: operation},
		{Key: objectListLogFieldOperationID, Value: operationID},
	}
	if kind != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: objectListLogFieldKind, Value: kind})
	}
	if objectCount >= 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: objectListLogFieldObjectCount, Value: objectCount})
	}
	if groupCount >= 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: objectListLogFieldGroupCount, Value: groupCount})
	}
	if err != nil {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: objectListLogFieldError, Value: err.Error()})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: k, Value: v})
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil, // List operations don't create audit events
		MetricsData:   nil,
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationID, objectListEventType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, false) // Logging only

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine(objectListEmitterName, objectListEmitterPurpose).
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
}

// emitObjectListErrorViaCoordinator emits error events for object list operations
func emitObjectListErrorViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operation string,
	_ string,
	kind string,
	err error,
	profile string,
) {
	operationID := fmt.Sprintf(objectListOperationPrefix, operation, time.Now().UnixNano())
	emitObjectListEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, operation, objectListStatusError, kind, -1, -1, err, profile, nil)
}

// emitObjectListDebugViaCoordinator emits debug events for object list operations
func emitObjectListDebugViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operation string,
	kind string,
	profile string,
	fields map[string]any,
) {
	operationID := fmt.Sprintf(objectListOperationPrefix, operation, time.Now().UnixNano())
	emitObjectListEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, operation, objectListStatusDebug, kind, -1, -1, nil, profile, fields)
}

// emitObjectListOutputErrorViaCoordinator emits error events for output formatting errors
func emitObjectListOutputErrorViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	format string, // objectFormatJSON, objectFormatYAML
	err error,
	profile string,
) {
	operationID := fmt.Sprintf(objectListOutputPrefix, format, time.Now().UnixNano())
	emitObjectListEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, fmt.Sprintf(objectListOutputOperationPrefix, format), objectListStatusError, "", -1, -1, err, profile, nil)
}
