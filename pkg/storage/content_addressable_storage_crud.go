package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/locknames"
	"gopkg.in/yaml.v3"
)

const (
	// casReadDirPoolWorkers kept low to reduce readdir CPU (see INDEX_FIRST_LOW_CPU_SCAN_DESIGN.md).
	// Sampling showed __getdirentries64 as a major hotspot; 16 workers still allows parallel CAS lookups without dominating CPU.
	casReadDirPoolWorkers = 16
	casReadDirPoolQueue   = 256

	// casIndexUpdateWaitTimeout limits how long Update waits for the index write queue.
	// Prevents scheduler_job (and other CAS) updates from hanging when the queue is congested (e.g. daemon + CLI).
	casIndexUpdateWaitTimeout = 30 * time.Second
)

var (
	casReadDirPoolOnce sync.Once
	casReadDirPool     *goroutinelabels.Pool

	skipIndexUpdateWait atomic.Bool
)

// SetSkipIndexUpdateWait toggles whether CAS Create/Update waits for the listing index write queue.
// When enabled, index updates are queued asynchronously without blocking, suitable for bulk state-restore.
func SetSkipIndexUpdateWait(val bool) {
	skipIndexUpdateWait.Store(val)
}

func getSkipIndexUpdateWait() bool {
	return skipIndexUpdateWait.Load()
}

// getCASReadDirPool returns the process-wide bounded pool for CAS bucket-search ReadDir.
// Uses the same bounded-pool pattern as list/count and triggered jobs (see .zqk/unbounded-concurrency-fixes.md §10).
func getCASReadDirPool() *goroutinelabels.Pool {
	casReadDirPoolOnce.Do(func() {
		bud := goroutinelabels.DefaultBudget()
		casReadDirPool = goroutinelabels.NewPool(bud, "cas_readdir", ConstMiscCasBucketSearchReaddirWithTimeout, casReadDirPoolWorkers, casReadDirPoolQueue)
		casReadDirPool.Start(context.Background())
	})
	return casReadDirPool
}

// isHighVolumeKindForIndex returns true for kinds that store created_at in the CAS index (enables OldestIDs).
func isHighVolumeKindForIndex(kind string) bool {
	return kind == objects.KindAuditEvent || kind == objects.KindChangeJournalEntry || kind == objects.KindMcpSession ||
		(len(kind) > 7 && kind[len(kind)-7:] == "_metric")
}

// parseCreatedAtFromObjectData extracts created_at from YAML/JSON object data; returns RFC3339 or empty.
func parseCreatedAtFromObjectData(data []byte) string {
	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return ""
	}
	if v, ok := obj[objects.FieldKeyCreatedAt]; !ok {
		return ""
	} else if s, ok := v.(string); ok && s != emptyValue {
		return s
	}
	return ""
}

