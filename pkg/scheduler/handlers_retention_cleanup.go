package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
)

// cleanupOldObjects lists objects of kind with created_at < cutoff and status not in protect_statuses, then deletes in batches.
// Strategy does not remove objects in active-like status.
func (h *RetentionToleranceHandler) cleanupOldObjects(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	jobID, kind string,
	cutoff time.Time,
	protectStatuses []string,
	batchSize, maxBatches, bulkDeleteWorkers int,
) int {
	batchSize, maxBatches = normalizeRetentionBatchLimits(batchSize, maxBatches)

	cutoffStr := cutoff.Format(time.RFC3339)
	// Fast path: high volume cache avoids full List() and Stream Registry parsing overheads.
	if storagepkg.IsHighVolumeKindForCache(kind) {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil && cache.IsPopulatedForProject(h.projectRoot) {
			toDelete := batchSize * maxBatches
			if maxBatches == retentionToleranceUnlimitedMaxBatches {
				toDelete = cache.CountByKind(kind) // or some large number
			}

			// If we have protect statuses, request more IDs from the cache so we still have enough after filtering
			queryCount := toDelete
			if len(protectStatuses) > 0 {
				queryCount = toDelete * 5
				if queryCount > cache.CountByKind(kind) {
					queryCount = cache.CountByKind(kind)
				}
			}

			idsToDelete := cache.QueryOlderThan(cutoff, queryCount)
			if len(idsToDelete) > 0 {
				// Filter by protect statuses
				if len(protectStatuses) > 0 {
					var filteredIDs []string
					for _, id := range idsToDelete {
						obj, err := h.storage.Read(ctx, secCtx, id)
						if err == nil {
							status, _ := obj[objects.FieldKeyStatus].(string)
							protected := false
							for _, p := range protectStatuses {
								if status == p {
									protected = true
									break
								}
							}
							if !protected {
								filteredIDs = append(filteredIDs, id)
								if len(filteredIDs) >= toDelete {
									break
								}
							}
						} else if errors.Is(err, storagepkg.ErrObjectNotFound) || strings.Contains(err.Error(), "not found") {
							// If it's not found, it's already deleted or phantom, so we can "delete" it to clear it
							filteredIDs = append(filteredIDs, id)
							if len(filteredIDs) >= toDelete {
								break
							}
						}
					}
					idsToDelete = filteredIDs
				}

				if len(idsToDelete) > 0 {
					RetentionToleranceLog(h.logger).Info(LogEventRetentionToleranceCleanedUpByTolerance).
						JobID(jobID).
						Kind(kind).
						CandidateCount(len(idsToDelete)).
						Log()

					var totalDeleted int
					interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
					for i := 0; i < len(idsToDelete) && totalDeleted < len(idsToDelete); i += batchSize {
						if err := interrupt.Check(ctx); err != nil {
							break
						}
						end := i + batchSize
						if end > len(idsToDelete) {
							end = len(idsToDelete)
						}
						batch := idsToDelete[i:end]

						h.emitProgress(fmt.Sprintf("Cleanup (fast-path) %s: batch %d/%d...", kind, (i/batchSize)+1, (len(idsToDelete)+batchSize-1)/batchSize))

						var n int
						when.When(func() bool { _, ok := h.storage.(*storagepkg.FileObjectStorage); return ok }).Then(func() {
							fileStorage := h.storage.(*storagepkg.FileObjectStorage)
							// CRITICAL: force=true bypasses f.Get(), which completely bypasses loadStreamRegistrySnapshot() memory OOMs!
							optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, batch, true, bulkDeleteWorkers)
							if delErr == nil {
								n = optRes.SuccessCount
							}
						}).OrElse(func() {
							bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, batch, true)
							if delErr == nil {
								n = bulkRes.SuccessCount
							}
						}).Run()

						totalDeleted += n
						storagepkg.InvalidateListCacheForKind(kind)
					}
					if totalDeleted > 0 {
						cache.InvalidateForProject(h.projectRoot)
					}
					return totalDeleted
				}

				// If the cache was populated but yielded 0 items to delete (either because none were older than cutoff,
				// or all were protected statuses), DO NOT fall back to the slow path. For high volume stream kinds,
				// the slow path will OOM. We trust the cache.
				return 0
			}

			// If cache is populated but QueryOlderThan returned 0 items, DO NOT fall back.
			return 0
		}
	}

	return h.cleanupOldObjectsSlowPath(ctx, secCtx, storageCtx, jobID, kind, protectStatuses, batchSize, maxBatches, bulkDeleteWorkers, cutoffStr)
}

