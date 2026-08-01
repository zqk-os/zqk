package storage

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// Save saves the ID index to disk
// CRITICAL: This method reads the current state and saves it atomically
func (idx *IDIndex) Save() error {
	var mappingsCopy map[string]string
	err := concurrency.RunInRLockWithLogger(&idx.mu, locknames.LockNameListingIndexSave, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		mappingsCopy = make(map[string]string, len(idx.Mappings))
		maps.Copy(mappingsCopy, idx.Mappings)
		return nil
	})
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToAcquireLockForIndexSave).Wrap(err)
	}

	// Save the copied mappings (no lock needed since we have a snapshot)
	return idx.saveMappings(mappingsCopy)
}

// saveMappingsLocked saves the provided mappings, optional bucket keys, and optional created_at to disk (must be called with lock held).
//
// IMPORTANT: "lock held" here means the caller holds the CAS index file lock (cross-process),
// not just idx.mu. This avoids lost updates when multiple processes update the same index.
//
// Internal helper for use by the CAS index write queue worker.
// bucketKeys and createdAts may be nil; when non-nil they are written (created_at for OldestIDs).
func (idx *IDIndex) saveMappingsLocked(mappings, bucketKeys, createdAts map[string]string) error {
	return idx.saveMappingsNoLock(mappings, bucketKeys, createdAts)
}

// saveMappings saves the provided mappings to disk (internal helper).
// Acquires the CAS index file lock to serialize writes across processes.
// Uses current idx.BucketKeys and idx.CreatedAt when saving (for callers that only pass mappings).
func (idx *IDIndex) saveMappings(mappings map[string]string) error {
	metrics := GetObjectStorageMetrics()
	lockStart := time.Now()
	lockPath := idx.filePath + ".lock"

	lockStrategy := NewAutoCleanupStrategy()
	lockHandle, err := lockStrategy.AcquireLock(lockPath, 5*time.Second)
	if err != nil {
		metrics.RecordIndexFileLock(false, time.Since(lockStart))
		return errfmt.Newf(ConstMiscFailedToAcquireCasIndexLock).Wrap(err)
	}
	metrics.RecordIndexFileLock(true, time.Since(lockStart))
	defer func() {
		var err_swallow_6 = lockHandle.Release()
		if err_swallow_6 != nil {
			logging.LogSwallowedError(err_swallow_6)
		}
	}()

	var bucketKeysCopy, createdAtsCopy map[string]string
	var err_swallow_5 = concurrency.RunInRLockWithLogger(&idx.mu, locknames.LockNameListingIndexSaveBucketKeys, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if len(idx.BucketKeys) > 0 {
			bucketKeysCopy = make(map[string]string, len(idx.BucketKeys))
			maps.Copy(bucketKeysCopy, idx.BucketKeys)
		}
		if len(idx.CreatedAt) > 0 {
			createdAtsCopy = make(map[string]string, len(idx.CreatedAt))
			maps.Copy(createdAtsCopy, idx.CreatedAt)
		}
		return nil
	})
	if err_swallow_5 != nil {
		logging.LogSwallowedError(err_swallow_5)
	}
	return idx.saveMappingsNoLock(mappings, bucketKeysCopy, createdAtsCopy)
}

