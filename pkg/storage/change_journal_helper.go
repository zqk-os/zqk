package storage

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/when"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	changeJournalEnum "github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/change_journal_entry"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	bldraudit "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
)

var (
	changeJournalEntriesCreatedTotal atomic.Int64
	changeJournalEntriesFailedTotal  atomic.Int64
)

// GetChangeJournalHelperStats returns lifetime counters for created and failed change journal entries.
func GetChangeJournalHelperStats() (created, failed int64) {
	return changeJournalEntriesCreatedTotal.Load(), changeJournalEntriesFailedTotal.Load()
}

const (
	changeJournalKind            = MetricKindChangeJournalEntry
	changeJournalStatusCompleted = "completed"
	changeJournalTitleMaxLength  = 120

	changeJournalFieldChangeType    = "change_type"
	changeJournalFieldObjectRef     = "object_ref"
	changeJournalFieldDiffSummary   = "diff_summary"
	changeJournalFieldTitle         = "title"
	changeJournalFieldPreviousState = "previous_state"

	logKeyChangeJournalID = "journal_id"
)

func normalizeChangeJournalTitle(title string) string {
	title = strings.TrimSpace(title)
	if len(title) <= changeJournalTitleMaxLength {
		return title
	}
	// Keep title lifecycle-valid for base_object.title max_length=120.
	return title[:changeJournalTitleMaxLength-3] + "..."
}

// ChangeJournalEventCallback is a callback for emitting events via coordinator
// This avoids import cycles by using dependency injection
type ChangeJournalEventCallback func(
	ctx context.Context,
	projectRoot string,
	storage ObjectStorageProvider,
	operationID string,
	operationType string,
	status string,
	changeType string,
	objectRef string,
	kind string,
	objectID string,
	duration time.Duration,
	err error,
)

var (
	globalChangeJournalEventCallback atomic.Pointer[ChangeJournalEventCallback]
)

// SetChangeJournalEventCallback sets the global callback for emitting events via coordinator
// This should be called during system initialization to wire up coordinator integration
func SetChangeJournalEventCallback(callback ChangeJournalEventCallback) {
	if callback == nil {
		globalChangeJournalEventCallback.Store(nil)
		return
	}
	cb := callback
	globalChangeJournalEventCallback.Store(&cb)
}

