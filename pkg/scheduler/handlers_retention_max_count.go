package scheduler

import (
	"context"
	"fmt"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// isHighVolumeKind returns true for kinds that use the high-volume event cache / OldestIDs
// fast path for retention max_count. Single source: high_volume_kinds.yaml via
// storage.IsHighVolumeKindForCache (stream and cas entries). See HIGH_VOLUME_EVENT_INDEXES.md.
func isHighVolumeKind(kind string) bool {
	return storagepkg.IsHighVolumeKindForCache(kind)
}

// skipArchiveForOldestIDsPath is true when archive BulkUpdate would fight the OldestIDs
// cleanup/enforce path (empty protect_statuses on HV kinds). TRACK: BLI-REDACTED
func skipArchiveForOldestIDsPath(kind string, protectStatuses []string) bool {
	return isHighVolumeKind(kind) && len(protectStatuses) == 0
}

// enforceMaxCountViaHVNoProtect uses CAS OldestIDs or high-volume cache when protect_statuses is empty.
// Returns handled=true when this path completed enforcement (caller should return totalDeleted, nil).
func (h *RetentionToleranceHandler) enforceMaxCountViaHVNoProtect(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	jobID, kind string,
	toDelete, batchSize int,
	bulkDeleteWorkers, maxCount int,
) (totalDeleted int, handled bool) {
	var idsToDelete []string
	// Prefer stream registry for stream-backed kinds (CAS/HV often miss or under-count them).
	// TRACK: BLI-REDACTED
	if storagepkg.StreamStorageEnabledForKind(kind) && h.projectRoot != emptyValue {
		idsToDelete = storagepkg.OldestStreamIDsFromPersistentRegistry(h.projectRoot, kind, toDelete)
		if len(idsToDelete) > 0 {
			RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingStreamRegistryOldestIDsForMaxCount).
				JobID(jobID).
				Kind(kind).
				CandidateCount(len(idsToDelete)).
				Log()
		}
	}
	if len(idsToDelete) == 0 {
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			if cas, casErr := fileStorage.GetContentAddressableStorage(kind); casErr == nil && cas != nil {
				idx := cas.GetIndex()
				if idx != nil {
					idsToDelete = idx.OldestIDs(toDelete)
					if len(idsToDelete) > 0 {
						RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingCASOldestIDsForMaxCount).
							JobID(jobID).
							Kind(kind).
							CandidateCount(len(idsToDelete)).
							Log()
					}
				}
			}
		}
	}
	if len(idsToDelete) == 0 {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil && cache.IsPopulatedForProject(h.projectRoot) {
			idsToDelete = cache.QueryOldestByKind(kind, toDelete)
			if len(idsToDelete) > 0 {
				RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingHVCacheForMaxCount).
					JobID(jobID).
					Kind(kind).
					CandidateCount(len(idsToDelete)).
					Log()
			}
		}
	}
	if len(idsToDelete) == 0 {
		return 0, false
	}
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
	for i := 0; i < len(idsToDelete) && totalDeleted < toDelete; i += batchSize {
		if err := interrupt.Check(ctx); err != nil {
			break
		}
		end := i + batchSize
		if end > len(idsToDelete) {
			end = len(idsToDelete)
		}
		if totalDeleted+(end-i) > toDelete {
			end = i + (toDelete - totalDeleted)
		}
		batch := idsToDelete[i:end]
		if len(batch) == 0 {
			break
		}
		h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d/%d (deleted: %d/%d)...", kind, (i/batchSize)+1, (len(idsToDelete)+batchSize-1)/batchSize, totalDeleted, toDelete))
		var n int
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batch, true, bulkDeleteWorkers)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteMaxCountPathFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = optRes.SuccessCount
		} else {
			bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batch, true)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteMaxCountPathFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = bulkRes.SuccessCount
		}
		totalDeleted += n
		storagepkg.InvalidateListCacheForKind(kind)
		if n == 0 && totalDeleted == 0 {
			break
		}
	}
	if totalDeleted > 0 {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
			cache.InvalidateForProject(h.projectRoot)
		}
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceEnforcedMaxCountDeletedOldestObjects).
			JobID(jobID).
			Kind(kind).
			Deleted(totalDeleted).
			MaxCount(maxCount).
			Log()
		return totalDeleted, true
	}
	return 0, false
}

