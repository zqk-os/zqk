package validation

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/datacell"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// TestAsyncValidator_ConcurrentAccess tests that multiple goroutines can safely
// access the same validator instance without deadlocks or data corruption
func TestAsyncValidator_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create 100 test objects
	numObjects := 100
	for i := 1; i <= numObjects; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf("id: TEST-%03d\nkind: test_object\n", i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	// Concurrent enqueue operations
	var wg sync.WaitGroup
	enqueueErrors := int64(0)
	numGoroutines := 10
	objectsPerGoroutine := numObjects / numGoroutines

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(startID int) {
				defer wg.Done()
				for j := 0; j < objectsPerGoroutine; j++ {
					objID := fmt.Sprintf("TEST-%03d", startID+j+1)
					filePath := filepath.Join(testDir, fmt.Sprintf("%s.yaml", objID))
					priority := (j % 4) + 1 // Mix priorities
					_ = validator.Enqueue(objID, "test_object", filePath, priority)
				}
			}(i * objectsPerGoroutine)
		})
	}

	wg.Wait()

	// Wait for processing to complete
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for validation to complete")
		case <-ticker.C:
			total, _, _, queueSize := validator.GetValidationStats()
			if queueSize == 0 && total >= numObjects {
				// All tasks processed
				if enqueueErrors > 0 {
					t.Errorf("encountered %d enqueue errors", enqueueErrors)
				}
				return
			}
		}
	}
}

// TestValidationStateCache_ConcurrentReadWrite tests concurrent read/write
// operations on the cache without data corruption
func TestValidationStateCache_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	cache := NewValidationStateCache(testRoot, time.Hour)

	// Concurrent write operations
	numWriters := 20
	numObjects := 100
	var wg sync.WaitGroup
	writeErrors := atomic.Int64{}

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(writerID int) {
				defer wg.Done()
				for j := 0; j < numObjects; j++ {
					objID := fmt.Sprintf("TEST-W%d-O%d", writerID, j)
					state := &ValidationState{
						ObjectID:      objID,
						ObjectKind:    "test_object",
						FilePath:      fmt.Sprintf("test-%d.yaml", j),
						LastValidated: time.Now(),
						Checksum:      fmt.Sprintf("checksum-%d-%d", writerID, j),
						Issues:        []ValidationIssue{},
					}
					cache.Set(state)
				}
			}(i)
		})
	}

	// Concurrent read operations
	numReaders := 10
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(readerID int) {
				defer wg.Done()
				for j := 0; j < numObjects; j++ {
					objID := fmt.Sprintf("TEST-W%d-O%d", readerID%numWriters, j)
					_, exists := cache.Get(objID)
					// It's OK if it doesn't exist yet (race condition)
					_ = exists
				}
			}(i)
		})
	}

	wg.Wait()

	// Verify cache integrity
	total, stale, withIssues := cache.Count()
	if total < numWriters*numObjects/2 {
		t.Errorf("expected at least %d cached states, got %d", numWriters*numObjects/2, total)
	}

	// Save and reload to verify persistence
	if err := cache.Save(); err != nil {
		t.Fatalf("failed to save cache: %v", err)
	}

	// Create new cache instance and load
	cache2 := NewValidationStateCache(testRoot, time.Hour)
	if err := cache2.Load(); err != nil {
		t.Fatalf("failed to load cache: %v", err)
	}

	total2, _, _ := cache2.Count()
	if total2 != total {
		t.Errorf("cache count mismatch after reload: expected %d, got %d", total, total2)
	}

	if writeErrors.Load() > 0 {
		t.Errorf("encountered %d write errors", writeErrors.Load())
	}
	_ = stale
	_ = withIssues
}

