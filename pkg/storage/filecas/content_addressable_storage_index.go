package filecas

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

func isStreamKind(kind string) bool {
	return kind == objects.KindAuditEvent || kind == objects.KindChangeJournalEntry || kind == objects.KindZqkSession || kind == objects.KindAgentFeed
}

// Save saves the ID index to disk.
// Under the file lock, reload disk and merge so a stale in-memory snapshot cannot
// clobber a fresher index written by another process (sync-cas-index / heal).
// TRACK: follow-up in kernel backlog
func (idx *IDIndex) Save() error {
	if isStreamKind(idx.Kind) {
		return nil
	}
	if len(idx.Mappings) == 0 {
		if _, statErr := fileutil.Stat(filepath.Dir(idx.FilePath)); statErr != nil && fileutil.IsNotExist(statErr) {
			return nil
		}
	}
	metrics := getSafeMetrics()
	lockStart := time.Now()
	lockPath := idx.FilePath + ".lock"
	lockStrategy := getSafeLockStrategy()
	lockHandle, lockErr := lockStrategy.AcquireLock(lockPath, 30*time.Second)
	if lockErr != nil {
		metrics.RecordIndexFileLock(false, time.Since(lockStart))
		return errfmt.Newf(ConstMiscFailedToAcquireCasIndexLock).Wrap(lockErr)
	}
	metrics.RecordIndexFileLock(true, time.Since(lockStart))
	defer func() {
		if err := lockHandle.Release(); err != nil {
			logging.LogSwallowedError(err)
		}
	}()

	var mappingsCopy, bucketKeysCopy, createdAtsCopy map[string]string
	err := concurrency.RunInLockWithLogger(&idx.Mu, locknames.LockNameListingIndexSave, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		preserved := make(map[string]string, len(idx.Mappings))
		if idx.Mappings != nil {
			maps.Copy(preserved, idx.Mappings)
		}
		_ = idx.LoadLocked() //nolint:errcheck // empty/missing index is fine
		kindDir := filepath.Dir(idx.FilePath)
		idx.Mappings = MergeCASIndexMaps(kindDir, idx.Mappings, preserved)
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
		return errfmt.Newf(ConstMiscFailedToAcquireLockForIndexSave).Wrap(err)
	}
	return idx.SaveMappingsLocked(mappingsCopy, bucketKeysCopy, createdAtsCopy)
}

// saveMappingsLocked saves the provided mappings, optional bucket keys, and optional created_at to disk (must be called with lock held).
//
// IMPORTANT: "lock held" here means the caller holds the CAS index file lock (cross-process),
// not just idx.Mu. This avoids lost updates when multiple processes update the same index.
//
// Internal helper for use by the CAS index write queue worker.
// bucketKeys and createdAts may be nil; when non-nil they are written (created_at for OldestIDs).
func (idx *IDIndex) SaveMappingsLocked(mappings, bucketKeys, createdAts map[string]string) error {
	if isStreamKind(idx.Kind) {
		return nil
	}
	if len(mappings) == 0 {
		if _, statErr := fileutil.Stat(filepath.Dir(idx.FilePath)); statErr != nil && fileutil.IsNotExist(statErr) {
			return nil
		}
	}
	return idx.saveMappingsNoLock(mappings, bucketKeys, createdAts)
}

