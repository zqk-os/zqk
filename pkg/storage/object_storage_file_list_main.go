package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/when"
	"gopkg.in/yaml.v3"
)

// listCasWGCounter ensures unique WaitGroup IDs per List call when using CAS (avoids concurrent Lists sharing one group and panicking on Done after DeleteGroup).
var listCasWGCounter uint64

// fileStorageWGCounter ensures unique WaitGroup IDs for list_parse and count_filter (same wgManager; nanosecond collision would cause same race).
var fileStorageWGCounter uint64

const pipelineKindObjectStorageFileList = "storage.object_storage_file_list_main"

// List lists objects with filtering, sorting, pagination, and grouping.
//
// This is a pipeline wrapper for the main orchestration in ListImpl so stage-level metrics/logs
// can correlate cache/fast-path decisions and storage reads.
func (f *FileObjectStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	type state struct {
		result *QueryResult
		err    error
	}
	st := &state{}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	runCtx := ctx
	if runCtx == nil {
		runCtx = pkgctx.NewSystemContext()
	}

	pl := pipeline.NewBuilder(pipelineKindObjectStorageFileList, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			st.result, st.err = f.ListImpl(runCtx, secCtx, storageCtx, filter)
			return st, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return st, nil
		}).
		Build()

	_, runErr := pl.Run(&pipeline.Context{Ctx: runCtx, Outcome: make(map[string]any)}, st)
	if runErr != nil {
		return nil, runErr
	}
	return st.result, st.err
}

