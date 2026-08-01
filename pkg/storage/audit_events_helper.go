package storage

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqktime"
)

const (
	// Audit event kind and spec (avoid magic strings).
	auditEventKind     = objects.KindAuditEvent
	auditEventSpecFile = "audit_event.yaml"

	// Audit event field names (spec-aligned; use for SetField and map keys).
	auditFieldID             = "id"
	auditFieldTargetID       = "target_id"
	auditFieldEventType      = "event_type"
	auditFieldCreatedAt      = "created_at"
	auditFieldStatus         = "status"
	auditStatusCompleted     = "completed"
	auditFieldOccurrenceCnt  = "occurrence_count"
	auditFieldOccurrenceTs   = "occurrence_timestamps"
	auditFieldOriginSystem   = "origin_system"
	auditFieldOriginProject  = "origin_project"
	auditFieldNamespaceID    = "namespace_id"
	auditFieldOperation      = "operation"
	auditFieldSeverity       = "severity"
	auditFieldTitle          = "title"
	auditFieldCreatedBy      = "created_by"
	auditFieldUpdatedAt      = "updated_at"
	auditFieldUpdatedBy      = "updated_by"
	auditFieldTargetKind     = "target_kind"
	auditFieldTargetPath     = "target_path"
	auditFieldMetadata       = "metadata"
	auditFieldOriginalValue  = "original_value"
	auditFieldNewValue       = "new_value"
	auditFieldRecoveryMethod = "recovery_method"
	auditFieldReason         = "reason"

	// Logging key names (structured log fields).
	logKeyAuditDir    = "audit_dir"
	logKeyAuditID     = "audit_id"
	logKeyEventType   = "event_type"
	logKeyTargetID    = "target_id"
	logKeyOperation   = "operation"
	logKeyTargetKind  = "target_kind"
	logKeyErrorDetail = "error_detail"
	logKeyProjectRoot = "project_root"

	// Target kind for merge-skip (scheduler_job).
	targetKindSchedulerJob = objects.KindSchedulerJob

	// Event type fallback when spec allowlist is loaded (stable, general-purpose).
	eventTypeFallbackSystemConfig = "system_config_change"
)

// AuditEventOptions contains options for creating an audit event
type AuditEventOptions struct {
	EventType            string         // Required: event type (object_creation, object_update, object_deletion, etc.)
	Operation            string         // Required: operation description
	Severity             string         // Required: severity (low, medium, high)
	TargetKind           string         // Optional: target object kind
	TargetID             string         // Optional: target object ID
	TargetPath           string         // Optional: target file path
	Metadata             map[string]any // Optional: additional metadata
	OriginalValue        string         // Optional: original value for change events
	NewValue             string         // Optional: new value for change events
	RecoveryMethod       string         // Optional: recovery method
	Reason               any            // Optional: reason
	CreatedAt            string         // Optional: custom created_at timestamp (defaults to now)
	CreatedBy            string         // Optional: custom created_by (defaults to secCtx.AccountID or "system")
	UpdatedAt            string         // Optional: custom updated_at timestamp (defaults to CreatedAt)
	OccurrenceCount      int            // Optional: occurrence count for aggregated events
	OccurrenceTimestamps []string       // Optional: occurrence timestamps for aggregated events
	OnError              func(error)    // Optional: callback to receive creation errors (for test verification)
	OnBuffered           func()         // Optional: callback to indicate event was buffered (for test verification)
}

// AllowedAuditEventTypes mirrors the audit_event spec enum (loaded at runtime).
// Exported so coordinator routers can normalize inferred event types.
var (
	AllowedAuditEventTypes        map[string]struct{}
	allowedAuditEventTypesOnce    sync.Once
	allowedAuditEventTypeFallback string
)

// Cached storage providers per projectRoot to avoid expensive factory creation on every audit event
// Uses ResourceCache abstraction for thread-safe caching
var (
	cachedStorageProviders = GetGlobalStorageProviderCache()
)

const OriginalEventTypeMetadataKey = "original_event_type"

type allowedAuditEventTypeInfo struct {
	allowed  map[string]struct{}
	fallback string
}

