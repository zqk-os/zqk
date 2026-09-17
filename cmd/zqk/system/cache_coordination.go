package system

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
)

// emitObjectIDCacheProgressViaCoordinator emits object ID cache progress to the global coordinator
// so CLI subscribers (e.g. TerminalProgressSubscriber) can show progress without direct stderr.
// operationID must be the system_check operation ID. status is "loading"|"ready"|"error"|"timeout".
// Emits asynchronously so the warm path never blocks on coordinator or stderr (Fprintf can block if terminal isn't reading).
func emitObjectIDCacheProgressViaCoordinator(ctx context.Context, operationID string, status string, message string) {
	if operationID == emptyValue {
		return
	}
	eventStatus := eventStatusProgress
	if status == eventStatusLoading {
		eventStatus = eventStatusStart
	}
	eventData := &coordination.EventData{
		LoggingFields: []coordination.LoggingField{
			{Key: eventKeyPhase, Value: targetKindCache},
			{Key: eventKeyCacheStatus, Value: status},
			{Key: eventKeyMessage, Value: message},
		},
		AuditMetadata: nil,
		MetricsData:   nil,
	}
	eventCtx := coordination.NewEventContext(operationID, eventTypeSystemCheck, eventStatus).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, false, false, true) // Operational only

	globalCoordinator := coordination.GetCoordinator()
	if globalCoordinator == nil {
		return
	}
	// Emit from a goroutine so we never block the warm/check path on coordinator or subscriber stderr writes.
	coord := globalCoordinator
	progressBud := goroutinelabels.DefaultBudget()
	progressBuilder := goroutinelabels.NewGoroutine("cache_progress_emit", fmt.Sprintf("emitting cache progress: %s", message)).
		WithPanicHandler(func(r any) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn("Cache progress emit panic (best-effort, continuing)").
				String("panic", fmt.Sprint(r)).
				String("message", message).
				Log()
		})
	if progressBud != nil {
		progressBuilder = progressBuilder.WithBudget(progressBud)
	}
	progressBuilder.StartSimple(func() {
		if syncCoordinator, ok := coord.(*coordination.Coordinator); ok {
			_ = syncCoordinator.EmitOperationalSync(ctx, eventCtx) //nolint:errcheck // Best-effort for CLI
		} else {
			_ = coord.Emit(ctx, eventCtx) //nolint:errcheck // Best-effort
		}
	})
}

// emitCacheBuildEventViaCoordinator emits cache build events via the coordination system
// This tracks when the object ID cache is built, loaded, or rebuilt
func emitCacheBuildEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operation string, // "build", "load", "rebuild"
	entryCount int,
	forceRebuild bool,
	buildDuration time.Duration,
	profile string,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Build operation description
	opDescription := fmt.Sprintf("Object ID cache %s", operation)
	if forceRebuild {
		opDescription = fmt.Sprintf("Object ID cache force %s", operation)
	}

	// Build metadata
	metadata := map[string]any{
		eventKeySource:        sourceCacheBuild,
		eventKeyProjectRoot:   projectRoot,
		eventKeyOperation:     operation,
		eventKeyEntryCount:    entryCount,
		eventKeyForceRebuild:  forceRebuild,
		eventKeyBuildDuration: buildDuration.String(),
		eventKeyCacheType:     cacheTypeObjectID,
	}

	// Build audit event options
	options := &storage.AuditEventOptions{
		EventType:  eventTypeCacheOperation,
		Operation:  opDescription,
		TargetKind: targetKindCache,
		Severity:   severityLow, // Low severity - routine operation
		Metadata:   metadata,
		CreatedBy:  pkgctx.SystemAccountID,
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Use global coordinator to ensure subscribers receive events
	// The global coordinator will route to appropriate channels based on event context
	coordinator := coordination.GetCoordinator()

	// Ensure audit router is available for this coordinator
	// Note: If global coordinator doesn't have audit router, events will still be emitted
	// but audit channel won't work. This is acceptable for cache events (best effort).

	// Build audit metadata from options
	auditMetadata := make(map[string]any)
	mergeMetadata(auditMetadata, options.Metadata)
	auditMetadata[eventKeyEventType] = options.EventType
	auditMetadata[eventKeyOperation] = options.Operation
	auditMetadata[eventKeySeverity] = options.Severity
	auditMetadata[eventKeyTargetKind] = options.TargetKind

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: []coordination.LoggingField{
			{Key: eventKeyCacheOperation, Value: operation},
			{Key: eventKeyEntryCount, Value: entryCount},
			{Key: eventKeyForceRebuild, Value: forceRebuild},
		},
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Cache build events don't create metrics
	}

	// Determine status
	status := eventStatusComplete
	if entryCount == 0 {
		status = eventStatusWarning // Empty cache is a warning
	}

	// Create operation ID
	operationID := fmt.Sprintf("cache_%s_%d", operation, time.Now().Unix())

	// Create event context (audit + operational for cache availability tracking)
	eventCtx := coordination.NewEventContext(operationID, eventTypeCacheOperation, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, false, true) // Audit + operational (for cache availability tracking)

	// Emit via coordinator (async, non-blocking)
	bud := goroutinelabels.DefaultBudget()
	emitBuilder := goroutinelabels.NewGoroutine("cache_event_emit", fmt.Sprintf("emitting cache %s event", operation))
	if bud != nil {
		emitBuilder = emitBuilder.WithBudget(bud)
	}
	emitBuilder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}

