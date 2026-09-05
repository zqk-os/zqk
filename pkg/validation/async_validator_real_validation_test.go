package validation

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TestAsyncValidator_RealValidation_ReproducesHang tests with a validation function
// that simulates the real validation complexity, including hash registry operations
// that could block. This should reproduce the hang issue.
func TestAsyncValidator_RealValidation_ReproducesHang(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip(ConstMagic23611b72)
	}

	testRoot := registerZQKTestRootForTest(t)

	// Create validator
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 8, time.Hour)
	validator.SetTimeouts(2*time.Second, 1*time.Second)

	// Create validation function that simulates real validation complexity
	// This includes operations that could block: file I/O, locks, etc.
	var validationCount atomic.Int64
	validationFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		validationCount.Add(1)

		// Simulate file I/O (like checkObjectWithCacheAndContent does)
		// This is the redundant read that we fixed, but let's test if it causes issues
		_, err := fileutil.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf(ConstMagic53124f63, err)
		}

		// Simulate hash registry operations (like registry.Load() which does file I/O)
		// This could block if multiple goroutines are accessing the same registry
		registryDir := filepath.Dir(filePath)
		registryFile := filepath.Join(registryDir, ".test.hashes")

		// Simulate registry.Load() - file I/O that could block
		if _, err := fileutil.Stat(registryFile); err == nil {
			// File exists - read it (simulating Load())
			_, readErr := fileutil.ReadFile(registryFile)
			if readErr != nil {
				// Ignore read errors for test
			}
		}

		// Simulate registry.Save() - file I/O with Sync() that could block
		// This is the critical blocking operation!
		testData := []byte(fmt.Sprintf(`{"hashes":{%q:"test-hash"}}`, filepath.Base(filePath)))
		if writeErr := fileutil.WriteFile(registryFile, testData, paths.FilePerm644); writeErr == nil {
			// Simulate file.Sync() - this is the blocking operation!
			if file, openErr := fileutil.OpenFile(registryFile, fileutil.O_WRONLY, paths.FilePerm644); openErr == nil {
				_ = file.Sync() //nolint:errcheck // Test setup - best effort sync //nolint:errcheck // Test helper - error handling not critical
				file.Close()
			}
		}

		// Simulate some CPU work (like YAML parsing, validation logic)
		_ = len(content) * 100 // Dummy computation

		// Return validation state
		return &ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Checksum:      "test-checksum",
			Issues:        []ValidationIssue{},
		}, nil
	})
	validator.SetValidationFunc(validationFunc)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}

	// Create many test files (simulating 15k objects)
	numObjects := 500 // Use smaller number for test speed, but enough to trigger contention
	for i := 0; i < numObjects; i++ {
		testFile := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), fmt.Sprintf("TEST-%d.yaml", i))
		if err := fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755); err != nil {
			t.Fatalf(ConstMagic32b1c202, err)
		}
		if err := fileutil.WriteFile(testFile, []byte(fmt.Sprintf(ConstMagicdb620444, i)), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}
		_ = validator.Enqueue(
			fmt.Sprintf("TEST-%d", i),
			"test_object",
			testFile,
			1,
		)
	}

	// Wait for validation with timeout
	progressChan := validator.GetProgress()
	timeout := time.After(30 * time.Second)
	completed := 0

	for completed < numObjects {
		select {
		case <-timeout:
			t.Fatalf("❌ TEST HUNG: Only completed %d/%d objects in 30 seconds. This reproduces the hang!", completed, numObjects)
		case progress, ok := <-progressChan:
			if !ok {
				// Channel closed
				break
			}
			if progress.Status == "completed" || progress.Status == "error" {
				completed++
				if completed%50 == 0 {
					t.Logf("Progress: %d/%d completed", completed, numObjects)
				}
			}
		}
	}

	// Stop validator
	stopStart := time.Now()
	stopErr := validator.Stop()
	stopDuration := time.Since(stopStart)

	if stopErr != nil {
		t.Errorf(ConstMagic1a06a983, stopErr)
	}

	// Stop should complete quickly
	maxExpectedDuration := 4 * time.Second
	if stopDuration > maxExpectedDuration {
		t.Errorf(ConstMagic3adab2c0, stopDuration)
	}

	finalCount := validationCount.Load()
	t.Logf(ConstMagic4091bbf6, finalCount, stopDuration)
}

