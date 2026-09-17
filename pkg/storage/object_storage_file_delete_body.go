// Extracted from object_storage_file_delete_impl.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/audit"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"github.com/lanceman/zqk/pkg/storage/crud"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"
)

func (f *FileObjectStorage) deleteImpl(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
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
	if kind == objects.KindSchedulerJob && id == "SCH-maintenance-wal" {
		return errfmt.Errorf(ErrMsgProtectedDelete)
	}
	if err := denyCoreKernelHardDelete(ctx, secCtx, kind, id); err != nil {
		return err
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
		dependents = filterBlockingDependents(dependents)
	}

	if len(dependents) > 0 {
		if UnlinkReferencesBeforeDelete(ctx) && !cascade {
			if err := UnlinkReferencesFromDependents(ctx, secCtx, f, id, dependents); err != nil {
				return errfmt.Newf(ErrMsgUnlinkRefsFail).Wrap(err)
			}
			if f.projectRoot != emptyValue {
				_ = caspkg.FlushAllListingIndexesForProjectRootWithTimeout(f.projectRoot, IndexFlushAfterCreateTimeout)
			}
			dependents, depErr = f.findDependents(ctx, id, kind)
			if depErr != nil {
				return errfmt.Newf(ErrMsgCheckDepsAfterUnlink).Wrap(depErr)
			}
			dependents = filterBlockingDependents(dependents)
		}
		if len(dependents) > 0 {
			if !cascade {
				return errfmt.Errorf(ErrMsgCannotDeleteDeps, id, len(dependents))
			}
			// Iterative BFS cascade (cycle-safe; no recursive Delete stack).
			// TRACK: BLI-REDACTED
			if err := f.cascadeDeleteDependentsBFS(ctx, secCtx, id); err != nil {
				return err
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
		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpDelete, existing, nil, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		return nil
	}

	// Write-behind path (Option B): enqueue delete and return; worker persists in background.
	// Do not remove from in-memory index here: worker needs the mapping to resolve hash and delete file.
	// Read/List already treat pending delete as not-found / excluded via buffer merge.
	skipWriteBehind := isSkipWriteBehind(ctx)
	if audit.HasCLIMarker(ctx) {
		skipWriteBehind = true
	}
	var appendErr error
	var didAppend bool
	if !skipWriteBehind && f.writeBuf != nil && f.wal != nil {
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
		if err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, cascade, secCtx, dependents, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Create change journal entry
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpDelete, existing, nil, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}

		return nil
	}

	// Draft-plane objects: remove id-keyed YAML (may also have legacy CAS — clean both).
	// TRACK: [REDACTED-ID]
	if f.objectDraftPlaneExists(kind, id) {
		draftPath := f.objectDraftPlanePath(kind, id)
		if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, draftPath, cascade, secCtx, dependents, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, draftPath, OpDelete, existing, nil, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := f.deleteObjectDraftPlane(kind, id); err != nil {
			return err
		}
		// Best-effort: drop any leftover CAS mapping for the same id (migration dual-write).
		if f.usesContentAddressableStorage(kind) && !StreamStorageEnabledForKind(kind) {
			if cas, casErr := f.getContentAddressableStorage(kind); casErr == nil && cas != nil {
				_ = cas.Delete(id)
				if kindDir := f.GetKindDir(kind); kindDir != emptyValue {
					removeOrphanCASFilesForObjectID(id, kindDir)
				}
			}
		}
		updateReverseReferenceIndexOnDelete(id)
		executeChangeNotificationWithPath(ctx, OpDelete, kind, id, draftPath, existing)
		RecordObjectStateChange(ctx, OpDelete, id)
		return nil
	}

	// Stream-backed objects: resolve from registry and soft-delete (no CAS index entry).
	// Must run before CAS path so BulkDeleteOptimized and retention succeed for stream-backed audit_events.
	if StreamStorageEnabledForKind(kind) {
		if loc := f.getStreamLocation(id, kind); loc != emptyValue {
			if err := RemoveStreamBackedCurrentState(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			f.removeStreamLocation(id, kind)
			if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
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
			if err := f.touchProcessDirectory(); err != nil && !IsExpectedMissingErr(err) {
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
		if err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Best effort for telemetry
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := f.deleteCASThroughMembrane(ctx, id, kind, func() error {
			return cas.Delete(id)
		}); err != nil {
			return errfmt.Newf(ErrMsgDeleteCASFail).Wrap(err)
		}
		// Remove any other hash blobs for this logical id (orphans left after CAS Update)
		if kindDir != emptyValue {
			removeOrphanCASFilesForObjectID(id, kindDir)
		}
		if err := RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Still need to update hash registry for compatibility
				Error(ErrMsgSwallowedError, err).Log()
		}

		if kindDir != "" {
			if !crud.ShouldSuppressHashRegistryUpdate(ctx) {
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
		if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, cascade, secCtx, dependents, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Create change journal entry
				Error(ErrMsgSwallowedError, err).Log()
		}

		if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpDelete, existing, nil, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Update reverse reference index (best effort - don't fail delete if this fails)
				Error(ErrMsgSwallowedError, err).Log()
		}

		updateReverseReferenceIndexOnDelete(id)
		// BLI-643: Notify subscribers of object delete (best-effort; existing is pre-delete state)
		executeChangeNotificationWithPath(ctx, OpDelete, kind, id, filePath, existing)

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
	if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, cascade, secCtx, dependents, f); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Create change journal entry
			Error(ErrMsgSwallowedError, err).Log()
	}

	if err := createChangeJournalEntry(ctx, f.projectRoot, id, kind, filePath, OpDelete, existing, nil, secCtx, f); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Stream-backed current-state overlay: remove overlay and stream registry entry
			Error(ErrMsgSwallowedError, err).Log()
	}

	if IsStreamCurrentPath(f.projectRoot, filePath) {
		if err := RemoveStreamBackedCurrentState(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		f.removeStreamLocation(id, kind)
		if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
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
		if err := f.touchProcessDirectory(); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		RecordObjectStateChange(ctx, OpDelete, id)
		executeChangeNotification(ctx, OpDelete, kind, id, existing)
		return nil
	}

	// Stream-backed objects: soft delete (remove overlay, registry and cache; no file delete)
	if segmentPath, _, ok := StreamPathAndOffset(filePath); ok && segmentPath != emptyValue {
		if err := RemoveStreamBackedCurrentState(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		f.removeStreamLocation(id, kind)
		if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
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
		if err := f.touchProcessDirectory(); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		RecordObjectStateChange(ctx, OpDelete, id)
		executeChangeNotification(ctx, OpDelete, kind, id, existing)
		return nil
	}

	// Delete file
	if err := fileutil.Remove(filePath); err != nil {
		if fileutil.IsNotExist(err) {
			return ErrObjectNotFound
		}
		return errfmt.Newf(ErrMsgDeleteFile).Wrap(err)
	}

	// Update hash registry
	hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(filePath))
	if err := hashRegistry.Load(); err == nil {
		hashRegistry.DeleteHash(filepath.Base(filePath))
		//nolint:errcheck // Intentional error ignored
		if err := f.saveHashRegistry(hashRegistry); err != nil && !IsExpectedMissingErr(err) {
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
	// not the parent .zqk/process directory. This causes cache staleness checks to fail.
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

	// BLI-643: Notify subscribers of object delete (best-effort; existing is pre-delete state)
	executeChangeNotificationWithPath(ctx, OpDelete, kind, id, filePath, existing)

	return nil
}

// findDependents finds all objects that reference the given object.
// Uses the reverse-reference index only. Never walks processDir on the delete
// hot path (that was O(all objects) and hang-prone).
// TRACK: [REDACTED-ID]
