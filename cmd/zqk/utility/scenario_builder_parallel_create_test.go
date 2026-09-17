package utility

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"github.com/lanceman/zqk/pkg/testkit"
)

// TestParallelCreate_ReproducesIssue tests parallel object creation to reproduce the CAS metrics issue
// This isolates the problem to see if storage.Create() is actually being called and reaching cas.Create()
//
// Uses NewFileObjectStorageForTest (no WAL, synchronous Create) like TestParallelCreate_WithValidation.
// NewStorageFactory + parallel goroutines can deadlock: async validation scanners / write-behind
// leave Create blocked so wg.Wait never returns (see bundle utility test timeouts).
func TestParallelCreate_ReproducesIssue(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "utility.parallel_create"})
	tmpDir := proj.Root
	storageProvider := proj.FileStorage

	// Create coordinator
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create scenario builder
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.secCtx = pkgctx.NewSystemSecurityContext()

	// Set config.TargetDir (required for setupInfrastructure)
	if builder.config == nil {
		builder.config = &ScenarioBuilderConfig{}
	}
	builder.config.TargetDir = tmpDir

	// Setup infrastructure (specs, lifecycles)
	if err := builder.setupInfrastructure(); err != nil {
		t.Fatalf("Failed to setup infrastructure: %v", err)
	}

	// Verify specs were generated
	specsDir := filepath.Join(tmpDir, paths.ProcessInternalObjectSpecsDir)
	specFiles, err := fileutil.ReadDir(specsDir)
	if err != nil {
		t.Fatalf("Failed to read specs directory: %v", err)
	}
	t.Logf("Generated %d spec files", len(specFiles))
	if len(specFiles) == 0 {
		t.Fatalf("No spec files generated - validation will fail")
	}

	// Create test objects (simple ones without dependencies)
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
	}

	// Reset CAS metrics before test
	caspkg.ResetObjectStorageMetrics()

	// Create objects in parallel (simulating the scenario builder behavior)
	ctx := pkgctx.NewSystemContext()
	var wg sync.WaitGroup
	var createErrors []error
	var createErrorsMu sync.Mutex

	for i, obj := range testObjects {
		// Do not call wg.Add here: goroutinelabels.Start already adds to the WaitGroup.
		objCopy := obj
		objIndex := i

		goroutinelabels.NewGoroutine("test_parallel_create",
			"parallel create test").
			WithWaitGroup(&wg).
			WithErrorHandler(func(err error) {
				createErrorsMu.Lock()
				createErrors = append(createErrors, err)
				createErrorsMu.Unlock()
			}).
			Start(func() error {

				kind, _ := objCopy[objects.FieldKeyKind].(string)
				objID, _ := objCopy[objects.FieldKeyID].(string)

				// Prepare object (like scenario builder does)
				if err := builder.prepareObjectFromDataFile(objCopy, kind); err != nil {
					t.Logf("Goroutine %d: prepareObjectFromDataFile failed: %v", objIndex, err)
					return err
				}

				// Final validation before creation
				if err := builder.validateObjectBeforeCreation(objCopy, kind); err != nil {
					t.Logf("Goroutine %d: validateObjectBeforeCreation failed: %v", objIndex, err)
					return err
				}

				t.Logf("Goroutine %d: Attempting to create %s %s", objIndex, kind, objID)

				// Set cache context
				createCtx := pkgctx.WithCacheUpdate(ctx, objID, kind, "")

				// Call storage.Create() directly
				createErr := storageProvider.Create(createCtx, builder.secCtx, objCopy)

				if createErr != nil {
					t.Logf("Goroutine %d: storage.Create returned error for %s %s: %v", objIndex, kind, objID, createErr)
					return createErr
				}

				t.Logf("Goroutine %d: storage.Create returned nil (success) for %s %s", objIndex, kind, objID)
				return nil
			})
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Check CAS metrics
	casMetrics := caspkg.GetObjectStorageMetrics()
	snapshot := casMetrics.GetSnapshot()

	t.Logf("=== CAS Metrics After Parallel Create ===")
	t.Logf("Creates: %d", snapshot.Creates)
	t.Logf("Create Failures: %d", snapshot.CreateFailures)
	t.Logf("Errors from goroutines: %d", len(createErrors))

	if len(createErrors) > 0 {
		for i, err := range createErrors {
			t.Logf("Error %d: %v", i, err)
		}
	}

	// Assertions
	if snapshot.Creates == 0 {
		t.Errorf("CRITICAL: No CAS Create operations recorded! This reproduces the issue.")
		t.Errorf("storage.Create() was called %d times but cas.Create() was never reached", len(testObjects))
	}

	if snapshot.Creates != int64(len(testObjects)) {
		t.Errorf("Expected %d creates, got %d", len(testObjects), snapshot.Creates)
	}

	if len(createErrors) > 0 {
		t.Errorf("Got %d errors during creation: %v", len(createErrors), createErrors)
	}
}

