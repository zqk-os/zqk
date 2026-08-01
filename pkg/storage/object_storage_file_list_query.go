package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/when"
)

const (
	// highVolumeCacheStaleDelta: if index (or disk) count exceeds cache count by this much, treat cache as stale and invalidate.
	highVolumeCacheStaleDelta = 500
	// highVolumeCacheStaleRatio: if cache count is less than indexCount/this, treat cache as stale (e.g. indexCount/2).
	highVolumeCacheStaleRatio = 2
	// casIndexReconcileDelta: if disk count exceeds CAS index count by this much, repopulate index from disk.
	casIndexReconcileDelta = 100
	// highVolumeCASCountFastPathThreshold: when index has at least this many IDs for audit_event/*_metric, skip
	// reconcile check and legacy ID-based scan so Count() stays fast (avoids 500s+ catch_up phase in aggregation).
	highVolumeCASCountFastPathThreshold = 5000
)

func (f *FileObjectStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	if query.Type != QueryTypeFilter {
		return nil, errfmt.Errorf(ConstStreamFileBackendOnlySupportsFilterQueries)
	}

	// Convert query to ListFilter
	filter := ListFilter{
		Kind:    query.Kind,
		Filters: query.Parameters,
	}

	result, err := f.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}

	// List already returns a QueryResult with grouping applied if requested
	return result, nil
}

// Exists checks if an object exists by ID
// More efficient than Read() as it doesn't load the object
// Checks cache first for performance, then validates against file system
func (f *FileObjectStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// Normalize account IDs: convert filename format (account-username) to ID format (account:username)
	// This handles cases where account IDs are passed in filename format instead of ID format
	normalizedID := id
	if strings.HasPrefix(id, "account-") && !strings.HasPrefix(id, "account:") {
		// Convert account-cursor-vscode -> account:cursor-vscode
		username := strings.TrimPrefix(id, "account-")
		normalizedID = fmt.Sprintf("account:%s", username)
	}

	// Infer kind from ID
	if err := f.idValidator.LoadPatterns(); err != nil {
		return false, errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	kind := f.idValidator.InferKindFromID(normalizedID)
	if kind == emptyValue {
		return false, errfmt.Errorf(ConstStreamCouldNotInferKindFromIdStr, id)
	}

	// Use normalized ID for file path lookup
	id = normalizedID

	// Check permission
	if err := f.checkPermission(secCtx, "read", kind); err != nil {
		return false, err
	}

	// Stream-backed kinds: resolve path from stream registry only; no CAS.
	if StreamStorageEnabledForKind(kind) {
		return f.existsStreamBacked(ctx, id, kind)
	}

	// For CAS-enabled kinds, check the CAS index first so "not found" returns (false, nil)
	// instead of an error (getObjectFilePath returns error for non-existing CAS objects).
	if f.usesContentAddressableStorage(kind) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		cas, err := f.getContentAddressableStorage(kind)
		if err == nil {
			_, hashErr := cas.GetHashForID(id)
			if hashErr != nil {
				return false, nil
			}
			// Index can list an ID after the hash file was removed; existence requires on-disk file.
			_, pathErr := cas.GetFilePathForID(id)
			return pathErr == nil, nil
		}
		// CAS kind but could not get CAS instance (e.g. kind dir missing). Do not call
		// getObjectFilePath—it would error for missing IDs. Treat as not found.
		return false, nil
	}

	// Get file path (for non-CAS kinds or CAS fallback)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	filePath, err := f.getObjectFilePath(id, kind)
	if err != nil {
		return false, err
	}

	// For non-CAS kinds, check file existence
	// Note: We don't check cache here because:
	// 1. Cache is primarily for reference checking in the check command
	// 2. File system is the source of truth
	// 3. os.Stat() is very fast (just metadata, no file content read)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, err = os.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	// Other error (permission, etc.)
	return false, errfmt.Newf(ConstStreamFailedToCheckFileExistence).Wrap(err)
}

