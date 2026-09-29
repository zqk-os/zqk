package cas

import (
	"fmt"
	"maps"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	file_pkg "github.com/zqk-os/zqk/pkg/storage/file"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

// wakeWorkerIfNeeded starts the worker if it's not already running
// Thread-safe (uses atomic operations)
func (iq *indexQueue) wakeWorkerIfNeeded() {
	// Try to set workerRunning from 0 to 1 (atomic compare-and-swap)
	if iq.workerRunning.CompareAndSwap(0, 1) {
		// Successfully acquired lock - start worker
		wgID := fmt.Sprintf("%s_worker", iq.queueID)
		wg := iq.wgManager.CreateGroupForGoroutine(wgID, ConstStreamCasIndexWriteQueueWorker)
		goroutinelabels.NewGoroutine(ConstStreamCasIndexWriteQueueWorker, fmt.Sprintf(ConstStreamProcessingCasIndexWriteQueueForStr, iq.kind)).
			WithWaitGroup(wg).
			StartSimple(iq.startWorker)
	}
	// If worker is already running, do nothing (worker will process the new item)
}

// startWorker starts the background worker that processes index updates in batches
// Implements on-demand pattern: processes batches until idle timeout, then shuts down
// This ensures all updates are serialized and prevents race conditions
// Batches are processed when either:
//   - Batch size reaches casIndexBatchSize (100 requests)
//   - casIndexBatchTimeout (200ms) elapses
func (iq *indexQueue) startWorker() {
	defer func() {
		iq.workerRunning.Store(0)
		if len(iq.queue) > 0 && iq.ctx.Err() == nil {
			iq.wakeWorkerIfNeeded()
		}
		// Note: wg.Done() is called by goroutinelabels.WithWaitGroup() on goroutine exit
	}()

	// Emit worker start event via coordinator
	callback := getListingIndexBatchEventCallback()
	projectRoot := iq.getProjectRoot()
	if callback != nil && projectRoot != emptyValue {
		// Use system context for background event emission
		ctx := pkgctx.NewSystemContext()
		goroutinelabels.NewGoroutine(ConstStreamCasIndexWorkerLifecycle, fmt.Sprintf(ConstStreamEmittingWorkerStartEventForStr, iq.kind)).
			StartSimple(func() {
				callback(
					ctx,
					projectRoot,
					iq.getStorage(),
					iq.kind,
					0, // No batch
					0, // No duration
					"start",
					nil,
				)
			})
	}

	batch := make([]*indexUpdateRequest, 0, iq.batchSize)
	batchTicker := time.NewTicker(iq.timeout)
	defer batchTicker.Stop()

	idleTicker := time.NewTicker(casIndexIdleTimeout)
	defer idleTicker.Stop()

	lastWorkTime := time.Now()

	for {
		select {
		case <-iq.ctx.Done():
			// Drain any remaining items in the queue without blocking
			doneDraining := false
			for !doneDraining {
				select {
				case req, ok := <-iq.queue:
					if ok {
						batch = append(batch, req)
					}
				default:
					doneDraining = true
				}
			}

			// Process any remaining items in batch before shutdown
			if len(batch) > 0 {
				err := iq.processBatch(batch)
				if err != nil {
					logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToProcessFinalBatchForStrDuringShutdownValN, iq.kind, err), nil).Log()
				}
				for _, req := range batch {
					signalIndexUpdateCompletion(req, err)
				}
				iq.pendingItems.Add(int64(-len(batch)))
			}
			// Emit worker stop event
			projectRoot := iq.getProjectRoot()
			if callback != nil && projectRoot != emptyValue {
				// Use system context for background event emission
				ctx := pkgctx.NewSystemContext()
				goroutinelabels.NewGoroutine(ConstStreamCasIndexWorkerLifecycle, fmt.Sprintf(ConstStreamEmittingWorkerShutdownEventForStr, iq.kind)).
					StartSimple(func() {
						callback(
							ctx,
							projectRoot,
							iq.getStorage(),
							iq.kind,
							0,
							0,
							"shutdown",
							nil,
						)
					})
			}
			return

		case req, ok := <-iq.queue:
			if !ok {
				// Channel closed - shutdown initiated
				if len(batch) > 0 {
					err := iq.processBatch(batch)
					if err != nil {
						logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToProcessFinalBatchForStrDuringShutdownValN, iq.kind, err), nil).Log()
					}
					for _, req := range batch {
						signalIndexUpdateCompletion(req, err)
					}
					iq.pendingItems.Add(int64(-len(batch)))
				}
				// Emit worker stop event
				projectRoot := iq.getProjectRoot()
				if callback != nil && projectRoot != emptyValue {
					// Use system context for background event emission
					ctx := pkgctx.NewSystemContext()
					goroutinelabels.NewGoroutine(ConstStreamCasIndexWorkerLifecycle, fmt.Sprintf(ConstStreamEmittingWorkerShutdownEventForStr, iq.kind)).
						StartSimple(func() {
							callback(ctx, projectRoot, iq.getStorage(), iq.kind, 0, 0, "shutdown", nil)
						})
				}
				return
			}
			// New work arrived - reset idle timer
			lastWorkTime = time.Now()
			idleTicker.Stop()
			idleTicker.Reset(casIndexIdleTimeout)

			batch = append(batch, req)
			if len(batch) >= iq.batchSize {
				// Batch is full - process immediately
				err := iq.processBatch(batch)
				// Signal completion
				for _, req := range batch {
					signalIndexUpdateCompletion(req, err)
				}
				iq.pendingItems.Add(int64(-len(batch)))
				batch = batch[:0] // Reset batch
				batchTicker.Stop()
				batchTicker.Reset(iq.timeout)
			}

		case <-batchTicker.C:
			// Batch timeout reached - process current batch
			// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting
			if iq.ctx.Err() != nil {
				continue
			}
			if len(batch) > 0 {
				lastWorkTime = time.Now()
				err := iq.processBatch(batch)
				// Signal completion
				for _, req := range batch {
					signalIndexUpdateCompletion(req, err)
				}
				iq.pendingItems.Add(int64(-len(batch)))
				batch = batch[:0] // Reset batch
			}

		case <-idleTicker.C:
			// Idle timeout reached - check if we should shut down
			// CRITICAL: Check context FIRST - it may have been cancelled while we were waiting
			if iq.ctx.Err() != nil {
				continue
			}
			queueSize := len(iq.queue)
			if queueSize == 0 && len(batch) == 0 {
				// Queue is empty and no batch pending - check if we've been idle long enough
				idleDuration := time.Since(lastWorkTime)
				if idleDuration >= casIndexIdleTimeout {
					// Been idle long enough - shut down worker (on-demand pattern)
					// Worker will wake up again when new work arrives (via wakeWorkerIfNeeded)
					projectRoot := iq.getProjectRoot()
					if callback != nil && projectRoot != emptyValue {
						// Use system context for background event emission
						ctx := pkgctx.NewSystemContext()
						goroutinelabels.NewGoroutine(ConstStreamCasIndexWorkerLifecycle, fmt.Sprintf(ConstStreamEmittingWorkerShutdownEventForStr, iq.kind)).
							StartSimple(func() {
								callback(
									ctx,
									projectRoot,
									iq.getStorage(),
									iq.kind,
									0,
									0,
									"shutdown",
									nil,
								)
							})
					}
					return // Exit worker goroutine
				}
				// Not idle long enough yet - reset timer
				idleTicker.Stop()
				idleTicker.Reset(casIndexIdleTimeout - idleDuration)
			} else {
				// There's work - reset idle timer
				lastWorkTime = time.Now()
				idleTicker.Stop()
				idleTicker.Reset(casIndexIdleTimeout)
			}
		}
	}
}

