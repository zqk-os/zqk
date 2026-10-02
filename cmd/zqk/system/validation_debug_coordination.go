package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// emitValidationDebugEventViaCoordinator emits debug validation events via coordinator
func emitValidationDebugEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	eventType string, // "validation_input", "validation_result", "validation_error"
	objectID string,
	message string,
	fields map[string]any,
	profile string,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Create coordinator with routers (debug events don't create audit events)
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       nil, // Debug events don't create audit events
		MetricsRouter:     nil,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "operation_id", Value: operationID},
		{Key: "operation_type", Value: "validation_debug"},
		{Key: eventKeyEventType, Value: eventType},
		{Key: "message", Value: message},
	}
	if objectID != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "object_id", Value: objectID})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: k, Value: v})
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil, // Debug events don't create audit events
		MetricsData:   nil,
	}

	eventCtx := buildEventContext(ctx, operationID, "validation_debug", "debug", eventData, 0, nil, true, false, false, false)
	emitAsyncCoordinationEvent(ctx, coordinator, "validation_debug_event_emit", "emitting validation debug event", eventCtx)
}

// emitValidationInputDebugViaCoordinator emits debug events for validation input
func emitValidationInputDebugViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	objectID string,
	objMap map[string]any,
	filePath string,
	profile string,
) {
	operationID := fmt.Sprintf("validation_input_%s_%d", objectID, time.Now().UnixNano())
	fields := map[string]any{
		"objMap_keys":              len(objMap),
		objects.FieldKeyPolicyType: fmt.Sprintf("%v", objMap[objects.FieldKeyPolicyType]),
		objects.FieldKeyCategory:   fmt.Sprintf("%v", objMap[objects.FieldKeyCategory]),
		"body_exists":              objMap[objects.FieldKeyBody] != nil,
		objects.FieldKeyFilePath:   filePath,
	}

	emitValidationDebugEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"validation_input", objectID,
		"checkInstanceValidation objMap check",
		fields, profile,
	)

	// Emit second debug event for POL-DEBUG-001 specific details
	bodyExists := objMap[objects.FieldKeyBody] != nil && objMap[objects.FieldKeyBody] != emptyValue
	fields2 := map[string]any{
		objects.FieldKeyPolicyType: fmt.Sprintf("%v", objMap[objects.FieldKeyPolicyType]),
		objects.FieldKeyCategory:   fmt.Sprintf("%v", objMap[objects.FieldKeyCategory]),
		"body_exists":              bodyExists,
		"body_type":                fmt.Sprintf("%T", objMap[objects.FieldKeyBody]),
		objects.FieldKeyFilePath:   filePath,
		"objMap_keys":              len(objMap),
	}

	operationID2 := fmt.Sprintf("validation_input_%s_%d", objectID, time.Now().UnixNano()+1)
	emitValidationDebugEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID2,
		"validation_input", objectID,
		"Validating POL-DEBUG-001",
		fields2, profile,
	)
}

// emitValidationResultDebugViaCoordinator emits debug events for validation results
func emitValidationResultDebugViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	objectID string,
	err error,
	errorsCount int,
	warningsCount int,
	validationErrors []map[string]any,
	profile string,
) {
	operationID := fmt.Sprintf("validation_result_%s_%d", objectID, time.Now().UnixNano())

	if err != nil {
		fields := map[string]any{
			eventKeyError: err.Error(),
		}
		emitValidationDebugEventViaCoordinator(
			ctx, projectRoot, storageProvider, operationID,
			"validation_error", objectID,
			"Validator returned error",
			fields, profile,
		)
	} else {
		fields := map[string]any{
			"errors_count":   errorsCount,
			"warnings_count": warningsCount,
		}
		emitValidationDebugEventViaCoordinator(
			ctx, projectRoot, storageProvider, operationID,
			"validation_result", objectID,
			"=== POL-DEBUG-001 DEBUG: VALIDATOR RESULT ===",
			fields, profile,
		)

		// Emit individual validation errors
		for i, validationError := range validationErrors {
			errorFields := map[string]any{
				"index":               i,
				objects.FieldKeyField: validationError[objects.FieldKeyField],
				"message":             validationError["message"],
				"validation_rule":     validationError["validation_rule"],
			}
			errorOperationID := fmt.Sprintf("validation_error_%s_%d_%d", objectID, time.Now().UnixNano(), i)
			emitValidationDebugEventViaCoordinator(
				ctx, projectRoot, storageProvider, errorOperationID,
				"validation_error", objectID,
				"Validation error",
				errorFields, profile,
			)
		}
	}
}