// ListImpl lists objects with filtering, sorting, pagination, and grouping.
//
//nolint:gocyclo // Complex function handling multiple code paths (CAS, file-based, bucketing, etc.)
func (f *FileObjectStorage) ListImpl(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	if filter.Kind == objects.KindAuditEvent {
	}
	// Check permission
	if err := f.checkPermission(secCtx, "read", filter.Kind); err != nil {
		return nil, err
	}

	// Effective limit for pagination (and list cache key)
	cacheable := filter.Kind != objects.KindKeystoreEntry
	effectiveLimit := effectiveListLimit(&filter, storageCtx)
	// Skip cache when listing all (limit 0) so results always reflect current storage
	if cacheable && effectiveLimit > 0 {
		if cached, ok := GetListCache(f.projectRoot, &filter, effectiveLimit); ok {
			return cached, nil
		}
	}

	// Log only when doing real list work (cache miss or non-cacheable)
	eventLogger := logging.NewEventLogger(ctx)
	StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainOperationStartingDebug).
		Kind(filter.Kind).
		Log()

	// Stream-backed kinds: list from stream registry + write-behind only; no CAS index or reads.
	if StreamStorageEnabledForKind(filter.Kind) {
		return f.listStreamBackedOnly(ctx, secCtx, storageCtx, filter, effectiveLimit, cacheable, eventLogger)
	}

	// Check if this kind uses content-addressable storage
	if f.usesContentAddressableStorage(filter.Kind) {
		cas, err := f.getContentAddressableStorage(filter.Kind)
		if err != nil {
			return nil, errfmt.Newf(ErrMsgGetCAS).Wrap(err)
		}
		// Fast path: high-volume kind list with created_at time window and sort by created_at when cache is populated.
		// Avoids loading all objects then filtering/sorting; uses cache for O(log n) ID range then loads only the page (CAS or stream).
		if IsHighVolumeKindForCache(filter.Kind) && filter.SortBy == objects.FieldKeyCreatedAt && filter.SortAsc &&
			f.listFilterIsOnlyCreatedAtRange(filter.Filters) {
			if cache := GetGlobalHighVolumeEventCache(); cache != nil && cache.IsPopulatedForProject(f.projectRoot) {
				startTime, endTime, ok := f.parseCreatedAtRangeFromFilters(filter.Filters)
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
			f.listFilterIsOnlyCreatedAtRange(filter.Filters) {
			startTime, endTime, ok := f.parseCreatedAtRangeFromFilters(filter.Filters)
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
			if cutoff, ok := parseCreatedAtOlderThan(filter.Filters); ok {
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

		// Get all IDs from index. If index is empty, populate from disk synchronously so we have something to return.
		// If index is non-empty but on-disk count disagrees (disparity), trigger background refresh so next list sees full set; return current index now (minimal blocking). See CAS_LIST_GET_CONSISTENCY.md.
		casIDs, err := cas.ListIDs()
		if err != nil {
			return nil, errfmt.Newf(ConstStreamFailedToListIdsFromContentAddressableStorage).Wrap(err)
		}
		if len(casIDs) == 0 {
			var _err_83760248 = f.EnsureCASIndexPopulatedFromScan(ctx, filter.Kind)
			if _err_83760248 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83760248).Log()
			}
			casIDs, err = cas.ListIDs()
			if err != nil {
				return nil, errfmt.Newf(ConstStreamFailedToListIdsFromContentAddressableStorage).Wrap(err)
			}
		}
		when.When(func() bool { return len(casIDs) > 0 }).Then(func() {
			diskCount, countErr := f.getCachedDiskCountOrScan(filter.Kind)
			if countErr == nil && diskCount != len(casIDs) {
				f.triggerBackgroundCASIndexRefresh(filter.Kind)
			}
		}).Run()

		// Also scan for legacy ID-named files (migration); all new objects are CAS (hash-named). Keep idToPath when bucketed.
		// Stream-backed kinds: do not read YAML dir; IDs come from listStreamIDsForKind below.
		dirName := objects.GetDirectoryFromKind(filter.Kind)
		if dirName == emptyValue {
			return nil, errfmt.Errorf(ConstStreamUnknownObjectKindStr, filter.Kind)
		}
		kindDir := filepath.Join(f.processDir, dirName)
		var idToPath map[string]string // only set for bucketed; used to avoid repeated getObjectFilePath
		idSet := make(map[string]bool)

		explicitIDs := idsFromListFilter(filter.Filters)
		if len(explicitIDs) > 0 {
			for _, id := range explicitIDs {
				idSet[id] = true
			}
		} else {
			for _, id := range casIDs {
				idSet[id] = true
			}
			if !StreamStorageEnabledForKind(filter.Kind) {
				when.When(func() bool { return f.usesBucketedStorage(filter.Kind, kindDir) }).Then(func() {
					idToPath = f.scanIDBasedFilesRecursive(kindDir, filter.Kind)
					for id := range idToPath {
						idSet[id] = true
					}
				}).OrElse(func() {
					idBasedFiles := f.scanIDBasedFiles(kindDir, filter.Kind)
					for _, id := range idBasedFiles {
						idSet[id] = true
					}
				}).Run()
			}
			if StreamStorageEnabledForKind(filter.Kind) {
				for _, id := range f.listStreamIDsForKind(filter.Kind) {
					idSet[id] = true
				}
			}
		}

		// Write-behind: merge pending create/update IDs and exclude pending deletes
		if f.writeBuf != nil {
			for _, id := range f.writeBuf.ListPendingIDs(filter.Kind) {
				idSet[id] = true
			}
			for id := range f.writeBuf.PendingDeletesForKind(filter.Kind) {
				delete(idSet, id)
			}
		}

		// Build deterministic ID slice for parallel dispatch (sort so list order is stable)
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		// Limit concurrent CAS list operations so N jobs don't create N*64 workers (see list_count_concurrency.go)
		EmitListCountWaitProgress(ctx)
		listCtx, err := AcquireListCountSlot(ctx)
		if err != nil {
			return nil, err
		}
		defer ReleaseListCountSlot(ctx)

		// Bounded parallel read+parse: fixed worker count to avoid thread exhaustion when listing
		// many objects (one goroutine per ID was causing 10k+ goroutines and fatal thread exhaustion).
		listMaxConcurrentReads := getListReadWorkers()
		var parsedObjects []*objects.ParsedObject
		type casParseResult struct {
			parsed *objects.ParsedObject
		}
		results := make(chan casParseResult, listMaxConcurrentReads*2)
		workCh := make(chan string, len(ids))
		for _, id := range ids {
			workCh <- id
		}
		close(workCh)
		numWorkers := listMaxConcurrentReads
		if len(ids) < numWorkers {
			numWorkers = len(ids)
		}
		var listWg sync.WaitGroup
		listWg.Add(numWorkers)
		listBud := goroutinelabels.DefaultBudget()
		for w := 0; w < numWorkers; w++ {
			casBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListCasRead, ConstStreamListCasWorker)
			if listBud != nil {
				casBuilder = casBuilder.WithBudget(listBud)
			}
			casBuilder.StartSimple(func() {
				defer listWg.Done()
				for id := range workCh {
					select {
					case <-listCtx.Done():
						return
					default:
					}
					var result casParseResult
					var obj map[string]any
					data, err := cas.Read(id)
					if err == nil {
						var _err_83764173 = yaml.Unmarshal(data, &obj)
						if _err_83764173 != nil {
							logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83764173).Log()
						}
					} else if errors.Is(err, ErrObjectNotFound) || strings.Contains(err.Error(), "not found") {
						filePath := ""
						if idToPath != nil {
							filePath = idToPath[id]
						}
						if filePath == emptyValue {
							var _err_83764428 error
							filePath, _err_83764428 = f.getObjectFilePath(id, filter.Kind)
							if _err_83764428 != nil {
								logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83764428).Log()
							}
						}
						if filePath != emptyValue {
							var _err_83764528 error
							obj, _err_83764528 = f.readObjectFileFast(filePath)
							if _err_83764528 != nil {
								logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83764528).Log()
							}
						}
					}
					if obj != nil {
						obj = materializeCasYAMLMapAfterLoad(f.projectRoot, filter.Kind, id, obj)
						parsed, _err_83764611 := objects.ParseObject(obj)
						if _err_83764611 != nil {
							logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83764611).Log()
						}
						when.When(func() bool { return parsed != nil }).Then(func() {
							result.parsed = parsed
						}).OrElse(func() {
							result.parsed = &objects.ParsedObject{Raw: obj}
						}).Run()
					}
					// Don't send on results after context cancelled (closer may have closed channel)
					select {
					case <-listCtx.Done():
						return
					case results <- result:
					}
				}
			})
		}
		closerBud := goroutinelabels.DefaultBudget()
		var closeOnce sync.Once
		closeResults := func() { closeOnce.Do(func() { close(results) }) }
		wgDone := make(chan struct{})
		goroutinelabels.NewGoroutine(ConstStreamFileStorageListCasWaiter, ConstStreamWaitingForListCasWorkers).StartSimple(func() {
			listWg.Wait()
			close(wgDone)
		})
		casCloserBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListCasResultsCloser, ConstStreamWaitingForListCasWorkersAndClosingResultsChannel).
			WithCleanup(closeResults)
		if closerBud != nil {
			casCloserBuilder = casCloserBuilder.WithBudget(closerBud)
		}
		casCloserBuilder.StartSimple(func() {
			select {
			case <-listCtx.Done():
				closeResults()
			case <-wgDone:
				closeResults()
			}
		})

		parsedObjects = make([]*objects.ParsedObject, 0, len(ids))
		for r := range results {
			if r.parsed != nil {
				parsedObjects = append(parsedObjects, r.parsed)
			}
		}
		if err := listCtx.Err(); err != nil {
			return nil, err
		}

		// Apply filters using parsed objects (same as regular List)
		filteredObjects := make([]*objects.ParsedObject, 0, len(parsedObjects))
		for _, parsed := range parsedObjects {
			if f.matchesFiltersParsed(parsed, filter.Filters) {
				filteredObjects = append(filteredObjects, parsed)
			}
		}

		// Sort using parsed objects (same as regular List)
		when.When(func() bool { return filter.SortBy != emptyValue }).Then(func() {
			f.sortParsedObjectsSlice(filteredObjects, filter.SortBy, filter.SortAsc)
		}).OrElse(func() {
			f.sortParsedObjectsSlice(filteredObjects, objects.FieldKeyID, true)
		}).Run()

		// Convert parsed objects back to maps for output
		objectList := make([]map[string]any, len(filteredObjects))
		for i, parsed := range filteredObjects {
			objectList[i] = parsed.ToMap()
		}

		// Final deduplication by ID (safety check - should already be deduplicated above)
		finalSeenIDs := make(map[string]bool)
		deduplicatedList := make([]map[string]any, 0, len(objectList))
		for _, obj := range objectList {
			objID, ok := obj[objects.FieldKeyID].(string)
			when.When(func() bool { return ok && objID != emptyValue }).Then(func() {
				if !finalSeenIDs[objID] {
					finalSeenIDs[objID] = true
					deduplicatedList = append(deduplicatedList, obj)
				}
			}).OrElse(func() {
				deduplicatedList = append(deduplicatedList, obj)
			}).Run()
		}
		objectList = deduplicatedList

		// Build result (same as regular List; effectiveLimit computed at start of List)
		result := &QueryResult{
			Objects: []map[string]any{},
			Groups:  make(map[string][]map[string]any),
			Meta:    map[string]any{"total_count": len(filteredObjects)},
		}

		// Group if requested (same as regular List)
		if filter.GroupBy != emptyValue && storageCtx != nil && storageCtx.EnableGrouping {
			grouped := f.groupObjects(objectList, filter.GroupBy, storageCtx.MaxGroupSize)
			result.Groups = grouped
			result.Meta["total_groups"] = len(grouped)
			objectList = f.flattenGroups(grouped)
		}

		// Paginate (same as regular List)
		if filter.Offset > 0 || effectiveLimit > 0 {
			start := filter.Offset
			if start > len(objectList) {
				start = len(objectList)
			}
			end := start + effectiveLimit
			if end > len(objectList) {
				end = len(objectList)
			}
			if effectiveLimit == 0 {
				end = len(objectList)
			}
			objectList = objectList[start:end]
		}

		// Apply keystore-specific access control if needed (for CAS objects)
		keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
		if keystoreDir != emptyValue && objects.GetDirectoryFromKind(filter.Kind) == keystoreDir {
			filteredObjects := []map[string]any{}
			for _, obj := range objectList {
				// Check ownership before applying access control
				entryAccountID, _ := obj[objects.FieldKeyAccountID].(string)
				userOwnsEntry := entryAccountID != emptyValue && entryAccountID == secCtx.AccountID
				isAdmin := slices.Contains(secCtx.Roles, "admin")

				// Only include entries the user owns, or if they're admin/system
				if userOwnsEntry || isAdmin || secCtx.AccountID == pkgctx.SystemAccountID {
					filtered := f.applyKeystoreAccessControl(obj, secCtx)
					filteredObjects = append(filteredObjects, filtered)
				}
				// Otherwise, skip this entry (user doesn't own it and isn't admin)
			}
			objectList = filteredObjects
		}

		objectList = applyListFieldProjection(filter.Kind, filter, objectList)
		result.Objects = objectList
		projectQueryResultGroups(filter.Kind, filter, result)
		result.Meta[ConstStreamReturnedCount] = len(objectList)

		// Do not cache empty list for audit_event created_at range so a timeout or transient 0 doesn't poison future runs
		skipCacheEmpty := filter.Kind == objects.KindAuditEvent && f.listFilterIsOnlyCreatedAtRange(filter.Filters) && len(objectList) == 0
		if cacheable && effectiveLimit > 0 && !skipCacheEmpty {
			SetListCache(f.projectRoot, &filter, effectiveLimit, result)
		}
		return result, nil
	}

	// Get directory for this kind
	dirName := objects.GetDirectoryFromKind(filter.Kind)
	if dirName == emptyValue {
		return nil, errfmt.Errorf(ConstStreamUnknownObjectKindStr, filter.Kind)
	}

	kindDir := filepath.Join(f.processDir, dirName)
	StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainOperationDebug).
		String("kind_dir", kindDir).
		String("process_dir", f.processDir).
		String("dir_name", dirName).
		Log()

	// Check if directory exists
	if _, err := os.Stat(kindDir); err != nil {
		if os.IsNotExist(err) {
			StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainKindDirMissingDebug).
				String("kind_dir", kindDir).
				Log()
			emptyResult := &QueryResult{
				Objects: []map[string]any{},
				Groups:  make(map[string][]map[string]any),
				Meta:    map[string]any{"total_count": 0},
			}
			if cacheable && effectiveLimit > 0 {
				SetListCache(f.projectRoot, &filter, effectiveLimit, emptyResult)
			}
			return emptyResult, nil
		}
		return nil, errfmt.Newf(ConstStreamFailedToStatKindDirectory).Wrap(err)
	}

	// Get bucketing strategy for this kind to understand storage structure
	// Skip for system objects that don't need bucketing to avoid circular dependencies
	var bucketStrategy BucketStrategy
	if filter.Kind != objects.KindBucketingStrategy && filter.Kind != objects.KindSynonym {
		registry := f.getBucketStrategyRegistry(ctx)
		if registry != nil {
			strategy, err := registry.GetStrategyForKind(ctx, filter.Kind)
			if err == nil {
				bucketStrategy = strategy
			}
		}
	}

	// Limit concurrent file-heavy list operations so N jobs don't create N*64 workers (see list_count_concurrency.go)
	EmitListCountWaitProgress(ctx)
	listCtx, err := AcquireListCountSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer ReleaseListCountSlot(ctx)

	// Collect file paths using bucketing strategy to understand storage structure
	StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainCollectingFilePathsDebug).
		Kind(filter.Kind).
		String("kind_dir", kindDir).
		String("has_strategy", fmt.Sprintf("%v", bucketStrategy != nil)).
		Log()
	timeRange := f.extractTimeRangeFromFilters(filter.Filters)
	filePaths, err := f.collectFilePathsWithStrategy(listCtx, filter.Kind, kindDir, bucketStrategy, timeRange, eventLogger)
	if err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToCollectFilePaths).Wrap(err)
	}

	// Deduplicate file paths (in case same file is found multiple times)
	// Use absolute paths for reliable deduplication
	seenPaths := make(map[string]bool)
	deduplicatedPaths := make([]string, 0, len(filePaths))
	for _, path := range filePaths {
		// Normalize to absolute path for reliable comparison
		absPath, err := filepath.Abs(path)
		if err != nil {
			// If abs fails, use original path
			absPath = path
		}
		// Also normalize separators
		absPath = filepath.Clean(absPath)
		if !seenPaths[absPath] {
			seenPaths[absPath] = true
			deduplicatedPaths = append(deduplicatedPaths, path) // Keep original path format
		}
	}
	filePaths = deduplicatedPaths

	// Parse objects once for optimized filtering/sorting
	// Complex nested structures and lists are extracted as typed fields
	// Original is preserved on disk, so we modify the map in place
	// Use parallel reads for better performance with many files
	var parsedObjects []*objects.ParsedObject
	// Track seen IDs to deduplicate as we collect (more efficient than post-processing)
	seenIDs := make(map[string]bool)

	StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainParallelReadStartDebug).
		Int("file_count", len(filePaths)).
		Log()

	// Read and parse files in parallel
	type parseResult struct {
		parsed *objects.ParsedObject
		err    error
	}

	// Bounded worker pool: fixed number of goroutines (not one per file) to avoid thread
	// exhaustion when listing many files. Each worker pulls paths from workCh and sends parseResult.
	listMaxWorkers := getListReadWorkers()
	results := make(chan parseResult, listMaxWorkers*2)
	workCh := make(chan string, listMaxWorkers*2)
	numWorkers := listMaxWorkers
	if len(filePaths) < numWorkers {
		numWorkers = len(filePaths)
	}

	// Check context before starting
	if listCtx.Err() != nil {
		return nil, listCtx.Err()
	}

	if numWorkers == 0 {
		parsedObjects = make([]*objects.ParsedObject, 0)
		StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainNoFilePathsDebug).Log()
		close(results)
	} else {
		// Send all paths to work channel
		for _, p := range filePaths {
			select {
			case <-listCtx.Done():
				return nil, listCtx.Err()
			case workCh <- p:
			}
		}
		close(workCh)

		var listWg sync.WaitGroup
		listWg.Add(numWorkers)
		fileListBud := goroutinelabels.DefaultBudget()
		for w := 0; w < numWorkers; w++ {
			parseBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageParseFile, ConstStreamListFileWorker)
			if fileListBud != nil {
				parseBuilder = parseBuilder.WithBudget(fileListBud)
			}
			parseBuilder.StartSimple(func() {
				defer listWg.Done()
				for filePathCopy := range workCh {
					select {
					case <-listCtx.Done():
						results <- parseResult{nil, listCtx.Err()}
						continue
					default:
					}
					obj, err := f.readObjectFileFast(filePathCopy)
					if err != nil {
						results <- parseResult{nil, err}
						continue
					}
					parsed, err := objects.ParseObject(obj)
					if err != nil {
						parsed = &objects.ParsedObject{Raw: obj}
					}
					objKind, hasKind := parsed.GetField(objects.FieldKeyKind)
					_ = hasKind
					if objKind == nil {
						if rawKind := objects.GetString(parsed.Raw, objects.FieldKeyKind); rawKind != "" {
							objKind = rawKind
						}
					}
					objKindStr, ok := objKind.(string)
					if ok && objKindStr != filter.Kind {
						results <- parseResult{nil, nil}
						continue
					}
					matches := f.matchesFiltersParsed(parsed, filter.Filters)
					if matches {
						select {
						case results <- parseResult{parsed, nil}:
						case <-listCtx.Done():
							return
						}
					} else {
						select {
						case results <- parseResult{nil, nil}:
						case <-listCtx.Done():
							return
						}
					}
				}
			})
		}

		fileCloserBud := goroutinelabels.DefaultBudget()
		fileCloserBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListResultsCloser, ConstStreamWaitingForListWorkersAndClosingResultsChannel).
			WithCleanup(func() {
				close(results)
			})
		if fileCloserBud != nil {
			fileCloserBuilder = fileCloserBuilder.WithBudget(fileCloserBud)
		}
		fileCloserBuilder.StartSimple(func() {
			listWg.Wait()
			StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainWorkersCompletedDebug).Log()
		})
	}

	StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainCollectingResultsDebug).
		Int("file_count", len(filePaths)).
		Log()

	// Collect results with context cancellation
	collectedCount := 0
	for {
		select {
		case <-listCtx.Done():
			StorageLog(eventLogger.Logger()).Warn(LogEventStorageListMainContextCancelledCollectWarn).
				Int("collected", collectedCount).
				Int("expected", len(filePaths)).
				Log()
			return nil, listCtx.Err()
		case result, ok := <-results:
			if !ok {
				// Channel closed, all results collected
				StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainAllResultsCollectedDebug).
					Int(ConstStreamTotalCollected, collectedCount).
					Int("total_files", len(filePaths)).
					Log()
				goto done
			}
			collectedCount++
			if collectedCount%10 == 0 {
				StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainCollectingResultsDebug).
					Int("collected", collectedCount).
					Int("total_files", len(filePaths)).
					Log()
			}
			if result.err != nil {
				// Skip files that can't be read (log error if needed)
				StorageLog(eventLogger.Logger()).Warn(LogEventStorageListMainFailedReadFileWarn).
					WithError(result.err).
					Log()
				continue
			}
			if result.parsed != nil {
				// Deduplicate by ID as we collect (more efficient)
				var idStr string
				objID, hasID := result.parsed.GetField(objects.FieldKeyID)
				_ = hasID
				when.When(func() bool { return objID != nil }).Then(func() {
					if str, ok := objID.(string); ok {
						idStr = str
					} else {
						idStr = fmt.Sprintf("%v", objID)
					}
				}).OrElse(func() {
					if rawID := objects.GetString(result.parsed.Raw, objects.FieldKeyID); rawID != "" {
						idStr = rawID
					} else if rawID := result.parsed.Raw[objects.FieldKeyID]; rawID != nil {
						idStr = fmt.Sprintf("%v", rawID)
					}
				}).Run()
				when.When(func() bool { return idStr != emptyValue }).Then(func() {
					if !seenIDs[idStr] {
						seenIDs[idStr] = true
						parsedObjects = append(parsedObjects, result.parsed)
					}
				}).OrElse(func() {
					parsedObjects = append(parsedObjects, result.parsed)
				}).Run()
			}
		}
	}
