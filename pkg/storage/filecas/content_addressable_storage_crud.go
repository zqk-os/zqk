package filecas

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

const (
	// casReadDirPoolWorkers kept low to reduce readdir CPU (see INDEX_FIRST_LOW_CPU_SCAN_DESIGN.md).
	// Sampling showed __getdirentries64 as a major hotspot; 16 workers still allows parallel CAS lookups without dominating CPU.
	casReadDirPoolWorkers = 4
	casReadDirPoolQueue   = 256
)

var (
	casReadDirPoolOnce sync.Once
	casReadDirPool     *goroutinelabels.Pool

	skipIndexUpdateWait atomic.Bool

	// casIndexUpdateWaitTimeout limits how long Create/Update wait for the index write queue.
	// Prevents scheduler_job (and other CAS) updates from hanging when the queue is congested (e.g. daemon + CLI).
	// TRACK: BLI-CEF-R19-CAS-INTERMEDIATE-LEAK-001 — timeout after WriteFileWithSync must still sweep extras.
	casIndexUpdateWaitTimeout = 30 * time.Second
)

// SetSkipIndexUpdateWait toggles whether CAS Create/Update waits for the listing index write queue.
// When enabled, index updates are queued asynchronously without blocking, suitable for bulk state-restore.
func SetSkipIndexUpdateWait(val bool) {
	skipIndexUpdateWait.Store(val)
}

func GetSkipIndexUpdateWait() bool {
	return skipIndexUpdateWait.Load()
}

// getCASReadDirPool returns the process-wide bounded pool for CAS bucket-search ReadDir.
// Uses the same bounded-pool pattern as list/count and triggered jobs (see .zqk/unbounded-concurrency-fixes.md §10).
func getCASReadDirPool() *goroutinelabels.Pool {
	casReadDirPoolOnce.Do(func() {
		bud := goroutinelabels.DefaultBudget()
		casReadDirPool = goroutinelabels.NewPool(bud, "cas_readdir", ConstMiscCasBucketSearchReaddirWithTimeout, casReadDirPoolWorkers, casReadDirPoolQueue)
		casReadDirPool.Start(context.Background()) // Background: request-or-shutdown derived
	})
	return casReadDirPool
}

// isHighVolumeKindForIndex returns true for kinds that store created_at in the CAS index (enables OldestIDs).
// Derived from high_volume_kinds.yaml via IsHighVolumeKindForCache — do not maintain a parallel kind list.
func isHighVolumeKindForIndex(kind string) bool {
	return IsHighVolumeKindForCache(kind)
}

// parseCreatedAtFromObjectData extracts created_at from YAML/JSON object data; returns RFC3339 or empty.
func parseCreatedAtFromObjectData(data []byte) string {
	return objects.ExtractProperty(data, objects.FieldKeyCreatedAt)
}