// getChangeJournalEventCallback returns the global event callback (if set)
func getChangeJournalEventCallback() ChangeJournalEventCallback {
	ptr := globalChangeJournalEventCallback.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// ChangeJournalEntryOptions contains options for creating a change journal entry
type ChangeJournalEntryOptions struct {
	ChangeType    string         // Required: change type (create, update, delete, move)
	ObjectRef     string         // Required: object reference (format: "kind:id")
	DiffSummary   string         // Required: human-readable summary of changes
	ChangedPaths  []string       // Optional: flattened dotted paths (e.g. meta.tags, status) for analytics and compaction
	Title         string         // Optional: title (defaults to "{ChangeType}: {ObjectRef}")
	PreviousState map[string]any // Optional: previous state for rollback (required for update/delete)
	CreatedAt     string         // Optional: custom created_at timestamp (defaults to now)
	CreatedBy     string         // Optional: custom created_by (defaults to secCtx.AccountID or "system")
}

// CreateChangeJournalEntryWithBuilder creates a change journal entry using instance builder and storageProvider.Create()
// This ensures CAS routing and provides consistency with other system object creation
// storageProvider can be any backend (file, graph, etc.) - backend-agnostic
// If storageProvider is nil, creates one from storageFactory
func CreateChangeJournalEntryWithBuilder(
	ctx context.Context,
	projectRoot string,
	secCtx *pkgctx.SecurityContext,
	storageProvider ObjectStorageProvider,
	options *ChangeJournalEntryOptions,
) error {
	if projectRoot == emptyValue {
		return nil // Best effort - can't create without project root
	}

	// Get storage provider - use provided one, or create from factory
	if storageProvider == nil {
		storageFactory, err := NewStorageFactory(ctx, projectRoot)
		if err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageChangeJournalHelperFactoryFailedWarn).
				WithError(err).
				Log()
			return nil // Best effort
		}
		storageProvider = storageFactory.GetStorage()
		if storageProvider == nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageChangeJournalHelperNilProviderWarn).Log()
			return nil // Best effort
		}
	}

	now := time.Now().UTC()

	// Generate change journal entry ID and determine storage path using monthly bucketing
	month := now.Format("2006-01")
	processDir := paths.ResolvePath(projectRoot, ConstAuditPrefixProcess)
	journalDir := filepath.Join(processDir, ConstAuditChangeJournal, month)
	if err := os.MkdirAll(journalDir, paths.DirPerm755); err != nil {
		changeJournalEntriesFailedTotal.Add(1)
		return nil // Best effort
	}

	// Find next sequence number (month dir for scan; when stream enabled, sequence file under .zqk/state)
	journalID, err := findNextChangeJournalID(ctx, projectRoot, journalDir)
	if err != nil {
		changeJournalEntriesFailedTotal.Add(1)
		return nil // Best effort
	}

	// Get actor from security context or options
	actor := options.CreatedBy
	if actor == emptyValue && secCtx != nil {
		actor = secCtx.AccountID
	}
	if actor == emptyValue {
		actor = "system"
	}

	// Get timestamp (RFC3339 so instance validation pattern matches)
	createdAt := options.CreatedAt
	when.When(func() bool { return createdAt == emptyValue }).Then(func() {
		createdAt = now.Format(time.RFC3339)
	}).OrElseWhen(func() bool { _, err := time.Parse(time.RFC3339, createdAt); return err == nil }).Then(func() {
		ts, parseErr := time.Parse(time.RFC3339, createdAt)
		if parseErr != nil {
			logging.LogSwallowedError(parseErr)
		}
		createdAt = zqktime.FormatRFC3339UTC(ts)
	}).OrElseWhen(func() bool { _, err := time.Parse(time.RFC3339Nano, createdAt); return err == nil }).Then(func() {
		ts, parseErr := time.Parse(time.RFC3339Nano, createdAt)
		if parseErr != nil {
			logging.LogSwallowedError(parseErr)
		}
		createdAt = zqktime.FormatRFC3339UTC(ts)
	}).OrElse(func() {
		createdAt = now.Format(time.RFC3339)
	}).Run()

	// Build title if not provided
	title := options.Title
	if title == emptyValue {
		title = options.ChangeType + ": " + options.ObjectRef
	}
	title = normalizeChangeJournalTitle(title)

	// Get instance builder schema version from registry
	// NOTE: We create a fresh builder instance for each use to avoid concurrent map writes.
	// Builders from the registry are singleton instances with stateful fields maps that are not thread-safe.
	// CreateChangeJournalEntryWithBuilder can be called concurrently from multiple object update operations.
	registry := instance_builders.GetGlobalRegistry()

	// Get latest schema version for change_journal_entry
	schemaVersion, err := registry.GetLatestVersion(changeJournalKind)
	if err != nil {
		changeJournalEntriesFailedTotal.Add(1)
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageChangeJournalHelperBuilderUnavailableWarn).
			String("journal_id", journalID).
			WithError(err).
			Log()
		return nil // Best effort
	}
	schemaVersion = objects.ValidSchemaVersion(schemaVersion)

	// Create a fresh builder instance for this use (not from registry singleton)
	builder := bldr_instance_v1.NewChangeJournalEntryInstanceBuilder(schemaVersion)

	// Build change journal entry using instance builder
	builder.ID(journalID).
		Status(changeJournalStatusCompleted).
		SetField(bldraudit.FieldOriginSystem, validation.DefaultOriginSystem).
		SetField(bldraudit.FieldOriginProject, validation.DefaultOriginProject)
	builder.ChangeType(changeJournalEnum.ChangeType(options.ChangeType)).
		ObjectRef(options.ObjectRef).
		DiffSummary(options.DiffSummary).
		SetField(changeJournalFieldTitle, title)
	if len(options.ChangedPaths) > 0 {
		builder.ChangedPaths(options.ChangedPaths)
	}

	// Set timestamps
	builder.SetField(bldraudit.FieldCreatedAt, createdAt).
		SetField(bldraudit.FieldCreatedBy, actor).
		SetField(bldraudit.FieldUpdatedAt, createdAt).
		SetField(bldraudit.FieldUpdatedBy, actor)

	// Set previous state if provided (for rollback capability)
	if options.PreviousState != nil {
		// Create a copy to avoid modifying the original
		previousStateCopy := make(map[string]any, len(options.PreviousState))
		maps.Copy(previousStateCopy, options.PreviousState)
		builder.PreviousState(previousStateCopy)
	}

	// Build the instance
	instance, err := builder.Build()
	if err != nil {
		changeJournalEntriesFailedTotal.Add(1)
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageChangeJournalHelperBuildFailedWarn).
			String(logKeyChangeJournalID, journalID).
			WithError(err).
			Log()
		return nil // Best effort
	}

	// Parse object ref to extract kind and ID
	kind, objectID := parseObjectRef(options.ObjectRef)

	// Emit change journal creation event via coordinator
	startTime := time.Now()
	status := "complete"

	// Use storageProvider.Create() which routes through CAS automatically (backend-agnostic)
	if createErr := storageProvider.Create(ctx, secCtx, instance); createErr != nil {
		changeJournalEntriesFailedTotal.Add(1)
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageChangeJournalHelperCreateFailedWarn).
			String(logKeyChangeJournalID, journalID).
			WithError(createErr).
			Log()
		status = "error"
		// Emit error event
		emitChangeJournalEvent(ctx, projectRoot, storageProvider, journalID, options.ChangeType, status, kind, objectID, time.Since(startTime), createErr)
		return nil // Best effort - don't fail caller
	}

	changeJournalEntriesCreatedTotal.Add(1)

	// Emit success event
	emitChangeJournalEvent(ctx, projectRoot, storageProvider, journalID, options.ChangeType, status, kind, objectID, time.Since(startTime), nil)

	return nil
}

// parseObjectRef parses an object reference (format: "kind:id") into kind and ID
func parseObjectRef(objectRef string) (kind, id string) {
	parts := strings.SplitN(objectRef, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", objectRef // Fallback: treat entire string as ID
}

// emitChangeJournalEvent emits a change journal entry creation event via coordinator
func emitChangeJournalEvent(
	ctx context.Context,
	projectRoot string,
	storageProvider ObjectStorageProvider,
	operationID string,
	changeType string,
	status string,
	kind string,
	objectID string,
	duration time.Duration,
	err error,
) {
	callback := getChangeJournalEventCallback()
	if callback == nil || projectRoot == emptyValue || storageProvider == nil {
		// Coordinator not available - skip
		return
	}

	objectRef := kind + ":" + objectID
	if kind == emptyValue {
		objectRef = objectID
	}

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine(ConstAuditChangeJournalCoordinatorEvent, fmt.Sprintf(ConstAuditEmittingChangeJournalEvent, changeType)).
		StartSimple(func() {
			callback(
				ctx,
				projectRoot,
				storageProvider,
				operationID,
				objects.KindChangeJournalEntry,
				status,
				changeType,
				objectRef,
				kind,
				objectID,
				duration,
				err,
			)
		})
}