// Create creates an object using content-addressable storage
// Hash is calculated from content BEFORE writing (Git's approach)
// bucketDir is optional - if provided, used instead of cas.kindDir for file storage (supports bucketed storage)
func (cas *ContentAddressableStorage) Create(objectID string, data []byte, bucketDir ...string) error {
	start := time.Now()
	metrics := GetObjectStorageMetrics()

	if len(data) == 0 {
		err := errfmt.Errorf(ConstMiscCannotCreateObjectWithEmptyContent)
		metrics.RecordCreate(time.Since(start), err)
		return err
	}

	// Calculate hash from content BEFORE writing (Git's approach)
	hash := CalculateSHA256Hash(data)

	// Determine storage directory: use bucketDir if provided (from bucket strategy), otherwise use cas.kindDir
	storageDir := cas.kindDir
	bucketKey := ""
	if len(bucketDir) > 0 && bucketDir[0] != emptyValue {
		storageDir = bucketDir[0]
		bucketKey = filepath.Base(storageDir) // strategy stores under kindDir/bucketKey
	}

	// Ensure directory exists
	if err := os.MkdirAll(storageDir, paths.DirPerm755); err != nil {
		metrics.RecordCreate(time.Since(start), err)
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	// Write file with hash as filename
	hashFile := filepath.Join(storageDir, hash+".yaml")
	if err := cas.writeFileWithSync(hashFile, data); err != nil {
		metrics.RecordCreate(time.Since(start), err)
		return errfmt.Newf(ConstMiscFailedToWriteHashFile).Wrap(err)
	}

	// Call post-sync callback if registered (after sync succeeds, before index update)
	var postSyncCB func(string, string, string, string) error
	_ = concurrency.RunInRLockOrLog(&cas.mu, locknames.LockNameCasGetPostSyncCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		postSyncCB = cas.postSyncCB
		return nil
	})
	if postSyncCB != nil {
		if err := postSyncCB(objectID, cas.kind, hash, hashFile); err != nil {
			// Log warning but don't fail creation - callback errors are non-critical
			// File is already synced, so creation succeeded
		}
	}

	// Update in-memory index immediately so same-process reads don't observe "missing hash".
	// Persistence is handled asynchronously by the write queue (or synchronously if queueing fails).
	cas.setIndexMappingInMemory(objectID, hash, bucketKey)

	// Optional created_at for high-volume kinds (enables OldestIDs from index without cache build)
	createdAt := ""
	if isHighVolumeKindForIndex(cas.kind) {
		createdAt = parseCreatedAtFromObjectData(data)
	}

	// Queue index update for batched processing (async, prevents race conditions)
	writeQueue := cas.getWriteQueue()
	opCallback := cas.getOperationCallback()
	var done <-chan error
	var enqueueErr error
	if getSkipIndexUpdateWait() {
		_, enqueueErr = writeQueue.enqueue(cas.kind, objectID, hash, bucketKey, createdAt, cas, opCallback, false, false)
	} else {
		if createdAt != emptyValue {
			done, enqueueErr = writeQueue.EnqueueUpdateWithOperationCallbackAndCreatedAt(cas.kind, objectID, hash, bucketKey, createdAt, cas, opCallback)
		} else {
			done, enqueueErr = writeQueue.EnqueueUpdateWithOperationCallback(cas.kind, objectID, hash, bucketKey, cas, opCallback)
		}
	}
	if enqueueErr != nil {
		// If queueing fails, fall back to a synchronous index write so the object is indexed.
		// CRITICAL: SetMapping reloads from disk which clears in-memory mappings.
		// The SetMapping implementation now preserves in-memory mappings during reload,
		// but we still need to ensure the mapping is set after SetMapping completes.
		setArgs := []string{bucketKey}
		if createdAt != emptyValue {
			setArgs = append(setArgs, createdAt)
		}
		if setErr := cas.index.SetMapping(objectID, hash, setArgs...); setErr != nil {
			// SetMapping failed - but in-memory mapping is already set, so reads will work
			// Log warning but don't fail creation (file is already written)
			StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Warn(LogEventStorageCASIndexPersistMappingFailedWarn).
				Kind(cas.kind).
				String("objectID", objectID).
				WithError(setErr).
				Log()
			return errfmt.Newf(ConstMiscFailedToPersistIndexUpdateForNewObject).Wrap(setErr)
		}
		// Re-set in-memory mapping after SetMapping to ensure it's definitely there
		cas.setIndexMappingInMemory(objectID, hash, bucketKey)
	} else if done != nil {
		// Wait for index update to complete (consistent with Update)
		var indexErr error
		select {
		case indexErr = <-done:
		case <-time.After(casIndexUpdateWaitTimeout):
			StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Warn(LogEventStorageCASIndexUpdateNewObjectTimeoutWarn).
				Kind(cas.kind).
				ObjectID(objectID).
				Log()
			return errfmt.Errorf(ConstMiscCasIndexUpdateDidNotCompleteWithinVQueue, casIndexUpdateWaitTimeout)
		}
		if indexErr != nil {
			return errfmt.Newf(ConstMiscFailedToPersistIndexUpdateForNewObject).Wrap(indexErr)
		}
	}

	metrics.RecordCreate(time.Since(start), nil)
	return nil
}