done:

	// Write-behind: merge pending create/update into list and exclude pending deletes (non-CAS path)
	if f.writeBuf != nil {
		deletes := f.writeBuf.PendingDeletesForKind(filter.Kind)
		for _, id := range f.writeBuf.ListPendingIDs(filter.Kind) {
			if seenIDs[id] {
				continue
			}
			op := f.writeBuf.GetPending(filter.Kind, id)
			if op == nil || len(op.Data) == 0 {
				continue
			}
			var obj map[string]any
			if err := yaml.Unmarshal(op.Data, &obj); err != nil {
				continue
			}
			obj[objects.FieldKeyKind] = filter.Kind
			parsed, _err_83778971 := objects.ParseObject(obj)
			if _err_83778971 != nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83778971).Log()
			}
			seenIDs[id] = true
			parsedObjects = append(parsedObjects, parsed)
		}
		if len(deletes) > 0 {
			filtered := parsedObjects[:0]
			for _, p := range parsedObjects {
				objID, hasID := p.GetField(objects.FieldKeyID)
				_ = hasID
				idStr, hasIDStr := objID.(string)
				_ = hasIDStr
				if idStr == emptyValue {
					if raw := objects.GetString(p.Raw, objects.FieldKeyID); raw != "" {
						idStr = raw
					}
				}
				if idStr != emptyValue && deletes[idStr] {
					continue
				}
				filtered = append(filtered, p)
			}
			parsedObjects = filtered
		}
	}

	StorageLog(eventLogger.Logger()).Debug(LogEventStorageListMainCollectedParsedSortDebug).
		Int("object_count", len(parsedObjects)).
		Int("unique_ids", len(seenIDs)).
		Log()

	// Sort using parsed objects (faster for typed field access)
	// Sort the parsedObjects slice, which maintains order for both parsed and raw
	// If no sort is specified, sort by ID for deterministic pagination
	when.When(func() bool { return filter.SortBy != emptyValue }).Then(func() {
		f.sortParsedObjectsSlice(parsedObjects, filter.SortBy, filter.SortAsc)
	}).OrElse(func() {
		f.sortParsedObjectsSlice(parsedObjects, objects.FieldKeyID, true)
	}).Run()

	// Convert parsed objects back to maps for output (restores extracted fields)
	objectList := make([]map[string]any, len(parsedObjects))
	for i, parsed := range parsedObjects {
		objectList[i] = parsed.ToMap()
	}

	// Final deduplication by ID (safety check - should already be deduplicated above)
	finalSeenIDs := make(map[string]bool)
	deduplicatedList := make([]map[string]any, 0, len(objectList))
	for _, obj := range objectList {
		objID, ok := obj[objects.FieldKeyID].(string)
		when.When(func() bool { return ok && objID != emptyValue }).Then(func() {
			if !finalSeenIDs[objID] {
				finalSeenIDs[objID] = true
				deduplicatedList = append(deduplicatedList, obj)
			}
		}).OrElse(func() {
			deduplicatedList = append(deduplicatedList, obj)
		}).Run()
	}
	objectList = deduplicatedList

	// Build result (effectiveLimit computed at start of List)
	result := &QueryResult{
		Objects: []map[string]any{},
		Groups:  make(map[string][]map[string]any),
		Meta:    map[string]any{"total_count": len(objectList)},
	}

	// Group if requested (grouping happens before pagination for proper grouping)
	if filter.GroupBy != emptyValue && storageCtx != nil && storageCtx.EnableGrouping {
		// Group objects
		grouped := f.groupObjects(objectList, filter.GroupBy, storageCtx.MaxGroupSize)
		result.Groups = grouped
		result.Meta["total_groups"] = len(grouped)
		// For List, we return flat list but preserve grouping in metadata if needed
		// Convert grouped map to flat list for backward compatibility
		objectList = f.flattenGroups(grouped)
	}

	// Paginate
	if filter.Offset > 0 || effectiveLimit > 0 {
		start := filter.Offset
		if start > len(objectList) {
			start = len(objectList)
		}
		end := start + effectiveLimit
		if end > len(objectList) {
			end = len(objectList)
		}
		if effectiveLimit == 0 {
			end = len(objectList)
		}
		objectList = objectList[start:end]
	}

	// Apply keystore-specific access control if needed
	if filter.Kind == objects.KindKeystoreEntry {
		filteredObjects := []map[string]any{}
		for _, obj := range objectList {
			// Check ownership before applying access control
			entryAccountID, _ := obj[objects.FieldKeyAccountID].(string)
			userOwnsEntry := entryAccountID != emptyValue && entryAccountID == secCtx.AccountID
			isAdmin := false
			for _, role := range secCtx.Roles {
				if role == "admin" {
					isAdmin = true
					break
				}
			}

			// Only include entries the user owns, or if they're admin/system
			if userOwnsEntry || isAdmin || (secCtx != nil && secCtx.AccountID == pkgctx.SystemAccountID) {
				filtered := f.applyKeystoreAccessControl(obj, secCtx)
				filteredObjects = append(filteredObjects, filtered)
			}
			// Otherwise, skip this entry (user doesn't own it and isn't admin)
		}
		objectList = filteredObjects
	}

	objectList = applyListFieldProjection(filter.Kind, filter, objectList)
	result.Objects = objectList
	projectQueryResultGroups(filter.Kind, filter, result)
	result.Meta[ConstStreamReturnedCount] = len(objectList)
	if cacheable && effectiveLimit > 0 {
		SetListCache(f.projectRoot, &filter, effectiveLimit, result)
	}
	return result, nil
}

