// Extracted from pkg/storage/filecas/content_addressable_storage_crud.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package filecas

import (
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Update updates an object
// Creates new hash-based file, updates index, deletes old file.
// For bucketed storage, uses the same bucket as the old file unless targetBucketDir is provided.
// When targetBucketDir is provided and different from the current bucket, the object is migrated
// to the target bucket (all kinds with a bucket strategy migrate on update the same way).
// If object is not in index, treats it as a new object and adds it to the index.
func (cas *ContentAddressableStorage) Update(objectID string, data []byte, targetBucketDir ...string) error {
	if len(data) == 0 {
		return errfmt.Errorf(ConstMiscCannotUpdateObjectWithEmptyContent)
	}

	oldHash, err := cas.GetHashForID(objectID)
	objectNotInIndex := err != nil

	// Resolve target directory for "not in index" path (migration / new index entry)
	var wantTargetDir string
	if len(targetBucketDir) > 0 && targetBucketDir[0] != emptyValue {
		wantTargetDir = targetBucketDir[0]
	}

	if objectNotInIndex {

		newHash := CalculateSHA256Hash(data)

		storageDir := cas.kindDir
		if wantTargetDir != emptyValue {
			storageDir = wantTargetDir
			if mkErr := fileutil.MkdirAll(storageDir, paths.DirPerm755); mkErr != nil {
				return errfmt.Newf(ConstMiscFailedToCreateTargetBucketDirectory).Wrap(mkErr)
			}
		}
		newHashFile := filepath.Join(storageDir, newHash+".yaml")

		return cas.WithSweepLock(func() error {
			if _, statErr := fileutil.Stat(newHashFile); statErr == nil {

			} else {

				if err := cas.WriteFileWithSync(newHashFile, data); err != nil {
					return errfmt.Newf(ConstMiscFailedToWriteNewHashFile).Wrap(err)
				}
			}

			targetBucketKey := ""
			if wantTargetDir != emptyValue {
				targetBucketKey = filepath.Base(wantTargetDir)
			}
			cas.SetIndexMappingInMemory(objectID, newHash, targetBucketKey)

			writeQueue := cas.getWriteQueue()
			var done <-chan error
			var enqErr error
			if targetBucketKey != emptyValue {
				opCallback := cas.getOperationCallback()
				done, enqErr = writeQueue.EnqueueUpdateWithOperationCallback(cas.kind, objectID, newHash, targetBucketKey, cas, opCallback)
			} else {
				done, enqErr = writeQueue.EnqueueUpdateWithCallback(cas.kind, objectID, newHash, cas)
			}
			if enqErr != nil {
				if err := cas.index.SetMapping(objectID, newHash); err != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
				}
				if err := cas.EnsureCASIndexMatchesContentHash(objectID, data, targetBucketKey); err != nil {
					return err
				}
				return cas.commitLiveIdentity(objectID, newHash, newHashFile)
			}
			var indexErr error
			select {
			case indexErr = <-done:
			case <-time.After(casIndexUpdateWaitTimeout):
				StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
					Warn(LogEventStorageCASIndexUpdateNewObjectTimeoutWarn).
					Kind(cas.kind).
					ObjectID(objectID).
					Log()
				cas.sweepAfterDurableBlob(objectID, newHash)
				return errfmt.Errorf(ConstMiscCasIndexUpdateDidNotCompleteWithinVQueue, casIndexUpdateWaitTimeout)
			}
			if indexErr != nil {

				return errfmt.Newf(ConstMiscFailedToPersistIndexUpdateForNewObject).Wrap(indexErr)
			}
			if err := cas.EnsureCASIndexMatchesContentHash(objectID, data, targetBucketKey); err != nil {
				return err
			}
			return cas.commitLiveIdentity(objectID, newHash, newHashFile)
		})
	}

	newHash := CalculateSHA256Hash(data)

	if oldHash == newHash {
		return cas.ensureExactlyOneLiveBlob(objectID, newHash)
	}

	bucketKey := cas.index.GetBucketKey(objectID)
	oldHashFile, oldBucketDir := cas.findHashFile(oldHash, bucketKey)
	if oldHashFile == emptyValue {

		oldHashFile = filepath.Join(cas.kindDir, oldHash+".yaml")
		oldBucketDir = cas.kindDir
	}

	storageDir := oldBucketDir
	indexBucketKey := ""
	if len(targetBucketDir) > 0 && targetBucketDir[0] != emptyValue && targetBucketDir[0] != oldBucketDir {
		storageDir = targetBucketDir[0]
		if err := fileutil.MkdirAll(storageDir, paths.DirPerm755); err != nil {
			return errfmt.Newf(ConstMiscFailedToCreateTargetBucketDirectoryForMi).Wrap(err)
		}
		indexBucketKey = filepath.Base(storageDir)
	}
	newHashFile := filepath.Join(storageDir, newHash+".yaml")

	return cas.WithSweepLock(func() error {
		if err := cas.WriteFileWithSync(newHashFile, data); err != nil {
			return errfmt.Newf(ConstMiscFailedToWriteNewHashFile).Wrap(err)
		}

		if indexBucketKey != emptyValue {
			cas.SetIndexMappingInMemory(objectID, newHash, indexBucketKey)
		} else {
			cas.SetIndexMappingInMemory(objectID, newHash)
		}

		pendingCache := GetCASPendingVisibilityCache(projectRootFromCASKindDir(cas.kindDir))
		if pendingCache != nil {
			_ = pendingCache.PublishPending(objectID, cas.kind, newHash, indexBucketKey)
		}

		writeQueue := cas.getWriteQueue()
		opCallback := cas.getOperationCallback()
		var done <-chan error
		if GetSkipIndexUpdateWait() {
			_, err = writeQueue.EnqueueInternal(cas.kind, objectID, newHash, indexBucketKey, "", cas, opCallback, false, false)
		} else {
			done, err = writeQueue.EnqueueUpdateWithOperationCallback(cas.kind, objectID, newHash, indexBucketKey, cas, opCallback)
		}

		rollbackMutation := func() {
			if err := fileutil.RemoveFile(newHashFile); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
			cas.SetIndexMappingInMemory(objectID, oldHash)
			if cas.index != nil {
				RestorePendingAfterFailedMutation(
					projectRootFromCASIndexPath(cas.index.FilePath),
					objectID, cas.kind, oldHash, bucketKey,
				)
			}
		}

		if err != nil {
			rollbackMutation()
			return errfmt.Newf(ConstMiscFailedToQueueIndexUpdate).Wrap(err)
		}

		if done != nil {
			// Wait for index update to complete (with timeout so updates don't hang when queue is congested)
			var indexUpdateErr error
			select {
			case indexUpdateErr = <-done:
			case <-time.After(casIndexUpdateWaitTimeout):
				StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
					Warn(LogEventStorageCASIndexUpdateTimeoutCongestedWarn).
					Kind(cas.kind).
					ObjectID(objectID).
					String("timeout", casIndexUpdateWaitTimeout.String()).
					Log()

				cas.sweepAfterDurableBlob(objectID, newHash)
				return errfmt.Errorf(ConstMiscCasIndexUpdateDidNotCompleteWithinVQueue, casIndexUpdateWaitTimeout)
			}
			if indexUpdateErr != nil {
				rollbackMutation()
				return errfmt.Newf(ConstMiscFailedToUpdateCasIndex).Wrap(indexUpdateErr)
			}
		}

		// Index update succeeded - now safe to clean up old hash file via callback
		var cleanupCB OrphanCleanupCallback
		_ = concurrency.RunInRLockOrLog(&cas.Mu, locknames.LockNameCasGetOrphanCleanupCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
			cleanupCB = cas.orphanCleanupCB
			return nil
		})

		if cleanupCB != nil {
			if cleanupErr := cleanupCB(oldHashFile); cleanupErr != nil {
				_ = cleanupErr
			}
			if oldHash != newHash {
				EvictCASBlob(oldHash)
			}
		} else {
			if oldHash != newHash {
				EvictCASBlob(oldHash)
				if err := RemoveOrphanCASHashFileSync(oldHashFile); err != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
						Error(ErrMsgSwallowedError, err).Log()
				}
			}
		}

		if err := cas.EnsureCASIndexMatchesContentHash(objectID, data, indexBucketKey); err != nil {
			return err
		}

		return cas.commitLiveIdentity(objectID, newHash, newHashFile)
	})
}

