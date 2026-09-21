package storage

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/storage/crud"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

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
	f.omitDraftPlaneOnlyFromList(st.result)
	return st.result, st.err
}

// ListImpl lists objects with filtering, sorting, pagination, and grouping.
//
//nolint:gocyclo // Complex function handling multiple code paths (CAS, file-based, bucketing, etc.)
func (f *FileObjectStorage) listStreamBackedOnly(ctx context.Context, secCtx *pkgctx.SecurityContext, _ *pkgctx.StorageContext, filter ListFilter, effectiveLimit int, cacheable bool, _ *logging.EventLogger) (*QueryResult, error) {
	// Fail-closed: never scanLimit=0 (open every PID shard) for SortBy/Offset.
	// HV streams are newest-segment-first and bounded; global sort of the whole
	// stream pegs MCP/scheduler daemons. TRACK
	scanLimit := effectiveLimit
	if scanLimit <= 0 {
		scanLimit = DefaultMaxStreamListLimit
	}
	if filter.Offset > 0 {
		scanLimit += filter.Offset
	}

	// Use memory-efficient parallel stream segment scan
	objectsList, total, err := f.listStreamSegmentsWithLimit(ctx, secCtx, filter, scanLimit)
	if err != nil {
		return nil, err
	}

	// Sort the objects if requested
	if filter.SortBy != "" {
		crud.SortObjects(objectsList, filter.SortBy, filter.SortAsc)
	} else if filter.Offset > 0 || scanLimit == 0 {
		// Provide stable ordering for pagination if we loaded all
		crud.SortObjects(objectsList, objects.FieldKeyID, true)
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

// listAuditEventsByIDs loads audit_event objects for the given IDs via CAS (bounded parallel read), applies filters, returns maps.
func (f *FileObjectStorage) listAuditEventsByIDs(ctx context.Context, cas *filecas.ContentAddressableStorage, kind string, ids []string, filters map[string]any) ([]map[string]any, error) {
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

	listBud := goroutinelabels.DefaultBudget()
	for w := 0; w < numWorkers; w++ {
		casBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListAuditByIds, ConstStreamListAuditByIdsWorker).WithWaitGroup(&listWg)
		if w > 0 && listBud != nil {
			casBuilder = casBuilder.WithBudget(listBud)
		}
		casBuilder.StartSimple(func() {

			for id := range workCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				var result casParseResult
				var obj map[string]any
				var parsed *objects.ParsedObject
				var hash string

				hash, _ = cas.GetHashForID(id)
				if hash != "" {
					if cached, ok := GetGlobalParseCache().Get(hash); ok {
						result.parsed = cached.Clone()
						result.parsed.Raw = f.MaterializeCasYAMLMapAfterLoad(f.projectRoot, kind, id, result.parsed.Raw)
						if objID, ok := result.parsed.Raw[objects.FieldKeyID].(string); ok {
							result.parsed.ID = objID
						}
						results <- result
						continue
					}
				}

				data, err := cas.Read(id)
				if err == nil {
					var _err_83789500 = yaml.Unmarshal(data, &obj)
					if _err_83789500 != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83789500).Log()
					}
				} else if errors.Is(err, ErrObjectNotFound) || strings.Contains(err.Error(), "not found") {
					filePath, pathErr := f.getObjectFilePath(id, kind)
					if pathErr == nil && filePath != emptyValue && !IsObjectDraftPlanePath(f.projectRoot, filePath) {
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
					obj = f.MaterializeCasYAMLMapAfterLoad(f.projectRoot, kind, id, obj)
					var _err_83789897 error
					parsed, _err_83789897 = objects.ParseObject(obj)
					if _err_83789897 != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83789897).Log()
					}
					if parsed != nil {
						result.parsed = parsed
						if hash != "" {
							GetGlobalParseCache().Put(hash, parsed)
						}
					} else {
						result.parsed = &objects.ParsedObject{Raw: obj}
					}
				}
				results <- result
			}
		})
	}
	closerBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListAuditByIdsCloser, ConstStreamWaitingForListAuditByIdsWorkers).
		WithCleanup(func() { close(results) })
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

	listBud := goroutinelabels.DefaultBudget()
	for w := 0; w < numWorkers; w++ {
		casBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListByPaths, ConstStreamListObjectsFromPathsWorker).WithWaitGroup(&listWg)
		if w > 0 && listBud != nil {
			casBuilder = casBuilder.WithBudget(listBud)
		}
		casBuilder.StartSimple(func() {

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
				obj = f.MaterializeCasYAMLMapAfterLoad(f.projectRoot, kind, idVal, obj)
				if len(filters) == 0 || f.matchesFilters(obj, filters) {
					results <- pathResult{obj: obj}
				} else {
					results <- pathResult{}
				}
			}
		})
	}
	closerBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListByPathsCloser, ConstStreamWaitingForListByPathsWorkers).
		WithCleanup(func() { close(results) })
	closerBuilder.StartSimple(func() { listWg.Wait() })
	var out []map[string]any
	for r := range results {
		if r.obj != nil {
			out = append(out, r.obj)
		}
	}
	return out, nil
}