// emitCacheSaveEventViaCoordinator emits cache save events via the coordination system
// This tracks when the object ID cache is saved to disk
func emitCacheSaveEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	entryCount int,
	saveDuration time.Duration,
	profile string,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Build metadata
	metadata := map[string]any{
		eventKeySource:       sourceCacheSave,
		eventKeyProjectRoot:  projectRoot,
		eventKeyOperation:    "save",
		eventKeyEntryCount:   entryCount,
		eventKeySaveDuration: saveDuration.String(),
		eventKeyCacheType:    cacheTypeObjectID,
	}

	// Build audit event options
	options := &storage.AuditEventOptions{
		EventType:  eventTypeCacheOperation,
		Operation:  "Object ID cache saved to disk",
		TargetKind: targetKindCache,
		Severity:   severityLow, // Low severity - routine operation
		Metadata:   metadata,
		CreatedBy:  pkgctx.SystemAccountID,
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Use global coordinator to ensure subscribers receive events
	coordinator := coordination.GetCoordinator()

	// Build audit metadata from options
	auditMetadata := make(map[string]any)
	mergeMetadata(auditMetadata, options.Metadata)
	auditMetadata[eventKeyEventType] = options.EventType
	auditMetadata[eventKeyOperation] = options.Operation
	auditMetadata[eventKeySeverity] = options.Severity
	auditMetadata[eventKeyTargetKind] = options.TargetKind

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: []coordination.LoggingField{
			{Key: eventKeyCacheOperation, Value: "save"},
			{Key: eventKeyEntryCount, Value: entryCount},
		},
		AuditMetadata: auditMetadata,
		MetricsData:   nil,
	}

	// Create operation ID
	operationID := fmt.Sprintf("cache_save_%d", time.Now().Unix())

	// Create event context (audit + operational for cache availability tracking)
	eventCtx := coordination.NewEventContext(operationID, eventTypeCacheOperation, eventStatusComplete).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, false, true) // Audit + operational

	// Emit via coordinator (async, non-blocking)
	saveBud := goroutinelabels.DefaultBudget()
	saveBuilder := goroutinelabels.NewGoroutine("cache_save_event_emit", "emitting cache save event")
	if saveBud != nil {
		saveBuilder = saveBuilder.WithBudget(saveBud)
	}
	saveBuilder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}

