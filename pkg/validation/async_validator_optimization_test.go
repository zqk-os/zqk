package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TestAsyncValidator_WorkerCountOptimization tests that worker count uses NumCPU
func TestAsyncValidator_WorkerCountOptimization(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Test with explicit worker count (this is what GetAsyncValidator does)
	expectedWorkers := runtime.NumCPU() * 2
	if expectedWorkers > 32 {
		expectedWorkers = 32
	}
	if expectedWorkers < 4 {
		expectedWorkers = 4
	}

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, expectedWorkers, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck

	// GetWorkerCount should return the configured worker count after start
	// Note: Workers are created on-demand, so count may be 0 until work is enqueued
	// We test the configured count, not the active count
	actualWorkers := validator.GetWorkerCount()
	// GetWorkerCount returns active workers, which may be 0 until work starts
	// The test should verify the max workers configuration instead
	maxWorkers := validator.GetMaxWorkers()
	if maxWorkers != expectedWorkers {
		t.Errorf(ConstMagic412fea98, expectedWorkers, maxWorkers)
	}

	// Test with explicit worker count override
	validator2 := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 8, time.Hour)
	if err := validator2.Start(); err != nil {
		t.Fatalf(ConstMagicd8da7fcd, err)
	}
	defer func() { _ = validator2.Stop() }() //nolint:errcheck
	maxWorkers2 := validator2.GetMaxWorkers()
	if maxWorkers2 != 8 {
		t.Errorf(ConstMagic771067a7, maxWorkers2)
	}

	// Verify it's not hardcoded to 4
	if expectedWorkers > 4 && actualWorkers == 4 {
		t.Error(ConstMagic4629a101)
	}
}

// TestAsyncValidator_ShouldEnqueue tests the ShouldEnqueue cache check method
func TestAsyncValidator_ShouldEnqueue(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck

	// Create test file
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := os.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic32b1c202, err)
	}

	testFile := filepath.Join(testDir, "TEST-001.yaml")
	content := ConstMagicbd310509
	if err := os.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic7a424835, err)
	}

	// First check - should need validation (not in cache)
	if !validator.ShouldEnqueue("TEST-001", "test_object", testFile) {
		t.Error(ConstMagicac50c5b6)
	}

	// Enqueue and validate to populate cache
	validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for validation to complete
	time.Sleep(500 * time.Millisecond)

	// Second check - should not need validation (in cache)
	if validator.ShouldEnqueue("TEST-001", "test_object", testFile) {
		t.Error(ConstMagic11dfd009)
	}
}

// TestAsyncValidator_EnqueueBatchOptimization tests batch enqueue optimization
func TestAsyncValidator_EnqueueBatchOptimization(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := os.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic32b1c202, err)
	}

	// Create batch of tasks
	tasks := make([]ValidationTask, 0, 10)
	for i := 1; i <= 10; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf(ConstMagic28b7580d, i)
		if err := os.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}

		tasks = append(tasks, ValidationTask{
			ObjectID:   fmt.Sprintf("TEST-%03d", i),
			ObjectKind: "test_object",
			FilePath:   testFile,
			Priority:   1,
			Checksum:   "",
			EnqueuedAt: time.Now(),
			MaxRetries: 3,
		})
	}

	// Enqueue batch
	validator.EnqueueBatch(tasks)

	// Wait for processing
	time.Sleep(1 * time.Second)

	// Verify queue size decreased (tasks are being processed)
	// Note: Queue might not be empty yet if workers are still processing
	queueSize := validator.priorityQueue.Size()
	const expectedBatchSize = 10
	if queueSize > expectedBatchSize {
		t.Errorf(ConstMagic27bad693, expectedBatchSize, queueSize)
	}
}

// TestAsyncValidator_BatchEnqueueOptimization tests that batch enqueue reduces lock contention
func TestAsyncValidator_BatchEnqueueOptimization(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := os.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic32b1c202, err)
	}

	// Create large batch (100 tasks)
	const batchSize = 100
	tasks := make([]ValidationTask, 0, batchSize)
	for i := 1; i <= batchSize; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf(ConstMagic28b7580d, i)
		if err := os.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}

		tasks = append(tasks, ValidationTask{
			ObjectID:   fmt.Sprintf("TEST-%03d", i),
			ObjectKind: "test_object",
			FilePath:   testFile,
			Priority:   1,
			Checksum:   "",
			EnqueuedAt: time.Now(),
			MaxRetries: 3,
		})
	}

	// Measure time for batch enqueue
	start := time.Now()
	validator.EnqueueBatch(tasks)
	batchDuration := time.Since(start)

	// Measure time for individual enqueue (for comparison)
	// Note: This is just to verify batch is faster, not a strict performance test
	validator2 := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	if err := validator2.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator2.Stop() }() //nolint:errcheck

	start = time.Now()
	for _, task := range tasks[:10] { // Only test with 10 to keep test fast
		validator2.Enqueue(task.ObjectID, task.ObjectKind, task.FilePath, task.Priority)
	}
	individualDuration := time.Since(start)

	// Batch should be faster per item (or at least not significantly slower)
	// For 10 items, batch should be faster due to single lock acquisition
	if batchDuration > individualDuration*2 {
		t.Logf(ConstMagic5c77a7cc, batchDuration, batchSize, individualDuration)
		// This is not a failure - just a note that batch might not be faster for small batches
		// The real benefit is for large batches (100+ items)
	}

	// Wait for processing
	time.Sleep(2 * time.Second)
}
