package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
	"gopkg.in/yaml.v3"
)

type contextKeySuppressHashReg struct{}

func WithSuppressHashRegistryUpdate(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeySuppressHashReg{}, true)
}

func shouldSuppressHashRegistryUpdate(ctx context.Context) bool {
	val := ctx.Value(contextKeySuppressHashReg{})
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// Delete deletes an object
//
//nolint:gocyclo
func (f *FileObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	// Require CLI authorization for deletions
	// This prevents direct API calls from deleting objects without going through CLI
	if !IsCLIOperation(ctx, secCtx) {
		return errfmt.Errorf(ErrMsgDeleteRequiresCLI)
	}

	// Read existing object to get kind
	existing, err := f.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}

	kind, ok := existing[objects.FieldKeyKind].(string)
	if !ok {
		return errfmt.Errorf(ErrMsgObjectNeedsKind)
	}

	// Check for blocking issues before write operations (except for automated kinds)
	if err := f.checkForBlockingIssuesBeforeWrite(ctx, OpDelete, kind, id); err != nil {
		return err
	}

	// Check permission (requires explicit delete permission)
	if err := f.checkPermission(secCtx, OpDelete, kind); err != nil {
		return errfmt.Errorf(ErrMsgPermissionDeniedDelete, err)
	}

	// Protect maintenance WAL trigger job so it cannot be deleted (avoids stream_deleted and missing maintenance cycles).
	if kind == objects.KindSchedulerJob && id == "SCH-101" {
		return errfmt.Errorf(ErrMsgProtectedDelete)
	}

	// Fast path: Skip dependent checks for leaf node kinds (nothing references them)
	// This significantly speeds up bulk deletions of audit events, metrics, etc.
	leafNodeKinds := map[string]bool{
		// Note: audit_event removed - objects can reference it via related_refs
		objects.KindAuditAggregationMetric: true,
		objects.KindBaseMetric:             true,
		objects.KindCommandMetric:          true,
		objects.KindChangeJournalEntry:     true,
		objects.KindSchedulerJob:           true, // No process objects reference scheduler_job
	}

	var dependents []string
	var depErr error
	when.When(func() bool { return leafNodeKinds[kind] }).Then(func() {
		dependents = []string{}
	}).OrElse(func() {
		dependents, depErr = f.findDependents(ctx, id, kind)
	}).Run()
	if depErr != nil {
		return errfmt.Newf(ErrMsgCheckDeps).Wrap(depErr)
	}

	if len(dependents) > 0 {
		// Filter out audit_event and change_journal_entry dependents (they are system logs and should not block deletion)
		filteredDependents := make([]string, 0, len(dependents))
		for _, depID := range dependents {
			if !strings.HasPrefix(depID, PrefixAudit) && !strings.HasPrefix(depID, "CHA-") {
				filteredDependents = append(filteredDependents, depID)
			}
		}
		dependents = filteredDependents
	}

	if len(dependents) > 0 {
		if UnlinkReferencesBeforeDelete(ctx) && !cascade {
			if err := UnlinkReferencesFromDependents(ctx, secCtx, f, id, dependents); err != nil {
				return errfmt.Newf(ErrMsgUnlinkRefsFail).Wrap(err)
			}
			dependents, depErr = f.findDependents(ctx, id, kind)
			if depErr != nil {
				return errfmt.Newf(ErrMsgCheckDepsAfterUnlink).Wrap(depErr)
			}
		}
		if len(dependents) > 0 {
			if !cascade {
				return errfmt.Errorf(ErrMsgCannotDeleteDeps, id, len(dependents))
			}
			// Cascade delete: delete dependents first (but not the original object)
			for _, dependentID := range dependents {
				// Read dependent to get its kind
				dependent, depErr := f.Read(ctx, secCtx, dependentID)
				if depErr != nil {
					// Skip if already deleted or not found
					continue
				}

				dependentKind, _ := dependent[objects.FieldKeyKind].(string)
				if dependentKind == emptyValue {
					continue
				}

				// Recursively cascade delete the dependent if it's not a parent
				if ShouldCascadeDeleteDependent(dependent, id) {
					if depErr := f.Delete(ctx, secCtx, dependentID, true); depErr != nil {
						return errfmt.Errorf(ErrMsgCascadeDeleteFail, dependentID, depErr)
					}
				} else {
					// Unlink instead
					if err := UnlinkReferencesFromDependents(ctx, secCtx, f, id, []string{dependentID}); err != nil {
						return errfmt.Errorf("failed to unlink parent reference in %s: %w", dependentID, err)
					}
				}
			}
		}
	}

	// CLI deletes must be durable before the process exits. The write-behind enqueue path can
	// otherwise report success while the object still exists on disk (worker backlog / WAL timing),
	// which breaks subprocess-based tests and any automation that immediately re-reads state.
	//
	// Apply the persisted delete immediately for CLI-authorized deletes when write-behind is enabled.
	if IsCLIOperation(ctx, secCtx) && f.writeBuf != nil && f.wal != nil {
		// Enqueue the delete BEFORE waiting, so the worker will definitively process it
		// even if the worker is currently writing an update.
		if err := concurrency.RunInLockWithLogger(&f.walMu, locknames.LockNameDeleteAppendWal, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			_ = AppendToWALAndBuffer(f.wal, f.writeBuf, "delete", kind, id, nil, true)
			return nil
		}); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGen, err).Log()
		}
		f.writeBehindWorker.Notify()

		// Apply the persisted delete immediately for durability
		if err := f.applyDeleteFromBuffer(ctx, id, kind, secCtx); err != nil {
			return err
		}
		RecordObjectStateChange(ctx, OpDelete, id)

		// Create change journal entry since we applied synchronously and bypassed Option B
		filePath, _ := f.getObjectFilePath(id, kind)
		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpDelete, existing, nil, secCtx, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return nil
	}

	// Write-behind path (Option B): enqueue delete and return; worker persists in background.
	// Do not remove from in-memory index here: worker needs the mapping to resolve hash and delete file.
	// Read/List already treat pending delete as not-found / excluded via buffer merge.
	var appendErr error
	var didAppend bool
	if !isSkipWriteBehind(ctx) && f.writeBuf != nil && f.wal != nil {
		if err := concurrency.RunInLockWithLogger(&f.walMu, locknames.LockNameDeleteAppendWal, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			didAppend = true
			appendErr = AppendToWALAndBuffer(f.wal, f.writeBuf, "delete", kind, id, nil, true)
			return nil
		}); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgLockFailedGen, err).Log()
		}
	}
	if didAppend {
		if appendErr != nil {
			return errfmt.Newf(ErrMsgEnqueueDeleteFail).Wrap(appendErr)
		}
		f.writeBehindWorker.Notify()
		executeChangeNotification(ctx, OpDelete, kind, id, existing)

		// Create audit event BEFORE deletion (so we have the object data)
		// Best effort - don't fail deletion if audit event creation fails
		//nolint:errcheck // Intentional error ignored
		filePath, err := f.getObjectFilePath(id, kind)
		if err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, cascade, secCtx, dependents, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Create change journal entry
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpDelete, existing, nil, secCtx, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}

		return nil
	}

	// Stream-backed objects: resolve from registry and soft-delete (no CAS index entry).
	// Must run before CAS path so BulkDeleteOptimized and retention succeed for stream-backed audit_events.
	if StreamStorageEnabledForKind(kind) {
		if loc := f.getStreamLocation(id, kind); loc != emptyValue {
			if err := RemoveStreamBackedCurrentState(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			f.removeStreamLocation(id, kind)
			if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			updateHighVolumeEventCacheOnDelete(id)
			updateReverseReferenceIndexOnDelete(id)
			if err := executeCacheOperation(ctx, ""); err != nil {
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				StorageLog(logger).Warn(LogEventStorageObjectDeleteCacheAfterStreamFailed).
					Kind(kind).
					ObjectID(id).
					WithError(err).
					Log()
			}
			if err := f.touchProcessDirectory(); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			RecordObjectStateChange(ctx, OpDelete, id)
			executeChangeNotification(ctx, OpDelete, kind, id, existing)
			return nil
		}
	}

	// Check if this kind uses content-addressable storage
	if f.usesContentAddressableStorage(kind) {
		cas, err := f.getContentAddressableStorage(kind)
		if err != nil {
			return errfmt.Newf(ErrMsgGetCAS).Wrap(err)
		}
		kindDir := f.GetKindDir(kind)

		// Get file path BEFORE any deletions so we have it for telemetry
		filePath, err := f.getObjectFilePath(id, kind)
		if err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Best effort for telemetry
				Error(ErrMsgSwallowedError, err).Log()
		}

		// Use content-addressable storage for Delete (dependents already deleted if cascade=true)
		if err := cas.Delete(id); err != nil {
			return errfmt.Newf(ErrMsgDeleteCASFail).Wrap(err)
		}
		// Remove any other hash blobs for this logical id (orphans left after CAS Update)
		if kindDir != emptyValue {
			removeOrphanCASFilesForObjectID(id, kindDir)
		}
		if err := RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Still need to update hash registry for compatibility
				Error(ErrMsgSwallowedError, err).Log()
		}

		if kindDir != "" {
			if !shouldSuppressHashRegistryUpdate(ctx) {
				hashRegistry := f.newHashRegistry(ctx, kind, kindDir)
				if err := hashRegistry.Load(); err == nil {
					config := GetStorageConfig()
					filename := fmt.Sprintf("%s%s", id, config.YAMLExtension)
					hashRegistry.DeleteHash(filename)
					if err := f.saveHashRegistry(hashRegistry); err != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSaveHashRegRetries, err).Log()
					}
				}
			}
		}

		// Create audit event
		//nolint:errcheck // Intentional error ignored - audit events are best effort
		if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, cascade, secCtx, dependents, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Create change journal entry
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpDelete, existing, nil, secCtx, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Update reverse reference index (best effort - don't fail delete if this fails)
				Error(ErrMsgSwallowedError, err).Log()
		}

		updateReverseReferenceIndexOnDelete(id)
		// ITEM-643: Notify subscribers of object delete (best-effort; existing is pre-delete state)
		executeChangeNotification(ctx, OpDelete, kind, id, existing)

		// Update high-volume event cache for audit_event and metrics
		if kind == objects.KindAuditEvent || (len(kind) > 7 && kind[len(kind)-7:] == "_metric") {
			updateHighVolumeEventCacheOnDelete(id)
		}

		return nil
	}

	// Get file path
	filePath, err := f.getObjectFilePath(id, kind)
	if err != nil {
		return err
	}

	// Create audit event BEFORE deletion (so we have the object data)
	// Best effort - don't fail deletion if audit event creation fails
	//nolint:errcheck // Intentional error ignored
	if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, cascade, secCtx, dependents, f); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Create change journal entry
			Error(ErrMsgSwallowedError, err).Log()
	}

	if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpDelete, existing, nil, secCtx, f); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Stream-backed current-state overlay: remove overlay and stream registry entry
			Error(ErrMsgSwallowedError, err).Log()
	}

	if IsStreamCurrentPath(f.projectRoot, filePath) {
		if err := RemoveStreamBackedCurrentState(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		f.removeStreamLocation(id, kind)
		if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		updateHighVolumeEventCacheOnDelete(id)
		updateReverseReferenceIndexOnDelete(id)
		if err := executeCacheOperation(ctx, ""); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectDeleteCacheAfterStreamFailed).
				Kind(kind).
				ObjectID(id).
				WithError(err).
				Log()
		}
		if err := f.touchProcessDirectory(); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		RecordObjectStateChange(ctx, OpDelete, id)
		executeChangeNotification(ctx, OpDelete, kind, id, existing)
		return nil
	}

	// Stream-backed objects: soft delete (remove overlay, registry and cache; no file delete)
	if segmentPath, _, ok := StreamPathAndOffset(filePath); ok && segmentPath != emptyValue {
		if err := RemoveStreamBackedCurrentState(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		f.removeStreamLocation(id, kind)
		if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		updateHighVolumeEventCacheOnDelete(id)
		updateReverseReferenceIndexOnDelete(id)
		if err := executeCacheOperation(ctx, ""); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageObjectDeleteCacheAfterStreamFailed).
				Kind(kind).
				ObjectID(id).
				WithError(err).
				Log()
		}
		if err := f.touchProcessDirectory(); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		RecordObjectStateChange(ctx, OpDelete, id)
		executeChangeNotification(ctx, OpDelete, kind, id, existing)
		return nil
	}

	// Delete file
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return ErrObjectNotFound
		}
		return errfmt.Newf(ErrMsgDeleteFile).Wrap(err)
	}

	// Update hash registry
	hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(filePath))
	if err := hashRegistry.Load(); err == nil {
		hashRegistry.DeleteHash(filepath.Base(filePath))
		//nolint:errcheck // Intentional error ignored
		if err := f.saveHashRegistry(hashRegistry); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Execute cache operation based on context
				// The context should have been set by the caller with WithCacheInvalidate
				// File path not needed for invalidation, but pass empty string for consistency
				Error(ErrMsgSwallowedError, err).Log()
		}
	}

	if err := executeCacheOperation(ctx, ""); err != nil {
		// Log warning but don't fail deletion - cache is best effort
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectDeleteCacheAfterDeletionFailed).
			Kind(kind).
			ObjectID(id).
			WithError(err).
			Log()
	}

	// Update reverse reference index (best effort - don't fail delete if this fails)
	// Remove this object from all dependent lists
	updateReverseReferenceIndexOnDelete(id)

	// Update high-volume event cache for audit_event and metrics
	if kind == objects.KindAuditEvent || (len(kind) > 7 && kind[len(kind)-7:] == "_metric") {
		updateHighVolumeEventCacheOnDelete(id)
	}

	// Touch process directory to ensure cache staleness detection works
	// When files are deleted from subdirectories, only the subdirectory mtime is updated,
	// not the parent docs/process directory. This causes cache staleness checks to fail.
	// By explicitly touching the process directory, we ensure the cache knows about changes.
	if err := f.touchProcessDirectory(); err != nil {
		// Log warning but don't fail - this is best effort for cache staleness detection
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Debug(LogEventStorageObjectDeleteTouchProcessDirFailed).
			Kind(kind).
			ObjectID(id).
			WithError(err).
			Log()
	}

	// Record state change in command execution tracker
	RecordObjectStateChange(ctx, OpDelete, id)

	// ITEM-643: Notify subscribers of object delete (best-effort; existing is pre-delete state)
	executeChangeNotification(ctx, OpDelete, kind, id, existing)

	return nil
}

