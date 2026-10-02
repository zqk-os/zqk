package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/storage/crud"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var streamMapPool = sync.Pool{
	New: func() any {
		m := make(map[string]any)
		return &m
	},
}

// DefaultMaxStreamListLimit is the upper bound on segment objects returned when limit <= 0 to prevent unbounded memory exhaustion.
const DefaultMaxStreamListLimit = 10000

func initWorkerPool(itemCount, maxWorkers int) (int, *sync.WaitGroup) {
	actualWorkers := min(itemCount, maxWorkers)
	var wg sync.WaitGroup
	wg.Add(actualWorkers)
	return actualWorkers, &wg
}

// listStreamSegmentsWithLimit lists matching objects in stream segments using a parallel worker pool, up to a limit.
func (f *FileObjectStorage) listStreamSegmentsWithLimit(ctx context.Context, _ *pkgctx.SecurityContext, filter ListFilter, limit int) ([]map[string]any, int, error) {
	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = DefaultMaxStreamListLimit
	}
	segDir, err := GetStreamSegmentDir(f.projectRoot, filter.Kind)
	if err != nil {
		return nil, 0, err
	}
	entries, err := fileutil.ReadDir(segDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	segments := f.filterStreamSegmentFiles(entries, segDir, filter.Filters)

	if len(segments) == 0 {
		return nil, 0, nil
	}

	// Newest date/PID shards first so a bounded Limit can stop without opening history.
	// TRACK: follow-up in kernel backlog
	sort.Slice(segments, func(i, j int) bool {
		return filepath.Base(segments[i]) > filepath.Base(segments[j])
	})

	listCtx, releaseSlot, workCh, maxWorkers, slotErr := prepareStreamSegmentWorkerPool(ctx, segments)
	if slotErr != nil {
		return nil, 0, slotErr
	}
	defer releaseSlot()
	ctx = listCtx

	type workerResult struct {
		objects []map[string]any
		total   int
	}
	results := make(chan workerResult, maxWorkers)
	actualWorkers, wg := initWorkerPool(len(segments), maxWorkers)
	// Pre-build fast byte-slice filters for all string values to aggressively drop non-matching lines before unmarshaling
	var requiredSubstrings [][]byte
	for _, val := range filter.Filters {
		if strVal, ok := val.(string); ok && strVal != "" {
			// In JSON, string values will be enclosed in quotes
			requiredSubstrings = append(requiredSubstrings, []byte("\""+strVal+"\""))
		}
	}

	// Use goroutine budget if available
	bud := goroutinelabels.DefaultBudget()

	deletedMap, liveSet := f.loadStreamDeletedAndLiveSets(filter.Kind)
	var collected atomic.Int32

	for i := 0; i < actualWorkers; i++ {

		workerName := ConstStreamStreamListWorker
		builder := goroutinelabels.NewGoroutine(workerName, ConstStreamScanningSegmentForList).
			WithContext(ctx).
			WithWaitGroup(wg)
		if bud != nil {
			builder = builder.WithBudget(bud)
		}
		builder.StartSimple(func() {
			defer wg.Done()
			var workerObjects []map[string]any
			workerTotal := 0
			for path := range workCh {
				if ctx.Err() != nil {
					return
				}
				if collected.Load() >= int32(effectiveLimit) { //nolint:gosec
					break
				}

				file, err := fileutil.Open(path)
				if err != nil {
					continue
				}

				scanner := bufio.NewScanner(file)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 10*1024*1024)

				for scanner.Scan() {
					if ctx.Err() != nil {
						_ = file.Close()
						return
					}
					if collected.Load() >= int32(effectiveLimit) { //nolint:gosec
						break
					}

					line := scanner.Bytes()
					if len(line) == 0 {
						continue
					}

					if len(requiredSubstrings) > 0 {
						matchedAll := true
						for _, sub := range requiredSubstrings {
							if !bytes.Contains(line, sub) {
								matchedAll = false
								break
							}
						}
						if !matchedAll {
							continue
						}
					}

					// Use sync.Pool map-based unmarshal to eliminate massive allocation churn
					mp := streamMapPool.Get().(*map[string]any)
					full := *mp
					clear(full)
					if err := json.Unmarshal(line, &full); err != nil {
						streamMapPool.Put(mp)
						continue
					}

					// Restore compressed field keys
					registry := GetFieldRegistry(f.projectRoot)
					full = registry.RestoreFieldKeys(full)
					decodeStreamRecordTimestamps(full)

					id, _ := full[objects.FieldKeyID].(string)
					if id == emptyValue || deletedMap[id] || !liveSet[id] {
						streamMapPool.Put(mp)
						continue
					}

					// Convert to ParsedObject for matchesFiltersParsed
					parsed, _ := objects.ParseObject(full)

					matched := f.matchesFiltersParsed(parsed, filter.Filters)
					if matched {
						workerTotal++
						if len(workerObjects) < effectiveLimit {
							full[objects.FieldKeyKind] = filter.Kind
							workerObjects = append(workerObjects, full)
							collected.Add(1)
							// Do NOT put mp back in pool, we've appended it
						} else {
							streamMapPool.Put(mp)
						}
					} else {
						streamMapPool.Put(mp)
					}
				}
				if err := scanner.Err(); err != nil { //nolint:gosec
					_ = file.Close()
					continue
				}
				_ = file.Close()
			}
			results <- workerResult{objects: workerObjects, total: workerTotal}
		})
	}

	// Deterministic wait for results (POL-CODE-004)
	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstStreamStreamListWaiter, ConstStreamWaitingForStreamListWorkers).
		WithContext(ctx).
		WithCleanup(func() {
			close(results)
			close(waitDone)
		}).
		StartSimple(func() {
			wg.Wait()
		})

	var allObjects []map[string]any
	totalCount := 0

	// Handle timeout/cancellation during wait
	waitTimeout := 10 * time.Minute // Bounded wait
	select {
	case <-waitDone:
		for res := range results {
			totalCount += res.total
			allObjects = append(allObjects, res.objects...)
		}
		// Do not truncate here: callers with SortBy need the bounded newest-first
		// page so they can sort then apply Limit. Worker+atomic caps already bound size.
	case <-time.After(waitTimeout):
		return nil, 0, errfmt.Errorf(ConstStreamStreamListWaitTimeoutExceededVal, waitTimeout)
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}

	// Early-stop bounds the page; unfiltered total is the live registry so
	// pagination still sees the universe (TestObjectStorage_ListBoundedReads).
	// TRACK: follow-up in kernel backlog
	if len(filter.Filters) == 0 {
		universe := 0
		for id := range liveSet {
			if id != emptyValue && !deletedMap[id] {
				universe++
			}
		}
		if universe > totalCount {
			totalCount = universe
		}
	}

	return allObjects, totalCount, nil
}