// saveMappingsNoLock saves the provided mappings, optional bucket keys, and optional created_at to disk (internal helper, no file locking).
// Caller MUST hold the CAS index file lock.
func (idx *IDIndex) saveMappingsNoLock(mappings, bucketKeys, createdAts map[string]string) error {
	// Create index structure for marshaling
	// CRITICAL: Access Version and Kind without lock (they're immutable after creation)
	indexData := struct {
		Version    string            `json:"version"`
		Kind       string            `json:"kind"`
		Mappings   map[string]string `json:"mappings"`
		BucketKeys map[string]string `json:"bucket_keys,omitempty"`
		CreatedAt  map[string]string `json:"created_at,omitempty"`
	}{
		Version:    idx.Version,
		Kind:       idx.Kind,
		Mappings:   mappings,
		BucketKeys: bucketKeys,
		CreatedAt:  createdAts,
	}

	// Use json.Marshal (not MarshalIndent) for smaller allocation and faster I/O on large indexes.
	data, err := json.Marshal(indexData)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToMarshalIndex).Wrap(err)
	}
	data = append(data, '\n')

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(idx.filePath), paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	metrics := GetObjectStorageMetrics()
	saveStart := time.Now()
	dir := filepath.Dir(idx.filePath)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(idx.filePath)+".tmp-*")
	if err != nil {
		metrics.RecordIndexSave(time.Since(saveStart), err, len(mappings))
		return errfmt.Newf(ConstMiscFailedToCreateTempIndexFile).Wrap(err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		var err_swallow_7 = tmpFile.Close()
		if err_swallow_7 != nil {
			logging.LogSwallowedError(err_swallow_7)
		}
		var err_swallow_8 = os.Remove(tmpName)
		if err_swallow_8 != nil {
			logging.LogSwallowedError(err_swallow_8)
		}
	}()

	if err := os.Chmod(tmpName, paths.FilePerm644); err != nil && !os.IsNotExist(err) {
		metrics.RecordIndexSave(time.Since(saveStart), err, len(mappings))
		return errfmt.Newf(ConstMiscFailedToChmodTempIndexFile).Wrap(err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		metrics.RecordIndexSave(time.Since(saveStart), err, len(mappings))
		return errfmt.Newf(ConstMiscFailedToWriteTempIndex).Wrap(err)
	}

	if err := tmpFile.Sync(); err != nil {
		metrics.RecordIndexSave(time.Since(saveStart), err, len(mappings))
		return errfmt.Newf(ConstMiscFailedToSyncTempIndex).Wrap(err)
	}

	if err := tmpFile.Close(); err != nil {
		metrics.RecordIndexSave(time.Since(saveStart), err, len(mappings))
		return errfmt.Newf(ConstMiscFailedToCloseTempIndex).Wrap(err)
	}

	// Atomic replace (POSIX): readers should never observe a partial index file.
	if err := os.Rename(tmpName, idx.filePath); err != nil {
		metrics.RecordIndexSave(time.Since(saveStart), err, len(mappings))
		return errfmt.Newf(ConstMiscFailedToRenameTempIndexIntoPlace).Wrap(err)
	}

	// Best-effort directory sync to make rename durable on disk.
	if dirFD, err := os.Open(dir); err == nil {
		var err_swallow_9 = dirFD.Sync()
		if err_swallow_9 != nil {
			logging.LogSwallowedError(err_swallow_9)
		}
		var err_swallow_10 = dirFD.Close()
		if err_swallow_10 != nil {
			logging.LogSwallowedError(err_swallow_10)
		}
	}

	metrics.RecordIndexSave(time.Since(saveStart), nil, len(mappings))
	return nil
}

// GetHash returns the hash for a given object ID
func (idx *IDIndex) GetHash(objectID string) (string, error) {
	var hash string
	var exists bool
	err := concurrency.RunInRLockWithLogger(&idx.mu, locknames.LockNameListingIndexGetHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var ok bool
		hash, ok = idx.Mappings[objectID]
		exists = ok
		return nil
	})
	if err != nil {
		return "", errfmt.Newf("getting hash").Wrap(err)
	}
	if !exists {
		return "", errfmt.Errorf(ConstMiscIdNotFoundInIndexS, objectID)
	}

	return hash, nil
}

// GetBucketKey returns the bucket key for an object ID (from bucket strategy at create time).
// Empty string means the object is in the base kind dir, not a bucket subdir.
func (idx *IDIndex) GetBucketKey(objectID string) string {
	var key string
	var err_swallow_11 = concurrency.RunInRLockWithLogger(&idx.mu, locknames.LockNameListingIndexGetBucketKey, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if idx.BucketKeys != nil {
			key = idx.BucketKeys[objectID]
		}
		return nil
	})
	if err_swallow_11 !=

		// SetMapping sets the hash and optional bucket key and optional created_at for an object ID
		// CRITICAL: This must be atomic - load existing index, merge, and save
		// bucketKey is from the bucket strategy at create time; empty means base dir.
		// createdAtOpt is RFC3339 string for high-volume kinds (enables OldestIDs); pass at most one extra string after bucketKey.
		nil {
		logging.LogSwallowedError(err_swallow_11)
	}
	return key
}