// findDependents finds all objects that reference the given object
// ctx: parent context from command entry point (should not be created here)
func (f *FileObjectStorage) findDependents(ctx context.Context, id, _ string) ([]string, error) {
	// Try to use reverse reference index cache first (O(1) lookup)
	index := GetGlobalReverseReferenceIndex()
	dependents := index.GetDependents(id)

	// If cache has entries, return them (filter to only existing objects as best-effort)
	if len(dependents) > 0 {
		// Filter to only existing objects (best-effort validation)
		existingDependents := make([]string, 0, len(dependents))
		secCtx := pkgctx.NewSystemSecurityContext()
		for _, depID := range dependents {
			// Quick check if object exists (best-effort)
			if _, err := f.Read(ctx, secCtx, depID); err == nil {
				existingDependents = append(existingDependents, depID)
			}
		}
		return existingDependents, nil
	}

	// If the index is ready, an empty dependents list means there are truly no dependents.
	if index.IsReady() {
		return nil, nil
	}

	// Cache miss or empty - fall back to scanning (backward compatibility)
	// This can happen if cache hasn't been built yet or is invalid
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Debug(LogEventStorageObjectDeleteReverseRefIndexMissFallbackScan).
		ObjectID(id).
		Log()

	// Sync the process directory to ensure file system metadata is up to date
	// This helps ensure we see the latest files when scanning for dependents
	if dirFile, err := os.Open(f.processDir); err == nil {
		//nolint:errcheck // Intentional error ignored - best effort
		if err := dirFile.Sync(); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := dirFile.Close(); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Scan all object directories for references to this ID
				// This is a simple implementation - for better performance, use the reverse reference index
				Error(ErrMsgSwallowedError, err).Log()
		}
	}

	dirs, err := os.ReadDir(f.processDir)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgReadProcessDir).Wrap(err)
	}

	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}

		kindDir := filepath.Join(f.processDir, dir.Name())

		// Sync the kind directory to ensure file system metadata is up to date
		if kindDirFile, err := os.Open(kindDir); err == nil {
			//nolint:errcheck // Intentional error ignored - best effort
			if err := kindDirFile.Sync(); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			if err := kindDirFile.Close(); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Infer kind from directory name
					Error(ErrMsgSwallowedError, err).Log()
			}
		}

		kind := objects.GetKindFromDirectory(dir.Name())
		if kind == emptyValue {
			kind = dir.Name() // Fallback to directory name if mapping not found
		}

		// Check if this kind uses CAS
		if f.usesContentAddressableStorage(kind) {
			// For CAS objects, use the index to get all object IDs
			cas, err := f.getContentAddressableStorage(kind)
			if err == nil {
				// Get object IDs from in-memory index (most up-to-date)
				// CRITICAL: Do NOT call index.Load() here - it would overwrite in-memory
				// mappings (updated immediately via setIndexMappingInMemory) with potentially
				// stale disk data. The in-memory index is the source of truth for same-process
				// operations and is updated immediately when objects are created.
				index := cas.GetIndex()
				mappings := index.SnapshotMappings()

				// Read each object directly from CAS to check for references
				for objID := range mappings {
					// Read raw YAML data from CAS
					data, err := cas.Read(objID)
					if err != nil {
						continue
					}

					// Unmarshal to check for references
					var obj map[string]any
					if err := yaml.Unmarshal(data, &obj); err != nil {
						continue
					}

					// Check if this object references the target ID
					if f.objectReferences(obj, id) {
						dependents = append(dependents, objID)
					}
				}
				continue // Skip file scanning for CAS kinds
			}
			// If CAS lookup fails, fall through to file scanning
		}

		// For non-CAS objects (or if CAS lookup failed), scan files directly
		entries, err := os.ReadDir(kindDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			config := GetStorageConfig()
			if !strings.HasSuffix(entry.Name(), config.YAMLExtension) && !strings.HasSuffix(entry.Name(), config.YAMLAltExtension) {
				continue
			}

			filePath := filepath.Join(kindDir, entry.Name())
			obj, err := f.readObjectFile(ctx, filePath)
			if err != nil {
				continue
			}

			// Check if this object references the target ID
			if f.objectReferences(obj, id) {
				objID, _ := obj[objects.FieldKeyID].(string)
				if objID != emptyValue {
					dependents = append(dependents, objID)
				}
			}
		}
	}

	return dependents, nil
}