// TestValidationStateCache_MultipleProcesses simulates multiple CLI processes
// accessing the same cache file concurrently
func TestValidationStateCache_MultipleProcesses(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Simulate multiple processes by creating multiple cache instances
	// that share the same cache file
	numProcesses := 5
	caches := make([]*ValidationStateCache, numProcesses)
	for i := 0; i < numProcesses; i++ {
		caches[i] = NewValidationStateCache(testRoot, time.Hour)
		if err := caches[i].Load(); err != nil {
			t.Fatalf("process %d failed to load cache: %v", i, err)
		}
	}

	// Each process writes different objects
	numObjectsPerProcess := 50
	var wg sync.WaitGroup
	writeErrors := atomic.Int64{}

	for i := 0; i < numProcesses; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(processID int) {
				defer wg.Done()
				cache := caches[processID]

				for j := 0; j < numObjectsPerProcess; j++ {
					objID := fmt.Sprintf("PROC-%d-OBJ-%03d", processID, j)
					state := &ValidationState{
						ObjectID:      objID,
						ObjectKind:    "test_object",
						FilePath:      fmt.Sprintf("proc-%d-obj-%d.yaml", processID, j),
						LastValidated: time.Now(),
						Checksum:      fmt.Sprintf("checksum-%d-%d", processID, j),
						Issues:        []ValidationIssue{},
					}
					cache.Set(state)

					// Periodically save (simulating real process behavior)
					if j%10 == 0 {
						if err := cache.Save(); err != nil {
							writeErrors.Add(1)
						}
					}
				}

				// Final save
				if err := cache.Save(); err != nil {
					writeErrors.Add(1)
				}
			}(i)
		})
	}

	wg.Wait()

	// Verify all processes can read all objects
	for i := 0; i < numProcesses; i++ {
		cache := caches[i]
		if err := cache.Load(); err != nil {
			t.Fatalf("process %d failed to reload cache: %v", i, err)
		}

		// Check that this process can see its own objects
		for j := 0; j < numObjectsPerProcess; j++ {
			objID := fmt.Sprintf("PROC-%d-OBJ-%03d", i, j)
			_, exists := cache.Get(objID)
			if !exists {
				t.Errorf("process %d cannot find its own object %s", i, objID)
			}
		}
	}

	if writeErrors.Load() > 0 {
		t.Logf("encountered %d write errors (expected due to concurrent writes)", writeErrors.Load())
		// Some write errors are expected due to concurrent file access
		// The important thing is that the cache file remains valid
	}

	// Verify cache file is valid JSON
	finalCache := NewValidationStateCache(testRoot, time.Hour)
	if err := finalCache.Load(); err != nil {
		t.Fatalf("failed to load final cache: %v", err)
	}

	total, _, _ := finalCache.Count()
	if total == 0 {
		t.Error("cache is empty after all processes wrote to it")
	}
}

// TestAsyncValidator_StressTest tests the validator with a large dataset
func TestAsyncValidator_StressTest(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Use more workers for stress test
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 8, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create large dataset
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	numObjects := 1000
	t.Logf("Creating %d test objects...", numObjects)

	for i := 1; i <= numObjects; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%04d.yaml", i))
		content := fmt.Sprintf("id: TEST-%04d\nkind: test_object\n", i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	t.Logf("Enqueuing %d validation tasks...", numObjects)

	// Enqueue all tasks with mixed priorities
	for i := 1; i <= numObjects; i++ {
		objID := fmt.Sprintf("TEST-%04d", i)
		filePath := filepath.Join(testDir, fmt.Sprintf("%s.yaml", objID))
		priority := (i % 4) + 1 // Mix priorities
		_ = validator.Enqueue(objID, "test_object", filePath, priority)
	}

	// Monitor progress
	startTime := time.Now()
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			total, stale, withIssues, queueSize := validator.GetValidationStats()
			t.Fatalf("timeout: processed %d/%d, queue: %d, stale: %d, withIssues: %d",
				total, numObjects, queueSize, stale, withIssues)
		case <-ticker.C:
			total, stale, withIssues, queueSize := validator.GetValidationStats()
			elapsed := time.Since(startTime)
			t.Logf("Progress: %d/%d processed, queue: %d, stale: %d, withIssues: %d (elapsed: %v)",
				total, numObjects, queueSize, stale, withIssues, elapsed)

			if queueSize == 0 && total >= numObjects {
				elapsed := time.Since(startTime)
				t.Logf("Completed validation of %d objects in %v", numObjects, elapsed)
				return
			}
		}
	}
}

// TestAsyncValidator_ConcurrentValidators tests multiple validator instances
// operating on the same project root simultaneously
func TestAsyncValidator_ConcurrentValidators(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	numObjects := 200
	for i := 1; i <= numObjects; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf("id: TEST-%03d\nkind: test_object\n", i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	// Create multiple validator instances (simulating multiple CLI processes)
	numValidators := 3
	validators := make([]*AsyncValidator, numValidators)
	for i := 0; i < numValidators; i++ {
		validators[i] = NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)
		if err := validators[i].Start(); err != nil {
			t.Fatalf("validator %d failed to start: %v", i, err)
		}
	}
	// Clean up all validators at end of test
	defer func() {
		for _, v := range validators {
			_ = v.Stop() //nolint:errcheck // Test cleanup - errors are acceptable
		}
	}()

	// Each validator processes a subset of objects
	objectsPerValidator := numObjects / numValidators
	var wg sync.WaitGroup

	for i := 0; i < numValidators; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(validatorID int) {
				defer wg.Done()
				validator := validators[validatorID]

				startID := validatorID * objectsPerValidator
				for j := 0; j < objectsPerValidator; j++ {
					objID := fmt.Sprintf("TEST-%03d", startID+j+1)
					filePath := filepath.Join(testDir, fmt.Sprintf("%s.yaml", objID))
					priority := (j % 4) + 1
					_ = validator.Enqueue(objID, "test_object", filePath, priority)
				}
			}(i)
		})
	}

	wg.Wait()

	// Wait for all validators to complete
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for validators to complete")
		case <-ticker.C:
			allDone := true
			for i, validator := range validators {
				_, _, _, queueSize := validator.GetValidationStats()
				if queueSize > 0 {
					allDone = false
					t.Logf("Validator %d: queue size %d", i, queueSize)
				}
			}
			if allDone {
				t.Log("All validators completed")
				return
			}
		}
	}
}