// Count counts objects matching the filter
// More efficient than List() as it doesn't load objects into memory
// Uses high-volume event cache for fast counts when available
//
// Count counts objects matching the filter
// More efficient than List() as it doesn't load objects into memory
// Uses high-volume event cache for fast counts when available
//
//nolint:gocritic // Interface requires value semantics for ListFilter
//nolint:gocritic // Interface requires value semantics for ListFilter
func (f *FileObjectStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// Check permission
	if err := f.checkPermission(secCtx, "read", filter.Kind); err != nil {
		return 0, err
	}

	// Try high-volume event cache for high-volume kinds (fast path; see high_volume_kinds.yaml)
	// Optimization: check cache FIRST before stream/CAS scans as it's the fastest path.
	if IsHighVolumeKindForCache(filter.Kind) {
		cache := GetGlobalHighVolumeEventCache()
		if cache != nil && cache.IsPopulatedForProject(f.projectRoot) {
			// For CAS kinds, verify cache is not stale vs index (index is source of truth after reconciliation)
			kindDir := filepath.Join(f.processDir, objects.GetDirectoryFromKind(filter.Kind))
			if kindDir != emptyValue && f.usesContentAddressableStorage(filter.Kind) {
				if cas, err := f.getContentAddressableStorage(filter.Kind); err == nil && cas != nil {
					if indexIDs, listErr := cas.ListIDs(); listErr == nil {
						indexCount := len(indexIDs)
						cacheCount := cache.CountByKind(filter.Kind)
						// If index differs from cache significantly (due to adds or deletes), cache is stale
						if indexCount > 0 && (cacheCount < indexCount/highVolumeCacheStaleRatio || indexCount > cacheCount+highVolumeCacheStaleDelta || cacheCount > indexCount+highVolumeCacheStaleDelta) {
							cache.InvalidateForProject(f.projectRoot)
							// Fall through to countFromCASIndex so we return accurate count and can trigger reconcile
						} else if indexCount == 0 {
							// Index empty but cache populated - disk may have files; don't trust cache
							diskCount, _err_83030943 := f.getCachedDiskCountOrScan(filter.Kind)
							if _err_83030943 != nil {
								logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83030943).Log()
							}
							if diskCount > cacheCount+highVolumeCacheStaleDelta || cacheCount > diskCount+highVolumeCacheStaleDelta {
								cache.InvalidateForProject(f.projectRoot)
							}
						}
					}
				}
			}

			// Re-check after possible invalidation
			if cache.IsPopulatedForProject(f.projectRoot) {
				// Pipelineize the "canUseCache + created_at filter parsing + decide cache count" portion
				// so the control-flow is criteria-scoped and stage-bound (instead of boolean-heavy nesting).
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

				type cacheCountInput struct {
					filter ListFilter
					cache  *HighVolumeEventCache
				}
				type cacheCountDecision struct {
					kind         string
					cache        *HighVolumeEventCache
					canUseCache  bool
					timeFilter   map[string]any
					haveTimeSpec bool
				}
				type cacheCountOutcome struct {
					used  bool
					count int
				}

				cacheCountDecisionWithCache := func(payload any) (*cacheCountDecision, bool) {
					in, ok := nildecode.DecodeNonNilPayload[*cacheCountDecision](payload)
					if !ok {
						return nil, false
					}
					if in.cache == nil {
						return nil, false
					}
					return in, true
				}

				pl := pipeline.NewBuilder(ConstStreamHighVolumeCacheCount, logger).
					WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
					WithProfile(string(pkgctx.ProfileSystem)).
					AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
						return payload, nil
					}).
					AddStage(pipeline.StageNormalize, func(pctx *pipeline.Context, payload any) (any, error) {
						in, ok := nildecode.DecodeNonNilPayload[*cacheCountInput](payload)
						if !ok {
							return nil, errfmt.Errorf(ConstStreamNormalizeExpectedCachecountinputGotType, payload)
						}

						canUseCache := true
						var timeFilter map[string]any
						if filters := in.filter.Filters; filters != nil {
							tf, ok := filters[objects.FieldKeyCreatedAt].(map[string]any)
							when.When(func() bool { return ok }).Then(func() {
								timeFilter = tf
							}).OrElse(func() {
								if len(filters) > 1 || (len(filters) == 1 && filters[objects.FieldKeyCreatedAt] == nil) {
									canUseCache = false
								}
							}).Run()
						}

						// Whether caller supplied any created_at map at all (timeFilter nil means either:
						// no filters, or created_at wasn't a map and we didn't explicitly disable cache).
						haveTimeSpec := timeFilter != nil
						return &cacheCountDecision{
							kind:         in.filter.Kind,
							cache:        in.cache,
							canUseCache:  canUseCache,
							timeFilter:   timeFilter,
							haveTimeSpec: haveTimeSpec,
						}, nil
					}).
					AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
						in, ok := cacheCountDecisionWithCache(payload)
						if !ok {
							return &cacheCountOutcome{used: false}, nil
						}
						if !in.canUseCache {
							return &cacheCountOutcome{used: false}, nil
						}

						// No filters on created_at -> use cache count by kind
						if in.timeFilter == nil {
							return &cacheCountOutcome{used: true, count: in.cache.CountByKind(in.kind)}, nil
						}

						// Parse time window from filters (RFC3339)
						var startTime, endTime time.Time
						if lt := objects.GetString(in.timeFilter, "$lt"); lt != "" {
							if t, err := time.Parse(time.RFC3339, lt); err == nil {
								endTime = t
							}
						}
						if lte := objects.GetString(in.timeFilter, "$lte"); lte != "" {
							if t, err := time.Parse(time.RFC3339, lte); err == nil {
								endTime = t
							}
						}
						if gte := objects.GetString(in.timeFilter, "$gte"); gte != "" {
							if t, err := time.Parse(time.RFC3339, gte); err == nil {
								startTime = t
							}
						}
						if gt := objects.GetString(in.timeFilter, "$gt"); gt != "" {
							if t, err := time.Parse(time.RFC3339, gt); err == nil {
								startTime = t.Add(time.Nanosecond) // Make exclusive
							}
						}

						if startTime.IsZero() && endTime.IsZero() {
							// created_at was a map, but it didn't yield a valid window; preserve fall-through.
							return &cacheCountOutcome{used: false}, nil
						}

						if startTime.IsZero() {
							startTime = time.Time{} // Beginning of time
						}
						if endTime.IsZero() {
							endTime = time.Now().UTC().Add(24 * time.Hour) // Future
						}

						return &cacheCountOutcome{used: true, count: in.cache.CountByTimeWindow(startTime, endTime)}, nil
					}).
					AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
						return payload, nil
					}).
					Build()

				out, runErr := pl.Run(&pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}, &cacheCountInput{filter: filter, cache: cache})
				if runErr == nil {
					if oc, ok := out.(*cacheCountOutcome); ok && oc != nil && oc.used {
						return oc.count, nil
					}
				}
			}
		}
	}

	// Stream-backed kinds: count from stream registry and segments.
	// Optimization: process these BEFORE the listing semaphore to avoid blocking high-volume cleanup
	// behind slow legacy YAML/CAS scans.
	if StreamStorageEnabledForKind(filter.Kind) {
		// Optimization: if no filters, use fast path (registry + segment lines)
		if len(filter.Filters) == 0 {
			casAndLegacy := 0
			if f.usesContentAddressableStorage(filter.Kind) {
				casIDs, _err_83036350 := f.getContentAddressableStorageIDs(filter.Kind)
				if _err_83036350 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83036350).Log()
				}
				casAndLegacy = len(casIDs)
			}
			streamCount := len(ListStreamIDsFromPersistentRegistry(f.projectRoot, filter.Kind))
			return casAndLegacy + streamCount, nil
		}
		// With filters: use stream-aware line-by-line count to avoid OOM for millions of objects
		return f.countStreamSegmentsWithFilters(ctx, secCtx, filter)
	}

	// Get directory for this kind
	dirName := objects.GetDirectoryFromKind(filter.Kind)
	if dirName == emptyValue {
		return 0, errfmt.Errorf(ConstStreamUnknownObjectKindStr, filter.Kind)
	}

	kindDir := filepath.Join(f.processDir, dirName)

	// Check if this kind uses bucketed storage
	usesBucketing := f.usesBucketedStorage(filter.Kind, kindDir)

	// If no filters, count efficiently. All kinds are CAS: use index + legacy ID-named scan (same as List) so Count matches List.
	if len(filter.Filters) == 0 {
		if f.usesContentAddressableStorage(filter.Kind) {
			return f.countFromCASIndex(ctx, filter.Kind, kindDir)
		}
		return f.countFiles(ctx, kindDir, usesBucketing)
	}

	// With filters, we need to parse objects to apply filters
	// But we can still optimize by not loading full objects into memory
	// We'll use the same filtering logic as List() but only count matches
	return f.countWithFilters(ctx, kindDir, filter, usesBucketing)
}

