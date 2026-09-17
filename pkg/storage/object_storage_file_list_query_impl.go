package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
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
		// Convert account-ide-seat-01 -> ACC-1785920548450214001-7b3cc2de
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

	if cachedLivePath(id) != emptyValue {
		return true, nil
	}

	// For CAS-enabled kinds, check the CAS index first so "not found" returns (false, nil)
	// instead of an error (getObjectFilePath returns error for non-existing CAS objects).
	if f.usesContentAddressableStorage(kind) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		cas, err := f.getContentAddressableStorage(kind)
		if err == nil {
			// Erase tombstone linger (TRACK: BLI-CAS-SW-LINGER-001): Exists stays true
			// during erase shockwave until FINALIZE blob GC.
			if cas.IsErasePending(id) {
				return true, nil
			}
			_, hashErr := cas.GetHashForID(id)
			if hashErr != nil {
				if f.objectDraftPlaneExists(kind, id) {
					_ = coupleObjectIDCacheLivePath(id, kind, f.objectDraftPlanePath(kind, id))
					return true, nil
				}
				return false, nil
			}
			// Index can list an ID after the hash file was removed; existence requires on-disk file.
			_, pathErr := cas.GetFilePathForID(id)
			if pathErr == nil {
				return true, nil
			}
			if f.objectDraftPlaneExists(kind, id) {
				_ = coupleObjectIDCacheLivePath(id, kind, f.objectDraftPlanePath(kind, id))
				return true, nil
			}
			return false, nil
		}
		if f.objectDraftPlaneExists(kind, id) {
			return true, nil
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
	// 3. fileutil.Stat() is very fast (just metadata, no file content read)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	_, err = fileutil.Stat(filePath)
	if err == nil {
		return true, nil
	}
	if fileutil.IsNotExist(err) {
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

	// Optimization for default namespace filter (TDE-1788055418678191000-32664603):
	// Bare "zqk object count" injects namespace_id="zqk:kernel" for workspace isolation.
	// When that is the only filter, all standard process objects belong to this namespace.
	// Route to fast index count instead of forcing an O(N) YAML parse across thousands of files.
	if len(filter.Filters) == 1 {
		if ns, ok := filter.Filters[objects.FieldKeyNamespaceID].(string); ok && (ns == "zqk:kernel" || ns == "") {
			if f.usesContentAddressableStorage(filter.Kind) {
				return f.countFromCASIndex(ctx, filter.Kind, kindDir)
			}
			return f.countFiles(ctx, kindDir, usesBucketing)
		}
	}

	// With filters, we need to parse objects to apply filters
	// But we can still optimize by not loading full objects into memory
	// We'll use the same filtering logic as List() but only count matches
	return f.countWithFilters(ctx, kindDir, filter, usesBucketing)
}

// countFromCASIndex counts objects using the CAS index and legacy ID-named files (same as List).
// Optimized for performance: when index is empty, uses cached disk count and triggers background refresh
// instead of blocking on full directory scan. This keeps Count() responsive (e.g. "zqk object count" over 78 kinds).
