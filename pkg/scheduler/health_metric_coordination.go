package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// createContextWithLoggingProfile creates a context with LoggingContext embedded from profile string
// This ensures coordinator logging events respect --context profile settings
func createContextWithLoggingProfile(ctx context.Context, profile string) context.Context {
	if ctx == nil {
		// Use system context as fallback when no context provided
		ctx = pkgctx.NewSystemContext()
	}

	if profile == emptyValue {
		profile = string(pkgctx.ProfileSystem) // Default for scheduler (system operation)
	}

	// Convert profile string to LoggingProfile enum
	var loggingCtx *pkgctx.LoggingContext
	switch profile {
	case string(pkgctx.ProfileMCP):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	case string(pkgctx.ProfileSystem):
		loggingCtx = pkgctx.NewSystemLoggingContext()
	case string(pkgctx.ProfileAIAgent):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent)
	case string(pkgctx.ProfileDebug):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileDebug)
	case string(pkgctx.ProfileHuman), "":
		loggingCtx = pkgctx.NewHumanLoggingContext()
	default:
		loggingCtx = pkgctx.NewSystemLoggingContext()
	}

	return pkgctx.WithLoggingContext(ctx, loggingCtx)
}

// emitSchedulerHealthMetricViaCoordinator emits scheduler health metric events via the coordination system
// This provides unified event routing for scheduler health monitoring (audit, logging, operational)
// Note: The actual metric object is created via storage.Create() in recordHealthMetric
func hasValidProjectRoot(projectRoot string) bool {
	return projectRoot != emptyValue && projectRoot != "."
}

func emitSchedulerHealthMetricViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storagepkg.ObjectStorageProvider,
	metricID string,
	timestamp time.Time,
	healthChecks, missedTriggers, recoveredJobs, cronRestarts int,
	healthCheckDuration time.Duration,
	goroutineCount, runtimeThreadCount, heapAllocBytes, sysMemoryBytes int,
	source string,
	profile string, // CLI context profile for logging format (optional, defaults to "system")
) {
	if !hasValidProjectRoot(projectRoot) {
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Create routers directly (we call them synchronously, not via coordinator.Emit which spawns goroutines)
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)
	loggingRouter := &coordination.DefaultLoggingRouter{}
	// Operational events are for CLI subscribers; scheduler background operations don't need them

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[objects.FieldKeyEventType] = "scheduler_health_metric_created"
	auditMetadata[objects.FieldKeyOperation] = "Scheduler health metric recorded"
	auditMetadata[objects.FieldKeyTargetKind] = objects.KindSchedulerHealthMetric
	auditMetadata[objects.FieldKeyTargetID] = metricID
	auditMetadata[objects.FieldKeySource] = source
	auditMetadata[objects.FieldKeySeverity] = "low" // Health metrics are informational
	if missedTriggers > 0 || cronRestarts > 0 {
		auditMetadata[objects.FieldKeySeverity] = "medium" // Issues detected
	}
	auditMetadata[objects.FieldKeyHealthChecks] = healthChecks
	auditMetadata[objects.FieldKeyMissedTriggers] = missedTriggers
	auditMetadata[objects.FieldKeyRecoveredJobs] = recoveredJobs
	auditMetadata[objects.FieldKeyCronRestarts] = cronRestarts
	auditMetadata[objects.FieldKeyHealthCheckDurationMs] = healthCheckDuration.Milliseconds()
	auditMetadata[objects.FieldKeyGoroutineCount] = goroutineCount
	auditMetadata[objects.FieldKeyRuntimeThreadCount] = runtimeThreadCount
	auditMetadata[objects.FieldKeyHeapAllocBytes] = heapAllocBytes
	auditMetadata[objects.FieldKeySysMemoryBytes] = sysMemoryBytes

	// Build logging fields
	loggingFields := []coordination.LoggingField{
		{Key: "metric_id", Value: metricID},
		{Key: "source", Value: source},
		{Key: "health_checks", Value: healthChecks},
		{Key: "missed_triggers", Value: missedTriggers},
		{Key: "recovered_jobs", Value: recoveredJobs},
		{Key: "cron_restarts", Value: cronRestarts},
		{Key: "health_check_duration_ms", Value: healthCheckDuration.Milliseconds()},
		{Key: "goroutine_count", Value: goroutineCount},
		{Key: "runtime_thread_count", Value: runtimeThreadCount},
		{Key: "heap_alloc_bytes", Value: heapAllocBytes},
		{Key: "sys_memory_bytes", Value: sysMemoryBytes},
	}

	// Create event data (no metrics data - these are metric objects, not events to aggregate)
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Skip - scheduler health metrics are metric objects themselves
	}

	// Determine status
	status := "complete"
	if missedTriggers > 0 || cronRestarts > 0 {
		status = "warning"
	}

	// Create operation ID
	operationID := fmt.Sprintf("scheduler_health_%s", metricID)

	// Create event context (enable audit, logging channels - skip metrics and operational)
	eventCtx := coordination.NewEventContext(operationID, "scheduler_health_monitoring", status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, false, false) // Audit, logging - skip metrics and operational

	if healthCheckDuration > 0 {
		eventCtx = eventCtx.WithDuration(healthCheckDuration)
	}

	// Call routers directly (synchronous) instead of coordinator.Emit() which spawns goroutines.
	// Health metrics are emitted once per cycle, but avoiding goroutines prevents any accumulation.
	if err := loggingRouter.Emit(ctx, eventCtx); err != nil {
		SLog(logging.GetLoggerFromProfile(profile)).Debug("Failed to emit health metric to logging router").WithError(err).Log()
	}
	if err := auditRouter.Emit(ctx, eventCtx); err != nil {
		SLog(logging.GetLoggerFromProfile(profile)).Debug("Failed to emit health metric to audit router").WithError(err).Log()
	}
}