// Read reads an object by ID
// Looks up hash in index, then reads the hash-based file
// For bucketed storage, searches all bucket directories to find the file
func (cas *ContentAddressableStorage) Read(objectID string) ([]byte, error) {
	start := time.Now()
	metrics := GetObjectStorageMetrics()

	hash, err := cas.index.GetHash(objectID)

	if err != nil {
		// Return ErrObjectNotFound if ID not in index (matches FileObjectStorage behavior)
		if strings.Contains(err.Error(), "not found") {
			metrics.RecordRead(time.Since(start), ErrObjectNotFound)
			return nil, ErrObjectNotFound
		}
		err = errfmt.Errorf(ConstMiscFailedToGetHashForIdSW, objectID, err)
		metrics.RecordRead(time.Since(start), err)
		return nil, err
	}

	// First try the base kindDir (for non-bucketed or flat storage)
	hashFile := filepath.Join(cas.kindDir, hash+".yaml")
	data, err := os.ReadFile(hashFile)
	if err == nil {
		// Found in base directory - verify and return
		if err := VerifyContentHash(data, hash); err != nil {
			metrics.RecordRead(time.Since(start), err)
			return nil, err
		}
		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err != nil {
			err = errfmt.Newf(ConstMiscFailedToParseYamlFileMayBeCorrupted).Wrap(err)
			metrics.RecordRead(time.Since(start), err)
			return nil, err
		}
		normalizedData, err := yaml.Marshal(obj)
		if err != nil {
			metrics.RecordRead(time.Since(start), nil)
			return data, nil
		}
		metrics.RecordRead(time.Since(start), nil)
		return normalizedData, nil
	}

	// Save the error from the base directory read attempt
	baseDirErr := err

	// If not found, search in subdirectories (for bucketed storage)
	// Search ALL subdirectories - works with any bucket strategy (date-based, categorical, state-based, etc.)
	// This is backend-agnostic and doesn't assume specific bucket patterns.
	// Use the same bounded pool pattern as list/triggered jobs so ReadDir runs in a fixed worker pool
	// instead of one goroutine per Read (see .zqk/unbounded-concurrency-fixes.md §10).
	type readDirResult struct {
		entries []os.DirEntry
		err     error
	}
	resultChan := make(chan readDirResult, 1)
	submitCtx, submitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer submitCancel()
	kindDir := cas.kindDir
	err = getCASReadDirPool().Submit(submitCtx, func(ctx context.Context) error {
		entries, readErr := os.ReadDir(kindDir)
		resultChan <- readDirResult{entries: entries, err: readErr}
		return nil
	})
	if err != nil {
		metrics.RecordRead(time.Since(start), errfmt.Newf(ConstMiscCasReaddirPoolSubmit).Wrap(err))
		return nil, baseDirErr
	}
	var entries []os.DirEntry
	select {
	case result := <-resultChan:
		entries = result.entries
		err = result.err
	case <-time.After(5 * time.Second):
		metrics.RecordRead(time.Since(start), errfmt.Errorf(ConstMiscDirectoryReadTimeoutDirectoryMayHaveTooM))
		return nil, baseDirErr
	}
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			// Search all subdirectories (not just date patterns) - supports any bucket strategy
			bucketFile := filepath.Join(cas.kindDir, entry.Name(), hash+".yaml")
			data, err := os.ReadFile(bucketFile)
			if err == nil {
				// Found in bucket directory - verify and return
				if err := VerifyContentHash(data, hash); err != nil {
					metrics.RecordRead(time.Since(start), err)
					return nil, err
				}
				var obj map[string]any
				if err := yaml.Unmarshal(data, &obj); err != nil {
					err = errfmt.Newf(ConstMiscFailedToParseYamlFileMayBeCorrupted).Wrap(err)
					metrics.RecordRead(time.Since(start), err)
					return nil, err
				}
				normalizedData, err := yaml.Marshal(obj)
				if err != nil {
					metrics.RecordRead(time.Since(start), nil)
					return data, nil
				}
				metrics.RecordRead(time.Since(start), nil)
				return normalizedData, nil
			}
		}
	}

	// File not found in base directory or any bucket
	// Use the original error from the base directory read attempt
	err = errfmt.Newf(ConstMiscFailedToReadHashFile).Wrap(baseDirErr)
	metrics.RecordRead(time.Since(start), err)
	return nil, err
}

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

	// Get old hash
	oldHash, err := cas.index.GetHash(objectID)
	objectNotInIndex := err != nil

	// Resolve target directory for "not in index" path (migration / new index entry)
	var wantTargetDir string
	if len(targetBucketDir) > 0 && targetBucketDir[0] != emptyValue {
		wantTargetDir = targetBucketDir[0]
	}

	if objectNotInIndex {
		// Object not in index - this can happen if:
		// 1. Object was created before CAS migration
		// 2. Object exists in filesystem but index wasn't updated
		// 3. Index was cleared/rebuilt
		// Treat this as adding a new object to the index
		// We'll write the new file and add it to the index, but won't try to delete an old file
		newHash := CalculateSHA256Hash(data)

		storageDir := cas.kindDir
		if wantTargetDir != emptyValue {
			storageDir = wantTargetDir
			if mkErr := os.MkdirAll(storageDir, paths.DirPerm755); mkErr != nil {
				return errfmt.Newf(ConstMiscFailedToCreateTargetBucketDirectory).Wrap(mkErr)
			}
		}
		newHashFile := filepath.Join(storageDir, newHash+".yaml")

		// Check if file already exists (might have been written but not indexed)
		if _, statErr := os.Stat(newHashFile); statErr == nil {
			// File already exists with this hash - just need to add to index
			// This handles the case where file was written but index update failed
		} else {
			// Write new file
			if err := cas.writeFileWithSync(newHashFile, data); err != nil {
				return errfmt.Newf(ConstMiscFailedToWriteNewHashFile).Wrap(err)
			}
		}

		targetBucketKey := ""
		if wantTargetDir != emptyValue {
			targetBucketKey = filepath.Base(wantTargetDir)
		}
		cas.setIndexMappingInMemory(objectID, newHash, targetBucketKey)

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
			var _err_82986027 = cas.index.SetMapping(objectID, newHash)
			if _err_82986027 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82986027).Log()
			}
			return nil
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
			return errfmt.Errorf(ConstMiscCasIndexUpdateDidNotCompleteWithinVQueue, casIndexUpdateWaitTimeout)
		}
		if indexErr != nil {
			// Index update failed - best effort: in-memory mapping is already set
			return errfmt.Newf(ConstMiscFailedToPersistIndexUpdateForNewObject).Wrap(indexErr)
		}
		return nil
	}

	// Object exists in index - proceed with normal update
	// Calculate new hash from updated content
	newHash := CalculateSHA256Hash(data)

	// If content hasn't changed, no update needed
	if oldHash == newHash {
		return nil
	}

	// Find where the old file is stored (use bucket key from index)
	bucketKey := cas.index.GetBucketKey(objectID)
	oldHashFile, oldBucketDir := cas.findHashFile(oldHash, bucketKey)
	if oldHashFile == emptyValue {
		// Old file not found - construct expected path
		oldHashFile = filepath.Join(cas.kindDir, oldHash+".yaml")
		oldBucketDir = cas.kindDir
	}

	// Determine where to write the new file: same bucket as old, or target bucket if migrating
	storageDir := oldBucketDir
	indexBucketKey := ""
	if len(targetBucketDir) > 0 && targetBucketDir[0] != emptyValue && targetBucketDir[0] != oldBucketDir {
		storageDir = targetBucketDir[0]
		if err := os.MkdirAll(storageDir, paths.DirPerm755); err != nil {
			return errfmt.Newf(ConstMiscFailedToCreateTargetBucketDirectoryForMi).Wrap(err)
		}
		indexBucketKey = filepath.Base(storageDir)
	}
	newHashFile := filepath.Join(storageDir, newHash+".yaml")

	// Write new file first (before updating index)
	if err := cas.writeFileWithSync(newHashFile, data); err != nil {
		return errfmt.Newf(ConstMiscFailedToWriteNewHashFile).Wrap(err)
	}

	// Update in-memory index immediately so same-process reads don't observe stale mapping
	if indexBucketKey != emptyValue {
		cas.setIndexMappingInMemory(objectID, newHash, indexBucketKey)
	} else {
		cas.setIndexMappingInMemory(objectID, newHash)
	}

	// Use callback-based index update to ensure transactional cleanup (pass indexBucketKey so migration persists new bucket)
	writeQueue := cas.getWriteQueue()
	opCallback := cas.getOperationCallback()
	var done <-chan error
	if getSkipIndexUpdateWait() {
		_, err = writeQueue.enqueue(cas.kind, objectID, newHash, indexBucketKey, "", cas, opCallback, false, false)
	} else {
		done, err = writeQueue.EnqueueUpdateWithOperationCallback(cas.kind, objectID, newHash, indexBucketKey, cas, opCallback)
	}
	if err != nil {
		var _err_82988705 = os.Remove(newHashFile)
		if _err_82988705 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82988705).Log()
		}
		cas.setIndexMappingInMemory(objectID, oldHash)
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
			return errfmt.Errorf(ConstMiscCasIndexUpdateDidNotCompleteWithinVQueue, casIndexUpdateWaitTimeout)
		}
		if indexUpdateErr != nil {
			var _err_82989552 = os.Remove(newHashFile)
			if _err_82989552 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82989552).Log()
			}
			cas.setIndexMappingInMemory(objectID, oldHash)
			return errfmt.Newf(ConstMiscFailedToUpdateCasIndex).Wrap(indexUpdateErr)
		}
	}

	// Index update succeeded - now safe to clean up old hash file via callback
	var cleanupCB OrphanCleanupCallback
	_ = concurrency.RunInRLockOrLog(&cas.mu, locknames.LockNameCasGetOrphanCleanupCallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		cleanupCB = cas.orphanCleanupCB
		return nil
	})

	if cleanupCB != nil {
		// Call cleanup callback to delete old hash file
		// This is safe because index update has succeeded and is persisted
		if cleanupErr := cleanupCB(oldHashFile); cleanupErr != nil {
			// Log but don't fail - the update succeeded, cleanup is best-effort
			// In practice, this should not fail, but if it does, the orphan can be cleaned up later
			_ = cleanupErr // Suppress unused variable warning for now
		}
	} else {
		// No callback set - remove old file using the same rename-then-delete path as the
		// orphan queue worker so discovery does not briefly see two hash blobs.
		if oldHash != newHash {
			var _err_82990752 = removeOrphanCASHashFileSync(oldHashFile)
			if //nolint:errcheck // best-effort cleanup
			_err_82990752 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

					// UpdateWithIDChange updates an object when its ID is changing
					// This handles the case where the object ID changes but the object still exists (e.g., ID migration)
					// oldID: The current ID in the index
					// newID: The new ID to use
					// data: The updated object content (which includes the new ID field)
					Error(ErrMsgSwallowedError, _err_82990752).Log()

			}
		}
	}

	return nil
}

