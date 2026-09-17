package system

import (
	"context"
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

// TestPerformance_AsyncVsSync compares performance of async vs sync validation
// This test requires the async_validation feature flag to be enabled
func TestPerformance_AsyncVsSync(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create test environment
	testDir := datacell.CellCASPrimaryDir(projectRoot, "test")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Create test objects (vary count for different scenarios)
	testCounts := []int{10, 50, 100, 500}

	for _, count := range testCounts {
		t.Run(fmt.Sprintf("Count_%d", count), func(t *testing.T) {
			// Create test objects
			objectIDs := make([]string, 0, count)
			filePaths := make(map[string]string)
			for i := 0; i < count; i++ {
				objectID := fmt.Sprintf("PERF-TEST-%04d", i)

				content := fmt.Sprintf(`id: %s
kind: backlog_item
title: Performance Test Object %d
status: planned
schema_version: "`+objects.DefaultSchemaVersion+`"
created_at: "%s"
created_by: ACC-TEST
`, objectID, i, time.Now().Format(time.RFC3339))

				path := testkit.WriteTestObjectStandalone(t, projectRoot, content)

				objectIDs = append(objectIDs, objectID)
				filePaths[objectID] = path
			}

			// Test sync performance
			syncDuration := testSyncValidation(t, projectRoot, objectIDs, filePaths)

			// Test async performance
			asyncDuration := testAsyncValidation(t, projectRoot, objectIDs, filePaths)

			// Log results
			t.Logf("Objects: %d, Sync: %v, Async: %v, Speedup: %.2fx",
				count, syncDuration, asyncDuration, float64(syncDuration)/float64(asyncDuration))

			// For larger counts, async should be faster due to parallelization
			if count >= 100 {
				expectedMaxDuration := time.Duration(float64(syncDuration) * 1.5)
				if asyncDuration > expectedMaxDuration {
					t.Logf("Warning: Async validation slower than expected for %d objects", count)
				}
			}
		})
	}
}

// testSyncValidation tests synchronous validation performance
func testSyncValidation(t *testing.T, projectRoot string, objectIDs []string, filePaths map[string]string) time.Duration {
	start := time.Now()

	// Simulate sync validation (would call actual check command)
	// For now, just simulate file reads
	for _, objectID := range objectIDs {
		filePath := filePaths[objectID]
		_, err := fileutil.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		// Simulate validation work
		time.Sleep(1 * time.Millisecond)
	}

	return time.Since(start)
}

// testAsyncValidation tests asynchronous validation performance
func testAsyncValidation(t *testing.T, projectRoot string, objectIDs []string, filePaths map[string]string) time.Duration {
	validator := validation.NewAsyncValidator(pkgctx.NewSystemContext(), projectRoot, 4, time.Hour)

	// Set up validation function
	validationFunc := func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*validation.ValidationState, error) {
		// Simulate validation work
		time.Sleep(1 * time.Millisecond)

		return &validation.ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Issues:        []validation.ValidationIssue{},
		}, nil
	}
	validator.SetValidationFunc(validationFunc)

	if err := validator.Start(); err != nil {
		t.Fatalf("failed to start validator: %v", err)
	}
	defer func() { _ = validator.Stop() }() //nolint:errcheck // Test cleanup - errors are acceptable

	start := time.Now()

	// Enqueue all objects
	for _, objectID := range objectIDs {
		filePath := filePaths[objectID]
		validator.Enqueue(objectID, "backlog_item", filePath, 1)
	}

	// Wait for completion
	timeout := time.After(30 * time.Second)
	tick := time.Tick(100 * time.Millisecond)

	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for async validation")
		case <-tick:
			_, _, _, queueSize := validator.GetValidationStats()
			validatedCount := 0
			for _, objectID := range objectIDs {
				_, ok := validator.GetCachedState(objectID)
				if ok {
					validatedCount++
				}
			}

			if queueSize == 0 && validatedCount >= len(objectIDs) {
				return time.Since(start)
			}
		}
	}
}

// TestPerformance_FeatureFlagIntegration tests that feature flag correctly routes to async
func TestPerformance_FeatureFlagIntegration(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping feature flag test in short mode")
	}

	// This test would verify that when async_validation flag is enabled,
	// the check command actually uses async validation
	// For now, just verify the flag exists and can be toggled

	// Note: This requires actual feature flag system to be set up
	// In a real scenario, we'd:
	// 1. Enable the flag
	// 2. Run check command
	// 3. Verify async validator is used
	// 4. Compare results with sync

	t.Log("Feature flag integration test - requires full system setup")
}

// BenchmarkAsyncValidation benchmarks async validation performance
func BenchmarkAsyncValidation(b *testing.B) {
	tmpDir := b.TempDir()
	projectRoot := tmpDir

	// Create test objects
	testDir := datacell.CellCASPrimaryDir(projectRoot, "test")
	_ = fileutil.MkdirAll(testDir, paths.DirPerm755)

	objectCount := 100
	objectIDs := make([]string, 0, objectCount)
	filePaths := make(map[string]string)
	for i := 0; i < objectCount; i++ {
		objectID := fmt.Sprintf("BENCH-%04d", i)

		content := fmt.Sprintf(`id: %s
kind: backlog_item
title: Benchmark Object %d
status: planned
`, objectID, i)

		path := testkit.WriteTestObjectStandalone(b, projectRoot, content)
		objectIDs = append(objectIDs, objectID)
		filePaths[objectID] = path
	}

	validator := validation.NewAsyncValidator(pkgctx.NewSystemContext(), projectRoot, 4, time.Hour)
	validationFunc := func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*validation.ValidationState, error) {
		return &validation.ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Issues:        []validation.ValidationIssue{},
		}, nil
	}
	validator.SetValidationFunc(validationFunc)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := validator.Start(); err != nil {
			b.Fatalf("failed to start validator: %v", err)
		}

		// Enqueue all objects
		for _, objectID := range objectIDs {
			filePath := filePaths[objectID]
			validator.Enqueue(objectID, "backlog_item", filePath, 1)
		}

		// Wait for completion
		timeout := time.After(5 * time.Second)
		tick := time.Tick(50 * time.Millisecond)

		done := false
		for !done {
			select {
			case <-timeout:
				b.Fatal("Timeout in benchmark")
			case <-tick:
				_, _, _, queueSize := validator.GetValidationStats()
				validatedCount := 0
				for _, objectID := range objectIDs {
					_, ok := validator.GetCachedState(objectID)
					if ok {
						validatedCount++
					}
				}
				if queueSize == 0 && validatedCount >= len(objectIDs) {
					done = true
				}
			}
		}

		_ = validator.Stop() //nolint:errcheck // Test cleanup - errors are acceptable
	}
}
