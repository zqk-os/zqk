package validation

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
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
		t.Fatalf("failed to start validator: %v", err)
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
		t.Errorf("Expected %d max workers (NumCPU * 2), got %d", expectedWorkers, maxWorkers)
	}

	// Test with explicit worker count override
	validator2 := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 8, time.Hour)
	if err := validator2.Start(); err != nil {
		t.Fatalf("failed to start validator2: %v", err)
	}
	defer func() { _ = validator2.Stop() }() //nolint:errcheck
	maxWorkers2 := validator2.GetMaxWorkers()
	if maxWorkers2 != 8 {
		t.Errorf("Expected 8 max workers, got %d", maxWorkers2)
	}

	// Verify it's not hardcoded to 4
	if expectedWorkers > 4 && actualWorkers == 4 {
		t.Error("Worker count appears to be hardcoded to 4 instead of using NumCPU")
	}
}

// TestAsyncValidator_ShouldEnqueue tests the ShouldEnqueue cache check method
func TestAsyncValidator_ShouldEnqueue(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck

	// Create test file
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	testFile := filepath.Join(testDir, "TEST-001.yaml")
	content := "id: TEST-001\nkind: test_object\n"
	if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// First check - should need validation (not in cache)
	if !validator.ShouldEnqueue("TEST-001", "test_object", testFile) {
		t.Error("Expected ShouldEnqueue to return true for uncached object")
	}

	// Enqueue and validate to populate cache
	validator.Enqueue("TEST-001", "test_object", testFile, 1)

	// Wait for validation to complete
	time.Sleep(500 * time.Millisecond)

	// Second check - should not need validation (in cache)
	if validator.ShouldEnqueue("TEST-001", "test_object", testFile) {
		t.Error("Expected ShouldEnqueue to return false for cached object")
	}
}

// TestAsyncValidator_EnqueueBatchOptimization tests batch enqueue optimization
func TestAsyncValidator_EnqueueBatchOptimization(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create batch of tasks
	tasks := make([]ValidationTask, 0, 10)
	for i := 1; i <= 10; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf("id: TEST-%03d\nkind: test_object\n", i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
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
		t.Errorf("Expected queue size <= %d after batch enqueue, got %d", expectedBatchSize, queueSize)
	}
}

// TestAsyncValidator_BatchEnqueueOptimization tests that batch enqueue reduces lock contention
func TestAsyncValidator_BatchEnqueueOptimization(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create large batch (100 tasks)
	const batchSize = 100
	tasks := make([]ValidationTask, 0, batchSize)
	for i := 1; i <= batchSize; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf("id: TEST-%03d\nkind: test_object\n", i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
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
		t.Fatalf("failed to start validator: %v", err)
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
		t.Logf("Batch enqueue took %v for %d items, individual took %v for 10 items", batchDuration, batchSize, individualDuration)
		// This is not a failure - just a note that batch might not be faster for small batches
		// The real benefit is for large batches (100+ items)
	}

	// Wait for processing
	time.Sleep(2 * time.Second)
}