// objectReferences checks if an object references the given ID
func (f *FileObjectStorage) objectReferences(obj map[string]any, targetID string) bool {
	// Extract reference fields
	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(obj)

	// Helper function to check if a reference string matches the target ID
	matchesTarget := func(refStr string) bool {
		// Parse reference to extract ID (handles namespace format like "domain:process:backlog_item:ITEM-002")
		parsed := validation.ParseNamespace(refStr)
		if parsed != nil && parsed.ObjectID == targetID {
			return true
		}

		// Check legacy format "kind:id" (e.g., "backlog_item:ITEM-002")
		if strings.Contains(refStr, ":") {
			parts := strings.SplitN(refStr, ":", 2)
			if len(parts) == 2 && parts[1] == targetID {
				return true
			}
			// Also check full namespace format (e.g., "domain:process:backlog_item:ITEM-002")
			parts = strings.Split(refStr, ":")
			if len(parts) > 0 && parts[len(parts)-1] == targetID {
				return true
			}
		}

		// Check if it's just the ID (simple format like "ITEM-002")
		return refStr == targetID
	}

	// Check all reference fields
	for _, refValue := range refFields {
		if refValue == nil {
			continue
		}

		// Handle both single references and lists
		switch v := refValue.(type) {
		case string:
			if matchesTarget(v) {
				return true
			}
		case []any:
			for _, item := range v {
				if str, ok := item.(string); ok && matchesTarget(str) {
					return true
				}
			}
		case []string:
			for _, str := range v {
				if matchesTarget(str) {
					return true
				}
			}
		}
	}

	return false
}