// countStreamSegmentsWithFilters counts matching objects in stream segments using a parallel worker pool and raw byte pre-filtering.
func (f *FileObjectStorage) countStreamSegmentsWithFilters(ctx context.Context, _ *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	segDir, err := GetStreamSegmentDir(f.projectRoot, filter.Kind)
	if err != nil {
		return 0, err
	}
	entries, err := fileutil.ReadDir(segDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	segments := f.filterStreamSegmentFiles(entries, segDir, filter.Filters)

	if len(segments) == 0 {
		return 0, nil
	}

	listCtx, releaseSlot, workCh, maxWorkers, slotErr := prepareStreamSegmentWorkerPool(ctx, segments)
	if slotErr != nil {
		return 0, slotErr
	}
	defer releaseSlot()
	ctx = listCtx

	results := make(chan int, maxWorkers)
	actualWorkers, wg := initWorkerPool(len(segments), maxWorkers)
	// Pre-build regex pre-filter
	var preFilterRegex *regexp.Regexp
	status, hasStatus := filter.Filters[objects.FieldKeyStatus].(string)
	if hasStatus && status != "" {
		// Support both uncompressed "status" and compressed 0x08 key
		// Status key is 0x08. In JSON it is escaped as \u0000\b or literal \x00\x08.
		// Regex pattern matches either key followed by the status value.
		pattern := "(\"" + objects.FieldKeyStatus + "\"|\"\\x00\\x08\"):\"" + regexp.QuoteMeta(status) + "\""
		preFilterRegex = regexp.MustCompile(pattern)
	}

	// Use goroutine budget if available
	bud := goroutinelabels.DefaultBudget()

	deletedMap, liveSet := f.loadStreamDeletedAndLiveSets(filter.Kind)

	for i := 0; i < actualWorkers; i++ {

		workerName := ConstStreamStreamCountWorker
		builder := goroutinelabels.NewGoroutine(workerName, ConstStreamScanningSegmentForCount).
			WithContext(ctx).
			WithWaitGroup(wg)
		if bud != nil {
			builder = builder.WithBudget(bud)
		}
		builder.StartSimple(func() {
			defer wg.Done()
			workerTotal := 0
			for path := range workCh {
				if ctx.Err() != nil {
					return
				}

				file, err := fileutil.Open(path)
				if err != nil {
					continue
				}

				scanner := bufio.NewScanner(file)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 10*1024*1024)

				for scanner.Scan() {
					if ctx.Err() != nil {
						_ = file.Close()
						return
					}

					line := scanner.Bytes()
					if len(line) == 0 {
						continue
					}

					// Optimization: Regex pre-filter on raw line before expensive JSON unmarshal
					if preFilterRegex != nil && !preFilterRegex.Match(line) {
						continue
					}

					// Use sync.Pool map-based unmarshal to eliminate massive allocation churn
					mp := streamMapPool.Get().(*map[string]any)
					full := *mp
					clear(full)
					if err := json.Unmarshal(line, &full); err != nil {
						streamMapPool.Put(mp)
						continue
					}

					// Restore compressed field keys
					registry := GetFieldRegistry(f.projectRoot)
					full = registry.RestoreFieldKeys(full)
					decodeStreamRecordTimestamps(full)

					id, _ := full[objects.FieldKeyID].(string)
					if id == emptyValue || deletedMap[id] || !liveSet[id] {
						streamMapPool.Put(mp)
						continue
					}

					// Convert to ParsedObject for matchesFiltersParsed
					parsed, _ := objects.ParseObject(full)

					matched := f.matchesFiltersParsed(parsed, filter.Filters)
					if matched {
						workerTotal++
					}
					// Always return to pool for counting since we don't retain the object
					streamMapPool.Put(mp)
				}
				_ = file.Close()
			}
			results <- workerTotal
		})
	}

	// Deterministic wait for results (POL-CODE-004)
	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstStreamStreamCountWaiter, ConstStreamWaitingForStreamCountWorkers).
		WithContext(ctx).
		WithCleanup(func() {
			close(results)
			close(waitDone)
		}).
		StartSimple(func() {
			wg.Wait()
		})

	totalCount := 0
	waitTimeout := 10 * time.Minute
	select {
	case <-waitDone:
		for count := range results {
			totalCount += count
		}
	case <-time.After(waitTimeout):
		return totalCount, errfmt.Errorf(ConstStreamStreamCountWaitTimeoutExceededVal, waitTimeout)
	case <-ctx.Done():
		return totalCount, ctx.Err()
	}

	return totalCount, nil
}