func (idx *IDIndex) SetMapping(objectID, hash string, bucketKey ...string) error {
	start := time.Now()
	metrics := GetObjectStorageMetrics()
	reloaded := false

	lockStart := time.Now()
	lockPath := idx.filePath + ".lock"
	lockStrategy := NewAutoCleanupStrategy()
	lockHandle, lockErr := lockStrategy.AcquireLock(lockPath, 5*time.Second)
	if lockErr != nil {
		metrics.RecordIndexFileLock(false, time.Since(lockStart))
		err := errfmt.Newf(ConstMiscFailedToAcquireCasIndexLock).Wrap(lockErr)
		metrics.RecordSetMapping(time.Since(start), err, false)
		return err
	}
	metrics.RecordIndexFileLock(true, time.Since(lockStart))
	defer func() {
		var err_swallow_12 = lockHandle.Release()
		if err_swallow_12 != nil {
			logging.LogSwallowedError(err_swallow_12)
		}
	}()

	var mappingsCopy, bucketKeysCopy, createdAtsCopy map[string]string
	err := concurrency.RunInLockWithLogger(&idx.mu, locknames.LockNameListingIndexSetMapping, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		preservedMappings := make(map[string]string)
		if idx.Mappings != nil {
			maps.Copy(preservedMappings, idx.Mappings)
		}

		if loadErr := idx.loadLocked(); loadErr == nil {
			reloaded = true
			metrics.RecordIndexReload()
		}

		if idx.Mappings == nil {
			idx.Mappings = make(map[string]string)
		}
		maps.Copy(idx.Mappings, preservedMappings)
		idx.Mappings[objectID] = hash

		if len(bucketKey) > 0 && bucketKey[0] != emptyValue {
			if idx.BucketKeys == nil {
				idx.BucketKeys = make(map[string]string)
			}
			idx.BucketKeys[objectID] = bucketKey[0]
		}
		// Optional created_at: variadic [bucketKey, createdAt]; if len==2 and second non-empty, set CreatedAt
		if len(bucketKey) >= 2 && bucketKey[1] != emptyValue {
			if idx.CreatedAt == nil {
				idx.CreatedAt = make(map[string]string)
			}
			idx.CreatedAt[objectID] = bucketKey[1]
		} else if len(bucketKey) == 1 && bucketKey[0] != emptyValue {
			// Only bucket key passed
		}

		mappingsCopy = make(map[string]string, len(idx.Mappings))
		maps.Copy(mappingsCopy, idx.Mappings)
		if len(idx.BucketKeys) > 0 {
			bucketKeysCopy = make(map[string]string, len(idx.BucketKeys))
			maps.Copy(bucketKeysCopy, idx.BucketKeys)
		}
		if len(idx.CreatedAt) > 0 {
			createdAtsCopy = make(map[string]string, len(idx.CreatedAt))
			maps.Copy(createdAtsCopy, idx.CreatedAt)
		}
		return nil
	})
	if err != nil {
		return errfmt.Newf(ConstMiscSettingMapping).Wrap(err)
	}

	// Save snapshot while still holding the file lock (cross-process atomicity).
	saveErr := idx.saveMappingsLocked(mappingsCopy, bucketKeysCopy, createdAtsCopy)
	metrics.RecordSetMapping(time.Since(start), saveErr, reloaded)
	return saveErr
}

// SetMappings sets multiple ID->hash mappings (and optional bucket keys and optional created_at) in one load/merge/save.
// Use this instead of multiple SetMapping calls when warming from cache to avoid O(n) index reads/writes.
// createdAts is optional: ID -> RFC3339 for high-volume kinds.
func (idx *IDIndex) SetMappings(mappings, bucketKeys map[string]string, createdAts ...map[string]string) error {
	if len(mappings) == 0 {
		return nil
	}
	start := time.Now()
	metrics := GetObjectStorageMetrics()

	lockStart := time.Now()
	lockPath := idx.filePath + ".lock"
	lockStrategy := NewAutoCleanupStrategy()
	lockHandle, lockErr := lockStrategy.AcquireLock(lockPath, 5*time.Second)
	if lockErr != nil {
		metrics.RecordIndexFileLock(false, time.Since(lockStart))
		err := errfmt.Newf(ConstMiscFailedToAcquireCasIndexLock).Wrap(lockErr)
		metrics.RecordSetMapping(time.Since(start), err, false)
		return err
	}
	metrics.RecordIndexFileLock(true, time.Since(lockStart))
	defer func() {
		var err_swallow_13 = lockHandle.Release()
		if err_swallow_13 != nil {
			logging.LogSwallowedError(err_swallow_13)
		}
	}()

	var mappingsCopy, bucketKeysCopy, createdAtsCopy map[string]string
	reloaded := false
	err := concurrency.RunInLockWithLogger(&idx.mu, locknames.LockNameListingIndexSetMappings, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		// Same as SetMapping: preserve in-memory index before loadLocked. Otherwise a reload from
		// disk (which can lag enqueue/setIndexMappingInMemory) wipes same-process updates. Batch
		// callers (e.g. EnsureCASIndexPopulatedFromScan) then merge scan results on top.
		preservedMappings := make(map[string]string)
		if idx.Mappings != nil {
			maps.Copy(preservedMappings, idx.Mappings)
		}
		var preservedBucketKeys map[string]string
		if len(idx.BucketKeys) > 0 {
			preservedBucketKeys = make(map[string]string, len(idx.BucketKeys))
			maps.Copy(preservedBucketKeys, idx.BucketKeys)
		}
		var preservedCreatedAt map[string]string
		if len(idx.CreatedAt) > 0 {
			preservedCreatedAt = make(map[string]string, len(idx.CreatedAt))
			maps.Copy(preservedCreatedAt, idx.CreatedAt)
		}

		if loadErr := idx.loadLocked(); loadErr == nil {
			reloaded = true
			metrics.RecordIndexReload()
		}

		if idx.Mappings == nil {
			idx.Mappings = make(map[string]string)
		}
		maps.Copy(idx.Mappings, preservedMappings)
		for id, hash := range mappings {
			idx.Mappings[id] = hash
		}
		if preservedBucketKeys != nil || bucketKeys != nil {
			if idx.BucketKeys == nil {
				idx.BucketKeys = make(map[string]string)
			}
			if preservedBucketKeys != nil {
				maps.Copy(idx.BucketKeys, preservedBucketKeys)
			}
			for id, key := range bucketKeys {
				idx.BucketKeys[id] = key
			}
		}
		if preservedCreatedAt != nil || (len(createdAts) > 0 && createdAts[0] != nil) {
			if idx.CreatedAt == nil {
				idx.CreatedAt = make(map[string]string)
			}
			if preservedCreatedAt != nil {
				maps.Copy(idx.CreatedAt, preservedCreatedAt)
			}
			if len(createdAts) > 0 && createdAts[0] != nil {
				for id, t := range createdAts[0] {
					idx.CreatedAt[id] = t
				}
			}
		}
		mappingsCopy = make(map[string]string, len(idx.Mappings))
		maps.Copy(mappingsCopy, idx.Mappings)
		if len(idx.BucketKeys) > 0 {
			bucketKeysCopy = make(map[string]string, len(idx.BucketKeys))
			maps.Copy(bucketKeysCopy, idx.BucketKeys)
		}
		if len(idx.CreatedAt) > 0 {
			createdAtsCopy = make(map[string]string, len(idx.CreatedAt))
			maps.Copy(createdAtsCopy, idx.CreatedAt)
		}
		return nil
	})
	if err != nil {
		return errfmt.Newf(ConstMiscSettingMappings).Wrap(err)
	}

	saveErr := idx.saveMappingsLocked(mappingsCopy, bucketKeysCopy, createdAtsCopy)
	metrics.RecordSetMapping(time.Since(start), saveErr, reloaded)
	return saveErr
}

