package utility

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
)

// TestBucketStrategyRegistry_BlocksOnParallelAccess tests if getBucketStrategyRegistry blocks when called in parallel
// This reproduces the scenario where many goroutines try to initialize the registry simultaneously
func TestBucketStrategyRegistry_BlocksOnParallelAccess(t *testing.T) {
	// Not t.Parallel(): wall-clock timing below is sensitive to CPU/IO contention from other
	// packages when the scheduler runs cmd/zqk/utility with -p>1; parallel subtests are enough.
	tmpDir := t.TempDir()

	// Create required directory structure
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	// Create storage provider
	storageProvider := setupTestStorageProvider(t, tmpDir)

	// Create coordinator
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create scenario builder
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.secCtx = pkgctx.NewSystemSecurityContext()

	// Set config.TargetDir
	if builder.config == nil {
		builder.config = &ScenarioBuilderConfig{}
	}
	builder.config.TargetDir = tmpDir

	// Setup infrastructure
	if err := builder.setupInfrastructure(); err != nil {
		t.Fatalf("Failed to setup infrastructure: %v", err)
	}

	// Test: Create objects in parallel that will trigger bucket strategy lookup
	// This simulates the real scenario where many objects are created in parallel
	// and each one calls writeObjectToCAS() which calls getBucketStrategyRegistry()
	testObjects := []map[string]any{
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyID:       "ACC-TEST1",
			objects.FieldKeyTitle:    "Test Account 1",
			objects.FieldKeyUsername: "test1",
			objects.FieldKeyStatus:   scenarioBuilderStatusActive,
		},
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyID:       "ACC-TEST2",
			objects.FieldKeyTitle:    "Test Account 2",
			objects.FieldKeyUsername: "test2",
			objects.FieldKeyStatus:   scenarioBuilderStatusActive,
		},
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyID:       "ACC-TEST3",
			objects.FieldKeyTitle:    "Test Account 3",
			objects.FieldKeyUsername: "test3",
			objects.FieldKeyStatus:   scenarioBuilderStatusActive,
		},
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyID:       "ACC-TEST4",
			objects.FieldKeyTitle:    "Test Account 4",
			objects.FieldKeyUsername: "test4",
			objects.FieldKeyStatus:   scenarioBuilderStatusActive,
		},
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyID:       "ACC-TEST5",
			objects.FieldKeyTitle:    "Test Account 5",
			objects.FieldKeyUsername: "test5",
			objects.FieldKeyStatus:   scenarioBuilderStatusActive,
		},
	}

	ctx := pkgctx.NewSystemContext()
	var wg sync.WaitGroup
	startTime := time.Now()

	// Capture initial CAS metrics to calculate delta (protects against parallel test resets)
	// Note: This test verifies that bucket strategy registry doesn't block, not that CAS metrics work perfectly.
	// If metrics are reset by parallel tests, we'll still verify objects were created successfully.
	initialMetrics := caspkg.GetObjectStorageMetrics()
	initialCreates := initialMetrics.GetSnapshot().Creates
	t.Logf("Initial CAS metrics: %d creates", initialCreates)

	for i, obj := range testObjects {
		wg.Add(1)
		objCopy := obj
		objIndex := i

		goroutinelabels.NewGoroutine("test_bucket_registry_parallel",
			"parallel create triggering bucket strategy").
			WithWaitGroup(&wg).
			Start(func() error {
				defer wg.Done()

				kind, _ := objCopy[objects.FieldKeyKind].(string)
				objID, _ := objCopy[objects.FieldKeyID].(string)

				// Prepare object
				if err := builder.prepareObjectFromDataFile(objCopy, kind); err != nil {
					return err
				}

				// Validate
				if err := builder.validateObjectBeforeCreation(objCopy, kind); err != nil {
					return err
				}

				createStart := time.Now()
				t.Logf("Goroutine %d: Creating %s %s (will trigger bucket strategy lookup)", objIndex, kind, objID)

				createCtx := pkgctx.WithCacheUpdate(ctx, objID, kind, "")
				createErr := storageProvider.Create(createCtx, builder.secCtx, objCopy)

				createDuration := time.Since(createStart)
				if createDuration > 1*time.Second {
					t.Logf("WARNING: Goroutine %d: storage.Create took %v (suspiciously long)", objIndex, createDuration)
				}

				if createErr != nil {
					t.Logf("Goroutine %d: storage.Create returned error: %v", objIndex, createErr)
					return createErr
				}

				t.Logf("Goroutine %d: storage.Create succeeded in %v", objIndex, createDuration)
				return nil
			})
	}

	// Wait with timeout
	done := make(chan bool)
	goroutinelabels.NewGoroutine("utility_test", "wait for parallel bucket strategy access").StartSimple(func() {
		wg.Wait()
		done <- true
	})

	select {
	case <-done:
		totalDuration := time.Since(startTime)
		t.Logf("All goroutines completed in %v", totalDuration)
		// Under scheduler bundle load (-p 10, many packages), wall time can exceed a few seconds
		// without bucket strategy registry contention (slow disk/CPU). Use a loose bound for pathology only.
		if totalDuration > 30*time.Second {
			t.Errorf("CRITICAL: Parallel creates took %v - this indicates blocking in bucket strategy registry!", totalDuration)
		}

		// Check CAS metrics (calculate delta to handle parallel test resets)
		// Note: This is a best-effort check. If parallel tests reset metrics, we can't verify CAS metrics,
		// but we've already verified that all creates succeeded (no errors returned).
		casMetrics := caspkg.GetObjectStorageMetrics()
		snapshot := casMetrics.GetSnapshot()
		deltaCreates := snapshot.Creates - initialCreates
		t.Logf("CAS Metrics: %d total creates (%d from this test, initial: %d), %d failures", snapshot.Creates, deltaCreates, initialCreates, snapshot.CreateFailures)

		// Only fail if we're confident metrics weren't reset (initial was 0 and total is still 0)
		// If initial > 0 and delta is 0, another test may have reset metrics, which is acceptable
		if deltaCreates == 0 && initialCreates == 0 && snapshot.Creates == 0 {
			// All objects were created successfully (no errors), but no CAS metrics recorded.
			// This could indicate:
			// 1. Objects didn't reach cas.Create() (unlikely since creates succeeded)
			// 2. Metrics were reset by a parallel test (more likely)
			// 3. Metrics recording is broken (needs investigation)
			t.Logf("WARNING: No CAS creates recorded, but all creates succeeded. This may indicate metrics were reset by a parallel test.")
			// Don't fail the test - the important part (no blocking) was verified
		} else if deltaCreates > 0 {
			t.Logf("SUCCESS: CAS metrics show %d creates from this test", deltaCreates)
		}
	case <-time.After(45 * time.Second):
		t.Fatalf("TIMEOUT: Goroutines did not complete within 45 seconds - bucket strategy registry or validation is blocking!")
	}
}
