package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// emitSchedulerCommandEventViaCoordinator emits scheduler command operation events via coordinator
func emitSchedulerCommandEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	_ storagepkg.ObjectStorageProvider,
	operationID string,
	operation string, // "start", "stop", "trigger", "status"
	status string, // "complete", "error", "warning", "info"
	err error,
	profile string,
	fields map[string]any,
) {
	if projectRoot == emptyValue || projectRoot == "." {
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)
	coordinator := newSchedulerCoordinator()

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "operation", Value: operation},
		{Key: "operation_id", Value: operationID},
	}
	if err != nil {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "error", Value: err.Error()})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: k, Value: v})
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil, // Command operations don't create audit events
		MetricsData:   nil,
	}

	// Create event context
	eventCtx := coordination.NewEventContext(operationID, "scheduler_command", status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, false) // Logging only

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	emitCoordinatorEventAsync(ctx, coordinator, eventCtx)
}

// emitSchedulerStartEventViaCoordinator emits start command events
func emitSchedulerStartEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storagepkg.ObjectStorageProvider,
	message string,
	profile string,
	fields map[string]any,
) {
	operationID := fmt.Sprintf("scheduler_start_%d", time.Now().UnixNano())
	status := "info"
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["message"] = message
	emitSchedulerCommandEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, "start", status, nil, profile, fields)
}

// emitSchedulerStopEventViaCoordinator emits stop command events.
// extraFields is optional; when set, config_at_shutdown and other metrics can be included for shutdown-time visibility.
func emitSchedulerStopEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storagepkg.ObjectStorageProvider,
	message string,
	pid int,
	profile string,
	extraFields map[string]any,
) {
	operationID := fmt.Sprintf("scheduler_stop_%d", time.Now().UnixNano())
	fields := map[string]any{
		"message": message,
		"pid":     pid,
	}
	for k, v := range extraFields {
		fields[k] = v
	}
	emitSchedulerCommandEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, "stop", "info", nil, profile, fields)
}

// emitSchedulerTriggerEventViaCoordinator emits trigger command events
func emitSchedulerTriggerEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storagepkg.ObjectStorageProvider,
	message string,
	jobID string,
	profile string,
	fields map[string]any,
) {
	operationID := fmt.Sprintf("scheduler_trigger_%d", time.Now().UnixNano())
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["message"] = message
	fields["job_id"] = jobID
	emitSchedulerCommandEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, "trigger", "info", nil, profile, fields)
}

// emitSchedulerWarningEventViaCoordinator emits warning events
func emitSchedulerWarningEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storagepkg.ObjectStorageProvider,
	message string,
	err error,
	profile string,
) {
	operationID := fmt.Sprintf("scheduler_warning_%d", time.Now().UnixNano())
	fields := map[string]any{
		"message": message,
	}
	emitSchedulerCommandEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, "warning", "warning", err, profile, fields)
}

// emitSchedulerDumpEventViaCoordinator emits process-dump (SIGUSR1) events via the async coordinator.
// status is "started", "complete", or "error". fields typically includes "dir" (diagnostics directory).
func emitSchedulerDumpEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storagepkg.ObjectStorageProvider,
	status string,
	err error,
	profile string,
	fields map[string]any,
) {
	if projectRoot == emptyValue || projectRoot == "." {
		return
	}
	if fields == nil {
		fields = make(map[string]any)
	}
	fields[objects.FieldKeyStatus] = status // so diagnostics.jsonl shows status=started | complete | error
	operationID := fmt.Sprintf("scheduler_dump_%d", time.Now().UnixNano())
	emitSchedulerCommandEventViaCoordinator(ctx, projectRoot, storageProvider, operationID, "dump", status, err, profile, fields)
}