func (cas *ContentAddressableStorage) UpdateWithIDChange(oldID, newID string, data []byte) error {
	if len(data) == 0 {
		return errfmt.Errorf(ConstMiscCannotUpdateObjectWithEmptyContent)
	}

	if oldID == newID {
		// No ID change - use regular Update
		return cas.Update(newID, data)
	}

	// Get old hash using the OLD ID (this is the key fix - use oldID, not newID)
	oldHash, err := cas.index.GetHash(oldID)
	if err != nil {
		return errfmt.Errorf(ConstMiscObjectNotFoundWithOldIdSW, oldID, err)
	}

	// Calculate new hash from updated content (which includes the new ID field)
	newHash := CalculateSHA256Hash(data)

	// Find where the old file is stored (use bucket key from index for old ID)
	bucketKey := cas.index.GetBucketKey(oldID)
	oldHashFile, oldBucketDir := cas.findHashFile(oldHash, bucketKey)
	if oldHashFile == emptyValue {
		// Old file not found - this shouldn't happen, but handle gracefully
		oldHashFile = filepath.Join(cas.kindDir, oldHash+".yaml")
		oldBucketDir = cas.kindDir
	}

	// Determine where to write the new file (same bucket as old file)
	storageDir := oldBucketDir
	newHashFile := filepath.Join(storageDir, newHash+".yaml")

	// If hash changed, write new hash file
	if oldHash != newHash {
		if err := cas.writeFileWithSync(newHashFile, data); err != nil {
			return errfmt.Newf(ConstMiscFailedToWriteNewHashFile).Wrap(err)
		}
	}

	// Queue index update for batched processing (async, prevents race conditions)
	// Note: For ID changes, we need to handle both adding new mapping and removing old
	// For now, queue the new mapping update. The old mapping removal can happen synchronously
	// or we could extend the queue to support remove operations in the future
	// For ID changes we must keep index consistent; perform synchronous updates.
	// This is less frequent than normal updates and avoids transient "indexed under old ID" states.
	if err := cas.index.SetMapping(newID, newHash); err != nil {
		// Non-fatal: file is written, system check can recover.
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Index update failed", err).Log()
	}

	// Remove OLD ID -> hash mapping from index
	// This is done synchronously since it's less frequent and needs to happen atomically with the add
	// In the future, we could extend the queue to support batch remove operations
	if err := cas.index.RemoveMapping(oldID); err != nil {
		// This is non-fatal - log but continue
		// The old mapping will be stale, but the new mapping is correct
		// In practice, this should not fail, but if it does, the system can continue
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error("Failed to remove mapping", err).Log()
	}

	// Delete old file if hash changed (if no other IDs reference it)
	// For now, we'll delete it - in a full implementation, we'd check reference count
	if oldHash != newHash {
		var _err_82993873 = removeOrphanCASHashFileSync(oldHashFile)
		if //nolint:errcheck // best-effort cleanup
		_err_82993873 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.

				// Delete deletes an object
				// Deletes hash-based file and removes index entry.
				// Resolves the hash file path using the same logic as Read (base dir then bucketed subdirs) so
				// bucketed CAS storage is deleted correctly.
				// Index removal is applied in-memory immediately (so same-process reads see it) then enqueued
				// so the write-queue worker persists it and keeps disk consistent.
				ProfileSystem))).Error(ErrMsgSwallowedError,

				_err_82993873).Log()
		}
	}

	return nil
}

