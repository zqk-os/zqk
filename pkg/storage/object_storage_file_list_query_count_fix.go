package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

// minimalStreamObject is a stack-allocated struct for fast JSON scanning
type minimalStreamObject struct {
	ID        string `json:"\u0000\u0001"`
	Kind      string `json:"\u0000\u0002"`
	Status    string `json:"\u0000\b"`
	CreatedAt any    `json:"\u0000\u0003"`
}

var streamMapPool = sync.Pool{
	New: func() any {
		m := make(map[string]any)
		return &m
	},
}

func loadStreamDeletedSetFast(projectRoot, kind string) map[string]bool {
	deleted := make(map[string]bool)
	delPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, "stream_deleted_"+kind+".jsonl")
	f, err := os.Open(delPath)
	if err != nil {
		return deleted
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		id := strings.TrimSpace(sc.Text())
		if id != "" {
			deleted[id] = true
		}
	}
	return deleted
}

// listStreamSegmentsWithLimit lists matching objects in stream segments using a parallel worker pool, up to a limit.
func (f *FileObjectStorage) listStreamSegmentsWithLimit(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter, limit int) ([]map[string]any, int, error) {
	segDir := filepath.Join(f.projectRoot, paths.ProjectDataDir, paths.StreamsDir, filter.Kind)
	entries, err := os.ReadDir(segDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	timeRange := f.extractTimeRangeFromFilters(filter.Filters)

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

	if len(segments) == 0 {
		return nil, 0, nil
	}

	maxWorkers := getListReadWorkers()
	if len(segments) < maxWorkers {
		maxWorkers = len(segments)
	}

	workCh := make(chan string, len(segments))
	for _, s := range segments {
		workCh <- s
	}
	close(workCh)

	type workerResult struct {
		objects []map[string]any
		total   int
	}
	results := make(chan workerResult, maxWorkers)
	var wg sync.WaitGroup
	actualWorkers := 0
	if len(segments) < maxWorkers {
		actualWorkers = len(segments)
	} else {
		actualWorkers = maxWorkers
	}
	wg.Add(actualWorkers)
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

	// Load deleted set directly to bypass massive streamRegistrySnapshot allocations
	deletedMap := loadStreamDeletedSetFast(f.projectRoot, filter.Kind)

	for i := 0; i < actualWorkers; i++ {

		workerName := ConstStreamStreamListWorker
		builder := goroutinelabels.NewGoroutine(workerName, ConstStreamScanningSegmentForList).
			WithContext(ctx).
			WithWaitGroup(&wg)
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

				file, err := os.Open(path)
				if err != nil {
					continue
				}

				scanner := bufio.NewScanner(file)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 10*1024*1024)

				for scanner.Scan() {
					if ctx.Err() != nil {
						file.Close()
						return
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

					if deletedMap != nil {
						id, _ := full[objects.FieldKeyID].(string)
						if deletedMap[id] {
							streamMapPool.Put(mp)
							continue
						}
					}

					// Convert to ParsedObject for matchesFiltersParsed
					parsed, _ := objects.ParseObject(full)

					matched := f.matchesFiltersParsed(parsed, filter.Filters)
					if matched {
						workerTotal++
						if limit == 0 || len(workerObjects) < limit {
							full[objects.FieldKeyKind] = filter.Kind
							workerObjects = append(workerObjects, full)
							// Do NOT put mp back in pool, we've appended it
						} else {
							streamMapPool.Put(mp)
						}
					} else {
						streamMapPool.Put(mp)
					}
				}
				if err := scanner.Err(); err != nil {
				}
				file.Close()
			}
			results <- workerResult{objects: workerObjects, total: workerTotal}
		})
	}

	// Deterministic wait for results (POLICY-CODE-004)
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
	case <-time.After(waitTimeout):
		return nil, 0, errfmt.Errorf(ConstStreamStreamListWaitTimeoutExceededVal, waitTimeout)
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}

	return allObjects, totalCount, nil
}

// countStreamSegmentsWithFilters counts matching objects in stream segments using a parallel worker pool and raw byte pre-filtering.
func (f *FileObjectStorage) countStreamSegmentsWithFilters(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	segDir := filepath.Join(f.projectRoot, paths.ProjectDataDir, paths.StreamsDir, filter.Kind)
	entries, err := os.ReadDir(segDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	timeRange := f.extractTimeRangeFromFilters(filter.Filters)

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

	if len(segments) == 0 {
		return 0, nil
	}

	maxWorkers := getListReadWorkers()
	if len(segments) < maxWorkers {
		maxWorkers = len(segments)
	}

	workCh := make(chan string, len(segments))
	for _, s := range segments {
		workCh <- s
	}
	close(workCh)

	results := make(chan int, maxWorkers)
	var wg sync.WaitGroup
	actualWorkers := 0
	if len(segments) < maxWorkers {
		actualWorkers = len(segments)
	} else {
		actualWorkers = maxWorkers
	}
	wg.Add(actualWorkers)
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

	// Load deleted set directly to bypass massive streamRegistrySnapshot allocations
	deletedMap := loadStreamDeletedSetFast(f.projectRoot, filter.Kind)

	for i := 0; i < actualWorkers; i++ {

		workerName := ConstStreamStreamCountWorker
		builder := goroutinelabels.NewGoroutine(workerName, ConstStreamScanningSegmentForCount).
			WithContext(ctx).
			WithWaitGroup(&wg)
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

				file, err := os.Open(path)
				if err != nil {
					continue
				}

				scanner := bufio.NewScanner(file)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 10*1024*1024)

				for scanner.Scan() {
					if ctx.Err() != nil {
						file.Close()
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

					if deletedMap != nil {
						id, _ := full[objects.FieldKeyID].(string)
						if deletedMap[id] {
							streamMapPool.Put(mp)
							continue
						}
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
				file.Close()
			}
			results <- workerTotal
		})
	}

	// Deterministic wait for results (POLICY-CODE-004)
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