// loadAllowedAuditEventTypeInfo loads the enum list from audit_event.yaml to avoid drift.
// It also derives a deterministic fallback value:
// - Prefer "system_config_change" if present (stable and general-purpose)
// - Otherwise, use the first enum entry as defined in the spec
func loadAllowedAuditEventTypeInfo() allowedAuditEventTypeInfo {
	loader := objects.GetGlobalSpecLoader()
	spec, err := loader.LoadSpecWithInheritance(auditEventSpecFile)
	if err != nil || spec == nil {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageAuditAllowedTypesSpecLoadFailed).
			WithError(err).
			Log()
		return allowedAuditEventTypeInfo{allowed: map[string]struct{}{}, fallback: ""}
	}

	fieldDef, ok := spec.ResolvedFields[auditFieldEventType].(map[string]any)
	if !ok {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageAuditAllowedTypesMissingFieldDef).
			Log()
		return allowedAuditEventTypeInfo{allowed: map[string]struct{}{}, fallback: ""}
	}

	validation, ok := fieldDef["validation"].(map[string]any)
	if !ok {
		StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn(LogEventStorageAuditAllowedTypesMissingValidation).
			Log()
		return allowedAuditEventTypeInfo{allowed: map[string]struct{}{}, fallback: ""}
	}

	rawEnum, ok := validation["enum"].([]any)
	if !ok {
		// YAML decoding can yield []any
		if alt, okAlt := validation["enum"].([]any); okAlt {
			rawEnum = alt
		} else {
			StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Warn(LogEventStorageAuditAllowedTypesEnumNotFound).
				Log()
			return allowedAuditEventTypeInfo{allowed: map[string]struct{}{}, fallback: ""}
		}
	}

	allowed := make(map[string]struct{}, len(rawEnum))
	fallback := ""
	for _, v := range rawEnum {
		if s, ok := v.(string); ok && s != emptyValue {
			allowed[s] = struct{}{}
			if fallback == emptyValue {
				fallback = s // first enum entry, preserves spec order
			}
		}
	}

	// Prefer a stable, general-purpose fallback if the spec includes it.
	if _, ok := allowed[eventTypeFallbackSystemConfig]; ok {
		fallback = eventTypeFallbackSystemConfig
	}

	return allowedAuditEventTypeInfo{allowed: allowed, fallback: fallback}
}

func getAllowedAuditEventTypes() map[string]struct{} {
	allowedAuditEventTypesOnce.Do(func() {
		info := loadAllowedAuditEventTypeInfo()
		AllowedAuditEventTypes = info.allowed
		allowedAuditEventTypeFallback = info.fallback
	})
	return AllowedAuditEventTypes
}

func getAuditEventTypeFallback() string {
	// Ensures allowlist init has run and fallback is set.
	getAllowedAuditEventTypes()
	return allowedAuditEventTypeFallback
}

// NormalizeAuditEventType ensures event_type is allowed; if not, it coerces to
// system_config_change and records the original value in metadata.
func NormalizeAuditEventType(eventType string, metadata map[string]any) (string, map[string]any) {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if _, ok := getAllowedAuditEventTypes()[eventType]; ok {
		return eventType, metadata
	}
	metadata[OriginalEventTypeMetadataKey] = eventType
	if fallback := getAuditEventTypeFallback(); fallback != emptyValue {
		return fallback, metadata
	}
	// If the spec couldn't be loaded and we have no fallback, keep behavior best-effort:
	// return original (caller may choose to persist or handle separately).
	return eventType, metadata
}