// saveMappingsNoLock saves the provided mappings, optional bucket keys, and optional created_at to disk (internal helper, no file locking).
// Caller MUST hold the CAS index file lock.
func (idx *IDIndex) saveMappingsNoLock(mappings, bucketKeys, createdAts map[string]string) error {
	if isStreamKind(idx.Kind) {
		return nil
	}
	if len(mappings) == 0 {
		if _, statErr := fileutil.Stat(filepath.Dir(idx.FilePath)); statErr != nil && fileutil.IsNotExist(statErr) {
			return nil
		}
	}
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
	if err := fileutil.MkdirAll(filepath.Dir(idx.FilePath), paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	metrics := getSafeMetrics()
	saveStart := time.Now()
	dir := filepath.Dir(idx.FilePath)
	tmpFile, err := fileutil.CreateTemp(dir, filepath.Base(idx.FilePath)+".tmp-*")
	if err != nil {
		metrics.RecordIndexSave(time.Since(saveStart), err, len(mappings))
		return errfmt.Newf(ConstMiscFailedToCreateTempIndexFile).Wrap(err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		logging.LogSwallowedError(tmpFile.Close())
		logging.LogSwallowedError(fileutil.RemoveFile(tmpName))
	}()

	if err := fileutil.Chmod(tmpName, paths.FilePerm644); err != nil && !fileutil.IsNotExist(err) {
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
	if err := fileutil.RenameFile(tmpName, idx.FilePath); err != nil {
		metrics.RecordIndexSave(time.Since(saveStart), err, len(mappings))
		return errfmt.Newf(ConstMiscFailedToRenameTempIndexIntoPlace).Wrap(err)
	}
	// Best-effort: remove abandoned sibling temps from killed mid-write processes.
	// TRACK: follow-up in kernel backlog
	cleanupStaleCASIndexTempFiles(dir, filepath.Base(idx.FilePath), tmpName)

	// Best-effort directory sync to make rename durable on disk.
	if dirFD, err := fileutil.Open(dir); err == nil {
		logging.LogSwallowedError(dirFD.Sync())
		logging.LogSwallowedError(dirFD.Close())
	}

	metrics.RecordIndexSave(time.Since(saveStart), nil, len(mappings))
	return nil
}

// GetHash returns the hash for a given object ID
func (idx *IDIndex) GetHash(id string) (string, error) {
	var hash string
	var found bool
	err := concurrency.RunInLockOrLog(&idx.Mu, locknames.LockNameListingIndexGetHash, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if idx.Mappings != nil {
			hash, found = idx.Mappings[id]
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found {
		return hash, nil
	}
	return "", errfmt.Errorf(ConstMiscIdNotFoundInIndexS, id)
}

// GetBucketKey returns the bucket key for an object ID (from bucket strategy at create time).
// Empty string means the object is in the base kind dir, not a bucket subdir.
func (idx *IDIndex) GetBucketKey(objectID string) string {
	var key string
	var err_swallow_11 = concurrency.RunInRLockWithLogger(&idx.Mu, locknames.LockNameListingIndexGetBucketKey, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
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
	metrics := getSafeMetrics()
	reloaded := false

	lockStart := time.Now()
	lockPath := idx.FilePath + ".lock"
	lockStrategy := getSafeLockStrategy()
	lockHandle, lockErr := lockStrategy.AcquireLock(lockPath, 30*time.Second)
	if lockErr != nil {
		metrics.RecordIndexFileLock(false, time.Since(lockStart))
		err := errfmt.Newf(ConstMiscFailedToAcquireCasIndexLock).Wrap(lockErr)
		metrics.RecordSetMapping(time.Since(start), err, false)
		return err
	}
	metrics.RecordIndexFileLock(true, time.Since(lockStart))
	defer func() {
		logging.LogSwallowedError(lockHandle.Release())
	}()

	var mappingsCopy, bucketKeysCopy, createdAtsCopy map[string]string
	err := concurrency.RunInLockWithLogger(&idx.Mu, locknames.LockNameListingIndexSetMapping, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		// Snapshot pre-load memory, then reload disk. Do NOT blanket-overlay memory onto
		// disk — that reverted sync-cas-index / heal when system check held a stale CAS.
		// TRACK: follow-up in kernel backlog
		preservedMappings := make(map[string]string)
		if idx.Mappings != nil {
			maps.Copy(preservedMappings, idx.Mappings)
		}

		if loadErr := idx.LoadLocked(); loadErr == nil {
			reloaded = true
			metrics.RecordIndexReload()
		}

		kindDir := filepath.Dir(idx.FilePath)
		idx.Mappings = MergeCASIndexMaps(kindDir, idx.Mappings, preservedMappings)
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
	saveErr := idx.SaveMappingsLocked(mappingsCopy, bucketKeysCopy, createdAtsCopy)
	metrics.RecordSetMapping(time.Since(start), saveErr, reloaded)
	if saveErr == nil {
		if ConfirmPendingAfterDurableMapping != nil {
			ConfirmPendingAfterDurableMapping(idx.FilePath, objectID, hash)
		}
	}
	return saveErr
}

// SetMappings sets multiple ID->hash mappings (and optional bucket keys and optional created_at) in one load/merge/save.
// Use this instead of multiple SetMapping calls when warming from cache to avoid O(n) index reads/writes.
// createdAts is optional: ID -> RFC3339 for high-volume kinds.
// When the merged result matches the on-disk index already loaded, the durable save is skipped
// (system check warm was rewriting every kind index every run — multi-second stall).
// TRACK: warm no-op; keep when: check warm stays <1s on warm caches.
func (idx *IDIndex) SetMappings(mappings, bucketKeys map[string]string, createdAts ...map[string]string) error {
	if len(mappings) == 0 {
		return nil
	}
	start := time.Now()
	metrics := getSafeMetrics()

	lockStart := time.Now()
	lockPath := idx.FilePath + ".lock"
	lockStrategy := getSafeLockStrategy()
	lockHandle, lockErr := lockStrategy.AcquireLock(lockPath, 30*time.Second)
	if lockErr != nil {
		metrics.RecordIndexFileLock(false, time.Since(lockStart))
		err := errfmt.Newf(ConstMiscFailedToAcquireCasIndexLock).Wrap(lockErr)
		metrics.RecordSetMapping(time.Since(start), err, false)
		return err
	}
	metrics.RecordIndexFileLock(true, time.Since(lockStart))
	defer func() {
		logging.LogSwallowedError(lockHandle.Release())
	}()

	var mappingsCopy, bucketKeysCopy, createdAtsCopy map[string]string
	reloaded := false
	unchanged := false
	err := concurrency.RunInLockWithLogger(&idx.Mu, locknames.LockNameListingIndexSetMappings, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		// Disk-preferred merge after reload (same as SetMapping). Explicit `mappings` win last.
		// TRACK: follow-up in kernel backlog
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

		var diskMappings, diskBucketKeys, diskCreatedAt map[string]string
		if loadErr := idx.LoadLocked(); loadErr == nil {
			reloaded = true
			metrics.RecordIndexReload()

			// Snapshot post-load disk state before merge so we can skip no-op saves.
			diskMappings = make(map[string]string, len(idx.Mappings))
			if idx.Mappings != nil {
				maps.Copy(diskMappings, idx.Mappings)
			}
			if len(idx.BucketKeys) > 0 {
				diskBucketKeys = make(map[string]string, len(idx.BucketKeys))
				maps.Copy(diskBucketKeys, idx.BucketKeys)
			}
			if len(idx.CreatedAt) > 0 {
				diskCreatedAt = make(map[string]string, len(idx.CreatedAt))
				maps.Copy(diskCreatedAt, idx.CreatedAt)
			}
		}

		kindDir := filepath.Dir(idx.FilePath)
		idx.Mappings = MergeCASIndexMaps(kindDir, idx.Mappings, preservedMappings)
		for id, hash := range mappings {
			idx.Mappings[id] = hash
		}
		if preservedBucketKeys != nil || bucketKeys != nil {
			if idx.BucketKeys == nil {
				idx.BucketKeys = make(map[string]string)
			}
			// Bucket keys: keep disk, fill gaps from memory, then apply explicit.
			for id, key := range preservedBucketKeys {
				if _, ok := idx.BucketKeys[id]; !ok {
					idx.BucketKeys[id] = key
				}
			}
			for id, key := range bucketKeys {
				idx.BucketKeys[id] = key
			}
		}
		if preservedCreatedAt != nil || (len(createdAts) > 0 && createdAts[0] != nil) {
			if idx.CreatedAt == nil {
				idx.CreatedAt = make(map[string]string)
			}
			for id, t := range preservedCreatedAt {
				if _, ok := idx.CreatedAt[id]; !ok {
					idx.CreatedAt[id] = t
				}
			}
			if len(createdAts) > 0 && createdAts[0] != nil {
				for id, t := range createdAts[0] {
					idx.CreatedAt[id] = t
				}
			}
		}

		if reloaded &&
			StringMapsEqual(idx.Mappings, diskMappings) &&
			StringMapsEqual(idx.BucketKeys, diskBucketKeys) &&
			StringMapsEqual(idx.CreatedAt, diskCreatedAt) {
			unchanged = true
			return nil
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
	if unchanged {
		metrics.RecordSetMapping(time.Since(start), nil, reloaded)
		return nil
	}

	saveErr := idx.SaveMappingsLocked(mappingsCopy, bucketKeysCopy, createdAtsCopy)
	metrics.RecordSetMapping(time.Since(start), saveErr, reloaded)
	if saveErr == nil && ConfirmPendingAfterDurableMapping != nil {
		for id, hash := range mappings {
			ConfirmPendingAfterDurableMapping(idx.FilePath, id, hash)
		}
	}
	return saveErr
}

// StringMapsEqual reports whether two string maps have the same keys and values.
// Nil and empty maps are treated as equal.
func StringMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || vb != va {
			return false
		}
	}
	return true
}

// RemoveMapping removes an object ID from the index
func (idx *IDIndex) RemoveMapping(objectID string) error {
	start := time.Now()
	metrics := getSafeMetrics()

	lockStart := time.Now()
	lockPath := idx.FilePath + ".lock"
	lockStrategy := getSafeLockStrategy()
	lockHandle, lockErr := lockStrategy.AcquireLock(lockPath, 30*time.Second)
	if lockErr != nil {
		metrics.RecordIndexFileLock(false, time.Since(lockStart))
		return errfmt.Newf(ConstMiscFailedToAcquireCasIndexLock).Wrap(lockErr)
	}
	metrics.RecordIndexFileLock(true, time.Since(lockStart))
	defer func() {
		logging.LogSwallowedError(lockHandle.Release())
	}()

	var mappingsCopy map[string]string
	var bucketKeysCopy map[string]string
	var createdAtsCopy map[string]string
	err := concurrency.RunInLockWithLogger(&idx.Mu, locknames.LockNameListingIndexRemoveMapping, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		logging.LogSwallowedError(idx.LoadLocked())
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
	saveErr := idx.SaveMappingsLocked(mappingsCopy, bucketKeysCopy, createdAtsCopy)
	metrics.RecordRemoveMapping(time.Since(start), saveErr)
	return saveErr
}