func (h *RetentionToleranceHandler) cleanupOldObjectsSlowPath(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	jobID, kind string,
	protectStatuses []string,
	batchSize, maxBatches, bulkDeleteWorkers int,
	cutoffStr string,
) int {
	filters := buildStatusExclusionFilter(protectStatuses)
	filters[objects.FieldKeyCreatedAt] = map[string]any{"$lt": cutoffStr}
	var totalDeleted int
	interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)

	var allIDs []string
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		if cas, casErr := fileStorage.GetContentAddressableStorage(kind); casErr == nil && cas != nil {
			if idx := cas.GetIndex(); idx != nil {
				limit := batchSize * maxBatches
				if maxBatches == retentionToleranceUnlimitedMaxBatches {
					limit = 100000
				} else {
					limit *= 2
				}
				allIDs = idx.OldestIDs(limit)
			}
		}
	}

	if len(allIDs) > 0 {
		for i := 0; i < len(allIDs); i += batchSize {
			if err := interrupt.Check(ctx); err != nil {
				break
			}
			if maxBatches != retentionToleranceUnlimitedMaxBatches && (i/batchSize) >= maxBatches {
				break
			}

			end := i + batchSize
			if end > len(allIDs) {
				end = len(allIDs)
			}
			chunk := allIDs[i:end]

			h.emitProgress(fmt.Sprintf("Cleanup %s: batch %d/%d (using ID pagination)...", kind, (i/batchSize)+1, (len(allIDs)+batchSize-1)/batchSize))

			chunkFilters := map[string]any{
				objects.FieldKeyID:        map[string]any{"$in": chunk},
				objects.FieldKeyCreatedAt: map[string]any{"$lt": cutoffStr},
			}
			if len(protectStatuses) > 0 {
				chunkFilters[objects.FieldKeyStatus] = map[string]any{"$nin": protectStatuses}
			}

			filter := storagepkg.ListFilter{
				Kind:    kind,
				Filters: chunkFilters,
				Limit:   batchSize,
			}
			result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
			if err != nil || len(result.Objects) == 0 {
				continue
			}

			ids := make([]string, 0, len(result.Objects))
			for _, obj := range result.Objects {
				if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
					ids = append(ids, id)
				}
			}

			if len(ids) == 0 {
				continue
			}

			var n int
			if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
				optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, ids, false, bulkDeleteWorkers)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteCleanupByAgeFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = optRes.SuccessCount
			} else {
				bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, ids, false)
				if delErr != nil {
					RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteCleanupByAgeFailed).
						WithFields(append(jobLogFieldsByIDAndErr(jobID, delErr), logging.String("kind", kind))...).
						Log()
					break
				}
				n = bulkRes.SuccessCount
			}
			totalDeleted += n
			storagepkg.InvalidateListCacheForKind(kind)
		}

		if totalDeleted > 0 && isHighVolumeKind(kind) {
			if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
				cache.InvalidateForProject(h.projectRoot)
			}
		}
		return totalDeleted
	}

	for batch := 0; batch < maxBatches; batch++ {
		if err := interrupt.Check(ctx); err != nil {
			break
		}
		if maxBatches == retentionToleranceUnlimitedMaxBatches {
			h.emitProgress(fmt.Sprintf("Cleanup %s: batch %d (unlimited cap)...", kind, batch+1))
		} else {
			h.emitProgress(fmt.Sprintf("Cleanup %s: batch %d/%d...", kind, batch+1, maxBatches))
		}
		filter := storagepkg.ListFilter{
			Kind:    kind,
			Filters: filters,
			Limit:   batchSize,
		}
		result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
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
			break
		}
		var n int
		when.When(func() bool { _, ok := h.storage.(*storagepkg.FileObjectStorage); return ok }).Then(func() {
			fileStorage := h.storage.(*storagepkg.FileObjectStorage)
			optRes, delErr := fileStorage.BulkDeleteOptimized(ctx, secCtx, ids, false, bulkDeleteWorkers)
			err = delErr
			if delErr == nil {
				n = optRes.SuccessCount
			}
		}).OrElse(func() {
			bulkRes, delErr := h.storage.BulkDelete(ctx, secCtx, ids, false)
			err = delErr
			if delErr == nil {
				n = bulkRes.SuccessCount
			}
		}).Run()
		if err != nil {
			RetentionToleranceLog(h.logger).Warn(LogEventRetentionToleranceBulkDeleteCleanupByAgeFailed).
				WithFields(append(jobLogFieldsByIDAndErr(jobID, err), logging.String("kind", kind))...).
				Log()
			break
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
	if totalDeleted > 0 && isHighVolumeKind(kind) {
		if cache := storagepkg.GetGlobalHighVolumeEventCache(); cache != nil {
			cache.InvalidateForProject(h.projectRoot)
		}
	}
	return totalDeleted
}