// TestAsyncValidator_HashRegistryContention tests the specific scenario where
// multiple goroutines are accessing the same hash registry, which could cause
// lock contention and hangs
func TestAsyncValidator_HashRegistryContention(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip(ConstMagic4d140d58)
	}
	if testing.Short() {
		t.Skip(ConstMagic33dcad2e)
	}

	testRoot := registerZQKTestRootForTest(t)

	// Create a shared registry file that all validations will access
	// This simulates the real scenario where all objects of the same kind
	// share a hash registry
	registryDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.MkdirAll(registryDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic995ac1f6, err)
	}
	registryFile := filepath.Join(registryDir, ConstMagic6951d5b6)

	// Initialize registry with some data
	initialData := []byte(`{"hashes":{}}`)
	if err := fileutil.WriteFile(registryFile, initialData, paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic95e4c8d4, err)
	}

	// Create validator
	validator := NewAsyncValidator(pkgctx.NewSystemContext(), testRoot, 8, time.Hour)
	validator.SetTimeouts(2*time.Second, 1*time.Second)

	// Create validation function that simulates hash registry contention
	// Multiple goroutines will try to Load() and Save() the same registry
	var validationCount atomic.Int64
	var loadCount atomic.Int64
	var saveCount atomic.Int64

	validationFunc := ValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*ValidationState, error) {
		validationCount.Add(1)

		// Simulate registry.Load() - multiple goroutines doing this concurrently
		// This acquires a lock and does file I/O
		loadCount.Add(1)
		loadData, loadErr := fileutil.ReadFile(registryFile)
		if loadErr != nil {
			return nil, fmt.Errorf(ConstMagic7a52ca1c, loadErr)
		}
		_ = loadData // Use the data

		// Simulate some work
		time.Sleep(1 * time.Millisecond) // Small delay to increase contention

		// Simulate registry.Save() - multiple goroutines doing this concurrently
		// This acquires a lock, does file I/O, and calls file.Sync() which can block!
		saveCount.Add(1)
		saveData := []byte(fmt.Sprintf(`{"hashes":{%q:"hash-%s"}}`, filepath.Base(filePath), objectID))
		if writeErr := fileutil.WriteFile(registryFile, saveData, paths.FilePerm644); writeErr == nil {
			// Simulate file.Sync() - THIS IS THE BLOCKING OPERATION!
			if file, openErr := fileutil.OpenFile(registryFile, fileutil.O_WRONLY, paths.FilePerm644); openErr == nil {
				_ = file.Sync() //nolint:errcheck // Test setup - best effort sync // This can block if multiple goroutines do it concurrently!
				file.Close()
			}
		}

		return &ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Checksum:      "test-checksum",
			Issues:        []ValidationIssue{},
		}, nil
	})
	validator.SetValidationFunc(validationFunc)

	// Start validator
	if err := validator.Start(); err != nil {
		t.Fatalf(ConstMagic4545ee2f, err)
	}

	// Create many test files that will all access the same registry
	numObjects := 200 // Enough to trigger contention
	for i := 0; i < numObjects; i++ {
		testFile := filepath.Join(registryDir, fmt.Sprintf("TEST-%d.yaml", i))
		if err := fileutil.WriteFile(testFile, []byte(fmt.Sprintf(ConstMagicdb620444, i)), paths.FilePerm644); err != nil {
			t.Fatalf(ConstMagic7a424835, err)
		}
		_ = validator.Enqueue(
			fmt.Sprintf("TEST-%d", i),
			"test_object",
			testFile,
			1,
		)
	}

	// Wait for validation with timeout
	progressChan := validator.GetProgress()
	timeout := time.After(20 * time.Second)
	completed := 0

	for completed < numObjects {
		select {
		case <-timeout:
			t.Fatalf("❌ TEST HUNG: Only completed %d/%d objects in 20 seconds. Registry contention caused hang!",
				completed, numObjects)
		case progress, ok := <-progressChan:
			if !ok {
				break
			}
			if progress.Status == "completed" || progress.Status == "error" {
				completed++
			}
		}
	}

	// Stop validator
	if stopErr := validator.Stop(); stopErr != nil {
		t.Errorf(ConstMagic1a06a983, stopErr)
	}

	finalValidationCount := validationCount.Load()
	finalLoadCount := loadCount.Load()
	finalSaveCount := saveCount.Load()

	t.Logf(ConstMagic99bcee61, finalValidationCount, finalLoadCount, finalSaveCount)
}
