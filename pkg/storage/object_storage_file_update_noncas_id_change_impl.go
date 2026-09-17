package storage

import (
	"context"
	"path/filepath"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func (f *FileObjectStorage) updateNonCASPathIDChange(ctx context.Context, secCtx *pkgctx.SecurityContext, kind, id, newID, oldFilePath string, existing, updates map[string]any) error {
	// Get new file path
	newFilePath, err := f.getObjectFilePath(newID, kind)
	if err != nil {
		return err
	}

	// Write to new location
	if err := f.writeObjectFile(ctx, newFilePath, existing); err != nil {
		return errfmt.Newf(ErrMsgWriteObjNewLoc).Wrap(err)
	}

	// Update hash registry for new file
	newHashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(newFilePath))
	if err := newHashRegistry.Load(); err != nil {
		// Log warning but don't fail update
	}
	// CRITICAL: Read file back from disk to get the exact bytes that are stored
	// This ensures the hash matches what system check will read
	newFileContent, err := fileutil.ReadFile(newFilePath)
	if err != nil {
		return errfmt.Newf(ErrMsgReadNewFileForHash).Wrap(err)
	}
	hash := f.calculateHash(newFileContent)
	newFilename := filepath.Base(newFilePath)
	newHashRegistry.SetHash(newFilename, hash)
	// Save hash registry with retry (critical for integrity - must succeed)
	if err := f.saveHashRegistryWithRetry(newHashRegistry, id, newFilename, hash); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		if IsHashRegistrySaveQueueFull(err) {
			StorageLog(logger).Warn(LogEventStorageObjectUpdateHashQueueFullIDChangeRollback).
				WithError(err).
				ObjectID(id).
				Kind(kind).
				String("new_file", newFilePath).
				Log()
		} else {
			StorageLog(logger).Error(LogEventStorageObjectUpdateHashPersistAfterIDChangeFailed, err).
				ObjectID(id).
				Kind(kind).
				String("new_file", newFilePath).
				Log()
		}
		return errfmt.Errorf(ErrMsgPersistHashRegIDChange, id, err)
	}

	// Remove old file
	if err := fileutil.Remove(oldFilePath); err != nil {
		// Log warning but don't fail - file might already be moved
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectUpdateRemoveOldFileAfterIDChangeFailed).
			String("old_path", oldFilePath).
			WithError(err).
			Log()
	}

	// Update hash registry for old file (remove old hash)
	oldHashRegistry := f.newHashRegistry(ctx, kind, filepath.Dir(oldFilePath))
	if err := oldHashRegistry.Load(); err == nil {
		oldHashRegistry.DeleteHash(filepath.Base(oldFilePath))
		//nolint:errcheck // Intentional error ignored
		if err := f.saveHashRegistry(oldHashRegistry); err != nil && !IsExpectedMissingErr(err) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Execute cache operation based on context
				// The context should have been set by the caller with WithCacheIDChange
				Error(ErrMsgSwallowedError, err).Log()
		}
	}

	if err := executeCacheOperation(ctx, newFilePath); err != nil {
		// Log warning but don't fail update - cache is best effort
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageObjectUpdateCacheAfterIDChangeFailed).
			String("old_id", id).
			String("new_id", newID).
			Kind(kind).
			WithError(err).
			Log()
	}
	// Part of the transaction: list cache must reflect the write
	f.InvalidateCachesForKind(kind)

	// Update reverse reference index for ID change (best effort - don't fail update if this fails)
	// Remove old ID from index, add new ID with same references
	updateReverseReferenceIndexOnIDChange(id, newID, existing)

	// BLI-643: Notify subscribers of object update (ID change path; object id is now newID)
	executeChangeNotification(ctx, OpUpdate, kind, newID, existing)

	return nil
}