// cascadeDelete deletes an object and all objects that reference it
//
//nolint:unused // Helper function - reserved for future use
func (f *FileObjectStorage) cascadeDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, id, kind string) error {
	// Find all dependents
	dependents, err := f.findDependents(ctx, id, kind)
	if err != nil {
		return errfmt.Newf(ErrMsgFindDependents).Wrap(err)
	}

	// Recursively delete dependents first (depth-first)
	for _, dependentID := range dependents {
		// Read dependent to get its kind
		dependent, err := f.Read(ctx, secCtx, dependentID)
		if err != nil {
			// Skip if already deleted or not found
			continue
		}

		dependentKind, _ := dependent[objects.FieldKeyKind].(string)
		if dependentKind == emptyValue {
			continue
		}

		// Recursively cascade delete the dependent
		if err := f.cascadeDelete(ctx, secCtx, dependentID, dependentKind); err != nil {
			return errfmt.Errorf(ErrMsgCascadeDeleteFail, dependentID, err)
		}
	}

	// Create audit event for the main object being deleted (cascade)
	// Best effort - don't fail deletion if audit event creation fails
	filePath, err := f.getObjectFilePath(id, kind)
	if err == nil {
		//nolint:errcheck // Intentional error ignored
		if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, true, secCtx, dependents, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Now delete the object itself
				Error(ErrMsgSwallowedError, err).Log()
		}
	}

	if err != nil {
		return err
	}

	// Stream-backed: soft delete only
	if _, _, ok := StreamPathAndOffset(filePath); ok {
		f.removeStreamLocation(id, kind)
		if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		updateHighVolumeEventCacheOnDelete(id)
		return nil
	}

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return ErrObjectNotFound
		}
		return errfmt.Newf(ErrMsgDeleteFile).Wrap(err)
	}

	// Update hash registry
	hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(filePath))
	if err := hashRegistry.Load(); err == nil {
		hashRegistry.DeleteHash(filepath.Base(filePath))
		//nolint:errcheck // Intentional error ignored
		if err := f.saveHashRegistry(hashRegistry); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
	}

	return nil
}