// CreateAuditEventWithBuilder creates an audit event using instance builder and storageProvider.Create()
// This ensures CAS routing and provides metrics tracking
// storageProvider can be any backend (file, graph, etc.) - backend-agnostic
// If storageProvider is nil, creates one from storageFactory
//
// NOTE: This is the low-level implementation used by coordinator's StorageAuditRouter.
// For cmd/ level code, use coordinator helpers (e.g., emitHashMismatchFixEventViaCoordinator).
// This function is exported for use by the coordinator router and internal storage operations.
func CreateAuditEventWithBuilder(
	ctx context.Context,
	projectRoot string,
	secCtx *pkgctx.SecurityContext,
	storageProvider ObjectStorageProvider,
	options *AuditEventOptions,
) error {
	if projectRoot == emptyValue {
		return nil // Best effort - can't create without project root
	}

	// Prevent cycles: don't create audit events during audit event creation
	if IsCreatingAuditEvent() {
		return nil // Best effort - skip to prevent infinite recursion
	}

	// Track audit event creation context
	defer BeginAuditEventCreation()()

	// Get storage provider - use provided one, or get from cache (avoids expensive factory creation)
	if storageProvider == nil {
		// Get or create cached provider for this projectRoot (thread-safe using ResourceCache)
		provider, err := cachedStorageProviders.GetOrCreate(ctx, projectRoot)
		if err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageAuditStorageProviderUnavailable).
				WithError(err).
				Log()
			return nil // Best effort - return early if provider creation fails
		}
		storageProvider = provider
	}

	now := time.Now().UTC()

	// Generate audit event ID and determine storage path using monthly bucketing
	month := now.Format("2006-01")
	auditDir := filepath.Join(projectRoot, paths.ProcessAuditDir, month)
	if err := os.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		return nil // Best effort
	}

	// Use thread-safe batch ID generator (no retry logic needed)
	// The generator acquires IDs in batches to minimize lock contention
	// Pass storageProvider to enable CAS-aware ID generation (finds existing IDs in CAS)
	generator := GetAuditIDGenerator(ctx, auditDir, storageProvider)
	auditID, err := generator.GenerateNextID()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditIDGenerateFailed).
			String(logKeyAuditDir, auditDir).
			WithError(err).
			Log()
		return nil // Best effort
	}

	// Get actor from security context or options
	actor := options.CreatedBy
	if actor == emptyValue && secCtx != nil {
		actor = secCtx.AccountID
	}
	if actor == emptyValue {
		actor = pkgctx.SystemAccountID // Use proper account format for validation
	}

	// Normalize/allowlist event type to prevent validation failures; preserve original for debugging
	eventType, metadata := NormalizeAuditEventType(options.EventType, options.Metadata)
	options.EventType = eventType
	options.Metadata = metadata

	// Get timestamp
	createdAt := options.CreatedAt
	if createdAt == emptyValue {
		createdAt = now.Format(time.RFC3339)
	}

	// Get instance builder schema version from registry
	// NOTE: We create a fresh builder instance for each use to avoid concurrent map writes.
	// Builders from the registry are singleton instances with stateful fields maps that are not thread-safe.
	registry := instance_builders.GetGlobalRegistry()

	// Get latest schema version for audit_event
	schemaVersion, err := registry.GetLatestVersion(auditEventKind)
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBuilderUnavailable).
			String(logKeyAuditID, auditID).
			WithError(err).
			Log()
		return nil // Best effort
	}

	// Create a fresh builder instance for this use (not from registry singleton)
	// This prevents concurrent map writes when multiple goroutines create audit events simultaneously
	builder := bldr_instance_v1.NewAuditEventInstanceBuilder(schemaVersion)

	// Build audit event using instance builder
	builder.ID(auditID)
	builder.Status(auditStatusCompleted)
	builder.SetField(auditFieldOriginSystem, validation.DefaultOriginSystem)
	builder.SetField(auditFieldOriginProject, validation.DefaultOriginProject)
	builder.SetField(auditFieldNamespaceID, validation.DefaultNamespaceKernel) // Set namespace for validation
	builder.SetField(auditFieldEventType, options.EventType)
	builder.SetField(auditFieldOperation, options.Operation)
	builder.SetField(auditFieldSeverity, options.Severity)

	// Set title from operation (audit_event overrides base_object requirement, but set it for consistency)
	// Title is optional in audit_event spec, but helps with display
	if options.Operation != emptyValue {
		// Use operation as title (truncate if needed for display)
		title := options.Operation
		if len(title) > 120 {
			title = title[:117] + "..."
		}
		builder.SetField(auditFieldTitle, title)
	}

	// Set timestamps
	builder.SetField(auditFieldCreatedAt, createdAt)
	builder.SetField(auditFieldCreatedBy, actor)

	// Use custom updated_at if provided, otherwise use created_at
	updatedAt := options.UpdatedAt
	if updatedAt == emptyValue {
		updatedAt = createdAt
	}
	builder.SetField(auditFieldUpdatedAt, updatedAt)
	builder.SetField(auditFieldUpdatedBy, actor)

	// Set optional fields
	if options.TargetKind != emptyValue {
		builder.SetField(auditFieldTargetKind, options.TargetKind)
	}
	if options.TargetID != emptyValue {
		builder.SetField(auditFieldTargetID, options.TargetID)
	}
	if options.TargetPath != emptyValue {
		// Get relative path
		relPath, err := filepath.Rel(projectRoot, options.TargetPath)
		if err != nil {
			relPath = options.TargetPath
		}
		builder.SetField(auditFieldTargetPath, relPath)
	}
	if options.Metadata != nil {
		builder.SetField(auditFieldMetadata, options.Metadata)
	}
	if options.OriginalValue != emptyValue {
		builder.SetField(auditFieldOriginalValue, options.OriginalValue)
	}
	if options.NewValue != emptyValue {
		builder.SetField(auditFieldNewValue, options.NewValue)
	}
	if options.RecoveryMethod != emptyValue {
		builder.SetField(auditFieldRecoveryMethod, options.RecoveryMethod)
	}
	if options.Reason != nil {
		builder.SetField(auditFieldReason, options.Reason)
	}

	// Add occurrence tracking fields if provided (for aggregated events)
	if options.OccurrenceCount > 0 {
		builder.SetField(auditFieldOccurrenceCnt, options.OccurrenceCount)
	}
	if len(options.OccurrenceTimestamps) > 0 {
		builder.SetField(auditFieldOccurrenceTs, options.OccurrenceTimestamps)
	}

	// Check if event should be buffered BEFORE building instance (avoid unnecessary work)
	// Build a minimal event map for buffer check
	eventMapForCheck := map[string]any{
		auditFieldEventType: options.EventType,
		auditFieldSeverity:  options.Severity,
	}
	if options.TargetKind != emptyValue {
		eventMapForCheck[auditFieldTargetKind] = options.TargetKind
	}
	if options.TargetID != emptyValue {
		eventMapForCheck[auditFieldTargetID] = options.TargetID
	}
	if options.Metadata != nil {
		eventMapForCheck[auditFieldMetadata] = options.Metadata
	}

	// Get buffer and check if event should be aggregated
	bufferRegistry := GetGlobalBufferRegistry()
	buffer := bufferRegistry.GetOrCreate(projectRoot, secCtx)
	buffer.SetProjectRoot(projectRoot)
	if secCtx != nil {
		buffer.SetSecurityContext(secCtx)
	}

	// Set file storage for CAS routing (if file-based storage)
	if fileStorage, ok := storageProvider.(*FileObjectStorage); ok {
		buffer.SetFileStorage(fileStorage)
	}

	// Initialize buffer with config if not already initialized
	if err := InitializeGlobalBufferWithConfig(projectRoot, secCtx); err != nil {
		// Config load failed - log warning and continue with defaults (buffer already has defaults)
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBufferInitFailedContinueDefaults).
			String(logKeyProjectRoot, projectRoot).
			WithError(err).
			Log()
	}

	// Check if event should be buffered
	if buffer.ShouldAggregate(eventMapForCheck) {
		// Build full instance for buffer (buffer needs complete event data)
		instance, err := builder.Build()
		if err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageAuditBuildForBufferFailed).
				String(logKeyAuditID, auditID).
				WithError(err).
				Log()
			return nil // Best effort
		}

		// Convert instance to map for buffer
		eventMap := make(map[string]any)
		maps.Copy(eventMap, instance)

		// Add to buffer - buffer will flush when threshold/time window reached
		// Buffer creates aggregated_summary events automatically
		if err := buffer.AddEvent(eventMap); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageAuditBufferAddFailedImmediate).
				String(logKeyAuditID, auditID).
				WithError(err).
				Log()
			// Fall through to immediate write
		} else {
			// Event successfully buffered - don't write immediately
			// Buffer will flush as aggregated_summary when threshold/time window reached
			// Record metrics for buffered event
			metrics := GetGlobalAuditMetricsCollector()
			metrics.RecordAuditEventCreation(ctx, options.EventType, false, 0, true) // Buffered = success, no CAS, no duration

			// Call buffered callback if provided (for test verification)
			if options.OnBuffered != nil {
				options.OnBuffered()
			}

			return nil // Event buffered, don't write immediately
		}
	}

	// Event should not be buffered (or buffer failed) - write immediately
	// Build the instance
	instance, err := builder.Build()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditBuildImmediateFailed).
			String(logKeyAuditID, auditID).
			WithError(err).
			Log()
		return nil // Best effort
	}

	// Use storageProvider.Create() which routes through CAS automatically (backend-agnostic)
	// This provides metrics tracking via the storage layer
	startTime := time.Now()

	// Check if CAS is being used (for metrics) - only applicable for file-based storage
	usedCAS := false
	if fileStorage, ok := storageProvider.(*FileObjectStorage); ok {
		usedCAS = fileStorage.usesContentAddressableStorage(auditEventKind)
	}
	// For graph backend or other backends, CAS detection is not applicable (metrics will show usedCAS=false)

	createErr := storageProvider.Create(ctx, secCtx, instance)

	// Handle "object already exists": retry once with a new ID (sequence file should prevent duplicates;
	// this covers rare fallback-to-scan or cross-process races), then treat as idempotent if still exists
	isAlreadyExists := createErr != nil && (createErr == ErrObjectExists || strings.Contains(createErr.Error(), ConstAuditAlreadyExists))
	if isAlreadyExists {
		if newAuditID, err := generator.GenerateNextID(); err == nil && newAuditID != auditID {
			builder.ID(newAuditID)
			if retryInstance, err := builder.Build(); err == nil {
				createErr = storageProvider.Create(ctx, secCtx, retryInstance)
				if createErr == nil {
					duration := time.Since(startTime)
					metrics := GetGlobalAuditMetricsCollector()
					metrics.RecordAuditEventCreation(ctx, options.EventType, usedCAS, duration, true)
					metrics.RecordAuditEventValidation(true)
					return nil
				}
			}
			// Retry failed; continue with original createErr and idempotent handling
			isAlreadyExists = createErr == ErrObjectExists || strings.Contains(createErr.Error(), ConstAuditAlreadyExists)
		}
	}

	duration := time.Since(startTime)
	success := createErr == nil || isAlreadyExists

	// Record metrics (treat "already exists" as success for metrics)
	metrics := GetGlobalAuditMetricsCollector()
	metrics.RecordAuditEventCreation(ctx, options.EventType, usedCAS, duration, success)
	metrics.RecordAuditEventValidation(success) // Validation happens in Create()

	// Update high-volume event cache on successful creation
	if success && instance != nil {
		if eventID := objects.GetString(instance, auditFieldID); eventID != emptyValue {
			updateHighVolumeEventCacheOnCreate(ctx, storageProvider, eventID, instance, options)
		}
		// Stream-backed kinds (including audit_event) are persisted via stream storage
		// through the normal FileObjectStorage.Create path. No extra dual-write is needed here.
	}

	if createErr != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		if isAlreadyExists {
			// Record duplicate metric
			metrics.RecordAuditEventDuplicate()

			// Skip merge attempt for scheduler_job creation to avoid expensive directory scans
			// Scheduler job creation doesn't need duplicate merging, and with 29k+ audit events,
			// the directory scan in attemptMergeDuplicateAuditEvent can hang indefinitely
			skipMerge := false
			if options.TargetKind == targetKindSchedulerJob {
				skipMerge = true
			}

			when.When(func() bool { return !skipMerge }).Then(func() {
				merged := attemptMergeDuplicateAuditEvent(ctx, projectRoot, storageProvider, secCtx, options, auditID, createdAt)
				when.When(func() bool { return merged }).Then(func() {
					metrics.RecordAuditEventMerged()
					StorageLog(logger).Debug(LogEventStorageAuditDuplicateMerged).
						String(logKeyAuditID, auditID).
						String(logKeyEventType, options.EventType).
						String(logKeyTargetID, options.TargetID).
						String(logKeyOperation, options.Operation).
						Log()
				}).OrElse(func() {
					StorageLog(logger).Debug(LogEventStorageAuditDuplicateIdempotent).
						String(logKeyAuditID, auditID).
						String(logKeyEventType, options.EventType).
						String(logKeyTargetID, options.TargetID).
						String(logKeyOperation, options.Operation).
						Log()
				}).Run()
			}).OrElse(func() {
				StorageLog(logger).Debug(LogEventStorageAuditDuplicateMergeSkippedPerformance).
					String(logKeyAuditID, auditID).
					String(logKeyEventType, options.EventType).
					String(logKeyTargetID, options.TargetID).
					String(logKeyTargetKind, options.TargetKind).
					Log()
			}).Run()
		} else {
			// Log actual errors at warn level (validation failures, etc.)
			StorageLog(logger).Warn(LogEventStorageAuditCreateViaProviderFailed).
				String(logKeyAuditID, auditID).
				String(logKeyEventType, options.EventType).
				String(logKeyOperation, options.Operation).
				String(logKeyErrorDetail, createErr.Error()).
				WithError(createErr).
				Log()
		}

		// Call error callback if provided (for test verification)
		if options.OnError != nil {
			options.OnError(createErr)
		}

		return nil // Best effort - don't fail caller
	}

	return nil
}

