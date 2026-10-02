package system

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
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
			if err := syncCoordinator.EmitOperationalSync(ctx, eventCtx); err != nil {
				return
			}
		} else {
			if err := coord.Emit(ctx, eventCtx); err != nil {
				return
			}
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
	opDesc := fmt.Sprintf("Object ID cache %s", operation)
	if forceRebuild {
		opDesc = fmt.Sprintf("Object ID cache force %s", operation)
	}
	status := eventStatusComplete
	if entryCount == 0 {
		status = eventStatusWarning
	}
	emitCacheAuditEvent(
		ctx, projectRoot, profile,
		operation, opDesc,
		sourceCacheBuild, severityLow, status,
		eventTypeCacheOperation, false, entryCount,
		map[string]any{
			eventKeyForceRebuild:  forceRebuild,
			eventKeyBuildDuration: buildDuration.String(),
		},
		[]coordination.LoggingField{
			{Key: eventKeyForceRebuild, Value: forceRebuild},
		},
	)
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
	emitCacheAuditEvent(
		ctx, projectRoot, profile,
		"save", "Object ID cache saved to disk",
		sourceCacheSave, severityLow, eventStatusComplete,
		eventTypeCacheOperation, false, entryCount,
		map[string]any{
			eventKeySaveDuration: saveDuration.String(),
		},
		nil,
	)
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
	opDesc := fmt.Sprintf("Cache availability check for %s", operation)
	severity := severityLow
	status := eventStatusComplete
	if !available {
		opDesc = fmt.Sprintf("Cache unavailable for %s", operation)
		severity = severityHigh
		status = eventStatusError
	}
	emitCacheAuditEvent(
		ctx, projectRoot, profile,
		operation, opDesc,
		sourceCacheAvailability, severity, status,
		eventTypeCacheAvailability, true, entryCount,
		map[string]any{
			eventKeyAvailable: available,
		},
		[]coordination.LoggingField{
			{Key: eventKeyCacheAvailable, Value: available},
		},
	)
}

func emitCacheAuditEvent(
	ctx context.Context,
	projectRoot string,
	profile string,
	operation string,
	opDescription string,
	source string,
	severity string,
	status string,
	eventType string,
	loggingChannel bool,
	entryCount int,
	extraMetadata map[string]any,
	extraLoggingFields []coordination.LoggingField,
) {
	root := ProjectRootOrResolveDot(projectRoot)
	if root == emptyValue {
		return
	}
	ctx = createContextWithLoggingProfile(ctx, profile)
	coordinator := coordination.GetCoordinator()
	metadata := map[string]any{
		eventKeySource:      source,
		eventKeyProjectRoot: root,
		eventKeyOperation:   operation,
		eventKeyEntryCount:  entryCount,
		eventKeyCacheType:   cacheTypeObjectID,
	}
	for k, v := range extraMetadata {
		metadata[k] = v
	}
	options := &storage.AuditEventOptions{
		EventType:  eventType,
		Operation:  opDescription,
		TargetKind: targetKindCache,
		Severity:   severity,
		Metadata:   metadata,
		CreatedBy:  pkgctx.SystemAccountID,
	}
	auditMetadata := buildAuditMetadataFromOptions(options)
	loggingFields := append([]coordination.LoggingField{
		{Key: eventKeyCacheOperation, Value: operation},
		{Key: eventKeyEntryCount, Value: entryCount},
	}, extraLoggingFields...)

	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   nil,
	}
	opID := fmt.Sprintf("%s_%d", source, time.Now().Unix())
	eventCtx := coordination.NewEventContext(opID, eventType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(loggingChannel, true, false, true)
	emitAsyncCoordinationEvent(ctx, coordinator, "cache_event_emit", "emitting cache coordination event", eventCtx)
}

// emitCacheSidecarsIdleViaCoordinator emits when background object-id cache and async reverse-reference
// work for projectRoot have all completed (counters reached zero). Observability and subscriber hooks;
// deterministic joining remains [WaitProjectCacheBackgroundWork] and [RegisterCacheSidecarsIdleCallback].
//
// Subscribers must not synchronously drive a closed loop (wait on idle → submit work → immediate
// idle → same subscriber) for the same root without an async boundary; see
// docs/architecture/concurrency-patterns-v1.0.md § Cache sidecar idle coordination.
func emitCacheSidecarsIdleViaCoordinator(ctx context.Context, projectRoot string) {
	root := ProjectRootOrResolveDot(projectRoot)
	if root == emptyValue {
		return
	}
	coordinator := coordination.GetCoordinator()
	if coordinator == nil {
		return
	}

	eventData := &coordination.EventData{
		LoggingFields: []coordination.LoggingField{
			{Key: eventKeyCacheEvent, Value: "cache_sidecars_idle"},
			{Key: eventKeyProjectRoot, Value: root},
		},
		AuditMetadata: nil,
		MetricsData: map[string]any{
			"cache_sidecars_idle": 1,
			eventKeyProjectRoot:   root,
		},
	}

	operationID := fmt.Sprintf("cache_sidecars_idle_%d", time.Now().UnixNano())
	eventCtx := coordination.NewEventContext(operationID, eventTypeCacheSidecars, eventStatusComplete).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, false, true, true) // logging + metrics + operational (no audit noise)

	ctx = createContextWithLoggingProfile(ctx, systemProfileSystem)
	emitAsyncCoordinationEvent(ctx, coordinator, "cache_sidecars_idle_emit", "emitting cache_sidecars_idle for project root", eventCtx)
}