// emitGoroutineCeilingBlockViaCoordinator emits a coordinator event when the scheduler blocked on the goroutine ceiling.
// Use when WaitUnderGoroutineCeiling actually waited (goroutineCount >= ceiling). Enables metrics/observability on ceiling blocks.
func emitGoroutineCeilingBlockViaCoordinator(
	ctx context.Context,
	projectRoot string,
	goroutineCountAtBlock int,
	ceiling int,
	blockDuration time.Duration,
	triggerType string, // "event" or "lifecycle"
	profile string,
) {
	if projectRoot == emptyValue || projectRoot == "." {
		return
	}
	ctx = createContextWithLoggingProfile(ctx, profile)
	loggingRouter := &coordination.DefaultLoggingRouter{}
	// Operational events are for CLI subscribers; scheduler background operations don't need them
	loggingFields := []coordination.LoggingField{
		{Key: "goroutine_count_at_block", Value: goroutineCountAtBlock},
		{Key: "ceiling", Value: ceiling},
		{Key: "block_duration_ms", Value: blockDuration.Milliseconds()},
		{Key: "trigger_type", Value: triggerType},
	}
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil,
		MetricsData:   nil,
	}
	operationID := fmt.Sprintf("scheduler_ceiling_block_%d", time.Now().UnixNano())
	eventCtx := coordination.NewEventContext(operationID, "goroutine_ceiling_block", "info").
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, false, false). // Logging only - skip operational (for CLI subscribers)
		WithDuration(blockDuration)
	// Call router directly (synchronous) instead of coordinator.Emit() which spawns goroutines.
	if err := loggingRouter.Emit(ctx, eventCtx); err != nil {
		SLog(logging.GetLoggerFromProfile(profile)).Debug("Failed to emit goroutine ceiling block event").WithError(err).Log()
	}
}

// emitPoolCreationDeclinedViaCoordinator emits a coordinator event when a goroutine pool could not be created
// from the budget (no budget or budget exceeded) and a single-worker fallback was used.
// Call from PoolCreationDeclinedNotifier so observability and metrics can track how often it occurs.
func emitPoolCreationDeclinedViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storagepkg.ObjectStorageProvider,
	poolName, purpose, reason string,
) {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	ctx = createContextWithLoggingProfile(ctx, string(pkgctx.ProfileSystem))

	loggingFields := []coordination.LoggingField{
		{Key: "pool_name", Value: poolName},
		{Key: "purpose", Value: purpose},
		{Key: "reason", Value: reason},
	}
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: nil,
		MetricsData:   nil,
	}
	if projectRoot != emptyValue && projectRoot != "." {
		eventData.AuditMetadata = map[string]any{
			objects.FieldKeyEventType:  "pool_creation_declined",
			objects.FieldKeyOperation:  "Goroutine pool creation declined; single-worker fallback used",
			objects.FieldKeyTargetKind: "goroutine_pool",
			"pool_name":                poolName,
			objects.FieldKeyPurpose:    purpose,
			objects.FieldKeyReason:     reason,
			objects.FieldKeySource:     "scheduler",
			objects.FieldKeySeverity:   "low",
		}
	}

	operationID := fmt.Sprintf("pool_creation_declined_%s_%d", poolName, time.Now().UnixNano())
	eventCtx := coordination.NewEventContext(operationID, "pool_creation_declined", "warning").
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, projectRoot != emptyValue && projectRoot != ".", false, false) // Logging always; audit when project root set

	loggingRouter := &coordination.DefaultLoggingRouter{}
	if err := loggingRouter.Emit(ctx, eventCtx); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to emit pool creation declined event to logging router").WithError(err).Log()
	}
	if projectRoot != emptyValue && projectRoot != "." && storageProvider != nil {
		auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)
		if err := auditRouter.Emit(ctx, eventCtx); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to emit pool creation declined event to audit router").WithError(err).Log()
		}
	}
}
