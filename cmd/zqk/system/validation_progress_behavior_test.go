package system

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/spf13/cobra"
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
		_ = filepath.Walk(tmpDir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && info.Name() == paths.ProjectDataDir {
				_ = fileutil.RemoveAll(path)
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
		if err := fileutil.WriteSecureFile(p, []byte("id: BLI-"+fmt.Sprintf("%03d", i)+"\n")); err != nil {
			t.Fatalf("write dummy yaml: %v", err)
		}
	}
	filesStream := make(chan []scannedFile, 4)
	goroutinelabels.NewGoroutine("system_test", "stream files").StartSimple(func() {
		defer close(filesStream)
		for i := 0; i < N; i += 10 {
			batch := make([]scannedFile, 0, 10)
			for j := i; j < i+10 && j < N; j++ {
				// Use BLI-* IDs: OBJ-* maps to object_spec via ID prefix, which would defer/skip
				// discovery entries that claim Kind backlog_item (enqueueFilesAsDiscovered).
				path := filepath.Join(tmpDir, fmt.Sprintf("bli-%03d.yaml", j))
				batch = append(batch, scannedFile{
					ObjectID: fmt.Sprintf("BLI-%03d", j),
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
		enqueuedIDs[fmt.Sprintf("BLI-%03d", i)] = true
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

func drainValidationOutputQueue(q *validation.OutputQueue) string {
	var b strings.Builder
	for {
		pkt, ok := q.DequeueNonBlocking()
		if !ok {
			break
		}
		switch d := pkt.Data.(type) {
		case string:
			b.WriteString(d)
		case []byte:
			b.Write(d)
		default:
			b.WriteString(fmt.Sprint(d))
		}
	}
	return b.String()
}

// TestEmitValidationFinishTrailer_Order is the regression for the dangling
// Progress line that used to print after Validation Metrics because metrics
// bypassed the output queue.
func TestEmitValidationFinishTrailer_Order(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().Bool(validationFlagVerbose, false, "")
	if err := cmd.Flags().Set(validationFlagVerbose, "true"); err != nil {
		t.Fatal(err)
	}

	metrics := validation.NewValidationMetrics()
	metrics.StartTime = time.Now().Add(-2 * time.Second)
	for i := 0; i < 10; i++ {
		metrics.IncrementValidated()
	}
	metrics.SetTotalObjects(10)

	queue := validation.NewOutputQueue(100)
	vpc := &ValidationProgressContext{
		Cmd:             cmd,
		Validator:       validation.NewAsyncValidator(pkgctx.NewSystemContext(), t.TempDir(), 0, time.Hour),
		TotalTasks:      10,
		Metrics:         metrics,
		OutputQueue:     queue,
		Logger:          logging.GetLoggerFromProfile("test"),
		StartTime:       time.Now().Add(-2 * time.Second),
		ValidationStart: time.Now().Add(-2 * time.Second),
	}

	emitValidationFinishTrailer(vpc, 10, 0)

	out := drainValidationOutputQueue(queue)
	progressIdx := strings.Index(out, "Progress:")
	metricsIdx := strings.Index(out, "Validation Metrics:")
	doneIdx := strings.Index(out, "Validation completed:")
	if progressIdx < 0 || metricsIdx < 0 || doneIdx < 0 {
		t.Fatalf("missing trailer pieces in %q", out)
	}
	if !(progressIdx < metricsIdx && metricsIdx < doneIdx) {
		t.Fatalf("want Progress then Metrics then completed, got indexes %d %d %d in %q",
			progressIdx, metricsIdx, doneIdx, out)
	}
	if last := strings.LastIndex(out, "Progress:"); last > metricsIdx {
		t.Fatalf("Progress after metrics in %q", out)
	}
	if strings.Contains(out, "Performance: 0.00 objects/sec") {
		t.Fatalf("zero throughput in trailer:\n%s", out)
	}
	if strings.Contains(out, "Cache:") {
		t.Fatalf("zero-hit cache line must be omitted from trailer:\n%s", out)
	}
	if !vpc.SuppressProgress {
		t.Fatal("expected SuppressProgress after trailer so ticker cannot reprint Progress")
	}
}

func TestValidationProgressStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                             string
		queueEmpty, flush, running       bool
		totalCompleted, processed, total int
		workers                          int
		since                            time.Duration
		want                             string
	}{
		{
			name:           "trailer owns completed",
			queueEmpty:     true,
			flush:          true,
			totalCompleted: 8201,
			processed:      8201,
			total:          8201,
			want:           " (completed)",
		},
		{
			name:           "ticker must not claim completed",
			queueEmpty:     true,
			flush:          false,
			running:        true,
			totalCompleted: 8201,
			processed:      8201,
			total:          8201,
			workers:        8,
			want:           " (finishing, workers: 8)",
		},
		{
			name:           "empty queue with in-flight workers is drain not processing-results",
			queueEmpty:     true,
			running:        true,
			totalCompleted: 8185,
			processed:      8185,
			total:          8201,
			workers:        8,
			since:          time.Second,
			want:           " (draining in-flight, workers: 8)",
		},
		{
			name:      "queue has work",
			workers:   8,
			total:     8201,
			processed: 100,
			want:      " (validating, workers: 8, goroutines: 0)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validationProgressStatus(
				tt.queueEmpty, tt.flush, tt.running,
				tt.totalCompleted, tt.processed, tt.total,
				tt.workers, 0, tt.since,
			)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestWriteValidationProgressLine_TickerSkipsFinalSnapshot(t *testing.T) {
	t.Parallel()
	queue := validation.NewOutputQueue(100)
	vpc := &ValidationProgressContext{
		Validator:            validation.NewAsyncValidator(pkgctx.NewSystemContext(), t.TempDir(), 0, time.Hour),
		TotalTasks:           10,
		OutputQueue:          queue,
		Logger:               logging.GetLoggerFromProfile("test"),
		StartTime:            time.Now().Add(-2 * time.Second),
		LastPrintedCompleted: 8,
		LastPrintedQueueSize: 0,
	}
	writeValidationProgressLine(vpc, 10, 0, true, 10, false)
	if out := drainValidationOutputQueue(queue); strings.Contains(out, "Progress:") {
		t.Fatalf("ticker must not print 100%% completed; got %q", out)
	}
	if vpc.LastPrintedCompleted != 8 {
		t.Fatalf("skip must not advance LastPrintedCompleted (trailer still needs to print), got %d", vpc.LastPrintedCompleted)
	}
}

func TestEmitValidationFinishTrailer_SkipsDuplicateProgress(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{}
	cmd.Flags().Bool(validationFlagVerbose, false, "")
	queue := validation.NewOutputQueue(100)
	vpc := &ValidationProgressContext{
		Cmd:                  cmd,
		Validator:            validation.NewAsyncValidator(pkgctx.NewSystemContext(), t.TempDir(), 0, time.Hour),
		TotalTasks:           10,
		OutputQueue:          queue,
		Logger:               logging.GetLoggerFromProfile("test"),
		StartTime:            time.Now().Add(-2 * time.Second),
		ValidationStart:      time.Now().Add(-2 * time.Second),
		LastPrintedCompleted: 10,
		LastPrintedQueueSize: 0,
	}
	emitValidationFinishTrailer(vpc, 10, 0)
	out := drainValidationOutputQueue(queue)
	if strings.Contains(out, "Progress:") {
		t.Fatalf("duplicate 100%% Progress after ticker already printed it: %q", out)
	}
	if !strings.Contains(out, "Validation completed:") {
		t.Fatalf("still need the completion summary, got %q", out)
	}
	if !vpc.SuppressProgress {
		t.Fatal("expected SuppressProgress after trailer")
	}
}
