package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/validation"
)

// TestAllTasksAccountedFor verifies cache-based completion logic.
// When (progress + cached) >= total we treat as completion so we don't stall at ~83%.
func TestAllTasksAccountedFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		progressCount int
		cachedCount   int
		totalTasks    int
		want          bool
	}{
		{"zero total is not complete", 0, 0, 0, false},
		{"progress equals total", 10, 0, 10, true},
		{"progress plus cached equals total", 8, 2, 10, true},
		{"progress plus cached exceeds total", 7, 4, 10, true},
		{"progress below total no cache", 7, 0, 10, false},
		{"progress plus cached below total", 6, 2, 10, false},
		{"all from cache", 0, 10, 10, true},
		{"single task complete", 1, 0, 1, true},
		{"single task incomplete", 0, 0, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allTasksAccountedFor(tt.progressCount, tt.cachedCount, tt.totalTasks)
			if got != tt.want {
				t.Errorf("allTasksAccountedFor(%d, %d, %d) = %v, want %v",
					tt.progressCount, tt.cachedCount, tt.totalTasks, got, tt.want)
			}
		})
	}
}

// TestEnqueueBookkeeping verifies that after enqueueFilesAsDiscovered, TotalTasks
// equals len(EnqueuedObjectIDs) and that every enqueued object ID is tracked.
func TestEnqueueBookkeeping(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Remove .zqk dirs under tmpDir before t.TempDir() cleanup so "directory not empty" does not occur
	t.Cleanup(func() {
		_ = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && info.Name() == paths.ProjectDataDir {
				_ = os.RemoveAll(path)
				return filepath.SkipDir
			}
			return nil
		})
	})

	// Workers must be 0: EnqueueBatch still notifies workers, but wakeWorkerIfNeeded returns
	// immediately when maxWorkers==0, so no validation goroutines run (this test only checks enqueue bookkeeping).
	validator := validation.NewAsyncValidator(pkgctx.NewSystemContext(), tmpDir, 0, time.Hour)

	checkCtx := &AsyncCheckContext{
		ProjectRoot:       tmpDir,
		AsyncValidator:    validator,
		Metrics:           validation.NewValidationMetrics(),
		Logger:            logging.GetLoggerFromProfile("test"),
		EnqueuedObjectIDs: make(map[string]bool),
		// Match production initialization so enqueueOrUseCache can record kind/file info without panicking.
		EnqueuedObjectInfo: make(map[string]struct{ Kind, FilePath string }),
	}
	checkCtx.Cmd = nil
	checkCtx.Ctx = nil

	// Create a stream of N files (all cache miss so they get enqueued)
	const N = 47
	for i := 0; i < N; i++ {
		p := filepath.Join(tmpDir, fmt.Sprintf("bli-%03d.yaml", i))
		if err := fileutil.WriteSecureFile(p, []byte("id: ITEM-"+fmt.Sprintf("%03d", i)+"\n")); err != nil {
			t.Fatalf("write dummy yaml: %v", err)
		}
	}
	filesStream := make(chan []scannedFile, 4)
	goroutinelabels.NewGoroutine("system_test", "stream files").StartSimple(func() {
		defer close(filesStream)
		for i := 0; i < N; i += 10 {
			batch := make([]scannedFile, 0, 10)
			for j := i; j < i+10 && j < N; j++ {
				// Use ITEM-* IDs: OBJ-* maps to object_spec via ID prefix, which would defer/skip
				// discovery entries that claim Kind backlog_item (enqueueFilesAsDiscovered).
				path := filepath.Join(tmpDir, fmt.Sprintf("bli-%03d.yaml", j))
				batch = append(batch, scannedFile{
					ObjectID: fmt.Sprintf("ITEM-%03d", j),
					Kind:     "backlog_item",
					Path:     path,
				})
			}
			if len(batch) > 0 {
				filesStream <- batch
			}
		}
	})

	err := enqueueFilesAsDiscovered(checkCtx, filesStream)
	if err != nil {
		t.Fatalf("enqueueFilesAsDiscovered: %v", err)
	}

	if checkCtx.TotalTasks != N {
		t.Errorf("TotalTasks = %d, want %d", checkCtx.TotalTasks, N)
	}
	if len(checkCtx.EnqueuedObjectIDs) != N {
		t.Errorf("len(EnqueuedObjectIDs) = %d, want %d", len(checkCtx.EnqueuedObjectIDs), N)
	}
	if checkCtx.TotalTasks != len(checkCtx.EnqueuedObjectIDs) {
		t.Errorf("TotalTasks (%d) != len(EnqueuedObjectIDs) (%d)", checkCtx.TotalTasks, len(checkCtx.EnqueuedObjectIDs))
	}

	// Ensure we have exactly N unique IDs (no duplicates)
	ids := make(map[string]bool)
	for id := range checkCtx.EnqueuedObjectIDs {
		if ids[id] {
			t.Errorf("duplicate EnqueuedObjectIDs entry: %s", id)
		}
		ids[id] = true
	}
	if len(ids) != N {
		t.Errorf("unique enqueued IDs = %d, want %d", len(ids), N)
	}
}

// TestCheckStuckCondition_ReturnsStuckWhenNoProgress verifies that when the queue is empty,
// progress is below total, and no progress has been made for longer than the stuck timeout,
// checkStuckCondition returns (false, non-nil error) so the caller can stop the validator.
func TestCheckStuckCondition_ReturnsStuckWhenNoProgress(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	validator := validation.NewAsyncValidator(pkgctx.NewSystemContext(), tmpDir, 2, time.Hour)

	enqueuedIDs := make(map[string]bool)
	for i := 0; i < 10; i++ {
		enqueuedIDs[fmt.Sprintf("ITEM-%03d", i)] = true
	}

	oldTime := time.Now().Add(-200 * time.Second)
	queueEmptySince := time.Now().Add(-195 * time.Second)

	vpc := &ValidationProgressContext{
		Validator:         validator,
		TotalTasks:        10,
		EnqueuedObjectIDs: enqueuedIDs,
		Completed:         5,
		Failed:            0,
		LastProgressTime:  oldTime,
		LastProgressCount: 5,
		StuckTimeout:      45 * time.Second,
		Logger:            logging.GetLoggerFromProfile("test"),
		QueueEmptySince:   &queueEmptySince,
	}

	// totalAccounted = 5 (no cache); no increase from LastProgressCount(5) -> stuck
	notStuck, err := checkStuckCondition(vpc, 5, 0, 0, 5)
	if notStuck {
		t.Error("checkStuckCondition: expected not stuck (false), got true")
	}
	if err == nil {
		t.Error("checkStuckCondition: expected non-nil error when stuck, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "stuck") {
		t.Errorf("checkStuckCondition: error should mention 'stuck', got: %q", err.Error())
	}
}
