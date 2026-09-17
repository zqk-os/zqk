package system

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

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
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Create routers for coordinator
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)

	// Create coordinator with routers (audit and logging for discovery operations)
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil, // Discovery cancellation events don't create metrics
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

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

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("discovery_cancellation_event_emit", fmt.Sprintf("emitting discovery cancellation event for kind %s", kind)).
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
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
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Build operation description
	var operation string
	if targetKind != emptyValue {
		operation = fmt.Sprintf("Object discovery started for kind: %s", targetKind)
	} else {
		operation = fmt.Sprintf("Object discovery started for %d kinds", len(kinds))
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = eventTypeSystemConfigChange
	auditMetadata[eventKeyOperation] = operation
	auditMetadata[eventKeyTargetKind] = "discovery"
	auditMetadata["kinds_count"] = len(kinds)
	auditMetadata["process_dir"] = processDir
	auditMetadata[eventKeySeverity] = severityLow // Discovery start is routine
	if targetKind != emptyValue {
		auditMetadata["target_kind_specific"] = targetKind
	} else {
		auditMetadata["kinds"] = kinds
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "kinds_count", Value: len(kinds)},
		{Key: "process_dir", Value: processDir},
		{Key: eventKeyStatus, Value: "started"},
		{Key: "phase", Value: "discovery"},
	}
	if targetKind != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: eventKeyTargetKind, Value: targetKind})
	} else {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "kinds", Value: kinds})
	}

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
	if storageProvider != nil {
		auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)
		coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
			LoggingRouter:     &coordination.DefaultLoggingRouter{},
			AuditRouter:       auditRouter,
			MetricsRouter:     nil,
			OperationalRouter: &coordination.DefaultOperationalRouter{},
		})
		goroutinelabels.NewGoroutine("discovery_start_event_emit_storage", "emitting discovery start event to storage").
			StartSimple(func() {
				_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
			})
	}

	// Also emit via global coordinator so in-process subscribers (e.g., TerminalProgressSubscriber) can see it
	globalCoordinator := coordination.GetCoordinator()
	if globalCoordinator != nil {
		// Only operational channel for global coordinator (storage coordinator handles audit/logging)
		// Use "start" status so it maps to "operation.start" for subscribers
		opEventCtx := coordination.NewEventContext(operationID, eventTypeSystemCheck, eventStatusStart).
			WithEventData(eventData).
			WithContext(ctx).
			WithChannels(false, false, false, true) // Operational only

		// Emit synchronously to global coordinator to ensure subscriber catches it
		// before discovery work begins (subscribers are already subscribed at this point)
		if syncCoordinator, ok := globalCoordinator.(*coordination.Coordinator); ok {
			_ = syncCoordinator.EmitOperationalSync(ctx, opEventCtx) //nolint:errcheck // Best-effort, synchronous
		} else {
			// Fallback to regular Emit if not a Coordinator instance
			_ = globalCoordinator.Emit(ctx, opEventCtx) //nolint:errcheck // Best-effort
		}
	}
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
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Create routers for coordinator
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)

	// Create coordinator with routers (audit and logging for discovery operations)
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil, // Discovery completion events don't create metrics
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build operation description
	var operation string
	if targetKind != emptyValue {
		operation = fmt.Sprintf("Object discovery completed for kind: %s", targetKind)
	} else {
		operation = fmt.Sprintf("Object discovery completed for %d kinds", len(kinds))
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[eventKeyEventType] = eventTypeSystemConfigChange
	auditMetadata[eventKeyOperation] = operation
	auditMetadata[eventKeyTargetKind] = "discovery"
	auditMetadata["kinds_count"] = len(kinds)
	auditMetadata["files_found"] = filesFound
	auditMetadata[eventKeyDurationSeconds] = duration.Seconds()
	auditMetadata[eventKeySeverity] = severityLow // Discovery completion is routine
	if targetKind != emptyValue {
		auditMetadata["target_kind_specific"] = targetKind
	} else {
		auditMetadata["kinds"] = kinds
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "kinds_count", Value: len(kinds)},
		{Key: "files_found", Value: filesFound},
		{Key: eventKeyDurationSeconds, Value: duration.Seconds()},
		{Key: eventKeyStatus, Value: "completed"},
	}
	if targetKind != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: eventKeyTargetKind, Value: targetKind})
	} else {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "kinds", Value: kinds})
	}

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

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("discovery_completion_event_emit", "emitting discovery completion event").
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
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
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}

	ctx = createContextWithLoggingProfile(ctx, profile)

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

	// Emit via storage-backed coordinator (for audit/persistence)
	if storageProvider != nil {
		auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)
		coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
			LoggingRouter:     &coordination.DefaultLoggingRouter{},
			AuditRouter:       auditRouter,
			MetricsRouter:     nil,
			OperationalRouter: &coordination.DefaultOperationalRouter{},
		})
		goroutinelabels.NewGoroutine("discovery_progress_event_emit_storage", "emitting discovery progress event to storage").
			StartSimple(func() {
				_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // best-effort
			})
	}

	// Also emit via global coordinator so in-process subscribers can see it
	// Emit synchronously to ensure subscriber receives it promptly
	globalCoordinator := coordination.GetCoordinator()
	if globalCoordinator != nil {
		// Only operational channel for global coordinator
		opEventCtx := coordination.NewEventContext(operationID, eventTypeSystemCheck, eventStatusProgress).
			WithEventData(eventData).
			WithContext(ctx).
			WithChannels(false, false, false, true) // Operational only

		// Emit synchronously so subscriber receives it before next progress update
		if syncCoordinator, ok := globalCoordinator.(*coordination.Coordinator); ok {
			_ = syncCoordinator.EmitOperationalSync(ctx, opEventCtx) //nolint:errcheck // Best-effort, synchronous
		} else {
			// Fallback to regular Emit if not a Coordinator instance
			_ = globalCoordinator.Emit(ctx, opEventCtx) //nolint:errcheck // Best-effort
		}
	}
}