// countFromCASIndex counts objects using the CAS index and legacy ID-named files (same as List).
// Optimized for performance: when index is empty, uses cached disk count and triggers background refresh
// instead of blocking on full directory scan. This keeps Count() responsive (e.g. "zqk object count" over 78 kinds).
func (f *FileObjectStorage) countFromCASIndex(ctx context.Context, kind string, kindDir string) (int, error) {
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		return 0, errfmt.Newf(ErrMsgGetCAS).Wrap(err)
	}
	casIDs, err := cas.ListIDs()
	if err != nil {
		return 0, errfmt.Newf(ConstStreamFailedToListIdsFromContentAddressableStorage).Wrap(err)
	}

	// If index is empty, use cached disk count for fast response and trigger background refresh
	if len(casIDs) == 0 {
		// Use cached disk count for immediate response (fast path)
		cachedCount, cacheErr := f.getCachedDiskCountOrScan(kind)
		if cacheErr == nil && cachedCount > 0 {
			// Trigger background refresh so next count/list sees updated index
			f.triggerBackgroundCASIndexRefresh(kind)
			// Also scan ID-based files for legacy objects (skip for stream-backed kinds; no YAML read)
			idSet := make(map[string]bool)
			if !StreamStorageEnabledForKind(kind) {
				when.When(func() bool { return f.usesBucketedStorage(kind, kindDir) }).Then(func() {
					for id := range f.scanIDBasedFilesRecursive(kindDir, kind) {
						idSet[id] = true
					}
				}).OrElse(func() {
					for _, id := range f.scanIDBasedFiles(kindDir, kind) {
						idSet[id] = true
					}
				}).Run()
			}
			n := cachedCount + len(idSet)
			if StreamStorageEnabledForKind(kind) {
				n += len(ListStreamIDsFromPersistentRegistry(f.projectRoot, kind))
			}
			return n, nil
		}
		var _err_83038590 = f.EnsureCASIndexPopulatedFromScan(ctx, kind)
		if _err_83038590 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83038590).Log()
		}
		casIDs, err = cas.ListIDs()
		if err != nil {
			return 0, errfmt.Newf(ConstStreamFailedToListIdsFromContentAddressableStorage).Wrap(err)
		}
	}

	// Fast path: high-volume CAS-only kinds (audit_event, *_metric) with large index. Skip reconcile and legacy
	// scan so Count() returns in milliseconds instead of 500s+ (catch_up phase was spending 8+ min on Count alone).
	if (kind == objects.KindAuditEvent || (len(kind) > 7 && kind[len(kind)-7:] == "_metric")) && len(casIDs) >= highVolumeCASCountFastPathThreshold {
		n := len(casIDs)
		if StreamStorageEnabledForKind(kind) {
			n += len(ListStreamIDsFromPersistentRegistry(f.projectRoot, kind))
		}
		return n, nil
	}

	// Reconcile: if disk has many more hash-named files than index, index is stale (e.g. files added elsewhere or index lost)
	// Repopulate index from disk so we don't abandon data and Count/List stay accurate. Respect ctx so job timeout aborts long scans.
	if diskCount, diskErr := f.getCachedDiskCountOrScan(kind); diskErr == nil && diskCount > len(casIDs)+casIndexReconcileDelta {
		var _err_83041010 = f.EnsureCASIndexPopulatedFromScan(ctx, kind)
		if _err_83041010 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83041010).Log()
		}
		casIDs, err = cas.ListIDs()
		if err != nil {
			return 0, errfmt.Newf(ConstStreamFailedToListIdsAfterReconciliation).Wrap(err)
		}
	}

	// Index has entries: use index as source of truth, only scan ID-based files for legacy objects
	idSet := make(map[string]bool)
	for _, id := range casIDs {
		idSet[id] = true
	}
	// Scan ID-based files only if index was empty (legacy migration case)
	// When index is populated, CAS is the source of truth, but we still check for legacy ID-based files
	// Stream-backed kinds: do not read YAML dir; source of truth is stream registry.
	// Optimization: Skip legacy scan if directory doesn't exist (no legacy files possible)
	if !StreamStorageEnabledForKind(kind) {
		if _, err := os.Stat(kindDir); err == nil {
			when.When(func() bool { return f.usesBucketedStorage(kind, kindDir) }).Then(func() {
				for id := range f.scanIDBasedFilesRecursive(kindDir, kind) {
					idSet[id] = true
				}
			}).OrElse(func() {
				for _, id := range f.scanIDBasedFiles(kindDir, kind) {
					idSet[id] = true
				}
			}).Run()
		}
	}
	casAndLegacy := len(idSet)
	// When stream storage is enabled for this kind, add count from segment files so Count() is accurate
	// when high-volume cache is empty (DATA_STORAGE_PRODUCTION_ROADMAP §1; no unbounded full scan).
	// Subtract stream-deleted count so logically deleted IDs are not counted.
	if StreamStorageEnabledForKind(kind) {
		streamCount := len(ListStreamIDsFromPersistentRegistry(f.projectRoot, kind))
		return casAndLegacy + streamCount, nil
	}
	return casAndLegacy, nil
}