// listStreamBackedOnly lists objects for a stream-backed kind using stream registry + write-behind only.
// No CAS index or CAS reads; avoids all CAS activity for scheduler_job, zqk_session, etc.
func (f *FileObjectStorage) listStreamBackedOnly(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, effectiveLimit int, cacheable bool, _ *logging.EventLogger) (*QueryResult, error) {
	// If sorting or offset is requested, we MUST load all matching objects to sort them globally.
	// We cannot apply the limit during the segment scan because we need the global top-N.
	scanLimit := effectiveLimit
	if filter.SortBy != "" || filter.Offset > 0 {
		scanLimit = 0
	}

	// Use memory-efficient parallel stream segment scan
	objectsList, total, err := f.listStreamSegmentsWithLimit(ctx, secCtx, filter, scanLimit)
	if err != nil {
		return nil, err
	}

	// Sort the objects if requested
	if filter.SortBy != "" {
		sortObjects(objectsList, filter.SortBy, filter.SortAsc)
	} else if filter.Offset > 0 || scanLimit == 0 {
		// Provide stable ordering for pagination if we loaded all
		sortObjects(objectsList, objects.FieldKeyID, true)
	}

	// Apply Offset and Limit globally
	start := filter.Offset
	if start > len(objectsList) {
		start = len(objectsList)
	}
	end := start + effectiveLimit
	if effectiveLimit == 0 {
		end = len(objectsList)
	} else if end > len(objectsList) {
		end = len(objectsList)
	}
	objectsList = objectsList[start:end]

	// Create result
	result := &QueryResult{
		Objects: objectsList,
		Groups:  make(map[string][]map[string]any),
		Meta:    map[string]any{"total_count": total, ConstStreamReturnedCount: len(objectsList)},
	}

	// Apply field projection
	result.Objects = applyListFieldProjection(filter.Kind, filter, result.Objects)

	if cacheable && effectiveLimit > 0 && len(result.Objects) > 0 {
		SetListCache(f.projectRoot, &filter, effectiveLimit, result)
	}
	return result, nil
}

