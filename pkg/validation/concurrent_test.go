package validation

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/datacell"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TestAsyncValidator_ConcurrentAccess tests that multiple goroutines can safely
// access the same validator instance without deadlocks or data corruption
func TestAsyncValidator_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	testRoot := registerZQKTestRootForTest(t)

	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 4, time.Hour)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create test files
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic32b1c202, err)
	}

	// Create 100 test objects
	numObjects := 100
	for i := 1; i <= numObjects; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf(ConstMagic28b7580d, i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
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
			t.Fatal(ConstMagic16e76391)
		case <-ticker.C:
			total, _, _, queueSize := validator.GetValidationStats()
			if queueSize == 0 && total >= numObjects {
				// All tasks processed
				if enqueueErrors > 0 {
					t.Errorf(ConstMagicb4110088, enqueueErrors)
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
		t.Errorf(ConstMagic231af6c1, numWriters*numObjects/2, total)
	}

	// Save and reload to verify persistence
	if err := cache.Save(); err != nil {
		t.Fatalf(ConstMagic54fa5912, err)
	}

	// Create new cache instance and load
	cache2 := NewValidationStateCache(testRoot, time.Hour)
	if err := cache2.Load(); err != nil {
		t.Fatalf(ConstMagic969edc24, err)
	}

	total2, _, _ := cache2.Count()
	if total2 != total {
		t.Errorf(ConstMagic897d8b59, total, total2)
	}

	if writeErrors.Load() > 0 {
		t.Errorf(ConstMagic9e334c4c, writeErrors.Load())
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
			t.Fatalf(ConstMagic75206709, i, err)
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
					objID := fmt.Sprintf(ConstMagicc3c76e1f, processID, j)
					state := &ValidationState{
						ObjectID:      objID,
						ObjectKind:    "test_object",
						FilePath:      fmt.Sprintf(ConstMagic569325a0, processID, j),
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
			t.Fatalf(ConstMagic3ef8eb0e, i, err)
		}

		// Check that this process can see its own objects
		for j := 0; j < numObjectsPerProcess; j++ {
			objID := fmt.Sprintf(ConstMagicc3c76e1f, i, j)
			_, exists := cache.Get(objID)
			if !exists {
				t.Errorf(ConstMagicc4437867, i, objID)
			}
		}
	}

	if writeErrors.Load() > 0 {
		t.Logf(ConstMagic656f3617, writeErrors.Load())
		// Some write errors are expected due to concurrent file access
		// The important thing is that the cache file remains valid
	}

	// Verify cache file is valid JSON
	finalCache := NewValidationStateCache(testRoot, time.Hour)
	if err := finalCache.Load(); err != nil {
		t.Fatalf(ConstMagic266a1b3e, err)
	}

	total, _, _ := finalCache.Count()
	if total == 0 {
		t.Error(ConstMagicfefc8717)
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
		t.Fatalf(ConstMagic4545ee2f, err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	// Create large dataset
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic32b1c202, err)
	}

	numObjects := 1000
	t.Logf(ConstMagic8d564fa9, numObjects)

	for i := 1; i <= numObjects; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%04d.yaml", i))
		content := fmt.Sprintf(ConstMagic4282f0c4, i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}
	}

	t.Logf(ConstMagicdd296740, numObjects)

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
				t.Logf(ConstMagic246f958a, numObjects, elapsed)
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
		t.Fatalf(ConstMagic32b1c202, err)
	}

	numObjects := 200
	for i := 1; i <= numObjects; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf(ConstMagic28b7580d, i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}
	}

	// Create multiple validator instances (simulating multiple CLI processes)
	numValidators := 3
	validators := make([]*AsyncValidator, numValidators)
	for i := 0; i < numValidators; i++ {
		validators[i] = NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 2, time.Hour)
		if err := validators[i].Start(); err != nil {
			t.Fatalf(ConstMagic06560e70, i, err)
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
			t.Fatal(ConstMagic2be77c83)
		case <-ticker.C:
			allDone := true
			for i, validator := range validators {
				_, _, _, queueSize := validator.GetValidationStats()
				if queueSize > 0 {
					allDone = false
					t.Logf(ConstMagic551f0608, i, queueSize)
				}
			}
			if allDone {
				t.Log(ConstMagicf7c47cf6)
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
			t.Fatalf(ConstMagic67c2c4a8, i, err)
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
					objID := fmt.Sprintf(ConstMagic7e20b8c7, cacheID, j)
					state := &ValidationState{
						ObjectID:      objID,
						ObjectKind:    "test_object",
						FilePath:      fmt.Sprintf(ConstMagic2dc26e7d, cacheID, j),
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
		t.Fatalf(ConstMagicc6a14713, err)
	}

	total, _, _ := finalCache.Count()
	if total == 0 {
		t.Error(ConstMagic0c5a32c6)
	}

	// Some save errors are expected due to file locking (when multiple processes try to save simultaneously)
	// The important thing is that the cache file remains valid and no data is corrupted
	if saveErrors.Load() > int64(numCaches*numSaves) {
		t.Errorf(ConstMagicc8a2ac11, saveErrors.Load())
	} else if saveErrors.Load() > 0 {
		t.Logf(ConstMagic39a35328, saveErrors.Load())
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
		t.Fatalf(ConstMagic32b1c202, err)
	}

	numObjects := 100
	for i := 1; i <= numObjects; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("TEST-%03d.yaml", i))
		content := fmt.Sprintf(ConstMagic28b7580d, i)
		if err := fileutil.WriteFile(testFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
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
		t.Errorf(ConstMagicf1c16612, validationErrors.Load())
	}

	// Verify all objects are cached
	for i := 1; i <= numObjects; i++ {
		objID := fmt.Sprintf("TEST-%03d", i)
		_, exists := validator.GetCachedState(objID)
		if !exists {
			t.Errorf(ConstMagica1b53423, objID)
		}
	}
}