// countFiles counts files in a directory without parsing
// Uses the same unified traversal logic as collectFilePaths
func (f *FileObjectStorage) countFiles(ctx context.Context, kindDir string, _ bool) (int, error) {
	EmitListCountWaitProgress(ctx)
	listCtx, err := AcquireListCountSlot(ctx)
	if err != nil {
		return 0, err
	}
	defer ReleaseListCountSlot(ctx)

	// Use unified file collection logic
	filePaths, err := f.collectFilePaths(listCtx, kindDir, nil, nil)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return len(filePaths), nil
}

// countWithFilters counts objects matching filters
// This is similar to List() but only counts, doesn't load objects
//
//nolint:gocritic // Internal helper; filter passed by value for consistency with interface
func (f *FileObjectStorage) countWithFilters(ctx context.Context, kindDir string, filter ListFilter, _ bool) (int, error) {
	EmitListCountWaitProgress(ctx)
	listCtx, err := AcquireListCountSlot(ctx)
	if err != nil {
		return 0, err
	}
	defer ReleaseListCountSlot(ctx)

	// Collect file paths using unified traversal logic
	timeRange := f.extractTimeRangeFromFilters(filter.Filters)
	filePaths, err := f.collectFilePaths(listCtx, kindDir, timeRange, nil)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, errfmt.Newf(ConstStreamFailedToCollectFilePaths).Wrap(err)
	}

	// Bounded worker pool: fixed number of goroutines (not one per file) to avoid thread exhaustion.
	count := 0
	maxWorkers := getListReadWorkers()
	results := make(chan bool, maxWorkers*2)
	workCh := make(chan string, maxWorkers*2)
	numWorkers := maxWorkers
	if len(filePaths) < numWorkers {
		numWorkers = len(filePaths)
	}
	if numWorkers == 0 {
		return 0, nil
	}

	for _, p := range filePaths {
		select {
		case <-listCtx.Done():
			return 0, listCtx.Err()
		case workCh <- p:
		}
	}
	close(workCh)

	var countWg sync.WaitGroup
	countWg.Add(numWorkers)
	countBud := goroutinelabels.DefaultBudget()
	for w := 0; w < numWorkers; w++ {
		countWorkerBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageCountFile, ConstStreamCountFilterWorker)
		if countBud != nil {
			countWorkerBuilder = countWorkerBuilder.WithBudget(countBud)
		}
		countWorkerBuilder.StartSimple(func() {
			defer countWg.Done()
			for filePath := range workCh {
				select {
				case <-listCtx.Done():
					return
				default:
				}
				obj, err := f.readObjectFile(listCtx, filePath)
				if err != nil {
					select {
					case <-listCtx.Done():
						return
					case results <- false:
					}
					continue
				}
				idVal, _ := obj[objects.FieldKeyID].(string)
				obj = materializeCasYAMLMapAfterLoad(f.projectRoot, filter.Kind, idVal, obj)
				parsed, err := objects.ParseObject(obj)
				if err != nil {
					parsed = &objects.ParsedObject{Raw: obj}
				}
				matches := f.matchesFiltersParsed(parsed, filter.Filters)
				// Don't send after context cancelled (closer may have closed channel)
				select {
				case <-listCtx.Done():
					return
				case results <- matches:
				}
			}
		})
	}

	countCloserBud := goroutinelabels.DefaultBudget()
	var closeOnce sync.Once
	closeResults := func() { closeOnce.Do(func() { close(results) }) }
	wgDone := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstStreamFileStorageCountWaiter, ConstStreamWaitingForCountWorkers).StartSimple(func() {
		countWg.Wait()
		close(wgDone)
	})
	countCloserBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageCountResultsCloser, ConstStreamWaitingForCountWorkersAndClosingResultsChannel).
		WithCleanup(closeResults)
	if countCloserBud != nil {
		countCloserBuilder = countCloserBuilder.WithBudget(countCloserBud)
	}
	countCloserBuilder.StartSimple(func() {
		select {
		case <-listCtx.Done():
			closeResults()
		case <-wgDone:
			closeResults()
		}
	})

	for match := range results {
		if match {
			count++
		}
	}
	if err := listCtx.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// extractTimeRangeFromFilters extracts time range from created_at filters