func (cas *ContentAddressableStorage) ensureExactlyOneLiveBlob(objectID, keeperHash string) error {
	return ensureExactlyOneLiveCASBlob(objectID, keeperHash, cas.kindDir)
}

// sweepAfterDurableBlob enforces one live hash YAML per id after the new blob is on disk.
// Index-queue timeout used to return here without sweeping, which is how untracked
// restamps (updated_at-only) left dual CAS blobs.
func (cas *ContentAddressableStorage) sweepAfterDurableBlob(objectID, keeperHash string) {
	if err := cas.ensureExactlyOneLiveBlob(objectID, keeperHash); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
	}
}

func (cas *ContentAddressableStorage) UpdateWithIDChange(oldID, newID string, data []byte) error {
	if len(data) == 0 {
		return errfmt.Errorf(ConstMiscCannotUpdateObjectWithEmptyContent)
	}

	if oldID == newID {

		return cas.Update(newID, data)
	}

	oldHash, err := cas.index.GetHash(oldID)
	if err != nil {
		return errfmt.Errorf(ConstMiscObjectNotFoundWithOldIdSW, oldID, err)
	}

	newHash := CalculateSHA256Hash(data)

	bucketKey := cas.index.GetBucketKey(oldID)
	oldHashFile, oldBucketDir := cas.findHashFile(oldHash, bucketKey)
	if oldHashFile == emptyValue {

		oldHashFile = filepath.Join(cas.kindDir, oldHash+".yaml")
		oldBucketDir = cas.kindDir
	}

	storageDir := oldBucketDir
	newHashFile := filepath.Join(storageDir, newHash+".yaml")

	return cas.WithSweepLock(func() error {
		if oldHash != newHash {
			if err := cas.WriteFileWithSync(newHashFile, data); err != nil {
				return errfmt.Newf(ConstMiscFailedToWriteNewHashFile).Wrap(err)
			}
		}

		if err := cas.index.SetMapping(newID, newHash); err != nil {

			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Index update failed", err).Log()
		}

		if err := cas.index.RemoveMapping(oldID); err != nil {

			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Failed to remove mapping", err).Log()
		}

		if oldHash != newHash {
			EvictCASBlob(oldHash)
			if err := RemoveRetiredCASHashFileOnIDChange(oldHashFile); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
			}
		}

		if err := cas.EnsureCASIndexMatchesContentHash(newID, data, ""); err != nil {
			return err
		}

		if handler := GetCacheOperationHandler(); handler != nil {
			cacheCtx := &pkgctx.CacheContext{
				Operation: pkgctx.CacheOperationInvalidateAndUpdate,
				OldID:     oldID,
				NewID:     newID,
				Kind:      cas.kind,
				FilePath:  newHashFile,
			}
			if herr := handler(cacheCtx); herr != nil {
				return errfmt.Newf(ConstMiscIdentityCachePostSyncFailed).Wrap(herr)
			}
		} else {
			if err := cas.invokeCASPostSync(newID, newHash, newHashFile); err != nil {
				return err
			}
		}
		return cas.ensureExactlyOneLiveBlob(newID, newHash)
	})
}
