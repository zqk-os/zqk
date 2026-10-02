package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func initDiscoveryContext(ctx context.Context, projectRoot, profile string) (context.Context, string, bool) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return ctx, emptyValue, false
	}
	return createContextWithLoggingProfile(ctx, profile), projectRoot, true
}

func emitStorageEventAsync(ctx context.Context, projectRoot string, storageProvider storage.ObjectStorageProvider, taskName, taskDesc string, eventCtx *coordination.EventContext) {
	if storageProvider == nil {
		return
	}
	coordinator := coordination.NewStorageCoordinator(projectRoot, storageProvider)
	goroutinelabels.NewGoroutine(taskName, taskDesc).
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
}

func emitOperationalSyncGlobal(ctx context.Context, operationID, status string, eventData *coordination.EventData) {
	globalCoordinator := coordination.GetCoordinator()
	if globalCoordinator == nil {
		return
	}
	opEventCtx := coordination.NewEventContext(operationID, eventTypeSystemCheck, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, false, false, true)
	if syncCoordinator, ok := globalCoordinator.(*coordination.Coordinator); ok {
		_ = syncCoordinator.EmitOperationalSync(ctx, opEventCtx) //nolint:errcheck // Best-effort, synchronous
	} else {
		_ = globalCoordinator.Emit(ctx, opEventCtx) //nolint:errcheck // Best-effort
	}
}

func appendDiscoveryScope(targetKind string, kinds []string, auditMetadata map[string]any, loggingFields []coordination.LoggingField) []coordination.LoggingField {
	if targetKind != emptyValue {
		auditMetadata["target_kind_specific"] = targetKind
		return append(loggingFields, coordination.LoggingField{Key: eventKeyTargetKind, Value: targetKind})
	}
	auditMetadata["kinds"] = kinds
	return append(loggingFields, coordination.LoggingField{Key: "kinds", Value: kinds})
}

func formatDiscoveryOpDesc(action string, targetKind string, kinds []string) string {
	if targetKind != emptyValue {
		return fmt.Sprintf("Object discovery %s for kind: %s", action, targetKind)
	}
	return fmt.Sprintf("Object discovery %s for %d kinds", action, len(kinds))
}

// emitDiscoveryCancellationEventViaCoordinator emits discovery cancellation events via the coordination system
// This tracks when object discovery operations are cancelled due to context cancellation
func emitDiscoveryCancellationEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	kind string,
	cancellationPoint string, // "before_semaphore" or "before_send_results"
	filesCount int,
	err error,
	profile string, // CLI context profile for logging format
) {
	ctx, projectRoot, ok := initDiscoveryContext(ctx, projectRoot, profile)
	if !ok {
		return
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = eventTypeSystemConfigChange
	auditMetadata[eventKeyOperation] = fmt.Sprintf("Object discovery cancelled for kind %s", kind)
	auditMetadata[eventKeyTargetKind] = kind
	auditMetadata["cancellation_point"] = cancellationPoint
	auditMetadata[eventKeySeverity] = severityMedium // Cancellation is notable but not critical
	if filesCount > 0 {
		auditMetadata["files_count"] = filesCount
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "kind", Value: kind},
		{Key: "cancellation_point", Value: cancellationPoint},
		{Key: eventKeyStatus, Value: "cancelled"},
	}
	if filesCount > 0 {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "files_count", Value: filesCount})
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Discovery cancellation events don't create metrics
	}

	// Create operation ID
	operationID := fmt.Sprintf("discovery_cancelled_%s_%d", kind, time.Now().Unix())

	// Create event context (enable audit and logging channels)
	eventCtx := coordination.NewEventContext(operationID, "discovery_cancellation", "cancelled").
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, false, false) // Audit and logging, no metrics/operational

	if err != nil {
		eventCtx = eventCtx.WithError(err)
	}

	emitStorageEventAsync(ctx, projectRoot, storageProvider, "discovery_cancellation_event_emit", fmt.Sprintf("emitting discovery cancellation event for kind %s", kind), eventCtx)
}

// emitDiscoveryStartEventViaCoordinator emits discovery start events via the coordination system
// This tracks when object discovery operations begin. Uses system_check operationID for multi-agent coordination.
func emitDiscoveryStartEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string, // system_check operation ID (shared across discovery + validation)
	kinds []string,
	targetKind string,
	processDir string,
	profile string, // CLI context profile for logging format
) {
	ctx, projectRoot, ok := initDiscoveryContext(ctx, projectRoot, profile)
	if !ok {
		return
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = eventTypeSystemConfigChange
	auditMetadata[eventKeyOperation] = formatDiscoveryOpDesc("started", targetKind, kinds)
	auditMetadata[eventKeyTargetKind] = "discovery"
	auditMetadata["kinds_count"] = len(kinds)
	auditMetadata["process_dir"] = processDir
	auditMetadata[eventKeySeverity] = severityLow // Discovery start is routine

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "kinds_count", Value: len(kinds)},
		{Key: "process_dir", Value: processDir},
		{Key: eventKeyStatus, Value: "started"},
		{Key: "phase", Value: "discovery"},
	}
	loggingFields = appendDiscoveryScope(targetKind, kinds, auditMetadata, loggingFields)

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Discovery start events don't create metrics
	}

	// Create event context with system_check operation type for unified coordination
	// Use "start" status (not "started") so it maps to "operation.start" for subscribers
	eventCtx := coordination.NewEventContext(operationID, eventTypeSystemCheck, eventStatusStart).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, false, true) // Audit, logging, no metrics, operational (for CLI subscribers)

	// Emit via storage-backed coordinator (for audit/persistence)
	emitStorageEventAsync(ctx, projectRoot, storageProvider, "discovery_start_event_emit_storage", "emitting discovery start event to storage", eventCtx)

	// Also emit via global coordinator so in-process subscribers (e.g., TerminalProgressSubscriber) can see it
	emitOperationalSyncGlobal(ctx, operationID, eventStatusStart, eventData)
}