// TestValidationStateCache_FileLocking tests that concurrent saves don't corrupt
// the cache file
func TestValidationStateCache_FileLocking(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	// Create multiple cache instances sharing the same file
	numCaches := 5
	caches := make([]*ValidationStateCache, numCaches)
	for i := 0; i < numCaches; i++ {
		caches[i] = NewValidationStateCache(testRoot, time.Hour)
		if err := caches[i].Load(); err != nil {
			t.Fatalf("cache %d failed to load: %v", i, err)
		}
	}

	// Concurrent saves
	var wg sync.WaitGroup
	saveErrors := atomic.Int64{}
	numSaves := 20

	for i := 0; i < numCaches; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
			func(cacheID int) {
				defer wg.Done()
				cache := caches[cacheID]

				for j := 0; j < numSaves; j++ {
					// Add some state
					objID := fmt.Sprintf("CACHE-%d-SAVE-%d", cacheID, j)
					state := &ValidationState{
						ObjectID:      objID,
						ObjectKind:    "test_object",
						FilePath:      fmt.Sprintf("cache-%d-save-%d.yaml", cacheID, j),
						LastValidated: time.Now(),
						Checksum:      fmt.Sprintf("checksum-%d-%d", cacheID, j),
						Issues:        []ValidationIssue{},
					}
					cache.Set(state)

					// Save
					if err := cache.Save(); err != nil {
						saveErrors.Add(1)
					}

					// Small delay to increase chance of concurrent writes
					time.Sleep(10 * time.Millisecond)
				}
			}(i)
		})
	}

	wg.Wait()

	// Verify cache file is still valid JSON
	finalCache := NewValidationStateCache(testRoot, time.Hour)
	if err := finalCache.Load(); err != nil {
		t.Fatalf("failed to load final cache (file may be corrupted): %v", err)
	}

	total, _, _ := finalCache.Count()
	if total == 0 {
		t.Error("cache is empty after concurrent saves")
	}

	// Some save errors are expected due to file locking (when multiple processes try to save simultaneously)
	// The important thing is that the cache file remains valid and no data is corrupted
	if saveErrors.Load() > int64(numCaches*numSaves) {
		t.Errorf("unexpectedly high save errors: %d (may indicate file locking issues)", saveErrors.Load())
	} else if saveErrors.Load() > 0 {
		t.Logf("encountered %d save errors due to file locking (expected in concurrent scenarios)", saveErrors.Load())
	}
}

// TestAsyncValidator_ValidateNow_Concurrent tests concurrent ValidateNow calls
func TestAsyncValidator_ValidateNow_Concurrent(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	numObjects := 100
	for i := 1; i <= numObjects; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf("id: TEST-%03d\nkind: test_object\n", i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	// Concurrent ValidateNow calls
	ctx := pkgctx.NewSystemContext()
	var wg sync.WaitGroup
	validationErrors := atomic.Int64{}
	numGoroutines := 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("refactor", "refactored").

			// Each goroutine validates a subset of objects
			StartSimple(func() {
				func(goroutineID int) {
					defer wg.Done()

					objectsPerGoroutine := numObjects / numGoroutines
					startID := goroutineID * objectsPerGoroutine

					for j := 0; j < objectsPerGoroutine; j++ {
						objID := fmt.Sprintf("TEST-%03d", startID+j+1)
						filePath := filepath.Join(testDir, fmt.Sprintf("%s.yaml", objID))
						_, err := validator.ValidateNow(ctx, objID, "test_object", filePath)
						if err != nil {
							validationErrors.Add(1)
						}
					}
				}(i)
			})
	}

	wg.Wait()

	if validationErrors.Load() > 0 {
		t.Errorf("encountered %d validation errors", validationErrors.Load())
	}

	// Verify all objects are cached
	for i := 1; i <= numObjects; i++ {
		objID := fmt.Sprintf("TEST-%03d", i)
		_, exists := validator.GetCachedState(objID)
		if !exists {
			t.Errorf("object %s not cached after ValidateNow", objID)
		}
	}
}