// effectiveListLimit returns the limit to use for list pagination (filter + storageCtx).
// When filter.Limit is 0 (no limit requested), return 0 so cacheable lists return the full set.
// When filter.Limit > 0 (explicit limit), return it unchanged so the caller's request is respected;
// MaxPageSize is not applied to explicit limits.
func effectiveListLimit(filter *ListFilter, storageCtx *pkgctx.StorageContext) int {
	if filter == nil {
		return 0
	}
	if filter.Limit == 0 {
		return 0
	}
	return filter.Limit
}

// idsFromListFilter returns object IDs mentioned in list filters (id=..., id $eq, id $in).
// Used so CAS list includes those IDs in the candidate set even when not yet in the index (discovery path).
func idsFromListFilter(filters map[string]any) []string {
	if len(filters) == 0 {
		return nil
	}
	raw, ok := filters[objects.FieldKeyID]
	if !ok {
		return nil
	}
	// Simple equality: id=SCH-020 -> "SCH-020"
	if s, ok := raw.(string); ok && s != emptyValue {
		return []string{s}
	}
	// Operator map: id={"$eq": "SCH-020"} or id={"$in": ["SCH-020", "SCH-021"]}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	if eq, ok := m["$eq"]; ok {
		if s, ok := eq.(string); ok && s != emptyValue {
			return []string{s}
		}
	}
	if in, ok := m["$in"]; ok {
		sl, ok := in.([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(sl))
		for _, v := range sl {
			if s, ok := v.(string); ok && s != emptyValue {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// listFilterIsOnlyCreatedAtRange returns true when filters contain only a created_at range ($gte and/or $lte).
func (f *FileObjectStorage) listFilterIsOnlyCreatedAtRange(filters map[string]any) bool {
	if len(filters) == 0 || len(filters) > 1 {
		return false
	}
	ca, ok := filters[objects.FieldKeyCreatedAt].(map[string]any)
	if !ok || len(ca) == 0 {
		return false
	}
	for k := range ca {
		if k != "$gte" && k != "$lte" && k != "$gt" && k != "$lt" {
			return false
		}
	}
	return true
}

// parseCreatedAtOlderThan parses created_at $lt or $lte from filters. Returns cutoff time and true if found (no $gte).
// Used for "older than X" retention/cleanup queries so we can use cache.QueryOlderThan and avoid loading all IDs.
func parseCreatedAtOlderThan(filters map[string]any) (cutoff time.Time, ok bool) {
	ca, _ := filters[objects.FieldKeyCreatedAt].(map[string]any)
	if ca == nil {
		return time.Time{}, false
	}
	if _, hasGte := ca["$gte"]; hasGte {
		return time.Time{}, false
	}
	if s, v := ca["$lt"].(string); v && s != emptyValue {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
	}
	if s, v := ca["$lte"].(string); v && s != emptyValue {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseCreatedAtRangeFromFilters parses created_at $gte and $lte from filters. Returns startTime, endTime, true if both found.
func (f *FileObjectStorage) parseCreatedAtRangeFromFilters(filters map[string]any) (startTime, endTime time.Time, ok bool) {
	ca, _ := filters[objects.FieldKeyCreatedAt].(map[string]any)
	if ca == nil {
		return time.Time{}, time.Time{}, false
	}
	var hasStart, hasEnd bool
	if s := objects.GetString(ca, "$gte"); s != emptyValue {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			startTime = t
			hasStart = true
		}
	}
	if s := objects.GetString(ca, "$lte"); s != emptyValue {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			endTime = t
			hasEnd = true
		}
	}
	return startTime, endTime, hasStart && hasEnd
}

// listAuditEventsByIDs loads audit_event objects for the given IDs via CAS (bounded parallel read), applies filters, returns maps.
func (f *FileObjectStorage) listAuditEventsByIDs(ctx context.Context, cas *ContentAddressableStorage, kind string, ids []string, filters map[string]any) ([]map[string]any, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	listMaxConcurrentReads := getListReadWorkers()
	type casParseResult struct {
		parsed *objects.ParsedObject
	}
	results := make(chan casParseResult, listMaxConcurrentReads*2)
	workCh := make(chan string, len(ids))
	for _, id := range ids {
		workCh <- id
	}
	close(workCh)
	numWorkers := listMaxConcurrentReads
	if len(ids) < numWorkers {
		numWorkers = len(ids)
	}
	var listWg sync.WaitGroup
	listWg.Add(numWorkers)
	listBud := goroutinelabels.DefaultBudget()
	for w := 0; w < numWorkers; w++ {
		casBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListAuditByIds, ConstStreamListAuditByIdsWorker)
		if listBud != nil {
			casBuilder = casBuilder.WithBudget(listBud)
		}
		casBuilder.StartSimple(func() {
			defer listWg.Done()
			for id := range workCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				var result casParseResult
				var obj map[string]any
				data, err := cas.Read(id)
				if err == nil {
					var _err_83789500 = yaml.Unmarshal(data, &obj)
					if _err_83789500 != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83789500).Log()
					}
				} else if errors.Is(err, ErrObjectNotFound) || strings.Contains(err.Error(), "not found") {
					filePath, pathErr := f.getObjectFilePath(id, kind)
					if pathErr == nil {
						var _err_83789713 error
						obj, _err_83789713 = f.readObjectFileFast(filePath)
						if _err_83789713 != nil {
							logging.Fluent(

								// Use obj != nil only: after CAS miss, disk fallback may succeed while err still reflects cas.Read.
								logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83789713).Log()
						}
					}
				}

				if obj != nil {
					obj = materializeCasYAMLMapAfterLoad(f.projectRoot, kind, id, obj)
					parsed, _err_83789897 := objects.ParseObject(obj)
					if _err_83789897 != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83789897).Log()
					}
					if parsed != nil {
						result.parsed = parsed
					} else {
						result.parsed = &objects.ParsedObject{Raw: obj}
					}
				}
				results <- result
			}
		})
	}
	closerBud := goroutinelabels.DefaultBudget()
	closerBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListAuditByIdsCloser, ConstStreamWaitingForListAuditByIdsWorkers).
		WithCleanup(func() { close(results) })
	if closerBud != nil {
		closerBuilder = closerBuilder.WithBudget(closerBud)
	}
	closerBuilder.StartSimple(func() { listWg.Wait() })
	var parsedObjects []*objects.ParsedObject
	for r := range results {
		if r.parsed != nil {
			parsedObjects = append(parsedObjects, r.parsed)
		}
	}
	filtered := make([]map[string]any, 0, len(parsedObjects))
	for _, parsed := range parsedObjects {
		if f.matchesFiltersParsed(parsed, filters) {
			filtered = append(filtered, parsed.ToMap())
		}
	}
	return filtered, nil
}

