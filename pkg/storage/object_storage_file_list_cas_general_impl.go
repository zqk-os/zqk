package storage

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/crud"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	"github.com/lanceman/zqk/pkg/when"
	"gopkg.in/yaml.v3"
)

func (f *FileObjectStorage) listCASPathGeneral(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, effectiveLimit int, cacheable bool, cas *filecas.ContentAddressableStorage, eventLogger *logging.EventLogger) (*QueryResult, error) {
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

	explicitIDs := crud.IdsFromListFilter(filter.Filters)
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
				idToPath = f.ScanIDBasedFilesRecursive(kindDir, filter.Kind)
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
		// Draft-plane IDs are intentionally omitted from normal List: only objects that
		// have left preliminary (materialized into CAS) appear here. Draft enumeration
		// is a separate plane/cache if needed — TRACK: [REDACTED-ID].
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

	listBud := goroutinelabels.DefaultBudget()
	for w := 0; w < numWorkers; w++ {
		casBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListCasRead, ConstStreamListCasWorker).WithWaitGroup(&listWg)
		// The first reader is an essential fallback. Optional parallel readers use the
		// global budget, but exhausting it must not leave List without a worker.
		if w > 0 && listBud != nil {
			casBuilder = casBuilder.WithBudget(listBud)
		}
		casBuilder.StartSimple(func() {
			for id := range workCh {
				select {
				case <-listCtx.Done():
					return
				default:
				}
				var result casParseResult
				var obj map[string]any
				var parsed *objects.ParsedObject
				var hash string

				// Get hash for cache lookup
				hash, _ = cas.GetHashForID(id)
				if hash != "" {
					if cached, ok := GetGlobalParseCache().Get(hash); ok {
						result.parsed = cached.Clone()
						result.parsed.Raw = f.MaterializeCasYAMLMapAfterLoad(f.projectRoot, filter.Kind, id, result.parsed.Raw)
						if objID, ok := result.parsed.Raw["id"].(string); ok {
							result.parsed.ID = objID
						}
						// Don't send on results after context cancelled
						select {
						case <-listCtx.Done():
							return
						case results <- result:
						}
						continue
					}
				}

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
					// Skip draft-plane paths in List (CAS index is the list source;
					// cache entries for drafts are not listable). Get follows cache.
					if filePath != emptyValue && !IsObjectDraftPlanePath(f.projectRoot, filePath) {
						var _err_83764528 error
						obj, _err_83764528 = f.readObjectFileFast(filePath)
						if _err_83764528 != nil {
							logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83764528).Log()
						}
					}
				}
				if obj != nil {
					obj = f.MaterializeCasYAMLMapAfterLoad(f.projectRoot, filter.Kind, id, obj)
					var _err_83764611 error
					parsed, _err_83764611 = objects.ParseObject(obj)
					if _err_83764611 != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83764611).Log()
					}
					when.When(func() bool { return parsed != nil }).Then(func() {
						result.parsed = parsed
						if hash != "" {
							GetGlobalParseCache().Put(hash, parsed)
						}
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
	goroutinelabels.NewGoroutine(
		ConstStreamFileStorageListCasResultsCloser,
		ConstStreamWaitingForListCasWorkersAndClosingResultsChannel,
	).WithCleanup(func() { close(results) }).StartSimple(func() {
		listWg.Wait()
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
		crud.SortParsedObjectsSlice(filteredObjects, filter.SortBy, filter.SortAsc)
	}).OrElse(func() {
		crud.SortParsedObjectsSlice(filteredObjects, objects.FieldKeyID, true)
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
		grouped := crud.GroupObjects(objectList, filter.GroupBy, storageCtx.MaxGroupSize)
		result.Groups = grouped
		result.Meta["total_groups"] = len(grouped)
		objectList = crud.FlattenGroups(grouped)
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
	skipCacheEmpty := filter.Kind == objects.KindAuditEvent && crud.ListFilterIsOnlyCreatedAtRange(filter.Filters) && len(objectList) == 0
	if cacheable && effectiveLimit > 0 && !skipCacheEmpty {
		SetListCache(f.projectRoot, &filter, effectiveLimit, result)
	}
	return result, nil

}