// RemoveMapping removes an object ID from the index
func (idx *IDIndex) RemoveMapping(objectID string) error {
	start := time.Now()
	metrics := GetObjectStorageMetrics()

	lockStart := time.Now()
	lockPath := idx.filePath + ".lock"
	lockStrategy := NewAutoCleanupStrategy()
	lockHandle, lockErr := lockStrategy.AcquireLock(lockPath, 5*time.Second)
	if lockErr != nil {
		metrics.RecordIndexFileLock(false, time.Since(lockStart))
		return errfmt.Newf(ConstMiscFailedToAcquireCasIndexLock).Wrap(lockErr)
	}
	metrics.RecordIndexFileLock(true, time.Since(lockStart))
	defer func() {
		var err_swallow_14 = lockHandle.Release()
		if err_swallow_14 != nil {
			logging.LogSwallowedError(err_swallow_14)
		}
	}()

	var mappingsCopy map[string]string
	var bucketKeysCopy map[string]string
	var createdAtsCopy map[string]string
	err := concurrency.RunInLockWithLogger(&idx.mu, locknames.LockNameListingIndexRemoveMapping, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		var err_swallow_15 = idx.loadLocked()
		if err_swallow_15 != nil {
			logging.LogSwallowedError(err_swallow_15)
		}
		delete(idx.Mappings, objectID)
		if idx.BucketKeys != nil {
			delete(idx.BucketKeys, objectID)
		}
		if idx.CreatedAt != nil {
			delete(idx.CreatedAt, objectID)
		}
		mappingsCopy = make(map[string]string, len(idx.Mappings))
		maps.Copy(mappingsCopy, idx.Mappings)
		if len(idx.BucketKeys) > 0 {
			bucketKeysCopy = make(map[string]string, len(idx.BucketKeys))
			maps.Copy(bucketKeysCopy, idx.BucketKeys)
		}
		if len(idx.CreatedAt) > 0 {
			createdAtsCopy = make(map[string]string, len(idx.CreatedAt))
			maps.Copy(createdAtsCopy, idx.CreatedAt)
		}
		return nil
	})
	if err != nil {
		return errfmt.Newf(ConstMiscTimeoutRemovingMapping).Wrap(err)
	}

	// Save snapshot while still holding the file lock (cross-process atomicity).
	saveErr := idx.saveMappingsLocked(mappingsCopy, bucketKeysCopy, createdAtsCopy)
	metrics.RecordRemoveMapping(time.Since(start), saveErr)
	return saveErr
}