// enforceMaxCountViaHVWithProtect uses the high-volume cache excluding protect_statuses when populated.
// Returns handled=true when this path completed enforcement.
func (h *RetentionToleranceHandler) enforceMaxCountViaHVWithProtect(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	jobID, kind string,
	toDelete, batchSize int,
	bulkDeleteWorkers, maxCount int,
	protectStatuses []string,
) (totalDeleted int, handled bool) {
	cache := storagepkg.GetGlobalHighVolumeEventCache()
	if cache == nil || !cache.IsPopulatedForProject(h.projectRoot) || !cache.HasStatus() {
		return 0, false
	}
	idsToDelete := cache.QueryOldestByKindExcludingStatus(kind, toDelete, protectStatuses)
	if len(idsToDelete) == 0 {
		return 0, false
	}
	RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingHVCacheExcludeProtectMaxCount).
		JobID(jobID).
		Kind(kind).
		CandidateCount(len(idsToDelete)).
		Log()
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
	for i := 0; i < len(idsToDelete) && totalDeleted < toDelete; i += batchSize {
		if err := interrupt.Check(ctx); err != nil {
			break
		}
		end := i + batchSize
		if end > len(idsToDelete) {
			end = len(idsToDelete)
		}
		if totalDeleted+(end-i) > toDelete {
			end = i + (toDelete - totalDeleted)
		}
		batch := idsToDelete[i:end]
		if len(batch) == 0 {
			break
		}
		h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d/%d (deleted: %d/%d)...", kind, (i/batchSize)+1, (len(idsToDelete)+batchSize-1)/batchSize, totalDeleted, toDelete))
		var n int
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batch, false, bulkDeleteWorkers)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteMaxCountCachePathFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = optRes.SuccessCount
		} else {
			bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batch, false)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteMaxCountCachePathFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = bulkRes.SuccessCount
		}
		totalDeleted += n
		storagepkg.InvalidateListCacheForKind(kind)
		if n == 0 && totalDeleted == 0 {
			break
		}
	}
	if totalDeleted > 0 {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
			cache.InvalidateForProject(h.projectRoot)
		}
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceEnforcedMaxCountDeletedOldestCacheExcludeProtect).
			JobID(jobID).
			Kind(kind).
			Deleted(totalDeleted).
			MaxCount(maxCount).
			Log()
		return totalDeleted, true
	}
	return 0, false
}