func (cas *ContentAddressableStorage) Delete(objectID string) error {
	// Get hash and bucket key from index (bucket key from bucket strategy at create time)
	hash, err := cas.index.GetHash(objectID)
	if err != nil {
		return errfmt.Newf(ConstMiscObjectNotFound).Wrap(err)
	}
	bucketKey := cas.index.GetBucketKey(objectID)

	// Resolve actual path using bucket strategy (kindDir/bucketKey/hash or kindDir/hash)
	hashFile, isFound := cas.findHashFile(hash, bucketKey)
	_ = isFound
	if hashFile == emptyValue {
		hashFile = filepath.Join(cas.kindDir, hash+".yaml")
	}

	// Remove from in-memory index immediately so same-process reads see the deletion
	// before the queue worker persists it (mirrors setIndexMappingInMemory on Create).
	cas.removeIndexMappingInMemory(objectID)

	// Enqueue index remove and wait for completion so the remove is serialized with the write queue
	writeQueue := cas.getWriteQueue()
	done, err := writeQueue.EnqueueRemove(cas.kind, objectID, cas)
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToEnqueueIndexRemove).Wrap(err)
	}
	// Wait for this remove to be processed (done is signalled when batch is saved)
	if done != nil {
		select {
		case err := <-done:
			if err != nil {
				return errfmt.Newf(ConstMiscIndexRemoveFailed).Wrap(err)
			}
		case <-time.After(5 * time.Second):
			return errfmt.Errorf(ConstMiscTimeoutWaitingForIndexRemove)
		}
	}
	if err := writeQueue.FlushKind(cas.kind, 5*time.Second); err != nil {
		return errfmt.Newf(ConstMiscFailedToFlushIndexAfterRemove).Wrap(err)
	}

	// Delete hash-based file after index is updated (so path resolution is consistent).
	// If the primary path did not exist (e.g. bucket key missing or wrong), try to find and remove
	// the hash file by scanning kind dir and subdirs so the file is always removed.
	if err := os.Remove(hashFile); err != nil && !os.IsNotExist(err) {
		return errfmt.Newf(ConstMiscFailedToDeleteHashFile).Wrap(err)
	}
	if _, err := os.Stat(hashFile); err == nil {
		var _err_82996305 = os.Remove(hashFile)
		if _err_82996305 !=

			// Primary path gone or never existed; ensure no hash file remains (e.g. in a bucket we didn't resolve)
			nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82996305).Log()
		}
	} else {

		cas.removeHashFileByScan(hash)
	}

	return nil
}

