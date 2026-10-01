package asynccheck

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// EmitAsyncRouterEventViaCoordinator emits async router lifecycle events via coordinator.
func EmitAsyncRouterEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider any,
	workerID string,
	eventType string,
	status string,
	workerCount int,
	processedCount int,
	failedCount int,
	duration time.Duration,
) {
	if projectRoot == "" {
		return
	}

	var storageProviderTyped storage.ObjectStorageProvider
	if storageProvider != nil {
		if sp, ok := storageProvider.(storage.ObjectStorageProvider); ok {
			storageProviderTyped = sp
		}
	}

	coordinator := newCoordinatorForProject(projectRoot, storageProviderTyped)

	auditMetadata := map[string]any{
		objects.FieldKeyEventType:       fmt.Sprintf("async_router_%s", eventType),
		objects.FieldKeyOperation:       fmt.Sprintf("Async router %s: %d workers, %d processed, %d failed", eventType, workerCount, processedCount, failedCount),
		"worker_id":                     workerID,
		"worker_count":                  workerCount,
		"processed_count":               processedCount,
		"failed_count":                  failedCount,
		"duration_seconds":              duration.Seconds(),
		"operation_type":                OperationTypeAsyncRouter,
		"source":                        SourceBackgroundWorker,
	}

	severity := SeverityLow
	switch {
	case status == "error" || failedCount > processedCount/2:
		severity = SeverityHigh
	case failedCount > 0:
		severity = SeverityMedium
	}
	auditMetadata[objects.FieldKeySeverity] = severity

	loggingFields := []coordination.LoggingField{
		{Key: objects.FieldKeyEventType, Value: eventType},
		{Key: "worker_id", Value: workerID},
		{Key: "worker_count", Value: workerCount},
		{Key: "processed_count", Value: processedCount},
		{Key: "failed_count", Value: failedCount},
		{Key: "status", Value: status},
	}
	if duration > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "duration_seconds", Value: duration.Seconds()})
	}

	metricsData := map[string]any{
		"worker_id":        workerID,
		"worker_count":     workerCount,
		"processed_count":  processedCount,
		"failed_count":     failedCount,
		"duration_seconds": duration.Seconds(),
		"operation":        OperationTypeAsyncRouter,
		"event_type":       eventType,
		"status":           status,
	}

	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	eventCtx := coordination.NewEventContext(workerID, OperationTypeAsyncRouter, status).
		WithLevel(severity).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, true, false)

	if emitErr := coordinator.Emit(ctx, eventCtx); emitErr != nil {
		// Handled or logged by callers
	}
}

// EmitAsyncValidatorEventViaCoordinator emits async validator events via coordinator.
func EmitAsyncValidatorEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	eventType string,
	objectID string,
	message string,
	fields map[string]any,
	severity string,
	profile string,
) {
	if projectRoot == "" {
		return
	}

	coordinator := newCoordinatorForProject(projectRoot, storageProvider)

	loggingFields := buildValidatorLoggingFields(operationID, eventType, objectID, message, fields)
	auditMetadata := buildValidatorAuditMetadata(eventType, message, objectID, severity, fields)
	metricsData := buildValidatorMetricsData(eventType, objectID, fields)

	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	status := objects.ObjectStatusSuccess
	if severity == SeverityHigh || severity == SeverityCritical {
		status = "error"
	}

	eventCtx := coordination.NewEventContext(operationID, OperationTypeAsyncValidation, status).
		WithLevel(severity).
		WithEventData(eventData).
		WithContext(ctx)

	switch {
	case eventType == "metrics":
		eventCtx = eventCtx.WithChannels(false, false, true, false)
	case severity == SeverityLow:
		eventCtx = eventCtx.WithChannels(true, false, true, false)
	default:
		eventCtx = eventCtx.WithChannels(true, true, true, false)
	}

	if emitErr := coordinator.Emit(ctx, eventCtx); emitErr != nil {
		// Handled or logged by callers
	}
}

func buildValidatorLoggingFields(operationID, eventType, objectID, message string, fields map[string]any) []coordination.LoggingField {
	loggingFields := []coordination.LoggingField{
		{Key: "operation_id", Value: operationID},
		{Key: "operation_type", Value: OperationTypeAsyncValidation},
		{Key: objects.FieldKeyEventType, Value: eventType},
	}
	if objectID != "" {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "object_id", Value: objectID})
	}
	if message != "" {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "message", Value: message})
	}
	for k, v := range fields {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: k, Value: v})
	}
	return loggingFields
}

func buildValidatorAuditMetadata(eventType, message, objectID, severity string, fields map[string]any) map[string]any {
	if eventType == "metrics" || severity == SeverityLow {
		return nil
	}
	auditMetadata := make(map[string]any)
	auditMetadata[objects.FieldKeyEventType] = "system_config_change"
	if message != "" {
		auditMetadata[objects.FieldKeyOperation] = fmt.Sprintf("Async validation %s: %s", eventType, message)
	} else {
		auditMetadata[objects.FieldKeyOperation] = fmt.Sprintf("Async validation %s", eventType)
	}
	if severity == "" {
		severity = SeverityMedium
	}
	auditMetadata[objects.FieldKeySeverity] = severity
	auditMetadata[objects.FieldKeyTargetKind] = OperationTypeAsyncValidation
	if objectID != "" {
		auditMetadata["object_id"] = objectID
	}
	for k, v := range fields {
		auditMetadata[k] = v
	}
	return auditMetadata
}

func buildValidatorMetricsData(eventType, objectID string, fields map[string]any) map[string]any {
	metricsData := map[string]any{
		objects.FieldKeyOperation: OperationTypeAsyncValidation,
		objects.FieldKeyEventType: eventType,
	}
	if objectID != "" {
		metricsData["object_id"] = objectID
	}
	for k, v := range fields {
		metricsData[k] = v
	}
	return metricsData
}