// enforceMaxCount deletes oldest objects (not in protect_statuses) when count > max_count.
// Processes in batches to avoid timeouts and memory issues when there are many objects to delete.
// If count > max_count and no deletable candidates (all in active-like status), returns ErrObjectOverfill so a human must resolve.
func (h *RetentionToleranceHandler) enforceMaxCount(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	jobID, kind string,
	maxCount int,
	protectStatuses []string,
	batchSize, maxBatches, bulkDeleteWorkers int,
) (int, error) {
	if batchSize <= 0 {
		batchSize = defaultRetentionToleranceBatchSize
	}
	switch {
	case maxBatches == -1:
		maxBatches = retentionToleranceUnlimitedMaxBatches
	case maxBatches <= 0:
		maxBatches = defaultRetentionToleranceMaxBatches
	}
	h.emitProgress(fmt.Sprintf("Enforcing max_count for %s (max=%d)...", kind, maxCount))
	var count int
	var err error
	if isHighVolumeKind(kind) && h.projectRoot != emptyValue {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil && cache.IsPopulatedForProject(h.projectRoot) {
			count = cache.CountByKind(kind)
			// Stream registry is authoritative for stream-backed kinds; a project-wide HV
			// cache can be "populated" while under-counting agent_instruction (etc.), which
			// previously skipped max_count (count<=max) and left tens of thousands of AGIs.
			// TRACK: BLI-REDACTED
			if storagepkg.StreamStorageEnabledForKind(kind) {
				if regN := len(storagepkg.ListStreamIDsFromPersistentRegistry(h.projectRoot, kind)); regN > count {
					count = regN
				}
			}
		} else {
			filter := storagepkg.ListFilter{Kind: kind}
			count, err = h.storage.Count(ctx, secCtx, filter)
		}
	} else {
		filter := storagepkg.ListFilter{Kind: kind}
		count, err = h.storage.Count(ctx, secCtx, filter)
	}
	// If Count() fails (e.g., timeout), try to get count from CAS index as fallback
	var allIDs []string
	if err != nil {
		RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceCountFailedCASFallback).
			JobID(jobID).
			Kind(kind).
			WithError(err).
			Log()
		// Try to get count from CAS index directly
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			if cas, casErr := fileStorage.GetContentAddressableStorage(kind); casErr == nil && cas != nil {
				if ids, listErr := cas.ListIDs(); listErr == nil {
					count = len(ids)
					allIDs = ids
					err = nil // Clear error so we can proceed
					RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingCASIndexCountFallback).
						JobID(jobID).
						Kind(kind).
						Count(count).
						Log()
				}
			}
		}
	}
	if err != nil || count <= maxCount {
		return 0, nil
	}
	toDelete := count - maxCount
	if toDelete <= 0 {
		return 0, nil
	}
	RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceEnforcingMaxCountExceeds).
		JobID(jobID).
		Kind(kind).
		Count(count).
		MaxCount(maxCount).
		ToDelete(toDelete).
		BatchSize(batchSize).
		MaxBatches(maxBatches).
		Log()

	// Ensure List sees current data (avoid stale empty result from list cache)
	storagepkg.InvalidateListCacheForKind(kind)

	// Cap toDelete so we don't exceed batch capacity this run (remaining in subsequent runs).
	// When maxBatches is "unlimited" (-1), do not artificially cap deletes in a single run.
	if maxBatches != retentionToleranceUnlimitedMaxBatches {
		maxToProcess := batchSize * maxBatches
		if toDelete > maxToProcess {
			RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceLimitingMaxCountToBatchLimit).
				JobID(jobID).
				Kind(kind).
				ToDelete(toDelete).
				Int("max_to_process", maxToProcess).
				Log()
			toDelete = maxToProcess
		}
	}

	// CRITICAL: For high-volume kinds with no protect_statuses, get oldest IDs from index or cache (HIGH_VOLUME_EVENT_INDEXES.md).
	var totalDeleted int
	if isHighVolumeKind(kind) && len(protectStatuses) == 0 && h.projectRoot != emptyValue {
		td, done := h.enforceMaxCountViaHVNoProtect(ctx, secCtx, jobID, kind, toDelete, batchSize, bulkDeleteWorkers, maxCount)
		if done {
			return td, nil
		}
	}

	// When protect_statuses is set, avoid full List (85k reads per batch): use high-volume cache if built with status. See RETENTION_MAX_COUNT_PERFORMANCE.md.
	if isHighVolumeKind(kind) && len(protectStatuses) > 0 && h.projectRoot != emptyValue {
		td, done := h.enforceMaxCountViaHVWithProtect(ctx, secCtx, jobID, kind, toDelete, batchSize, bulkDeleteWorkers, maxCount, protectStatuses)
		if done {
			return td, nil
		}
	}

	// OPTIMIZATION: Use CAS index directly instead of List() with SortBy (which reads all files)
	// Get all IDs from CAS index if we don't have them already
	if len(allIDs) == 0 {
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			if cas, casErr := fileStorage.GetContentAddressableStorage(kind); casErr == nil && cas != nil {
				if idx := cas.GetIndex(); idx != nil {
					limit := toDelete
					maxToProcess := batchSize * maxBatches
					if maxBatches != retentionToleranceUnlimitedMaxBatches && limit > maxToProcess {
						limit = maxToProcess
					} else if limit > 100000 {
						limit = 100000
					}
					if len(protectStatuses) > 0 {
						limit *= 5
					}
					allIDs = idx.OldestIDs(limit)
				}
			}
		}
		// Fallback to List() if CAS index unavailable (should be rare)
		if len(allIDs) == 0 {
			RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceCASUnavailableListFallback).
				JobID(jobID).
				Kind(kind).
				Log()
			// Continue with old List() approach below
		}
	}

	// Process in batches to avoid timeouts and memory issues
	filters := map[string]any{}
	if len(protectStatuses) > 0 {
		filters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
	}

	// Fast path: Use CAS index IDs directly when no protect_statuses (delete first N without loading objects).
	// When protect_statuses is set, use batched List (filter status $nin) instead of Read() per object to avoid 1000s of Read calls.
	if len(allIDs) > 0 && len(protectStatuses) == 0 {
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceFastPathCASDelete).
			JobID(jobID).
			Kind(kind).
			Int("total_ids", len(allIDs)).
			Int("to_delete", toDelete).
			Log()

		interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
		for i := 0; i < len(allIDs) && totalDeleted < toDelete; i += batchSize {
			if err := interrupt.Check(ctx); err != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceMaxCountFastPathCancelled).
					JobID(jobID).
					Kind(kind).
					Int("total_deleted", totalDeleted).
					WithError(err).
					Log()
				break
			}
			batch := allIDs[i:]
			if len(batch) > batchSize {
				batch = batch[:batchSize]
			}
			if totalDeleted+len(batch) > toDelete {
				batch = batch[:toDelete-totalDeleted]
			}
			if len(batch) == 0 {
				break
			}

			h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d (deleting %d objects)...", kind, (i/batchSize)+1, len(batch)))
			var n int
			if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
				optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batch, false, bulkDeleteWorkers)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = optRes.SuccessCount
			} else {
				bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batch, false)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = bulkRes.SuccessCount
			}
			totalDeleted += n
			storagepkg.InvalidateListCacheForKind(kind)
			// Break only when no progress in this batch and no progress so far (allows continuing past partial batches)
			if n == 0 && totalDeleted == 0 {
				break
			}
		}
	} else if len(allIDs) > 0 && len(protectStatuses) > 0 {
		// Have CAS IDs but must respect protect_statuses: use batched List (status $nin) instead of Read() per object.
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceUsingBatchedListMaxCountProtect).
			JobID(jobID).
			Kind(kind).
			Int("to_delete", toDelete).
			Log()
		var batchedErr error
		totalDeleted, batchedErr = h.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, jobID, kind, toDelete, protectStatuses, batchSize, maxBatches, bulkDeleteWorkers, count, maxCount, allIDs)
		if batchedErr != nil {
			return 0, batchedErr
		}
	} else {
		// Slow path: Fallback to List() (CAS index unavailable or no allIDs)
		var batchedErr error
		totalDeleted, batchedErr = h.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, jobID, kind, toDelete, protectStatuses, batchSize, maxBatches, bulkDeleteWorkers, count, maxCount, allIDs)
		if batchedErr != nil {
			return 0, batchedErr
		}
	}

	if totalDeleted > 0 {
		RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceEnforcedMaxCountDeletedOldestObjects).
			JobID(jobID).
			Kind(kind).
			Deleted(totalDeleted).
			MaxCount(maxCount).
			Int("remaining_over_limit", count-totalDeleted-maxCount).
			Log()
		if isHighVolumeKind(kind) {
			if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
				cache.InvalidateForProject(h.projectRoot)
			}
		}
	}
	return totalDeleted, nil
}

