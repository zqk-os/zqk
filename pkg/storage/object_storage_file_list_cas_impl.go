package storage

import (
	"context"
	"path/filepath"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/crud"
)

func (f *FileObjectStorage) listCASPath(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, effectiveLimit int, cacheable bool, eventLogger *logging.EventLogger) (*QueryResult, error) {
	cas, err := f.getContentAddressableStorage(filter.Kind)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgGetCAS).Wrap(err)
	}
	// Fast path: high-volume kind list with created_at time window and sort by created_at when cache is populated.
	// Avoids loading all objects then filtering/sorting; uses cache for O(log n) ID range then loads only the page (CAS or stream).
	if IsHighVolumeKindForCache(filter.Kind) && filter.SortBy == objects.FieldKeyCreatedAt && filter.SortAsc &&
		crud.ListFilterIsOnlyCreatedAtRange(filter.Filters) {
		if cache := GetGlobalHighVolumeEventCache(); cache != nil && cache.IsPopulatedForProject(f.projectRoot) {
			startTime, endTime, ok := crud.ParseCreatedAtRangeFromFilters(filter.Filters)
			if ok {
				requestCount := filter.Offset + effectiveLimit
				if effectiveLimit == 0 {
					requestCount = 0
				}
				eventIDs := cache.QueryByTimeWindow(startTime, endTime, requestCount)
				// When cache returns 0 or too few, fall through to full CAS path so aggregation/CLI still get events
				// (e.g. cache is stale, partial, or built with different time range).
				if len(eventIDs) > filter.Offset {
					pageIDs := eventIDs[filter.Offset:]
					if effectiveLimit > 0 && len(pageIDs) > effectiveLimit {
						pageIDs = pageIDs[:effectiveLimit]
					}
					// Load only this page of objects (bounded parallel read)
					EmitListCountWaitProgress(ctx)
					listCtx, err := AcquireListCountSlot(ctx)
					if err != nil {
						return nil, err
					}
					defer ReleaseListCountSlot(ctx)
					objectList, listErr := f.listAuditEventsByIDs(listCtx, cas, filter.Kind, pageIDs, filter.Filters)
					if listErr != nil {
						return nil, listErr
					}
					objectList = applyListFieldProjection(filter.Kind, filter, objectList)
					result := &QueryResult{
						Objects: objectList,
						Groups:  make(map[string][]map[string]any),
						Meta:    map[string]any{"total_count": len(eventIDs), ConstStreamReturnedCount: len(objectList)},
					}
					if cacheable && effectiveLimit > 0 {
						SetListCache(f.projectRoot, &filter, effectiveLimit, result)
					}
					return result, nil
				}
				// Cache returned 0 or too few for this offset; fall through to full path so aggregation gets events
				StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainHVTimeWindowCacheSparseDebug).
					Int("cache_ids", len(eventIDs)).
					Int("offset", filter.Offset).
					Log()
			}
		}
	}
	// Fast path: audit_event created_at range when cache didn't help — walk only date buckets in range and load those files.
	// Avoids loading all 194k CAS IDs (e.g. 120d window may have ~80k files, so we load 80k instead of 194k).
	// Always try the date-range walk when we have a created_at range; do not require usesBucketedStorage so that
	// audit_event with YYYY-MM subdirs is used even if the kind was not yet detected as bucketed (e.g. cache).
	if filter.Kind == objects.KindAuditEvent && filter.SortBy == objects.FieldKeyCreatedAt && filter.SortAsc &&
		crud.ListFilterIsOnlyCreatedAtRange(filter.Filters) {
		startTime, endTime, ok := crud.ParseCreatedAtRangeFromFilters(filter.Filters)
		if ok {
			dirName := objects.GetDirectoryFromKind(filter.Kind)
			if dirName != emptyValue {
				kindDir := filepath.Join(f.processDir, dirName)
				pathsInRange := f.walkBucketedStorageByDateRange(ctx, kindDir, startTime, endTime)
				if len(pathsInRange) > 0 {
					EmitListCountWaitProgress(ctx)
					listCtx, err := AcquireListCountSlot(ctx)
					if err != nil {
						return nil, err
					}
					defer ReleaseListCountSlot(ctx)
					objectList, listErr := f.listObjectsFromPaths(listCtx, pathsInRange, filter.Kind, filter.Filters)
					if listErr != nil {
						return nil, listErr
					}
					sortObjects(objectList, objects.FieldKeyCreatedAt, true)
					totalCount := len(objectList)
					start := filter.Offset
					if start > totalCount {
						start = totalCount
					}
					end := start + effectiveLimit
					if effectiveLimit == 0 {
						end = totalCount
					} else if end > totalCount {
						end = totalCount
					}
					objectList = objectList[start:end]
					objectList = applyListFieldProjection(filter.Kind, filter, objectList)
					result := &QueryResult{
						Objects: objectList,
						Groups:  make(map[string][]map[string]any),
						Meta:    map[string]any{"total_count": totalCount, ConstStreamReturnedCount: len(objectList)},
					}
					skipCacheEmpty := len(objectList) == 0
					if cacheable && effectiveLimit > 0 && !skipCacheEmpty {
						SetListCache(f.projectRoot, &filter, effectiveLimit, result)
					}
					return result, nil
				}
			}
		}
	}
	// Fast path: high volume kind "older than" (created_at $lt/$lte) for retention/cleanup. Use cache to get candidate IDs
	// and load only those instead of loading all objects (avoids legacy scan + reads per batch).
	if IsHighVolumeKindForCache(filter.Kind) && filter.SortBy == objects.FieldKeyCreatedAt && filter.SortAsc {
		if cutoff, ok := crud.ParseCreatedAtOlderThan(filter.Filters); ok {
			if cache := GetGlobalHighVolumeEventCache(); cache != nil && cache.IsPopulatedForProject(f.projectRoot) {
				requestCount := filter.Offset + effectiveLimit
				if effectiveLimit == 0 {
					requestCount = 2000
				} else if requestCount < 2000 {
					requestCount *= 2 // headroom for status filter
					if requestCount > 2000 {
						requestCount = 2000
					}
				}
				eventIDs := cache.QueryOlderThan(cutoff, requestCount)
				// When cache returns 0, fall through to full CAS path so retention can still find old events
				// (e.g. cache timestamps may differ from on-disk created_at, or cache is partial).
				if len(eventIDs) > 0 {
					EmitListCountWaitProgress(ctx)
					listCtx, err := AcquireListCountSlot(ctx)
					if err != nil {
						return nil, err
					}
					defer ReleaseListCountSlot(ctx)
					objectList, listErr := f.listAuditEventsByIDs(listCtx, cas, filter.Kind, eventIDs, filter.Filters)
					if listErr != nil {
						return nil, listErr
					}
					// Sort by created_at asc (cache order is already by time; listAuditEventsByIDs may not preserve it)
					sortObjects(objectList, objects.FieldKeyCreatedAt, true)
					totalCount := len(objectList)
					start := filter.Offset
					if start > len(objectList) {
						start = len(objectList)
					}
					end := start + effectiveLimit
					if effectiveLimit == 0 {
						end = len(objectList)
					} else if end > len(objectList) {
						end = len(objectList)
					}
					objectList = objectList[start:end]
					objectList = applyListFieldProjection(filter.Kind, filter, objectList)
					result := &QueryResult{
						Objects: objectList,
						Groups:  make(map[string][]map[string]any),
						Meta:    map[string]any{"total_count": totalCount, ConstStreamReturnedCount: len(objectList)},
					}
					return result, nil
				}
			}
		}
	}
	// When List has Limit and SortBy created_at ascending, use index OldestIDs so we only read that many (e.g. 5k for retention) instead of all CAS IDs (e.g. 86k). Respects batch size and avoids "List does 86k reads then returns 5k".
	if effectiveLimit > 0 && filter.SortBy == objects.FieldKeyCreatedAt && filter.SortAsc && !StreamStorageEnabledForKind(filter.Kind) {
		idx := cas.GetIndex()
		if idx != nil {
			oldest := idx.OldestIDs(filter.Offset + effectiveLimit)
			if len(oldest) > filter.Offset {
				ids := oldest[filter.Offset:]
				if len(ids) > effectiveLimit {
					ids = ids[:effectiveLimit]
				}
				EmitListCountWaitProgress(ctx)
				listCtx, err := AcquireListCountSlot(ctx)
				if err != nil {
					return nil, err
				}
				defer ReleaseListCountSlot(ctx)
				objectList, listErr := f.listAuditEventsByIDs(listCtx, cas, filter.Kind, ids, filter.Filters)
				if listErr != nil {
					return nil, listErr
				}
				sortObjects(objectList, objects.FieldKeyCreatedAt, true)
				objectList = applyListFieldProjection(filter.Kind, filter, objectList)
				result := &QueryResult{
					Objects: objectList,
					Groups:  make(map[string][]map[string]any),
					Meta:    map[string]any{"total_count": len(objectList), ConstStreamReturnedCount: len(objectList)},
				}
				if cacheable && effectiveLimit > 0 {
					SetListCache(f.projectRoot, &filter, effectiveLimit, result)
				}
				return result, nil
			}
		}
	}

	return f.listCASPathGeneral(ctx, secCtx, storageCtx, filter, effectiveLimit, cacheable, cas, eventLogger)
}
