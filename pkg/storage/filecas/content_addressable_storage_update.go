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

	var wantTargetDir string
	if len(targetBucketDir) > 0 && targetBucketDir[0] != emptyValue {
		wantTargetDir = targetBucketDir[0]
	}

	return cas.WithSweepLock(func() error {
		oldHash, err := cas.GetHashForID(objectID)
		if err != nil {
			return cas.updateNewObjectLocked(objectID, data, wantTargetDir)
		}
		return cas.updateExistingObjectLocked(objectID, data, oldHash, wantTargetDir)
	})
}

func (cas *ContentAddressableStorage) updateNewObjectLocked(objectID string, data []byte, wantTargetDir string) error {
	newHash := CalculateSHA256Hash(data)

	storageDir := cas.kindDir
	if wantTargetDir != emptyValue {
		storageDir = wantTargetDir
		if mkErr := fileutil.MkdirAll(storageDir, paths.DirPerm755); mkErr != nil {
			return errfmt.Newf(ConstMiscFailedToCreateTargetBucketDirectory).Wrap(mkErr)
		}
	}
	newHashFile := filepath.Join(storageDir, newHash+".yaml")

	if _, statErr := fileutil.Stat(newHashFile); statErr != nil {
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
	if writeQueue == nil {
		enqErr = errfmt.Errorf("index write queue unavailable")
	} else if targetBucketKey != emptyValue {
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
}

func (cas *ContentAddressableStorage) updateExistingObjectLocked(objectID string, data []byte, oldHash, wantTargetDir string) error {
	newHash := CalculateSHA256Hash(data)

	if oldHash == newHash {
		return cas.ensureExactlyOneLiveBlob(objectID, newHash)
	}

	oldHashFile, oldBucketDir := cas.resolveOldHashFileAndBucket(oldHash, objectID)

	storageDir := oldBucketDir
	indexBucketKey := ""
	if wantTargetDir != emptyValue && wantTargetDir != oldBucketDir {
		storageDir = wantTargetDir
		if err := fileutil.MkdirAll(storageDir, paths.DirPerm755); err != nil {
			return errfmt.Newf(ConstMiscFailedToCreateTargetBucketDirectoryForMi).Wrap(err)
		}
		indexBucketKey = filepath.Base(storageDir)
	}
	newHashFile := filepath.Join(storageDir, newHash+".yaml")

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

	done, err := cas.enqueueIndexWrite(objectID, newHash, indexBucketKey, "")

	rollbackMutation := func() {
		if err := fileutil.RemoveFile(newHashFile); err != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, err).Log()
		}
		cas.SetIndexMappingInMemory(objectID, oldHash)
		if cas.index != nil {
			RestorePendingAfterFailedMutation(
				projectRootFromCASIndexPath(cas.index.FilePath),
				objectID, cas.kind, oldHash, indexBucketKey,
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

	// Fail-closed pre-delete gate (GH-1255): verify the new hash blob exists on disk
	// before deleting the superseded hash file. If a concurrent sweep or race removed
	// newHashFile, re-materialize it first; never delete oldHashFile without a durable new blob.
	if err := cas.ensureDurableHashFileOnDisk(newHashFile, data); err != nil {
		return err
	}

	// Index update succeeded and new hash blob is verified on disk - safe to clean up old hash file.
	var cleanupCB OrphanCleanupCallback
	_ = concurrency.RunInRLockOrLog(&cas.Mu, locknames.LockNameCasGetOrphanCleanupCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		cleanupCB = cas.orphanCleanupCB
		return nil
	})

	if oldHash != newHash && oldHashFile != newHashFile {
		if cleanupCB != nil {
			if cleanupErr := cleanupCB(oldHashFile); cleanupErr != nil {
				_ = cleanupErr
			}
			EvictCASBlob(oldHash)
		} else {
			EvictCASBlob(oldHash)
			if err := RemoveOrphanCASHashFileSync(oldHashFile); err != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
					Error(ErrMsgSwallowedError, err).Log()
			}
		}
	}

	// Fail-closed post-cleanup gate (GH-1255): confirm newHashFile is still present on disk
	// before committing live identity and returning success.
	if _, statErr := fileutil.Stat(newHashFile); statErr != nil {
		return errfmt.Newf("CAS update verification failed: new hash file %s missing after cleanup", newHashFile).Wrap(statErr)
	}

	if err := cas.EnsureCASIndexMatchesContentHash(objectID, data, indexBucketKey); err != nil {
		return err
	}

	return cas.commitLiveIdentity(objectID, newHash, newHashFile)
}

// ensureDurableHashFileOnDisk verifies newHashFile exists on disk, re-materializing it
// via WriteFileWithSync if missing, and failing closed if still absent.
func (cas *ContentAddressableStorage) ensureDurableHashFileOnDisk(newHashFile string, data []byte) error {
	if _, statErr := fileutil.Stat(newHashFile); statErr == nil {
		return nil
	}
	if err := cas.WriteFileWithSync(newHashFile, data); err != nil {
		return errfmt.Newf("CAS pre-delete verification failed: could not re-materialize %s", newHashFile).Wrap(err)
	}
	if _, statErr := fileutil.Stat(newHashFile); statErr != nil {
		return errfmt.Newf("CAS pre-delete verification failed: new hash file %s missing on disk", newHashFile).Wrap(statErr)
	}
	return nil
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

	oldHashFile, oldBucketDir := cas.resolveOldHashFileAndBucket(oldHash, oldID)

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

func (cas *ContentAddressableStorage) resolveOldHashFileAndBucket(oldHash, objectID string) (string, string) {
	bucketKey := cas.index.GetBucketKey(objectID)
	oldHashFile, oldBucketDir := cas.findHashFile(oldHash, bucketKey)
	if oldHashFile == emptyValue {
		oldHashFile = filepath.Join(cas.kindDir, oldHash+".yaml")
		oldBucketDir = cas.kindDir
	}
	return oldHashFile, oldBucketDir
}