// enforceMaxCountBatchedList uses batched List (filter status $nin protectStatuses, sort by created_at) + delete.
// Avoids per-object Read() when protect_statuses is set. Returns (deleted count, nil) or (0, ErrObjectOverfill).
func (h *RetentionToleranceHandler) enforceMaxCountBatchedList(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	jobID, kind string,
	toDelete int,
	protectStatuses []string,
	batchSize, maxBatches, bulkDeleteWorkers int,
	count, maxCount int,
	allIDs []string,
) (int, error) {
	filters := map[string]any{}
	if len(protectStatuses) > 0 {
		filters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
	}
	var totalDeleted int
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)

	if len(allIDs) > 0 {
		// Fast batched path: paginate through allIDs using $in filter. Avoids List() reading all files into memory.
		for i := 0; i < len(allIDs) && totalDeleted < toDelete; i += batchSize {
			if err := interrupt.Check(ctx); err != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceMaxCountBatchedListCancelled).
					JobID(jobID).
					Kind(kind).
					Int("batches_processed", i/batchSize).
					Int("total_deleted", totalDeleted).
					WithError(err).
					Log()
				break
			}

			end := i + batchSize
			if end > len(allIDs) {
				end = len(allIDs)
			}
			chunk := allIDs[i:end]

			chunkFilters := map[string]any{
				objects.FieldKeyID: map[string]any{"$in": chunk},
			}
			if len(protectStatuses) > 0 {
				chunkFilters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
			}

			h.emitProgress(fmt.Sprintf("Enforcing max_count %s: ID batch %d/%d (deleted: %d/%d)...", kind, (i/batchSize)+1, (len(allIDs)+batchSize-1)/batchSize, totalDeleted, toDelete))

			listFilter := storagepkg.ListFilter{
				Kind:    kind,
				Limit:   batchSize,
				Filters: chunkFilters,
			}
			result, err := h.storage.List(ctx, secCtx, storageCtx, listFilter)
			if err != nil || len(result.Objects) == 0 {
				continue
			}

			var batchToDelete []string
			for _, obj := range result.Objects {
				if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
					batchToDelete = append(batchToDelete, id)
				}
			}

			if len(batchToDelete) == 0 {
				continue
			}

			// Only delete up to the remaining amount
			if totalDeleted+len(batchToDelete) > toDelete {
				batchToDelete = batchToDelete[:toDelete-totalDeleted]
			}

			var n int
			if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
				optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batchToDelete, false, bulkDeleteWorkers)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountBatchedListFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = optRes.SuccessCount
			} else {
				bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batchToDelete, false)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountBatchedListFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = bulkRes.SuccessCount
			}
			totalDeleted += n
			storagepkg.InvalidateListCacheForKind(kind)
		}

		if totalDeleted == 0 && len(allIDs) > 0 && toDelete > 0 {
			overfillErr := errfmt.Errorf("kind %s: %w (count=%d max_count=%d)", kind, ErrObjectOverfill, count, maxCount)
			RetentionToleranceLog(h.logger).Error(LogEventRetentionToleranceObjectOverfillProtectedStatus, overfillErr).
				JobID(jobID).
				Kind(kind).
				Int("count", count).
				Int("max_count", maxCount).
				Log()
			return 0, overfillErr
		}

		return totalDeleted, nil
	}

	for batch := 0; batch < maxBatches && totalDeleted < toDelete; batch++ {
		if err := interrupt.Check(ctx); err != nil {
			RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceMaxCountBatchedListCancelled).
				JobID(jobID).
				Kind(kind).
				Int("batches_processed", batch).
				Int("total_deleted", totalDeleted).
				WithError(err).
				Log()
			break
		}
		remaining := toDelete - totalDeleted
		batchLimit := batchSize
		if remaining < batchLimit {
			batchLimit = remaining
		}
		if maxBatches == retentionToleranceUnlimitedMaxBatches {
			h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d (unlimited cap) (deleted: %d/%d)...", kind, batch+1, totalDeleted, toDelete))
		} else {
			h.emitProgress(fmt.Sprintf("Enforcing max_count %s: batch %d/%d (deleted: %d/%d)...", kind, batch+1, maxBatches, totalDeleted, toDelete))
		}
		listFilter := storagepkg.ListFilter{
			Kind:    kind,
			Limit:   batchLimit,
			Filters: filters,
		}
		result, err := h.storage.List(ctx, secCtx, storageCtx, listFilter)
		if err != nil || len(result.Objects) == 0 {
			break
		}
		ids := make([]string, 0, len(result.Objects))
		for _, obj := range result.Objects {
			if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			if totalDeleted == 0 {
				overfillErr := errfmt.Errorf("kind %s: %w (count=%d max_count=%d)", kind, ErrObjectOverfill, count, maxCount)
				RetentionToleranceLog(h.logger).Error(LogEventRetentionToleranceObjectOverfillProtectedStatus, overfillErr).
					JobID(jobID).
					Kind(kind).
					Int("count", count).
					Int("max_count", maxCount).
					Log()
				return 0, overfillErr
			}
			break
		}
		var n int
		if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
			optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, ids, false, bulkDeleteWorkers)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountBatchedListFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = optRes.SuccessCount
		} else {
			bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, ids, false)
			if delErr != nil {
				RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteEnforceMaxCountBatchedListFailed).
					WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
					Log()
				break
			}
			n = bulkRes.SuccessCount
		}
		totalDeleted += n
		storagepkg.InvalidateListCacheForKind(kind)

		if h.projectRoot != emptyValue {
			if queue := caspkg.GetListingIndexWriteQueueForProjectRoot(h.projectRoot); queue != nil {
				_ = queue.FlushKind(kind, 5*time.Second)
			}
		}

		if n < len(ids) {
			break
		}
	}
	return totalDeleted, nil
}