func (f *FileObjectStorage) filterStreamSegmentFiles(entries []fileutil.DirEntry, segDir string, filters map[string]any) []string {
	timeRange := f.extractTimeRangeFromFilters(filters)

	var segments []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		if timeRange != nil && (!timeRange.start.IsZero() || !timeRange.end.IsZero()) {
			parts := strings.Split(entry.Name(), "_")
			if len(parts) > 0 {
				if t, err := time.Parse("2006-01-02", parts[0]); err == nil {
					if !timeRange.end.IsZero() && t.After(timeRange.end) {
						continue
					}
					if !timeRange.start.IsZero() && t.Add(24*time.Hour).Before(timeRange.start) {
						continue
					}
				}
			}
		}
		segments = append(segments, filepath.Join(segDir, entry.Name()))
	}
	return segments
}

func prepareStreamSegmentWorkerPool(ctx context.Context, segments []string) (context.Context, func(), <-chan string, int, error) {
	listCtx, releaseSlot, slotErr := AcquireListCountContext(ctx)
	if slotErr != nil {
		return nil, nil, nil, 0, slotErr
	}
	workCh, maxWorkers := createSegmentWorkChannel(segments, getListReadWorkers())
	return listCtx, releaseSlot, workCh, maxWorkers, nil
}

func (f *FileObjectStorage) loadStreamDeletedAndLiveSets(kind string) (map[string]bool, map[string]bool) {
	deletedMap := crud.LoadStreamDeletedSetFast(f.projectRoot, kind)
	liveSet := LiveStreamIDSet(f.projectRoot, kind)
	return deletedMap, liveSet
}