// Create creates an object using content-addressable storage
// Hash is calculated from content BEFORE writing (Git's approach)
// bucketDir is optional - if provided, used instead of cas.kindDir for file storage (supports bucketed storage)
func (cas *ContentAddressableStorage) Create(objectID string, data []byte, bucketDir ...string) error {
	start := time.Now()
	metrics := GetMetrics()

	if len(data) == 0 {
		err := errfmt.Errorf(ConstMiscCannotCreateObjectWithEmptyContent)
		metrics.RecordCreate(time.Since(start), err == nil)
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
	if err := fileutil.MkdirAll(storageDir, paths.DirPerm755); err != nil {
		metrics.RecordCreate(time.Since(start), err == nil)
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	// Write file with hash as filename
	hashFile := filepath.Join(storageDir, hash+".yaml")
	if err := cas.WriteFileWithSync(hashFile, data); err != nil {
		metrics.RecordCreate(time.Since(start), err == nil)
		return errfmt.Newf(ConstMiscFailedToWriteHashFile).Wrap(err)
	}

	// Prepare: hash YAML is on disk unreferenced until index + cache commit.

	// Update in-memory index immediately so same-process reads don't observe "missing hash".
	// Persistence is handled asynchronously by the write queue (or synchronously if queueing fails).
	cas.SetIndexMappingInMemory(objectID, hash, bucketKey)

	pendingCache := GetCASPendingVisibilityCache(projectRootFromCASKindDir(cas.kindDir))
	var pendErr error
	if pendingCache != nil {
		pendErr = pendingCache.PublishPending(objectID, cas.kind, hash, bucketKey)
	}
	if pendingCache == nil || pendErr != nil {
		// Pending layer is the cross-process bridge before durable index save.
		// Fall back to synchronous SetMapping so Create does not ACK a ghost.
		setArgs := []string{}
		if bucketKey != emptyValue {
			setArgs = append(setArgs, bucketKey)
		}
		if setErr := cas.index.SetMapping(objectID, hash, setArgs...); setErr != nil {
			metrics.RecordCreate(time.Since(start), setErr == nil)
			return errfmt.Newf("publish pending failed (or disabled) and sync index fallback failed").Wrap(setErr)
		}
		cas.SetIndexMappingInMemory(objectID, hash, bucketKey)
	}

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
	if GetSkipIndexUpdateWait() {
		_, enqueueErr = writeQueue.EnqueueInternal(cas.kind, objectID, hash, bucketKey, createdAt, cas, opCallback, false, false)
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
		cas.SetIndexMappingInMemory(objectID, hash, bucketKey)
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
			cas.sweepAfterDurableBlob(objectID, hash)
			return errfmt.Errorf(ConstMiscCasIndexUpdateDidNotCompleteWithinVQueue, casIndexUpdateWaitTimeout)
		}
		if indexErr != nil {
			return errfmt.Newf(ConstMiscFailedToPersistIndexUpdateForNewObject).Wrap(indexErr)
		}
	}

	if err := cas.EnsureCASIndexMatchesContentHash(objectID, data, bucketKey); err != nil {
		metrics.RecordCreate(time.Since(start), err == nil)
		return err
	}

	if err := cas.commitLiveIdentity(objectID, hash, hashFile); err != nil {
		metrics.RecordCreate(time.Since(start), false)
		return err
	}

	metrics.RecordCreate(time.Since(start), true)
	return nil
}

// Read reads an object by ID
// Looks up hash in index, then reads the hash-based file
// For bucketed storage, searches all bucket directories to find the file
func (cas *ContentAddressableStorage) Read(objectID string) ([]byte, error) {
	start := time.Now()
	metrics := GetMetrics()

	// Prefer GetHashForID: pending visibility + discover/heal. GetHash alone misses
	// just-created IDs when durable index lag or async validate stripped a mapping.
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		// Return ErrObjectNotFound if ID not in index (matches FileObjectStorage behavior)
		if strings.Contains(err.Error(), "not found") {
			metrics.RecordRead(time.Since(start), false)
			return nil, ErrObjectNotFound
		}
		err = errfmt.Errorf(ConstMiscFailedToGetHashForIdSW, objectID, err)
		metrics.RecordRead(time.Since(start), err == nil)
		return nil, err
	}

	// First try the base kindDir (for non-bucketed or flat storage)
	hashFile := filepath.Join(cas.kindDir, hash+".yaml")
	data, err := fileutil.ReadFileGated(hashFile)
	if err == nil {
		// Found in base directory - verify and return
		if err := VerifyContentHash(data, hash); err != nil {
			EvictCASBlob(hash)
			metrics.RecordRead(time.Since(start), err == nil)
			return nil, err
		}
		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err != nil {
			err = errfmt.Newf(ConstMiscFailedToParseYamlFileMayBeCorrupted).Wrap(err)
			metrics.RecordRead(time.Since(start), err == nil)
			return nil, err
		}
		normalizedData, err := yaml.Marshal(obj)
		if err != nil {
			storeCASBlob(hash, data)
			metrics.RecordRead(time.Since(start), true)
			return data, nil
		}
		storeCASBlob(hash, normalizedData)
		metrics.RecordRead(time.Since(start), true)
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
		entries []fileutil.DirEntry
		err     error
	}
	resultChan := make(chan readDirResult, 1)
	submitCtx, submitCancel := context.WithTimeout(context.Background(), 5*time.Second) // Background: request-or-shutdown derived
	defer submitCancel()
	kindDir := cas.kindDir
	err = getCASReadDirPool().Submit(submitCtx, func(ctx context.Context) error {
		entries, readErr := fileutil.ReadDir(kindDir)
		resultChan <- readDirResult{entries: entries, err: readErr}
		return nil
	})
	if err != nil {
		metrics.RecordRead(time.Since(start), false)
		return nil, baseDirErr
	}
	var entries []fileutil.DirEntry
	select {
	case result := <-resultChan:
		entries = result.entries
		err = result.err
	case <-time.After(5 * time.Second):
		metrics.RecordRead(time.Since(start), false)
		return nil, baseDirErr
	}
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			// Search all subdirectories (not just date patterns) - supports any bucket strategy
			bucketFile := filepath.Join(cas.kindDir, entry.Name(), hash+".yaml")
			data, err := fileutil.ReadFileGated(bucketFile)
			if err == nil {
				// Found in bucket directory - verify and return
				if err := VerifyContentHash(data, hash); err != nil {
					EvictCASBlob(hash)
					metrics.RecordRead(time.Since(start), err == nil)
					return nil, err
				}
				var obj map[string]any
				if err := yaml.Unmarshal(data, &obj); err != nil {
					err = errfmt.Newf(ConstMiscFailedToParseYamlFileMayBeCorrupted).Wrap(err)
					metrics.RecordRead(time.Since(start), err == nil)
					return nil, err
				}
				normalizedData, err := yaml.Marshal(obj)
				if err != nil {
					storeCASBlob(hash, data)
					metrics.RecordRead(time.Since(start), true)
					return data, nil
				}
				storeCASBlob(hash, normalizedData)
				metrics.RecordRead(time.Since(start), true)
				return normalizedData, nil
			}
		}
	}

	// File not found in base directory or any bucket
	// Use the original error from the base directory read attempt
	err = errfmt.Newf(ConstMiscFailedToReadHashFile).Wrap(baseDirErr)
	metrics.RecordRead(time.Since(start), err == nil)
	return nil, err
}

