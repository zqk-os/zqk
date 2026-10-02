package system

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	checkProgressEventTypeCompleted       = "completed"
	checkProgressEventTypeTimeout         = "timeout"
	checkProgressEventTypeStuck           = "stuck"
	checkProgressEventTypeProgressSummary = "progress_summary" // not objects.KindMilestone (MIL-*)
)

// emitCheckProgressEventViaCoordinator emits system check progress events via the coordination system
// This provides unified event routing for progress updates, progress-summary checkpoints, and completion events
func emitCheckProgressEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	eventType string, // "progress", "progress_summary", "completed", "timeout", "stuck"
	progress int,
	totalTasks int,
	completed int,
	failed int,
	queueSize int,
	message string,
	profile string, // CLI context profile for logging format
) {
	ctx, coordinator, ok := setupSystemCoordinator(ctx, projectRoot, storageProvider, profile, true)
	if !ok {
		return
	}

	// Calculate percentage
	var percent float64
	if totalTasks > 0 {
		percent = float64(progress) / float64(totalTasks) * 100
		if percent > 100 {
			percent = 100
		}
	}

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "operation_id", Value: operationID},
		{Key: objects.FieldKeyPhase, Value: eventType},
		{Key: "progress", Value: progress},
		{Key: "total_tasks", Value: totalTasks},
		{Key: "percent_complete", Value: percent},
		{Key: "completed", Value: completed},
		{Key: "failed", Value: failed},
		{Key: "queue_size", Value: queueSize},
	}
	if message != emptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "message", Value: message})
	}

	// Build audit metadata (only for progress-summary and completion events, not every progress update)
	auditMetadata := make(map[string]any)
	shouldAudit := eventType == checkProgressEventTypeProgressSummary || eventType == checkProgressEventTypeCompleted || eventType == checkProgressEventTypeTimeout || eventType == checkProgressEventTypeStuck
	if shouldAudit {
		auditMetadata[eventKeyEventType] = eventTypeSystemConfigChange // Use valid enum value
		auditMetadata[eventKeyOperation] = fmt.Sprintf("System check %s: %d/%d objects (%.1f%%)", eventType, progress, totalTasks, percent)
		auditMetadata[eventKeySeverity] = severityLow
		if eventType == checkProgressEventTypeTimeout || eventType == checkProgressEventTypeStuck {
			auditMetadata[eventKeySeverity] = severityMedium
		}
		auditMetadata[eventKeyTargetKind] = eventTypeSystemCheck
		auditMetadata[objects.FieldKeyPhase] = eventType
		auditMetadata["progress"] = progress
		auditMetadata["total_tasks"] = totalTasks
		auditMetadata[objects.FieldKeyPercentComplete] = percent
		auditMetadata["completed"] = completed
		auditMetadata[objects.FieldKeyFailed] = failed
		auditMetadata["queue_size"] = queueSize
	}

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[eventKeyOperation] = eventTypeSystemCheck
	metricsData[eventKeyEventType] = eventType
	metricsData[objects.FieldKeyPhase] = eventType
	metricsData["progress"] = progress
	metricsData["total_tasks"] = totalTasks
	metricsData[objects.FieldKeyPercentComplete] = percent
	metricsData["completed"] = completed
	metricsData[objects.FieldKeyFailed] = failed
	metricsData["queue_size"] = queueSize

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Determine status based on event type
	status := eventStatusComplete
	switch eventType {
	case "progress":
		status = eventStatusInProg
	case checkProgressEventTypeTimeout, checkProgressEventTypeStuck:
		status = eventStatusError
	}

	// Create event context for full routing (logging/audit/metrics/operational as configured)
	eventCtx := coordination.NewEventContext(operationID, eventTypeSystemCheck, status).
		WithEventData(eventData).
		WithContext(ctx)

	// Enable channels based on event type
	// High-frequency progress: logging + operational only. Metrics Create was a
	// stream-registry lock convoy (see TRACK on streamRegistrySnapshot.refresh).
	// Progress-summary / completion / timeout / stuck: all channels enabled
	if shouldAudit {
		eventCtx = eventCtx.WithChannels(true, true, true, true)
	} else {
		eventCtx = eventCtx.WithChannels(true, false, false, true)
	}



	bud := goroutinelabels.DefaultBudget()
	builder := goroutinelabels.NewGoroutine("check_progress_event_emit", fmt.Sprintf("emitting check progress event: %s", eventType))
	if bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})

	// Also emit a pared-down operational event via the global coordinator so
	// CLI subscribers (e.g., TerminalProgressSubscriber) can render progress
	// without needing direct access to storage-backed routers.
	// Use synchronous emission for CLI subscribers to ensure immediate feedback.
	globalCoordinator := coordination.GetCoordinator()
	if globalCoordinator != nil {
		// Only operational channel enabled; reuse the same event data.
		// Map "in_progress" status to "progress" for proper event type mapping
		opStatus := status
		if status == eventStatusInProg {
			opStatus = eventStatusProgress
		}
		opEventCtx := coordination.NewEventContext(operationID, eventTypeSystemCheck, opStatus).
			WithEventData(eventData).
			WithContext(ctx).
			WithChannels(false, false, false, true)

		// Emit synchronously for CLI subscribers to ensure immediate feedback
		if syncCoordinator, ok := globalCoordinator.(*coordination.Coordinator); ok {
			_ = syncCoordinator.EmitOperationalSync(ctx, opEventCtx) //nolint:errcheck // Synchronous for CLI
		} else {
			// Fallback to regular Emit if not a Coordinator instance
			_ = globalCoordinator.Emit(ctx, opEventCtx) //nolint:errcheck // Best-effort
		}
	}
}