// emitCacheAvailabilityEventViaCoordinator emits cache availability events via the coordination system
// This tracks when cache is available for validation operations
func emitCacheAvailabilityEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	available bool,
	entryCount int,
	operation string, // "validation_start", "reference_check", etc.
	profile string,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Build operation description
	opDescription := fmt.Sprintf("Cache availability check for %s", operation)
	if !available {
		opDescription = fmt.Sprintf("Cache unavailable for %s", operation)
	}

	// Build metadata
	metadata := map[string]any{
		eventKeySource:      sourceCacheAvailability,
		eventKeyProjectRoot: projectRoot,
		eventKeyOperation:   operation,
		eventKeyAvailable:   available,
		eventKeyEntryCount:  entryCount,
		eventKeyCacheType:   cacheTypeObjectID,
	}

	// Build audit event options
	severity := severityLow
	if !available {
		severity = severityHigh // High severity if cache is unavailable
	}

	options := &storage.AuditEventOptions{
		EventType:  eventTypeCacheAvailability,
		Operation:  opDescription,
		TargetKind: targetKindCache,
		Severity:   severity,
		Metadata:   metadata,
		CreatedBy:  pkgctx.SystemAccountID,
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Use global coordinator to ensure subscribers receive events
	coordinator := coordination.GetCoordinator()

	// Build audit metadata from options
	auditMetadata := make(map[string]any)
	mergeMetadata(auditMetadata, options.Metadata)
	auditMetadata[eventKeyEventType] = options.EventType
	auditMetadata[eventKeyOperation] = options.Operation
	auditMetadata[eventKeySeverity] = options.Severity
	auditMetadata[eventKeyTargetKind] = options.TargetKind

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: []coordination.LoggingField{
			{Key: eventKeyCacheAvailable, Value: available},
			{Key: eventKeyEntryCount, Value: entryCount},
			{Key: eventKeyOperation, Value: operation},
		},
		AuditMetadata: auditMetadata,
		MetricsData:   nil,
	}

	// Determine status
	status := eventStatusComplete
	if !available {
		status = eventStatusError // Cache unavailable is an error
	}

	// Create operation ID
	operationID := fmt.Sprintf("cache_availability_%s_%d", operation, time.Now().Unix())

	// Create event context (all channels for availability tracking)
	eventCtx := coordination.NewEventContext(operationID, eventTypeCacheAvailability, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, false, true) // Logging, audit, operational (no metrics)

	// Emit via coordinator (async, non-blocking)
	availBud := goroutinelabels.DefaultBudget()
	availBuilder := goroutinelabels.NewGoroutine("cache_event_emit", fmt.Sprintf("emitting cache %s event", operation))
	if availBud != nil {
		availBuilder = availBuilder.WithBudget(availBud)
	}
	availBuilder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}

// emitCacheSidecarsIdleViaCoordinator emits when background object-id cache and async reverse-reference
// work for projectRoot have all completed (counters reached zero). Observability and subscriber hooks;
// deterministic joining remains [WaitProjectCacheBackgroundWork] and [RegisterCacheSidecarsIdleCallback].
//
// Subscribers must not synchronously drive a closed loop (wait on idle → submit work → immediate
// idle → same subscriber) for the same root without an async boundary; see
// docs/architecture/concurrency-patterns-v1.0.md § Cache sidecar idle coordination.
func emitCacheSidecarsIdleViaCoordinator(ctx context.Context, projectRoot string) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return
	}
	coordinator := coordination.GetCoordinator()
	if coordinator == nil {
		return
	}

	eventData := &coordination.EventData{
		LoggingFields: []coordination.LoggingField{
			{Key: eventKeyCacheEvent, Value: "cache_sidecars_idle"},
			{Key: eventKeyProjectRoot, Value: projectRoot},
		},
		AuditMetadata: nil,
		MetricsData: map[string]any{
			"cache_sidecars_idle": 1,
			eventKeyProjectRoot:   projectRoot,
		},
	}

	operationID := fmt.Sprintf("cache_sidecars_idle_%d", time.Now().UnixNano())
	eventCtx := coordination.NewEventContext(operationID, eventTypeCacheSidecars, eventStatusComplete).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, true, true) // logging + metrics + operational (no audit noise)

	ctx = createContextWithLoggingProfile(ctx, systemProfileSystem)

	bud := goroutinelabels.DefaultBudget()
	emitBuilder := goroutinelabels.NewGoroutine("cache_sidecars_idle_emit", "emitting cache_sidecars_idle for project root").
		WithPanicHandler(func(r any) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn("cache_sidecars_idle emit panic (best-effort)").
				String("panic", fmt.Sprint(r)).
				ProjectRoot(projectRoot).
				Log()
		})
	if bud != nil {
		emitBuilder = emitBuilder.WithBudget(bud)
	}
	emitBuilder.StartSimple(func() {
		_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
	})
}