// BatchDelete removes multiple objects from the CAS index and deletes their files in one flush.
// Used by retention/bulk-delete so we do one FlushKind instead of N Delete() (each of which waited and called FlushKind).
// Caller must have already authorized the operation (e.g. CLI context); no Read/findDependents are done here.
func (cas *ContentAddressableStorage) BatchDelete(objectIDs []string) error {
	if len(objectIDs) == 0 {
		return nil
	}
	writeQueue := cas.getWriteQueue()
	type pathEntry struct {
		objectID string
		path     string
		hash     string
	}
	var entries []pathEntry
	for _, objectID := range objectIDs {
		hash, err := cas.index.GetHash(objectID)
		var hashFile string
		if err != nil {
			// Index miss: same situation as Read → getObjectFilePath (scan + SetMapping).
			// BatchDelete used to skip silently, leaving objects on disk while single Delete
			// succeeded after Read repaired the index.
			discoveredPath, discoveredHash, scanErr := discoverCASFilePathByScanning(objectID, cas.kindDir)
			if scanErr != nil {
				return errfmt.Errorf(ConstMiscBatchDeleteCannotResolveSInIndexOrByScan, objectID, scanErr)
			}
			hashFile = discoveredPath
			hash = discoveredHash
		} else {
			bucketKey := cas.index.GetBucketKey(objectID)
			hf, isFound := cas.findHashFile(hash, bucketKey)
			hashFile = hf
			_ = isFound
			if hashFile == emptyValue {
				hashFile = filepath.Join(cas.kindDir, hash+".yaml")
			}
		}
		entries = append(entries, pathEntry{objectID: objectID, path: hashFile, hash: hash})
	}
	for _, e := range entries {
		cas.removeIndexMappingInMemory(e.objectID)
		if err := writeQueue.EnqueueRemoveNoWait(cas.kind, e.objectID, cas); err != nil {
			return errfmt.Newf(ConstMiscBatchEnqueueRemove).Wrap(err)
		}
	}
	timeout := 30 * time.Second
	if n := len(entries); n > 1000 {
		if t := time.Duration(n/100) * time.Second; t > timeout {
			timeout = t
		}
	}
	if err := writeQueue.FlushKind(cas.kind, timeout); err != nil {
		return errfmt.Newf(ConstMiscBatchFlushIndexAfterRemove).Wrap(err)
	}
	for _, e := range entries {
		var _err_82998612 = os.Remove(e.path)
		if _err_82998612 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82998612).Log()
		}
		if _, err := os.Stat(e.path); err == nil {
			var _err_82998681 = os.Remove(e.path)
			if _err_82998681 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82998681).Log()
			}
		} else {
			cas.removeHashFileByScan(e.hash)
		}
	}
	return nil
}