// processBatch processes a batch of index updates
// Reloads index once, applies all updates, saves once
// This is much more efficient than reloading/saving for each update
// Thread-safe (called from single worker goroutine, CAS has its own locks)
//
// nolint:gocyclo // TRACK: split the existing transactional batch path after ghost-write convergence.
func (iq *indexQueue) processBatch(batch []*indexUpdateRequest) error {
	if len(batch) == 0 {
		return nil
	}

	iq.workerProcessing.Store(1)
	defer iq.workerProcessing.Store(0)

	startTime := time.Now()
	batchSize := len(batch)

	// Emit batch start event via coordinator (async, non-blocking)
	callback := getListingIndexBatchEventCallback()
	projectRoot := iq.getProjectRoot()
	if callback != nil && projectRoot != emptyValue {
		// Use system context for background event emission
		ctx := pkgctx.NewSystemContext()
		goroutinelabels.NewGoroutine(ConstStreamListingIndexBatchEvent, fmt.Sprintf(ConstStreamEmittingBatchStartEventForStrSizeInt, iq.kind, batchSize)).
			StartSimple(func() {
				callback(
					ctx,
					projectRoot,
					iq.getStorage(),
					iq.kind,
					batchSize,
					0, // Duration not known yet
					"start",
					nil,
				)
			})
	}

	// Emit worker lifecycle event (start) if this is the first batch after wake
	// Note: We could track this more precisely, but for now we emit on first batch

	// Record metrics: batch processing start
	metrics := GetObjectStorageMetrics()
	metrics.BatchesProcessed.Add(1)
	metrics.TotalUpdatesBatched.Add(int64(batchSize))
	iq.processedTotal.Add(int64(batchSize))

	// Snapshot CAS pointer under lock (transactional pattern).
	var cas *filecas.ContentAddressableStorage
	if err := concurrency.RunInRLock(&iq.mu, func() error {
		cas = iq.cas
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToSnapshotCasPointerValN, err), nil).Log()
	}

	// Merge batch into in-memory index under lock. Hold idx.mu only for the brief merge/copy.
	// Do NOT hold the CAS index file lock here so that single delete (RemoveMapping) can
	// acquire it while we do merge + validation; we acquire the file lock only for the save.
	// so that Read/GetHash (RLock) are not blocked by slow loadLocked (ReadFile + Unmarshal).
	// We do not reload from disk here: we preserve in-memory state and apply the batch, then
	// save; the on-disk index is updated from this process's view (same as before).
	reloaded := false
	var mappingsCopy, bucketKeysCopy, createdAtsCopy map[string]string
	if err := concurrency.RunInLock(&cas.GetIndex().Mu, func() error {
		// Preserve in-memory mappings and bucket keys (source of truth for this process)
		preservedMappings := make(map[string]string, len(cas.GetIndex().Mappings))
		if cas.GetIndex().Mappings != nil {
			maps.Copy(preservedMappings, cas.GetIndex().Mappings)
		}
		var preservedBucketKeys map[string]string
		if len(cas.GetIndex().BucketKeys) > 0 {
			preservedBucketKeys = make(map[string]string, len(cas.GetIndex().BucketKeys))
			maps.Copy(preservedBucketKeys, cas.GetIndex().BucketKeys)
		}

		// Use preserved state (no disk reload while holding lock — avoids blocking single delete/Read)
		cas.GetIndex().Mappings = preservedMappings
		if preservedBucketKeys != nil {
			cas.GetIndex().BucketKeys = preservedBucketKeys
		} else {
			cas.GetIndex().BucketKeys = nil
		}

		// Preserve and merge CreatedAt (for high-volume kinds)
		if cas.GetIndex().CreatedAt != nil {
			createdAtPreserved := make(map[string]string, len(cas.GetIndex().CreatedAt))
			maps.Copy(createdAtPreserved, cas.GetIndex().CreatedAt)
			cas.GetIndex().CreatedAt = createdAtPreserved
		}

		adds := int64(0)
		for _, req := range batch {
			if !req.remove {
				cas.GetIndex().Mappings[req.objectID] = req.hash
				adds++
				if req.bucketKey != emptyValue {
					if cas.GetIndex().BucketKeys == nil {
						cas.GetIndex().BucketKeys = make(map[string]string)
					}
					cas.GetIndex().BucketKeys[req.objectID] = req.bucketKey
				}
				if req.createdAt != emptyValue {
					if cas.GetIndex().CreatedAt == nil {
						cas.GetIndex().CreatedAt = make(map[string]string)
					}
					cas.GetIndex().CreatedAt[req.objectID] = req.createdAt
				}
			} else {
				delete(cas.GetIndex().Mappings, req.objectID)
				if cas.GetIndex().BucketKeys != nil {
					delete(cas.GetIndex().BucketKeys, req.objectID)
				}
				if cas.GetIndex().CreatedAt != nil {
					delete(cas.GetIndex().CreatedAt, req.objectID)
				}
			}
		}
		metrics.IndexEntriesAdded.Add(adds)

		// Copy mappings, bucket keys, and created_at for save while still holding idx.mu
		mappingsCopy = make(map[string]string, len(cas.GetIndex().Mappings))
		maps.Copy(mappingsCopy, cas.GetIndex().Mappings)
		if len(cas.GetIndex().BucketKeys) > 0 {
			bucketKeysCopy = make(map[string]string, len(cas.GetIndex().BucketKeys))
			maps.Copy(bucketKeysCopy, cas.GetIndex().BucketKeys)
		}
		if len(cas.GetIndex().CreatedAt) > 0 {
			createdAtsCopy = make(map[string]string, len(cas.GetIndex().CreatedAt))
			maps.Copy(createdAtsCopy, cas.GetIndex().CreatedAt)
		}
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToMergeBatchIntoInMemoryIndexValN, err), nil).Log()
	}

	// Validate mappings before save: remove entries pointing to non-existent files.
	// This prevents stale entries from being persisted (e.g., files deleted externally).
	// See core-backlog for root cause analysis.
	//
	// Uses ValidationStrategy pattern to allow sync (default) or async validation.
	// See core-backlog for strategy abstraction design.
	validationStart := time.Now()
	strategy := GlobalValidationRegistry.GetStrategy(iq.kind)
	mappingsCopy, bucketKeysCopy, staleCount := strategy.ValidateMappings(cas.GetKindDir(), mappingsCopy, bucketKeysCopy)
	// In-flight batch adds are authoritative: async cache lag must not omit them from the
	// process view before cross-process merge (re-apply after reload is a second belt).
	for _, req := range batch {
		if req == nil || req.remove {
			continue
		}
		if req.objectID != emptyValue && req.hash != emptyValue {
			if mappingsCopy == nil {
				mappingsCopy = make(map[string]string)
			}
			mappingsCopy[req.objectID] = req.hash
			if req.bucketKey != emptyValue {
				if bucketKeysCopy == nil {
					bucketKeysCopy = make(map[string]string)
				}
				bucketKeysCopy[req.objectID] = req.bucketKey
			}
		}
	}
	// Keep only created_at for IDs still in mappings (stale entries were removed by validation)
	if createdAtsCopy != nil && len(mappingsCopy) < len(createdAtsCopy) {
		filtered := make(map[string]string, len(mappingsCopy))
		for id := range mappingsCopy {
			if t, ok := createdAtsCopy[id]; ok {
				filtered[id] = t
			}
		}
		createdAtsCopy = filtered
	}
	validationDuration := time.Since(validationStart)

	// Record metrics for both global CAS metrics and per-kind validation metrics
	metrics.StaleValidationTimeNs.Add(int64(validationDuration))
	if staleCount > 0 {
		metrics.StaleEntriesRemoved.Add(int64(staleCount))
	}
	kindMetrics := GlobalGetValidationMetrics(iq.kind)
	kindMetrics.RecordValidation(int64(validationDuration), len(mappingsCopy)+staleCount, staleCount)

	// Acquire CAS index file lock only for the save so single delete (RemoveMapping) can
	// acquire it while we do merge + validation instead of blocking for the full batch.
	lockStart := time.Now()
	lockPath := cas.GetIndex().FilePath + ".lock"
	lockStrategy := file_pkg.NewAutoCleanupStrategy()
	lockHandle, lockErr := lockStrategy.AcquireLock(lockPath, 30*time.Second)
	if lockErr != nil {
		metrics.RecordIndexFileLock(false, time.Since(lockStart))
		return errfmt.Newf(ConstStreamFailedToAcquireCasIndexLockForSave).Wrap(lockErr)
	}
	metrics.RecordIndexFileLock(true, time.Since(lockStart))
	defer func() {
		if err := lockHandle.Release(); err != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToReleaseCasIndexLockForStrValN, iq.kind, err), nil).Log()
		}
	}()

	// Cross-process merge: each short-lived CLI process has a partial in-memory index.
	// Saving only that view clobbers peer creates (parallel `zqk object create`). Under the
	// file lock, reload disk and overlay this process's mappings (process wins on conflict).
	if err := concurrency.RunInLock(&cas.GetIndex().Mu, func() error {
		processMaps := mappingsCopy
		processBuckets := bucketKeysCopy
		processCreated := createdAtsCopy
		if loadErr := cas.GetIndex().LoadLocked(); loadErr != nil {
			// No disk yet / unreadable: keep process view.
			return nil
		}
		reloaded = true
		// Disk-preferred merge: a full process overlay clobbered healed indexes when a
		// long-lived process (scheduler / system check) held stale in-memory mappings.
		// TRACK: follow-up in kernel backlog
		mergedMaps := filecas.MergeCASIndexMaps(cas.GetKindDir(), cas.GetIndex().Mappings, processMaps)
		// Re-apply this batch after disk merge so ValidateMappings cache-lag drops cannot
		// omit in-flight creates from the durable save (adds are authoritative for this batch).
		for _, req := range batch {
			if req == nil {
				continue
			}
			if req.remove {
				delete(mergedMaps, req.objectID)
				continue
			}
			if req.objectID != emptyValue && req.hash != emptyValue {
				mergedMaps[req.objectID] = req.hash
			}
		}
		var mergedBuckets map[string]string
		if len(cas.GetIndex().BucketKeys) > 0 || len(processBuckets) > 0 {
			mergedBuckets = make(map[string]string)
			if cas.GetIndex().BucketKeys != nil {
				maps.Copy(mergedBuckets, cas.GetIndex().BucketKeys)
			}
			for id, k := range processBuckets {
				mergedBuckets[id] = k
			}
			for _, req := range batch {
				if req == nil {
					continue
				}
				if req.remove {
					delete(mergedBuckets, req.objectID)
					continue
				}
				if req.bucketKey != emptyValue {
					if mergedBuckets == nil {
						mergedBuckets = make(map[string]string)
					}
					mergedBuckets[req.objectID] = req.bucketKey
				}
			}
		}
		var mergedCreated map[string]string
		if len(cas.GetIndex().CreatedAt) > 0 || len(processCreated) > 0 {
			mergedCreated = make(map[string]string)
			if cas.GetIndex().CreatedAt != nil {
				maps.Copy(mergedCreated, cas.GetIndex().CreatedAt)
			}
			for id, t := range processCreated {
				mergedCreated[id] = t
			}
			for _, req := range batch {
				if req == nil {
					continue
				}
				if req.remove {
					delete(mergedCreated, req.objectID)
					continue
				}
				if req.createdAt != emptyValue {
					if mergedCreated == nil {
						mergedCreated = make(map[string]string)
					}
					mergedCreated[req.objectID] = req.createdAt
				}
			}
		}
		if mergedMaps != nil {
			mappingsCopy = make(map[string]string, len(mergedMaps))
			maps.Copy(mappingsCopy, mergedMaps)
		}
		if mergedBuckets != nil {
			bucketKeysCopy = make(map[string]string, len(mergedBuckets))
			maps.Copy(bucketKeysCopy, mergedBuckets)
		}
		if mergedCreated != nil {
			createdAtsCopy = make(map[string]string, len(mergedCreated))
			maps.Copy(createdAtsCopy, mergedCreated)
		}
		cas.GetIndex().Mappings = mergedMaps
		cas.GetIndex().BucketKeys = mergedBuckets
		cas.GetIndex().CreatedAt = mergedCreated
		return nil
	}); err != nil {
		logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf(ConstStreamFailedToMergeBatchIntoInMemoryIndexValN, err), nil).Log()
	}

	// Save index ONCE with all updates (atomic)
	// NOTE: saveMappingsLocked requires the index file lock already be held.
	saveStart := time.Now()
	err := cas.GetIndex().SaveMappingsLocked(mappingsCopy, bucketKeysCopy, createdAtsCopy)
	duration := time.Since(startTime)
	saveDuration := time.Since(saveStart)

	// Normal create/update path persists the index via this queue, not filecas.IDIndex.SetMapping.
	// Evict pending only after durable save succeeds (ADR-CAS-PENDING-VISIBILITY-LAYER §3).
	if err == nil {
		for _, req := range batch {
			if req == nil {
				continue
			}
			if req.remove {
				if pendingCache := GetCASPendingVisibilityCache(ProjectRootFromCASIndexPath(cas.GetIndex().FilePath)); pendingCache != nil {
					_ = pendingCache.EvictPending(req.objectID) //nolint:errcheck // best-effort; durable remove is source of truth
				}
				continue
			}
			// Only confirm when this save actually retained the mapping (not stripped as "stale").
			if mappingsCopy != nil && mappingsCopy[req.objectID] == req.hash {
				ConfirmPendingAfterDurableMapping(cas.GetIndex().FilePath, req.objectID, req.hash)
			}
		}
	}

	// Record metrics: batch processing completion
	batchDurationNs := int64(duration)
	if err != nil {
		metrics.BatchProcessingFailures.Add(1)
		metrics.IndexFailures.Add(1)
	} else {
		// Record successful batch save
		metrics.IndexSaves.Add(1)
		saveDurationNs := int64(saveDuration)
		metrics.TotalIndexTime.Add(saveDurationNs)
		metrics.TotalBatchProcessingTime.Add(batchDurationNs)

		// Update max index time
		for {
			current := metrics.MaxIndexTime.Load()
			if saveDurationNs <= current {
				break
			}
			if metrics.MaxIndexTime.CompareAndSwap(current, saveDurationNs) {
				break
			}
		}

		// Update max batch processing time
		for {
			current := metrics.MaxBatchProcessingTime.Load()
			if batchDurationNs <= current {
				break
			}
			if metrics.MaxBatchProcessingTime.CompareAndSwap(current, batchDurationNs) {
				break
			}
		}

		// Update average batch size (simple moving average)
		batches := metrics.BatchesProcessed.Load()
		if batches > 0 {
			totalUpdates := metrics.TotalUpdatesBatched.Load()
			avgSize := totalUpdates / batches
			metrics.AverageBatchSize.Store(avgSize)
		}
	}

	// Record reload metric if index was reloaded
	if reloaded {
		metrics.IndexReloadsDuringSetMapping.Add(1)
	}

	// Emit batch completion event via coordinator (async, non-blocking).
	// Use a separate variable so the "start" event callback's capture of projectRoot is not overwritten (avoids data race).
	callbackComplete := getListingIndexBatchEventCallback()
	projectRootComplete := iq.getProjectRoot()
	if callbackComplete != nil && projectRootComplete != emptyValue {
		// Use system context for background event emission
		ctx := pkgctx.NewSystemContext()
		status := "complete"
		if err != nil {
			status = "error"
		}
		goroutinelabels.NewGoroutine(ConstStreamListingIndexBatchEvent, fmt.Sprintf(ConstStreamEmittingBatchStrEventForStrSizeInt, status, iq.kind, batchSize)).
			StartSimple(func() {
				callbackComplete(
					ctx,
					projectRootComplete,
					iq.getStorage(),
					iq.kind,
					batchSize,
					duration,
					status,
					err,
				)
			})
	}

	return err
}

// signalIndexUpdateCompletion signals completion of an index update request
func signalIndexUpdateCompletion(req *indexUpdateRequest, err error) {
	if req == nil {
		return
	}

	if req.opCallback != nil {
		duration := time.Since(req.startedAt)
		if err != nil {
			req.opCallback.OnError(req.operationID, err)
		} else {
			req.opCallback.OnComplete(req.operationID, map[string]any{
				"objectID": req.objectID,
				"hash":     req.hash,
			}, duration)
		}
	}

	if req.done != nil {
		req.done <- err
		close(req.done)
	}
}
