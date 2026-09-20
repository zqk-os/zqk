package utility

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"

	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"
)

// cleanupStorageTestRoot registers the standard temp-project teardown pipeline (WAL, listing index,
// strip/scrub) so t.TempDir cleanup does not flake on leftover .zqk / process artifacts.
func cleanupStorageTestRoot(t *testing.T, tmpDir string, storageProvider storagepkg.ObjectStorageProvider) {
	t.Helper()
	storagepkg.SetCacheOperationHandler(func(*pkgctx.CacheContext) error { return nil })
	t.Cleanup(func() {
		fs, ok := nildecode.DecodeNonNilPayload[*storagepkg.FileObjectStorage](storageProvider)
		if !ok {
			return
		}
		if err := storagepkg.RunProjectTestTeardown(storagepkg.TempProjectTeardown(tmpDir, fs)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
}

// TestWaitGroupPanic_Reproduce tests the WaitGroup panic that occurs when objects already exist
// This test reproduces the exact scenario: creating objects that already exist triggers audit events
// which may use WaitGroups internally, causing a "negative WaitGroup counter" panic
func TestWaitGroupPanic_Reproduce(t *testing.T) {
	t.Parallel()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-waitgroup-reproduce-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	// Create storage factory
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()
	cleanupStorageTestRoot(t, tmpDir, storageProvider)

	// Create coordinator
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create scenario builder
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.secCtx = pkgctx.NewSystemSecurityContext()
	if builder.config == nil {
		builder.config = &ScenarioBuilderConfig{}
	}
	builder.config.TargetDir = tmpDir

	// Setup infrastructure (specs, lifecycles)
	if err := builder.setupInfrastructure(); err != nil {
		t.Fatalf("Failed to setup infrastructure: %v", err)
	}

	// Create account object first time (should succeed)
	accountObj := map[string]any{
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "test-user",
		objects.FieldKeyTitle:         "Test Account",
		objects.FieldKeyStatus:        scenarioBuilderStatusActive,
		objects.FieldKeySchemaVersion: scenarioBuilderSchemaV2,
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	createCtx := pkgctx.WithCacheUpdate(pkgctx.NewSystemContext(), "ACC-1785920548450214015-3df55bd1", "account", "")

	// First creation - should succeed
	err = storageProvider.Create(createCtx, secCtx, accountObj)
	if err != nil {
		t.Fatalf("First account creation should succeed: %v", err)
	}

	// Now try to create it again (should fail with "already exists")
	// This is the scenario that triggers the WaitGroup panic
	var wg sync.WaitGroup
	panicOccurred := false
	panicMu := &sync.Mutex{}

	goroutinelabels.NewGoroutine("test_waitgroup_panic", "testing WaitGroup panic on duplicate create").
		WithWaitGroup(&wg).
		WithPanicHandler(func(r any) {
			panicMu.Lock()
			panicOccurred = true
			panicMu.Unlock()
			t.Logf("Panic caught: %v", r)
		}).
		StartSimple(func() {

			// Track validation (like scenario builder does)
			var activeValidations sync.WaitGroup
			activeValidations.Add(1)
			defer func() {
				defer func() {
					if r := recover(); r != nil {
						panicMu.Lock()
						panicOccurred = true
						panicMu.Unlock()
						t.Logf("Recovered WaitGroup panic in defer: %v", r)
					}
				}()
				activeValidations.Done()
			}()

			// Try to create duplicate - this should trigger audit event creation
			// which may use WaitGroups internally
			duplicateCtx := pkgctx.WithCacheUpdate(pkgctx.NewSystemContext(), "ACC-1785920548450214015-3df55bd1", "account", "")
			err := storageProvider.Create(duplicateCtx, builder.secCtx, accountObj)
			if err != nil {
				// Expected - object already exists
				t.Logf("Expected error (object already exists): %v", err)
			}
		})

	// Wait for goroutine to complete
	wg.Wait()

	// Check if panic occurred
	panicMu.Lock()
	hadPanic := panicOccurred
	panicMu.Unlock()

	if hadPanic {
		t.Errorf("WaitGroup panic occurred - this is the bug we need to fix")
	} else {
		t.Log("No WaitGroup panic occurred - test passed")
	}
}

// TestWaitGroupPanic_ConcurrentDuplicateCreates tests concurrent creation of duplicate objects
// This more closely matches the scenario builder's concurrent processing
func TestWaitGroupPanic_ConcurrentDuplicateCreates(t *testing.T) {
	t.Parallel()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-waitgroup-dup-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	// Create storage factory
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()
	cleanupStorageTestRoot(t, tmpDir, storageProvider)

	// Create coordinator
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create scenario builder
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.secCtx = pkgctx.NewSystemSecurityContext()
	if builder.config == nil {
		builder.config = &ScenarioBuilderConfig{}
	}
	builder.config.TargetDir = tmpDir

	// Setup infrastructure (specs, lifecycles)
	if err := builder.setupInfrastructure(); err != nil {
		t.Fatalf("Failed to setup infrastructure: %v", err)
	}

	// Create account object first time
	accountObj := map[string]any{
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "test-user",
		objects.FieldKeyTitle:         "Test Account",
		objects.FieldKeyStatus:        scenarioBuilderStatusActive,
		objects.FieldKeySchemaVersion: scenarioBuilderSchemaV2,
	}

	createCtx := pkgctx.WithCacheUpdate(pkgctx.NewSystemContext(), "ACC-1785920548450214015-3df55bd1", "account", "")

	// First creation
	err = storageProvider.Create(createCtx, builder.secCtx, accountObj)
	if err != nil {
		t.Fatalf("First account creation should succeed: %v", err)
	}

	// Now simulate concurrent duplicate creates (like scenario builder with multiple objects)
	var layerWg sync.WaitGroup
	var activeValidations sync.WaitGroup
	panicOccurred := false
	panicMu := &sync.Mutex{}
	panicCount := 0

	// Create multiple goroutines trying to create the same duplicate (simulating concurrent processing)
	concurrency := 3
	for i := 0; i < concurrency; i++ {
		goroutinelabels.NewGoroutine(fmt.Sprintf("scenario_builder_create_account_%d", i), fmt.Sprintf("creating account object ACC-1785920548450214015-3df55bd1 in layer 1 (attempt %d)", i)).
			WithWaitGroup(&layerWg).
			WithPanicHandler(func(r any) {
				panicMu.Lock()
				panicOccurred = true
				panicCount++
				panicMu.Unlock()
				panicStr := fmt.Sprintf("%v", r)
				if strings.Contains(panicStr, "negative WaitGroup counter") {
					t.Logf("WaitGroup panic caught (count: %d): %v", panicCount, r)
				} else {
					t.Logf("Other panic caught: %v", r)
				}
			}).
			Start(func() error {
				// Track validation (exact code from scenario builder)
				validationAdded := false
				activeValidations.Add(1)
				validationAdded = true
				defer func() {
					defer func() {
						if r := recover(); r != nil {
							panicMu.Lock()
							panicOccurred = true
							panicCount++
							panicMu.Unlock()
							t.Logf("Recovered WaitGroup panic in defer: %v", r)
						}
					}()
					if validationAdded {
						activeValidations.Done()
					}
				}()

				// Try to create duplicate - this triggers the panic
				duplicateCtx := pkgctx.WithCacheUpdate(pkgctx.NewSystemContext(), "ACC-1785920548450214015-3df55bd1", "account", "")
				localAccountObj := map[string]any{
					objects.FieldKeyKind:          "account",
					objects.FieldKeyID:            "ACC-1785920548450214015-3df55bd1",
					objects.FieldKeyTitle:         "Test User Account",
					objects.FieldKeyStatus:        scenarioBuilderStatusActive,
					objects.FieldKeySchemaVersion: scenarioBuilderSchemaV2,
				}
				err := storageProvider.Create(duplicateCtx, builder.secCtx, localAccountObj)
				if err != nil {
					// Expected - object already exists
					// Return nil to simulate "skip silently" behavior
					return nil
				}
				return nil
			})
	}

	// Wait for layer to complete
	layerWg.Wait()

	// Wait for all validations to complete (like scenario builder does)
	activeValidations.Wait()

	// Check if panic occurred
	panicMu.Lock()
	hadPanic := panicOccurred
	count := panicCount
	panicMu.Unlock()

	if hadPanic {
		t.Errorf("WaitGroup panic occurred %d time(s) - this is the bug we need to fix", count)
	} else {
		t.Log("No WaitGroup panic occurred - test passed")
	}
}

// TestWaitGroupPanic_WithActiveValidations tests the exact scenario from scenario builder
// where activeValidations WaitGroup is used alongside layer WaitGroup
func TestWaitGroupPanic_WithActiveValidations(t *testing.T) {
	t.Parallel()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-waitgroup-activeval-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}

	// Create storage factory
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()
	cleanupStorageTestRoot(t, tmpDir, storageProvider)

	// Create coordinator
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create scenario builder
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.secCtx = pkgctx.NewSystemSecurityContext()
	if builder.config == nil {
		builder.config = &ScenarioBuilderConfig{}
	}
	builder.config.TargetDir = tmpDir

	// Setup infrastructure (specs, lifecycles)
	if err := builder.setupInfrastructure(); err != nil {
		t.Fatalf("Failed to setup infrastructure: %v", err)
	}

	// Create account object first time
	accountObj := map[string]any{
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "test-user",
		objects.FieldKeyTitle:         "Test Account",
		objects.FieldKeyStatus:        scenarioBuilderStatusActive,
		objects.FieldKeySchemaVersion: scenarioBuilderSchemaV2,
	}

	createCtx := pkgctx.WithCacheUpdate(pkgctx.NewSystemContext(), "ACC-1785920548450214015-3df55bd1", "account", "")

	// First creation
	err = storageProvider.Create(createCtx, builder.secCtx, accountObj)
	if err != nil {
		t.Fatalf("First account creation should succeed: %v", err)
	}

	// Now simulate the exact scenario builder flow
	var layerWg sync.WaitGroup
	var activeValidations sync.WaitGroup
	panicOccurred := false
	panicMu := &sync.Mutex{}

	// Simulate creating duplicate in goroutine (like scenario builder does)
	goroutinelabels.NewGoroutine("scenario_builder_create_account_0", "creating account object ACC-1785920548450214015-3df55bd1 in layer 1").
		WithWaitGroup(&layerWg).
		WithPanicHandler(func(r any) {
			panicMu.Lock()
			panicOccurred = true
			panicMu.Unlock()
			panicStr := fmt.Sprintf("%v", r)
			if strings.Contains(panicStr, "negative WaitGroup counter") {
				t.Logf("WaitGroup panic caught: %v", r)
			} else {
				t.Logf("Other panic caught: %v", r)
			}
		}).
		Start(func() error {
			// Simulate early return path (like prepareObjectFromDataFile failing)
			// This tests if early returns before activeValidations.Add(1) cause issues
			// In the actual code, if prepareObjectFromDataFile fails, we return early
			// and never call activeValidations.Add(1), so the defer won't be set up
			// But what if there's a panic before we reach activeValidations.Add(1)?

			// Track validation (exact code from scenario builder)
			// BUT: simulate a case where we might return early
			validationAdded := false

			// Simulate prepareObjectFromDataFile check (in real code, this happens before Add(1))
			// If this fails, we return early and never call Add(1)
			// But if it panics, the defer might still try to call Done()
			// Let's test this edge case

			activeValidations.Add(1)
			validationAdded = true
			defer func() {
				defer func() {
					if r := recover(); r != nil {
						panicMu.Lock()
						panicOccurred = true
						panicMu.Unlock()
						t.Logf("Recovered WaitGroup panic in defer: %v", r)
					}
				}()
				if validationAdded {
					activeValidations.Done()
				}
			}()

			// Try to create duplicate - this triggers the panic
			duplicateCtx := pkgctx.WithCacheUpdate(pkgctx.NewSystemContext(), "ACC-1785920548450214015-3df55bd1", "account", "")
			err := storageProvider.Create(duplicateCtx, builder.secCtx, accountObj)
			if err != nil {
				// Expected - object already exists
				// Return nil to simulate "skip silently" behavior
				return nil
			}
			return nil
		})

	// Wait for layer to complete
	layerWg.Wait()

	// Wait for all validations to complete (like scenario builder does)
	activeValidations.Wait()

	// Check if panic occurred
	panicMu.Lock()
	hadPanic := panicOccurred
	panicMu.Unlock()

	if hadPanic {
		t.Errorf("WaitGroup panic occurred - this is the bug we need to fix")
	} else {
		t.Log("No WaitGroup panic occurred - test passed")
	}
}