// listObjectsFromPaths reads YAML files at the given paths in parallel, parses to objects, and returns
// only those matching filters. Used by the audit_event date-range walk path so we load only files in
// the time window instead of all CAS IDs (e.g. 80k in 120d instead of 194k total).
func (f *FileObjectStorage) listObjectsFromPaths(ctx context.Context, paths []string, kind string, filters map[string]any) ([]map[string]any, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	listMaxConcurrentReads := getListReadWorkers()
	type pathResult struct {
		obj map[string]any
	}
	results := make(chan pathResult, listMaxConcurrentReads*2)
	workCh := make(chan string, len(paths))
	for _, p := range paths {
		workCh <- p
	}
	close(workCh)
	numWorkers := listMaxConcurrentReads
	if len(paths) < numWorkers {
		numWorkers = len(paths)
	}
	var listWg sync.WaitGroup
	listWg.Add(numWorkers)
	listBud := goroutinelabels.DefaultBudget()
	for w := 0; w < numWorkers; w++ {
		casBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListByPaths, ConstStreamListObjectsFromPathsWorker)
		if listBud != nil {
			casBuilder = casBuilder.WithBudget(listBud)
		}
		casBuilder.StartSimple(func() {
			defer listWg.Done()
			for path := range workCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				obj, err := f.readObjectFileFast(path)
				if err != nil || obj == nil {
					results <- pathResult{}
					continue
				}
				idVal, _ := obj[objects.FieldKeyID].(string)
				obj = materializeCasYAMLMapAfterLoad(f.projectRoot, kind, idVal, obj)
				if len(filters) == 0 || f.matchesFilters(obj, filters) {
					results <- pathResult{obj: obj}
				} else {
					results <- pathResult{}
				}
			}
		})
	}
	closerBud := goroutinelabels.DefaultBudget()
	closerBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListByPathsCloser, ConstStreamWaitingForListByPathsWorkers).
		WithCleanup(func() { close(results) })
	if closerBud != nil {
		closerBuilder = closerBuilder.WithBudget(closerBud)
	}
	closerBuilder.StartSimple(func() { listWg.Wait() })
	var out []map[string]any
	for r := range results {
		if r.obj != nil {
			out = append(out, r.obj)
		}
	}
	return out, nil
}