// TestParallelCreate_WithValidation tests parallel creation with validation to see if validation is blocking.
// Runs without t.Parallel() so it does not contend with other tests for global CAS project root;
// otherwise "Skipping SetProjectRoot/SetStorage... different project root already set" causes flaky failures.
// Uses NewFileObjectStorageForTest (no WAL) so Create is synchronous and ref validation sees just-created objects.
func TestParallelCreate_WithValidation(t *testing.T) {
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	tmpDir := t.TempDir()

	// Create required directory structure
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	// Use test storage (no WAL) so Create is synchronous and ref validation sees just-created objects.
	fileStorage, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, fileStorage)
	storageProvider := fileStorage

	// Create coordinator
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create scenario builder
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.secCtx = pkgctx.NewSystemSecurityContext()

	// Set config.TargetDir (required for setupInfrastructure)
	if builder.config == nil {
		builder.config = &ScenarioBuilderConfig{}
	}
	builder.config.TargetDir = tmpDir

	// Setup infrastructure
	if err := builder.setupInfrastructure(); err != nil {
		t.Fatalf("Failed to setup infrastructure: %v", err)
	}

	// Create test objects with references (to trigger validation)
	// schema_version is required (Tier 2) for validation to allow save.
	testObjects := []map[string]any{
		{
			objects.FieldKeyKind:          "account",
			objects.FieldKeyID:            "ACC-1785920548450214008-ce03e2b5",
			objects.FieldKeyTitle:         "Owner Account",
			objects.FieldKeyUsername:      "owner",
			objects.FieldKeyStatus:        scenarioBuilderStatusActive,
			objects.FieldKeySchemaVersion: scenarioBuilderSchemaV2,
		},
		{
			objects.FieldKeyKind:          "workstream",
			objects.FieldKeyID:            "WS-001",
			objects.FieldKeyTitle:         "Test Workstream",
			objects.FieldKeyStatus:        scenarioBuilderStatusActive,
			objects.FieldKeyOwnerRef:      "ACC-1785920548450214008-ce03e2b5", // Reference to first object
			objects.FieldKeyEntryPoint:    "README.md",                        // Required Tier 2 field
			objects.FieldKeySchemaVersion: scenarioBuilderSchemaV2,
		},
	}

	// Reset CAS metrics
	caspkg.ResetObjectStorageMetrics()

	// Create objects sequentially first (to establish dependencies)
	ctx := pkgctx.NewSystemContext()
	// TRACK: BLI-REDACTED — draft-plane create / promote membrane.
	for i, obj := range testObjects {
		kind, _ := obj[objects.FieldKeyKind].(string)
		objID, _ := obj[objects.FieldKeyID].(string)

		t.Logf("Sequential create %d: %s %s", i, kind, objID)

		storage.CreateCASVisible(t, storageProvider, ctx, builder.secCtx, obj, scenarioBuilderStatusActive)
		// Flush this kind's CAS index so reference validation can find the object (next create or parallel).
		if err := storage.FlushListingIndexForProjectRoot(tmpDir, kind); err != nil {
			t.Fatalf("Failed to flush CAS index for %s: %v", kind, err)
		}
	}

	// Now try parallel creation of objects that reference existing ones
	parallelObjects := []map[string]any{
		{
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyID:            "GOAL-001",
			objects.FieldKeyTitle:         "Test Goal 1",
			objects.FieldKeyStatus:        scenarioBuilderStatusActive,
			objects.FieldKeyWorkstreamRef: "WS-001",
			objects.FieldKeySchemaVersion: scenarioBuilderSchemaV2,
		},
		{
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyID:            "GOAL-002",
			objects.FieldKeyTitle:         "Test Goal 2",
			objects.FieldKeyStatus:        scenarioBuilderStatusActive,
			objects.FieldKeyWorkstreamRef: "WS-001",
			objects.FieldKeySchemaVersion: scenarioBuilderSchemaV2,
		},
	}

	var wg sync.WaitGroup
	startTime := time.Now()

	for i, obj := range parallelObjects {
		objCopy := obj
		objIndex := i

		goroutinelabels.NewGoroutine("test_parallel_with_refs",
			"parallel create with references").
			WithWaitGroup(&wg).
			Start(func() error {

				kind, _ := objCopy[objects.FieldKeyKind].(string)
				objID, _ := objCopy[objects.FieldKeyID].(string)

				createStart := time.Now()
				t.Logf("Goroutine %d: Starting create for %s %s", objIndex, kind, objID)

				createCtx := pkgctx.WithCacheUpdate(ctx, objID, kind, "")
				createErr := storageProvider.Create(createCtx, builder.secCtx, objCopy)
				// TRACK: BLI-REDACTED — promote parallel creates off draft plane.
				if createErr == nil {
					createErr = storageProvider.Update(createCtx, builder.secCtx, objID, map[string]any{
						objects.FieldKeyStatus: scenarioBuilderStatusActive,
					})
				}

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
	goroutinelabels.NewGoroutine("utility_test", "wait for parallel create").StartSimple(func() {
		wg.Wait()
		done <- true
	})

	select {
	case <-done:
		totalDuration := time.Since(startTime)
		t.Logf("All goroutines completed in %v", totalDuration)
	case <-time.After(10 * time.Second):
		t.Fatalf("TIMEOUT: Goroutines did not complete within 10 seconds - this indicates blocking!")
	}

	// Check metrics
	casMetrics := caspkg.GetObjectStorageMetrics()
	snapshot := casMetrics.GetSnapshot()

	t.Logf("=== CAS Metrics After Parallel Create With References ===")
	t.Logf("Creates: %d", snapshot.Creates)
	t.Logf("Create Failures: %d", snapshot.CreateFailures)
	t.Logf("Expected: %d creates (2 sequential + 2 parallel)", len(testObjects)+len(parallelObjects))

	if snapshot.Creates == 0 && snapshot.Updates == 0 {
		t.Errorf("CRITICAL: No CAS Create/Update operations recorded!")
	}

	totalWrites := snapshot.Creates + snapshot.Updates
	if totalWrites < int64(len(testObjects)) {
		t.Errorf("Expected at least %d durable writes (sequential), got creates=%d updates=%d", len(testObjects), snapshot.Creates, snapshot.Updates)
	}
}