// emitCheckProgressSummaryEvent emits a sparse progress-summary checkpoint
// (25%, 50%, …). The log label must not say "Milestone" — that is KindMilestone (MIL-*).
func emitCheckProgressSummaryEvent(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	threshold string, // "25%", "50%", "75%", "90%", "95%", "99%"
	progress int,
	totalTasks int,
	completed int,
	failed int,
	queueSize int,
	profile string,
) {
	emitCheckProgressEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		checkProgressEventTypeProgressSummary, progress, totalTasks, completed, failed, queueSize,
		fmt.Sprintf("Progress summary: %s", threshold),
		profile,
	)
}

// emitCheckCompletionEvent emits a completion event
func emitCheckCompletionEvent(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	totalTasks int,
	completed int,
	failed int,
	duration time.Duration,
	profile string,
) {
	emitCheckProgressEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"completed", totalTasks, totalTasks, completed, failed, 0,
		fmt.Sprintf("System check completed in %v", duration),
		profile,
	)
}

// emitCheckAggregationEvent emits a post-validation aggregation milestone event
func emitCheckAggregationEvent(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	profile string,
) {
	emitCheckProgressEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"aggregation", 0, 0, 0, 0, 0,
		"Aggregating validation layers and checking CAS membrane...",
		profile,
	)
}

// emitCheckTimeoutEvent emits a timeout event
func emitCheckTimeoutEvent(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	progress int,
	totalTasks int,
	completed int,
	failed int,
	queueSize int,
	timeoutDuration time.Duration,
	profile string,
) {
	emitCheckProgressEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"timeout", progress, totalTasks, completed, failed, queueSize,
		fmt.Sprintf("System check timed out after %v", timeoutDuration),
		profile,
	)
}

// emitCheckStuckEvent emits a stuck detection event
func emitCheckStuckEvent(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	progress int,
	totalTasks int,
	completed int,
	failed int,
	queueSize int,
	stuckDuration time.Duration,
	profile string,
) {
	emitCheckProgressEventViaCoordinator(
		ctx, projectRoot, storageProvider, operationID,
		"stuck", progress, totalTasks, completed, failed, queueSize,
		fmt.Sprintf("System check appears stuck (no progress for %v)", stuckDuration),
		profile,
	)
}