// Update updates an object
// Creates new hash-based file, updates index, deletes old file.
// For bucketed storage, uses the same bucket as the old file unless targetBucketDir is provided.
// When targetBucketDir is provided and different from the current bucket, the object is migrated
// to the target bucket (all kinds with a bucket strategy migrate on update the same way).
// If object is not in index, treats it as a new object and adds it to the index.

// Get old hash using GetHashForID (which checks index, pending visibility cache, and disk scan)

// Resolve target directory for "not in index" path (migration / new index entry)

// Even if not in index, scan disk for existing file for this objectID

// Object not in index - this can happen if:
// 1. Object was created before CAS migration
// 2. Object exists in filesystem but index wasn't updated
// 3. Index was cleared/rebuilt
// Treat this as adding a new object to the index
// We'll write the new file and add it to the index, but won't try to delete an old file

// Check if file already exists (might have been written but not indexed)

// File already exists with this hash - just need to add to index
// This handles the case where file was written but index update failed

// Write new file

// Index update failed - best effort: in-memory mapping is already set

// Object exists in index - proceed with normal update
// Calculate new hash from updated content

// If content hasn't changed, still sweep extras from a prior leak.

// Find where the old file is stored (use bucket key from index)

// Old file not found - construct expected path

// Determine where to write the new file: same bucket as old, or target bucket if migrating

// Write new file first (before updating index)

// Update in-memory index immediately so same-process reads don't observe stale mapping

// Use callback-based index update to ensure transactional cleanup (pass indexBucketKey so migration persists new bucket)

// Wait for index update to complete (with timeout so updates don't hang when queue is congested)

// Index may still complete asynchronously; leave pending at newHash for RYW.
// The new hash file is already on disk — sweep extras or POL-CODE-004 dupes remain.

// Index update succeeded - now safe to clean up old hash file via callback

// Call cleanup callback to delete old hash file
// This is safe because index update has succeeded and is persisted

// Log but don't fail - the update succeeded, cleanup is best-effort
// In practice, this should not fail, but if it does, the orphan can be cleaned up later
// Suppress unused variable warning for now

// No callback set - remove old file using the same rename-then-delete path as the
// orphan queue worker so discovery does not briefly see two hash blobs.

//nolint:errcheck // best-effort cleanup

// UpdateWithIDChange updates an object when its ID is changing
// This handles the case where the object ID changes but the object still exists (e.g., ID migration)
// oldID: The current ID in the index
// newID: The new ID to use
// data: The updated object content (which includes the new ID field)

// Index + blob durable: one live YAML then object-id-cache. ACK only if both succeed.
// TRACK: TDE-CEF-CAS-IDENTITY-TXN-001

// sweepAfterDurableBlob enforces one live hash YAML per id after the new blob is on disk.
// Index-queue timeout used to return here without sweeping, which is how untracked
// restamps (updated_at-only) left dual CAS blobs. TRACK: BLI-CEF-R19-CAS-INTERMEDIATE-LEAK-001

// No ID change - use regular Update

// Get old hash using the OLD ID (this is the key fix - use oldID, not newID)

// Calculate new hash from updated content (which includes the new ID field)

// Find where the old file is stored (use bucket key from index for old ID)

// Old file not found - this shouldn't happen, but handle gracefully

// Determine where to write the new file (same bucket as old file)

// If hash changed, write new hash file