// attemptMergeDuplicateAuditEvent attempts to find and merge a duplicate audit event
// by searching for an existing event with the same target_id and event_type, then
// incrementing its occurrence_count and adding the new timestamp.
// Returns true if merge succeeded, false otherwise (e.g., if event not found or update failed).
func attemptMergeDuplicateAuditEvent(
	ctx context.Context,
	projectRoot string,
	storageProvider ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	options *AuditEventOptions,
	attemptedAuditID string,
	newTimestamp string,
) bool {
	// Can only merge if we have target_id and event_type to search by
	if options.TargetID == emptyValue || options.EventType == emptyValue {
		return false
	}

	// Use List to find existing event with matching target_id and event_type
	// Limit to current month's events (where duplicates are most likely)
	filters := map[string]any{
		auditFieldTargetID:  options.TargetID,
		auditFieldEventType: options.EventType,
	}

	// Add time filter to limit search to recent events (last 24 hours)
	// This prevents expensive full-month scans
	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour)
	timeFilter := map[string]any{
		"$gte": yesterday.Format(time.RFC3339),
	}
	filters[auditFieldCreatedAt] = timeFilter

	// Add timeout to prevent hangs on large directories (e.g., 29k+ audit events)
	listCtx, listCancel := context.WithTimeout(ctx, 5*time.Second)
	defer listCancel()

	listResult, err := storageProvider.List(listCtx, secCtx, nil, ListFilter{
		Kind:    auditEventKind,
		Filters: filters,
		SortBy:  auditFieldCreatedAt,
		SortAsc: false, // Most recent first
		Limit:   1,     // Only need the first match
	})

	if err != nil || len(listResult.Objects) == 0 {
		// Event not found or search failed - can't merge
		return false
	}

	// Found existing event - read it to get full details
	existingEvent := listResult.Objects[0]
	existingID, ok := existingEvent[auditFieldID].(string)
	if !ok {
		return false
	}

	// Read the full event (List may return partial data)
	existingFull, err := storageProvider.Read(ctx, secCtx, existingID)
	if err != nil {
		return false
	}

	// Get current occurrence count
	currentCount := when.Result[int]().
		When(func() bool {
			_, ok := existingFull[auditFieldOccurrenceCnt].(int)
			return ok
		}).Then(func() int { return existingFull[auditFieldOccurrenceCnt].(int) }).
		OrElseWhen(func() bool {
			_, ok := existingFull[auditFieldOccurrenceCnt].(int64)
			return ok
		}).Then(func() int { return int(existingFull[auditFieldOccurrenceCnt].(int64)) }).
		OrElseWhen(func() bool {
			_, ok := existingFull[auditFieldOccurrenceCnt].(float64)
			return ok
		}).Then(func() int { return int(existingFull[auditFieldOccurrenceCnt].(float64)) }).
		OrElse(func() int { return 1 }).
		Run()

	// Get existing timestamps
	timestamps := []string{}
	if ts, ok := existingFull[auditFieldOccurrenceTs].([]any); ok {
		for _, t := range ts {
			if str, ok := t.(string); ok {
				timestamps = append(timestamps, str)
			}
		}
	} else if created := objects.GetString(existingFull, auditFieldCreatedAt); ok {
		// If no timestamps, use created_at as first occurrence
		timestamps = append(timestamps, created)
	}

	// Add new timestamp
	timestamps = append(timestamps, newTimestamp)

	// Prepare update
	updates := map[string]any{
		auditFieldOccurrenceCnt: currentCount + 1,
		auditFieldOccurrenceTs:  timestamps,
		auditFieldUpdatedAt:     zqktime.NowRFC3339UTC(),
	}

	// Merge metadata if provided
	if options.Metadata != nil {
		existingMeta, metaOk := existingFull[auditFieldMetadata].(map[string]any)
		when.When(func() bool { return metaOk }).Then(func() {
			maps.Copy(existingMeta, options.Metadata)
			updates[auditFieldMetadata] = existingMeta
		}).OrElse(func() {
			updates[auditFieldMetadata] = options.Metadata
		}).Run()
	}

	// Update the existing event
	if err := storageProvider.Update(ctx, secCtx, existingID, updates); err != nil {
		// Update failed - can't merge
		return false
	}

	return true
}