// emitDiscoveryCompletionEventViaCoordinator emits discovery completion events via the coordination system
// This tracks when object discovery operations complete
func emitDiscoveryCompletionEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	kinds []string,
	targetKind string,
	filesFound int,
	duration time.Duration,
	profile string, // CLI context profile for logging format
) {
	ctx, projectRoot, ok := initDiscoveryContext(ctx, projectRoot, profile)
	if !ok {
		return
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = eventTypeSystemConfigChange
	auditMetadata[eventKeyOperation] = formatDiscoveryOpDesc("completed", targetKind, kinds)
	auditMetadata[eventKeyTargetKind] = "discovery"
	auditMetadata["kinds_count"] = len(kinds)
	auditMetadata["files_found"] = filesFound
	auditMetadata[eventKeyDurationSeconds] = duration.Seconds()
	auditMetadata[eventKeySeverity] = severityLow // Discovery completion is routine

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "kinds_count", Value: len(kinds)},
		{Key: "files_found", Value: filesFound},
		{Key: eventKeyDurationSeconds, Value: duration.Seconds()},
		{Key: eventKeyStatus, Value: "completed"},
	}
	loggingFields = appendDiscoveryScope(targetKind, kinds, auditMetadata, loggingFields)

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Discovery completion events don't create metrics
	}

	// Create operation ID
	operationID := fmt.Sprintf("discovery_completed_%d", time.Now().Unix())

	// Create event context (enable audit and logging channels)
	eventCtx := coordination.NewEventContext(operationID, "discovery_completion", eventStatusComplete).
		WithEventData(eventData).
		WithContext(ctx).
		WithDuration(duration).
		WithChannels(true, true, false, false) // Audit and logging, no metrics/operational

	emitStorageEventAsync(ctx, projectRoot, storageProvider, "discovery_completion_event_emit", "emitting discovery completion event", eventCtx)
}

// emitDiscoveryProgressEventViaCoordinator emits discovery progress events via the coordination system.
// This provides early actionable feedback during discovery (counts + elapsed time).
// Uses system_check operationID for multi-agent coordination.
func emitDiscoveryProgressEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string, // system_check operation ID (shared across discovery + validation)
	kind string, // empty for overall progress
	filesFound int,
	elapsed time.Duration,
	profile string,
) {
	ctx, projectRoot, ok := initDiscoveryContext(ctx, projectRoot, profile)
	if !ok {
		return
	}

	// Ensure elapsed time is always included (even if 0) for reliable heartbeat
	elapsedSeconds := elapsed.Seconds()
	loggingFields := []coordination.LoggingField{
		{Key: eventKeyStatus, Value: eventStatusInProg},
		{Key: "phase", Value: "discovery"},
		{Key: "files_found", Value: filesFound},
		{Key: "elapsed_seconds", Value: elapsedSeconds}, // Always include for heartbeat
	}
	if kind != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "kind", Value: kind})
	}

	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: map[string]any{
			eventKeyEventType:    eventTypeSystemConfigChange,
			eventKeyOperation:    "Object discovery progress",
			eventKeyTargetKind:   "discovery",
			objects.FieldKeyKind: kind,
			"files_found":        filesFound,
			"elapsed_seconds":    elapsedSeconds, // Always include for heartbeat
			eventKeySeverity:     severityLow,
			"progress_message":   "discovery_in_progress",
		},
		MetricsData: map[string]any{
			eventKeyOperation:     eventTypeSystemCheck,
			eventKeyEventType:     eventStatusProgress,
			objects.FieldKeyPhase: "discovery",
			"files_found":         filesFound,
			"elapsed_seconds":     elapsedSeconds, // Always include for heartbeat
		},
	}

	// Create event context with system_check operation type for unified coordination
	eventCtx := coordination.NewEventContext(operationID, eventTypeSystemCheck, eventStatusInProg).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, true) // Audit, no logging (high frequency), no metrics, operational

	emitStorageEventAsync(ctx, projectRoot, storageProvider, "discovery_progress_event_emit_storage", "emitting discovery progress event to storage", eventCtx)

	// Also emit via global coordinator so in-process subscribers can see it
	emitOperationalSyncGlobal(ctx, operationID, eventStatusProgress, eventData)
}