// Queue index update for batched processing (async, prevents race conditions)
// Note: For ID changes, we need to handle both adding new mapping and removing old
// For now, queue the new mapping update. The old mapping removal can happen synchronously
// or we could extend the queue to support remove operations in the future
// For ID changes we must keep index consistent; perform synchronous updates.
// This is less frequent than normal updates and avoids transient "indexed under old ID" states.

// Non-fatal: file is written, system check can recover.

// Remove OLD ID -> hash mapping from index
// This is done synchronously since it's less frequent and needs to happen atomically with the add
// In the future, we could extend the queue to support batch remove operations

// This is non-fatal - log but continue
// The old mapping will be stale, but the new mapping is correct
// In practice, this should not fail, but if it does, the system can continue

// Delete old file if hash changed. ID change retires oldID — do not use
// RemoveOrphanCASHashFileSync: refuseOrphanCASHashDelete treats the old blob
// as the sole survivor for the peeked old id (the new blob peeks as newID).
// TRACK: BLI-1785723654802038000-b14064bc

//nolint:errcheck // best-effort cleanup

// Delete deletes an object
// Deletes hash-based file and removes index entry.
// Resolves the hash file path using the same logic as Read (base dir then bucketed subdirs) so
// bucketed CAS storage is deleted correctly.
// Index removal is applied in-memory immediately (so same-process reads see it) then enqueued
// so the write-queue worker persists it and keeps disk consistent.

// Invalidate old ID and register new ID→path so object-id-cache cannot keep the
// deleted hash under the previous id. Prefer InvalidateAndUpdate when a handler
// is wired; always fire post-sync for the new mapping.
// TRACK: BLI-1785723654802038000-b14064bc

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

	projectRoot := projectRootFromCASKindDir(cas.kindDir)
	NoteObjectIDCachePending(projectRoot, string(ObjectIDCachePendingOpInvalidate), objectID, cas.kind, hashFile, "cas_delete")

	// Remove from in-memory index immediately so same-process reads see the deletion
	// before the queue worker persists it (mirrors setIndexMappingInMemory on Create).
	cas.removeIndexMappingInMemory(objectID)

	pendingCache := GetCASPendingVisibilityCache(projectRootFromCASKindDir(cas.kindDir))
	if pendingCache != nil {
		_ = pendingCache.EvictPending(objectID)
	}

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
	if err := fileutil.RemoveFile(hashFile); err != nil && !fileutil.IsNotExist(err) {
		return errfmt.Newf(ConstMiscFailedToDeleteHashFile).Wrap(err)
	}
	if _, err := fileutil.Stat(hashFile); err == nil {
		var _err_82996305 = fileutil.RemoveFile(hashFile)
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
	var missIDs []string
	indexed := make(map[string]pathEntry, len(objectIDs))
	for _, objectID := range objectIDs {
		hash, err := cas.index.GetHash(objectID)
		if err != nil {
			missIDs = append(missIDs, objectID)
			continue
		}
		bucketKey := cas.index.GetBucketKey(objectID)
		hf, isFound := cas.findHashFile(hash, bucketKey)
		hashFile := hf
		_ = isFound
		if hashFile == emptyValue {
			hashFile = filepath.Join(cas.kindDir, hash+".yaml")
		}
		indexed[objectID] = pathEntry{objectID: objectID, path: hashFile, hash: hash}
	}
	if len(missIDs) > 0 {
		// One directory walk for all index misses — never N×DiscoverCASFilePathByScanning.
		discovered, scanErr := DiscoverCASFilePathsByScanning(missIDs, cas.kindDir)
		if scanErr != nil {
			return errfmt.Errorf(ConstMiscBatchDeleteCannotResolveSInIndexOrByScan, missIDs[0], scanErr)
		}
		for _, objectID := range missIDs {
			d, ok := discovered[objectID]
			if !ok {
				return errfmt.Errorf(ConstMiscBatchDeleteCannotResolveSInIndexOrByScan, objectID, errfmt.Errorf(ConstStreamIdStrNotFoundInCasDirectoryStr, objectID, cas.kindDir))
			}
			indexed[objectID] = pathEntry{objectID: objectID, path: d.path, hash: d.hash}
		}
	}
	entries = make([]pathEntry, 0, len(objectIDs))
	for _, objectID := range objectIDs {
		if e, ok := indexed[objectID]; ok {
			entries = append(entries, e)
		}
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
		var _err_82998612 = fileutil.RemoveFile(e.path)
		if _err_82998612 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82998612).Log()
		}
		if _, err := fileutil.Stat(e.path); err == nil {
			var _err_82998681 = fileutil.RemoveFile(e.path)
			if _err_82998681 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82998681).Log()
			}
		} else {
			cas.removeHashFileByScan(e.hash)
		}
	}
	return nil
}
