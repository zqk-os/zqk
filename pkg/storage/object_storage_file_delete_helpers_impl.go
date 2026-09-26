package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func (f *FileObjectStorage) findDependents(ctx context.Context, id, _ string) ([]string, error) {
	index := GetGlobalReverseReferenceIndex()
	dependents := index.GetDependents(id)

	if len(dependents) > 0 {
		existingDependents := make([]string, 0, len(dependents))
		secCtx := pkgctx.NewSystemSecurityContext()
		for _, depID := range dependents {
			if depObj, err := f.Read(ctx, secCtx, depID); err == nil {
				for _, refID := range GetReferencedObjectIDs(depObj) {
					if refID == id {
						existingDependents = append(existingDependents, depID)
						break
					}
				}
			}
		}
		return existingDependents, nil
	}

	if index.IsReady() {
		return nil, nil
	}

	// Index not ready: try disk cache once. Align with ensureReverseReferenceIndexLoaded:
	// missing cache → ready-empty (incremental CUD populates); LoadCache I/O error → fail closed.
	// Never processDir scan. TRACK: / BLI-CEF-R2-REL-REVINDEX-FAILOPEN
	if f.projectRoot != emptyValue {
		loaded, err := index.LoadCache(f.projectRoot)
		if err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Warn(LogEventStorageReverseRefIndexLoadedDebug).
				WithError(err).
				String("op", "find_dependents_load_cache").
				ObjectID(id).
				Log()
			return nil, err
		}
		if !loaded {
			index.isReady.Store(true)
			return nil, nil
		}
		dependents = index.GetDependents(id)
		if len(dependents) == 0 {
			return nil, nil
		}
		existingDependents := make([]string, 0, len(dependents))
		secCtx := pkgctx.NewSystemSecurityContext()
		for _, depID := range dependents {
			if _, err := f.Read(ctx, secCtx, depID); err == nil {
				existingDependents = append(existingDependents, depID)
			}
		}
		return existingDependents, nil
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Warn(LogEventStorageObjectDeleteReverseRefIndexMissFallbackScan).
		ObjectID(id).
		String("action", "refuse_process_dir_scan").
		Log()
	return nil, errfmt.Errorf("reverse reference index not ready for %s (refusing processDir scan); refresh caches then retry", id)
}

// applyDeleteFromBuffer persists a delete from the write-behind buffer (CAS + hash registry + audit).
// Used by ObjectWriteBehindWorker. Skips dependency checks and CLI check (already done before enqueue).
func (f *FileObjectStorage) applyDeleteFromBuffer(ctx context.Context, id, kind string, secCtx *pkgctx.SecurityContext) error {
	ctx = withSkipWriteBehind(ctx)

	if f.objectDraftPlaneExists(kind, id) {
		draftPath := f.objectDraftPlanePath(kind, id)
		if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, draftPath, false, secCtx, nil, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if err := f.deleteObjectDraftPlane(kind, id); err != nil {
			return err
		}
		if f.usesContentAddressableStorage(kind) && !StreamStorageEnabledForKind(kind) {
			if cas, casErr := f.getContentAddressableStorage(kind); casErr == nil && cas != nil {
				_ = cas.Delete(id)
			}
		}
		updateReverseReferenceIndexOnDelete(id)
		executeChangeNotification(ctx, OpDelete, kind, id, nil)
		return nil
	}

	if !f.usesContentAddressableStorage(kind) {
		// Non-CAS: resolve path and delete file, then hash registry and audit
		filePath, err := f.getObjectFilePath(id, kind)
		if err != nil {
			return err
		}
		//nolint:errcheck // Best effort
		if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, false, secCtx, nil, f); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		if _, _, ok := StreamPathAndOffset(filePath); ok {
			f.removeStreamLocation(id, kind)
			if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			updateHighVolumeEventCacheOnDelete(id)
			updateReverseReferenceIndexOnDelete(id)
			executeChangeNotification(ctx, OpDelete, kind, id, nil)
			return nil
		}
		if err := fileutil.Remove(filePath); err != nil && !fileutil.IsNotExist(err) {
			return errfmt.Newf(ErrMsgDeleteFile).Wrap(err)
		}
		hashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(filePath))
		if err := hashRegistry.Load(); err == nil {
			hashRegistry.DeleteHash(filepath.Base(filePath))
			if err := f.saveHashRegistry(hashRegistry); err != nil && !IsExpectedMissingErr(err) {
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
		if err := AddStreamDeletedID(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
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
	if err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}

	if err := f.deleteCASThroughMembrane(ctx, id, kind, func() error {
		return cas.Delete(id)
	}); err != nil {
		// Idempotent delete: missing ID (daemon or local) is success so write-behind does not spam.
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
	if err := RemoveRuntimeDeltaCurrentState(f.projectRoot, kind, id); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
	hashRegistry := f.newHashRegistry(ctx, kind, kindDir)
	if err := hashRegistry.Load(); err == nil {
		config := GetStorageConfig()
		filename := fmt.Sprintf("%s%s", id, config.YAMLExtension)
		hashRegistry.DeleteHash(filename)
		if err := f.saveHashRegistry(hashRegistry); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
	}

	if err := createDeleteAuditEvent(ctx, f.projectRoot, id, kind, filePath, false, secCtx, nil, f); err != nil && !IsExpectedMissingErr(err) {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

			// Update reverse reference index (best effort)
			ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}

	updateReverseReferenceIndexOnDelete(id)
	executeChangeNotification(ctx, OpDelete, kind, id, nil)
	return nil
}