// applyDeleteFromBuffer persists a delete from the write-behind buffer (CAS + hash registry + audit).
// Used by ObjectWriteBehindWorker. Skips dependency checks and CLI check (already done before enqueue).
func (f *FileObjectStorage) applyDeleteFromBuffer(ctx context.Context, id, kind string, secCtx *pkgctx.SecurityContext) error {
	ctx = withSkipWriteBehind(ctx)

	if !f.usesContentAddressableStorage(kind) {
		// Non-CAS: resolve path and delete file, then hash registry and audit
		filePath, err := f.getObjectFilePath(id, kind)
		if err != nil {
			return err
		}
		//nolint:errcheck // Best effort
		if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, false, secCtx, nil, f); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if _, _, ok := StreamPathAndOffset(filePath); ok {
			f.removeStreamLocation(id, kind)
			if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			updateHighVolumeEventCacheOnDelete(id)
			updateReverseReferenceIndexOnDelete(id)
			executeChangeNotification(ctx, OpDelete, kind, id, nil)
			return nil
		}
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return errfmt.Newf(ErrMsgDeleteFile).Wrap(err)
		}
		hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(filePath))
		if err := hashRegistry.Load(); err == nil {
			hashRegistry.DeleteHash(filepath.Base(filePath))
			if err := f.saveHashRegistry(hashRegistry); err != nil && !isExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// Update reverse reference index (best effort)
					Error(ErrMsgSwallowedError, err).Log()
			}
		}

		updateReverseReferenceIndexOnDelete(id)
		executeChangeNotification(ctx, OpDelete, kind, id, nil)
		return nil
	}
	// Stream-backed: soft delete only (no CAS entry)
	if f.getStreamLocation(id, kind) != emptyValue {
		f.removeStreamLocation(id, kind)
		if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		updateHighVolumeEventCacheOnDelete(id)
		updateReverseReferenceIndexOnDelete(id)
		executeChangeNotification(ctx, OpDelete, kind, id, nil)
		return nil
	}
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		return errfmt.Newf(ErrMsgGetCAS).Wrap(err)
	}

	// Get file path BEFORE deletion so we have it for telemetry
	filePath, err := f.getObjectFilePath(id, kind) // Best effort
	if err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}

	if err := cas.Delete(id); err != nil {
		// Idempotent delete: if ID is not in CAS index (e.g. index rebuilt after restart),
		// object is already gone — treat as success so write-behind doesn't spam errors.
		if strings.Contains(err.Error(), "not found") {
			updateReverseReferenceIndexOnDelete(id)
			return nil
		}
		return errfmt.Newf(ErrMsgDeleteCASFail).Wrap(err)
	}
	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}
	// Mirror synchronous Delete: CAS Update can leave extra hash blobs; if any remain,
	// getObjectFilePath's scan will re-index the object and "undelete" it from the CLI's perspective.
	removeOrphanCASFilesForObjectID(id, kindDir)
	if err := RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	hashRegistry := f.newHashRegistry(ctx, kind, kindDir)
	if err := hashRegistry.Load(); err == nil {
		config := GetStorageConfig()
		filename := fmt.Sprintf("%s%s", id, config.YAMLExtension)
		hashRegistry.DeleteHash(filename)
		if err := f.saveHashRegistry(hashRegistry); err != nil && !isExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
	}

	if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, false, secCtx, nil, f); err != nil && !isExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// Update reverse reference index (best effort)
			ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}

	updateReverseReferenceIndexOnDelete(id)
	executeChangeNotification(ctx, OpDelete, kind, id, nil)
	return nil
}