// BuildAuditEventInstance builds a single audit event instance map from options without creating or buffering.
// Used by FlushPendingAuditEvents to build a batch for BulkCreate. storageProvider is used for ID generation.
func BuildAuditEventInstance(
	ctx context.Context,
	projectRoot string,
	secCtx *pkgctx.SecurityContext,
	options *AuditEventOptions,
	storageProvider ObjectStorageProvider,
) (map[string]any, error) {
	if projectRoot == emptyValue || options == nil {
		return nil, nil
	}
	if storageProvider == nil {
		provider, err := cachedStorageProviders.GetOrCreate(ctx, projectRoot)
		if err != nil {
			return nil, err
		}
		storageProvider = provider
	}

	now := time.Now().UTC()
	month := now.Format("2006-01")
	auditDir := filepath.Join(projectRoot, paths.ProcessAuditDir, month)
	if err := os.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		return nil, err
	}
	generator := GetAuditIDGenerator(ctx, auditDir, storageProvider)
	auditID, err := generator.GenerateNextID()
	if err != nil {
		return nil, err
	}
	actor := options.CreatedBy
	if actor == emptyValue && secCtx != nil {
		actor = secCtx.AccountID
	}
	if actor == emptyValue {
		actor = pkgctx.SystemAccountID
	}
	eventType, metadata := NormalizeAuditEventType(options.EventType, options.Metadata)
	createdAt := options.CreatedAt
	if createdAt == emptyValue {
		createdAt = now.Format(time.RFC3339)
	}
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion, err := registry.GetLatestVersion(auditEventKind)
	if err != nil {
		return nil, err
	}
	builder := bldr_instance_v1.NewAuditEventInstanceBuilder(schemaVersion)
	builder.ID(auditID)
	builder.Status(auditStatusCompleted)
	builder.SetField(auditFieldOriginSystem, validation.DefaultOriginSystem)
	builder.SetField(auditFieldOriginProject, validation.DefaultOriginProject)
	builder.SetField(auditFieldNamespaceID, validation.DefaultNamespaceKernel)
	builder.SetField(auditFieldEventType, eventType)
	builder.SetField(auditFieldOperation, options.Operation)
	builder.SetField(auditFieldSeverity, options.Severity)
	if options.Operation != emptyValue {
		title := options.Operation
		if len(title) > 120 {
			title = title[:117] + "..."
		}
		builder.SetField(auditFieldTitle, title)
	}
	builder.SetField(auditFieldCreatedAt, createdAt)
	builder.SetField(auditFieldCreatedBy, actor)
	updatedAt := options.UpdatedAt
	if updatedAt == emptyValue {
		updatedAt = createdAt
	}
	builder.SetField(auditFieldUpdatedAt, updatedAt)
	builder.SetField(auditFieldUpdatedBy, actor)
	if options.TargetKind != emptyValue {
		builder.SetField(auditFieldTargetKind, options.TargetKind)
	}
	if options.TargetID != emptyValue {
		builder.SetField(auditFieldTargetID, options.TargetID)
	}
	if options.TargetPath != emptyValue {
		relPath, relErr := filepath.Rel(projectRoot, options.TargetPath)
		if relErr != nil {
			relPath = options.TargetPath
		}
		builder.SetField(auditFieldTargetPath, relPath)
	}
	if len(metadata) > 0 {
		builder.SetField(auditFieldMetadata, metadata)
	}
	if options.OriginalValue != emptyValue {
		builder.SetField(auditFieldOriginalValue, options.OriginalValue)
	}
	if options.NewValue != emptyValue {
		builder.SetField(auditFieldNewValue, options.NewValue)
	}
	if options.RecoveryMethod != emptyValue {
		builder.SetField(auditFieldRecoveryMethod, options.RecoveryMethod)
	}
	if options.Reason != nil {
		builder.SetField(auditFieldReason, options.Reason)
	}
	if options.OccurrenceCount > 0 {
		builder.SetField(auditFieldOccurrenceCnt, options.OccurrenceCount)
	}
	if len(options.OccurrenceTimestamps) > 0 {
		builder.SetField(auditFieldOccurrenceTs, options.OccurrenceTimestamps)
	}
	return builder.Build()
}
