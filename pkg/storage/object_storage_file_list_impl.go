package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/lanceman/zqk/pkg/storage/crud"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/when"
)

func (f *FileObjectStorage) ListImpl(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	// Check permission
	if err := f.checkPermission(secCtx, "read", filter.Kind); err != nil {
		return nil, err
	}

	// Effective limit for pagination (and list cache key)
	cacheable := filter.Kind != objects.KindKeystoreEntry
	effectiveLimit := crud.EffectiveListLimit(&filter, storageCtx)
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
	// Check if this kind uses content-addressable storage
	if f.usesContentAddressableStorage(filter.Kind) {
		return f.listCASPath(ctx, secCtx, storageCtx, filter, effectiveLimit, cacheable, eventLogger)
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
	if _, err := fileutil.Stat(kindDir); err != nil {
		if fileutil.IsNotExist(err) {
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
	// Buffer workCh to len(filePaths) — enqueue-before-workers with a small buffer deadlocks
	// when file count exceeds the buffer (same failure mode as countWithFilters).
	// TRACK: BLI-1785903709847957000-8c7a991c
	listMaxWorkers := getListReadWorkers()
	results := make(chan parseResult, listMaxWorkers*2)
	workCh := make(chan string, len(filePaths))
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

		fileListBud := goroutinelabels.DefaultBudget()
		for w := 0; w < numWorkers; w++ {
			parseBuilder := goroutinelabels.NewGoroutine(
				ConstStreamFileStorageParseFile,
				ConstStreamListFileWorker,
			).WithWaitGroup(&listWg)
			// Keep one essential reader outside the optional parallelism budget.
			if w > 0 && fileListBud != nil {
				parseBuilder = parseBuilder.WithBudget(fileListBud)
			}
			parseBuilder.StartSimple(func() {
				for filePathCopy := range workCh {
					select {
					case <-listCtx.Done():
						results <- parseResult{nil, listCtx.Err()}
						continue
					default:
					}
					if IsObjectDraftPlanePath(f.projectRoot, filePathCopy) {
						results <- parseResult{nil, nil}
						continue
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

		fileCloserBuilder := goroutinelabels.NewGoroutine(ConstStreamFileStorageListResultsCloser, ConstStreamWaitingForListWorkersAndClosingResultsChannel).
			WithCleanup(func() {
				close(results)
			})
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
		crud.SortParsedObjectsSlice(parsedObjects, filter.SortBy, filter.SortAsc)
	}).OrElse(func() {
		crud.SortParsedObjectsSlice(parsedObjects, objects.FieldKeyID, true)
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
		grouped := crud.GroupObjects(objectList, filter.GroupBy, storageCtx.MaxGroupSize)
		result.Groups = grouped
		result.Meta["total_groups"] = len(grouped)
		// For List, we return flat list but preserve grouping in metadata if needed
		// Convert grouped map to flat list for backward compatibility
		objectList = crud.FlattenGroups(grouped)
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
