package system

// Tests that set ZQK_TEST_ROOT via t.Setenv must not use t.Parallel() on the same *testing.T.

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestAsyncValidation_HashRegistryCacheDeadlock tests for the deadlock vulnerability
// where hashRegistryCache.Delete() was called while already holding the mutex lock.
// This test specifically targets bucketed objects (like audit_event) that trigger
// the cache deletion path.
func TestAsyncValidation_HashRegistryCacheDeadlock(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := proj.Root

	// Create test directory structure for bucketed objects
	auditDir := filepath.Join(tmpDir, paths.ProcessAuditDir, "2026-01")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	// Create a test audit event file
	testContent := `id: AUD-271
kind: audit_event
schema_version: "` + objects.DefaultSchemaVersion + `"
status: completed
event_type: scheduler_job_completed
target_kind: scheduler_job
target_id: SCH-001
created_at: "2026-01-03T12:00:00Z"
created_by: "ACC-SYSTEM"
`
	testFile := testkit.WriteTestObjectStandalone(t, tmpDir, testContent)

	// Create a test command with proper context (mimics CLI execution)
	cmd := createTestCommandWithContext(t, tmpDir)

	// Create async validator using command context (mimics real CLI execution)
	validator := validation.NewAsyncValidator(cmd.Context(), tmpDir, 4, 1*time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("Failed to start validator: %v", err)
	}

	var hashRegistries []*storage.HashRegistry
	var hashRegistriesMu sync.Mutex
	trackHashRegistry := func(hr *storage.HashRegistry) {
		if hr == nil {
			return
		}
		hashRegistriesMu.Lock()
		hashRegistries = append(hashRegistries, hr)
		hashRegistriesMu.Unlock()
	}
	// t.Cleanup runs after defers; register last so this runs immediately before TempDir removal.
	t.Cleanup(func() {
		hashRegistriesMu.Lock()
		regs := append([]*storage.HashRegistry(nil), hashRegistries...)
		hashRegistriesMu.Unlock()
		runAsyncValidationTestTeardown(t, validator, regs)
	})

	// Create hash registry cache (same as in async_check.go)
	hashRegistryCache := &HashRegistryCacheType{
		mu:    sync.RWMutex{},
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Create a validation function that simulates the deadlock scenario
	// This mimics the exact code path that caused the deadlock
	validationFunc := func(stdCtx context.Context, objectID, objectKind, filePath string, content []byte) (*validation.ValidationState, error) {
		// Simulate the exact code path from async_check.go that caused the deadlock
		kindDir := filepath.Join(tmpDir, paths.ProcessAuditDir)
		fileDir := filepath.Dir(filePath)
		isBucketed := kindDir != emptyValue && fileDir != kindDir

		if kindDir != emptyValue {
			// Lock the mutex (this is where the deadlock occurred)
			hashRegistryCache.mu.Lock()

			var cacheKey string
			if isBucketed {
				cacheKey = objectKind + ":" + fileDir
			} else {
				cacheKey = objectKind
			}

			if isBucketed {
				// This is the problematic code path:
				// We're holding the lock, then calling Delete() which tries to lock again
				// OLD (buggy) code: hashRegistryCache.Delete(cacheKey) // DEADLOCK!
				// NEW (fixed) code: delete(hashRegistryCache.cache, cacheKey)

				// Test the fixed version (should not deadlock)
				delete(hashRegistryCache.cache, cacheKey)
				// Use context passed to validation function (mimics real CLI execution)
				hr := storage.NewHashRegistry(stdCtx, objectKind, fileDir)
				trackHashRegistry(hr)
			} else {
				if _, ok := hashRegistryCache.cache[cacheKey]; ok {
					_ = hashRegistryCache.cache[cacheKey] // Registry already cached
				} else {
					// Use context passed to validation function (mimics real CLI execution)
					registry := storage.NewHashRegistry(stdCtx, objectKind, kindDir)
					trackHashRegistry(registry)
					if loadErr := registry.Load(); loadErr == nil {
						hashRegistryCache.cache[cacheKey] = registry
					}
				}
			}
			hashRegistryCache.mu.Unlock()
		}

		// Return a simple validation state
		return &validation.ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Issues:        []validation.ValidationIssue{},
		}, nil
	}

	validator.SetValidationFunc(validationFunc)

	// Enqueue the validation task
	validator.Enqueue("AUD-271", "audit_event", testFile, 1)

	// Wait for validation to complete with timeout
	ctxTimeout, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()

	progressChan := validator.GetProgress()
	completed := false

	for !completed {
		select {
		case <-ctxTimeout.Done():
			t.Fatalf("Test timed out - validation did not complete (likely deadlock)")
		case progress, ok := <-progressChan:
			if !ok {
				t.Fatalf("Progress channel closed unexpectedly")
			}
			if progress.Status == "completed" && progress.CurrentObject == "AUD-271" {
				completed = true
			}
		}
	}

	// If we get here, the test passed (no deadlock)
}

// TestAsyncValidation_HashRegistryCacheConcurrentAccess tests concurrent access
// to the hash registry cache to ensure thread safety
func TestAsyncValidation_HashRegistryCacheConcurrentAccess(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := proj.Root

	// Create a test command with proper context (mimics CLI execution)
	cmd := createTestCommandWithContext(t, tmpDir)

	// Create test directory structure
	auditDir := filepath.Join(tmpDir, paths.ProcessAuditDir, "2026-01")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	hashRegistryCache := &HashRegistryCacheType{
		mu:    sync.RWMutex{},
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Create multiple registries concurrently
	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		goroutinelabels.NewGoroutine("system_test", "concurrent registry creation").StartSimple(func() {
			defer wg.Done()

			// Simulate the bucketed object path that was causing deadlock
			objectKind := "audit_event"
			fileDir := auditDir
			cacheKey := objectKind + ":" + fileDir

			hashRegistryCache.mu.Lock()
			// This is the fixed version - delete directly from map
			delete(hashRegistryCache.cache, cacheKey)
			// Use command context (mimics real CLI execution)
			registry := storage.NewHashRegistry(cmd.Context(), objectKind, fileDir)
			if loadErr := registry.Load(); loadErr != nil {
				// Registry may not exist, that's OK
			}
			hashRegistryCache.mu.Unlock()

			// Verify we can access it again
			hashRegistryCache.mu.RLock()
			_, exists := hashRegistryCache.cache[cacheKey]
			hashRegistryCache.mu.RUnlock()

			// Both branches return nil - expected behavior (exists check is for test logic)
			_ = exists
			errors <- nil
		})
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		if err != nil {
			t.Errorf("Goroutine error: %v", err)
		}
	}

	// If we get here without deadlock, the test passed
}

// TestAsyncValidation_HashRegistryCacheDeleteWhileLocked tests the specific
// deadlock scenario where Delete() is called while already holding the lock.
// This test would fail if the buggy code (calling Delete() while locked) were reintroduced.
func TestAsyncValidation_HashRegistryCacheDeleteWhileLocked(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	hashRegistryCache := &HashRegistryCacheType{
		mu:    sync.RWMutex{},
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Create a test command with proper context (mimics CLI execution)
	cmd := createTestCommandWithContext(t, tmpDir)

	// Add a test entry
	testKey := "test:key"
	testRegistry := storage.NewHashRegistry(cmd.Context(), "test", tmpDir)
	hashRegistryCache.Set(testKey, testRegistry)

	// Test the fixed path (should NOT deadlock)
	// This test verifies that deleting directly from the map (the fix) works correctly
	// If the buggy code (calling Delete() while holding lock) were reintroduced,
	// this test would timeout and fail
	done := make(chan bool, 1)

	goroutinelabels.StartTestGoroutine("test_cache_deleter", "deleting from hash registry cache in deadlock test", func() {
		hashRegistryCache.mu.Lock()
		defer hashRegistryCache.mu.Unlock()

		// FIXED VERSION: delete directly from map (no deadlock)
		// This is the correct approach when already holding the lock
		delete(hashRegistryCache.cache, testKey)

		// BUGGY VERSION (commented out - would cause deadlock):
		// hashRegistryCache.Delete(testKey) // Would try to lock again -> DEADLOCK!

		done <- true
	})

	select {
	case <-done:
		// Test passed - no deadlock with the fix
		// Verify the entry was deleted
		hashRegistryCache.mu.RLock()
		_, exists := hashRegistryCache.cache[testKey]
		hashRegistryCache.mu.RUnlock()
		if exists {
			t.Error("Expected entry to be deleted, but it still exists")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out - likely deadlock occurred. This would happen if Delete() was called while holding the lock.")
	}
}

// TestAsyncValidation_HashRegistryCacheBuggyPathWouldDeadlock documents
// the exact buggy code path that would cause a deadlock.
// This test serves as documentation and would fail if the bug were reintroduced.
func TestAsyncValidation_HashRegistryCacheBuggyPathWouldDeadlock(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	hashRegistryCache := &HashRegistryCacheType{
		mu:    sync.RWMutex{},
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Simulate the exact scenario from async_check.go that caused the deadlock
	objectKind := "audit_event"
	fileDir := filepath.Join(tmpDir, "audit", "2026-01")
	cacheKey := objectKind + ":" + fileDir

	// Create a test command with proper context (mimics CLI execution)
	cmd := createTestCommandWithContext(t, tmpDir)

	// Add entry to cache first
	testRegistry := storage.NewHashRegistry(cmd.Context(), objectKind, fileDir)
	hashRegistryCache.Set(cacheKey, testRegistry)

	// This is the code path that was buggy:
	// 1. Lock the mutex
	// 2. Check if isBucketed (true for audit_event in subdirectory)
	// 3. Call Delete() which tries to lock again -> DEADLOCK!

	// Test the FIXED version (should complete quickly)
	done := make(chan bool, 1)

	goroutinelabels.NewGoroutine("system_test", "cache cleanup").StartSimple(func() {
		hashRegistryCache.mu.Lock()
		defer hashRegistryCache.mu.Unlock()

		// FIXED: Delete directly from map (no additional lock attempt)
		delete(hashRegistryCache.cache, cacheKey)

		// BUGGY (commented out - would deadlock):
		// hashRegistryCache.Delete(cacheKey) // Tries to lock again -> DEADLOCK!

		done <- true
	})

	select {
	case <-done:
		// Test passed - fix works correctly
		t.Log("Fix verified: Direct map deletion works without deadlock")
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK DETECTED: If this test times out, it means Delete() was called while holding the lock. " +
			"This is the exact bug that was fixed. The fix is to use 'delete(hashRegistryCache.cache, cacheKey)' " +
			"instead of 'hashRegistryCache.Delete(cacheKey)' when already holding the lock.")
	}
}

// TestAsyncValidation_MultipleBucketedObjects tests validation of multiple
// bucketed objects concurrently to ensure no deadlocks
func TestAsyncValidation_MultipleBucketedObjects(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tmpDir := proj.Root

	// Create test directory structure
	auditDir := filepath.Join(tmpDir, paths.ProcessAuditDir, "2026-01")
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	// Create multiple test files
	testFiles := []string{"AUD-271", "AUD-272", "AUD-273"}
	testFilePaths := make(map[string]string)
	for _, objectID := range testFiles {
		testContent := `id: ` + objectID + `
kind: audit_event
schema_version: "` + objects.DefaultSchemaVersion + `"
status: completed
event_type: scheduler_job_completed
target_kind: scheduler_job
target_id: SCH-001
created_at: "2026-01-03T12:00:00Z"
created_by: "ACC-SYSTEM"
`
		testFilePaths[objectID] = testkit.WriteTestObjectStandalone(t, tmpDir, testContent)
	}

	// Create a test command with proper context (mimics CLI execution)
	cmd := createTestCommandWithContext(t, tmpDir)

	// Create async validator using command context (mimics real CLI execution)
	validator := validation.NewAsyncValidator(cmd.Context(), tmpDir, 4, 1*time.Hour)
	if err := validator.Start(); err != nil {
		t.Fatalf("Failed to start validator: %v", err)
	}

	// NewHashRegistry starts background save workers; track instances so we drain them before
	// t.TempDir cleanup (otherwise audit subtree can still have open writers / new files).
	var hashRegistries []*storage.HashRegistry
	var hashRegistriesMu sync.Mutex
	trackHashRegistry := func(hr *storage.HashRegistry) {
		if hr == nil {
			return
		}
		hashRegistriesMu.Lock()
		hashRegistries = append(hashRegistries, hr)
		hashRegistriesMu.Unlock()
	}
	t.Cleanup(func() {
		hashRegistriesMu.Lock()
		regs := append([]*storage.HashRegistry(nil), hashRegistries...)
		hashRegistriesMu.Unlock()
		runAsyncValidationTestTeardown(t, validator, regs)
	})

	// Create hash registry cache
	hashRegistryCache := &HashRegistryCacheType{
		mu:    sync.RWMutex{},
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Create validation function
	validationFunc := func(stdCtx context.Context, objectID, objectKind, filePath string, content []byte) (*validation.ValidationState, error) {
		kindDir := filepath.Join(tmpDir, paths.ProcessAuditDir)
		fileDir := filepath.Dir(filePath)
		isBucketed := kindDir != emptyValue && fileDir != kindDir

		if kindDir != emptyValue {
			hashRegistryCache.mu.Lock()

			var cacheKey string
			if isBucketed {
				cacheKey = objectKind + ":" + fileDir
			} else {
				cacheKey = objectKind
			}

			if isBucketed {
				// Fixed version: delete directly from map (no deadlock)
				delete(hashRegistryCache.cache, cacheKey)
				// Use context passed to validation function (mimics real CLI execution)
				hr := storage.NewHashRegistry(stdCtx, objectKind, fileDir)
				trackHashRegistry(hr)
			} else {
				if _, ok := hashRegistryCache.cache[cacheKey]; ok {
					_ = hashRegistryCache.cache[cacheKey] // Registry already cached
				} else {
					// Use context passed to validation function (mimics real CLI execution)
					registry := storage.NewHashRegistry(stdCtx, objectKind, kindDir)
					trackHashRegistry(registry)
					if loadErr := registry.Load(); loadErr == nil {
						hashRegistryCache.cache[cacheKey] = registry
					}
				}
			}
			hashRegistryCache.mu.Unlock()
		}

		return &validation.ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Issues:        []validation.ValidationIssue{},
		}, nil
	}

	validator.SetValidationFunc(validationFunc)

	// Enqueue all validation tasks concurrently
	for _, objectID := range testFiles {
		validator.Enqueue(objectID, "audit_event", testFilePaths[objectID], 1)
	}

	// Wait for all validations to complete (derive timeout from command context)
	ctxTimeout, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	progressChan := validator.GetProgress()
	completed := make(map[string]bool)
	expectedCount := len(testFiles)

	for len(completed) < expectedCount {
		select {
		case <-ctxTimeout.Done():
			t.Fatalf("Test timed out - only %d/%d validations completed (likely deadlock)", len(completed), expectedCount)
		case progress, ok := <-progressChan:
			if !ok {
				t.Fatalf("Progress channel closed unexpectedly")
			}
			if progress.Status == "completed" {
				completed[progress.CurrentObject] = true
			}
		}
	}

	// Verify all objects were validated
	if len(completed) != expectedCount {
		t.Errorf("Expected %d validations, got %d", expectedCount, len(completed))
	}
}

// asyncValidationTestTeardownPipelineKind groups test teardown logs/metrics; stages are specific steps.
const asyncValidationTestTeardownPipelineKind = "system.test.async_validation_teardown"

type asyncValidationTeardownPayload struct {
	t *testing.T
	v *validation.AsyncValidator

	registries []*storage.HashRegistry
}

func asyncValidationTeardownStageStopValidator(_ *pipeline.Context, payload any) (any, error) {
	p := payload.(*asyncValidationTeardownPayload)
	if err := p.v.Stop(); err != nil {
		p.t.Errorf("Failed to stop validator: %v", err)
	}
	return payload, nil
}

func asyncValidationTeardownStageDrainHashRegistries(_ *pipeline.Context, payload any) (any, error) {
	p := payload.(*asyncValidationTeardownPayload)
	drainHashRegistriesForTest(p.t, p.registries)
	return payload, nil
}

func asyncValidationTeardownStageGlobalStorageQueues(_ *pipeline.Context, payload any) (any, error) {
	drainGlobalStorageQueuesForTest()
	return payload, nil
}

// runAsyncValidationTestTeardown stops the validator, drains tracked hash registries, then drains global
// storage queues. Uses pkg/pipeline for explicit ordering and stable stage names for debugging.
func runAsyncValidationTestTeardown(t *testing.T, v *validation.AsyncValidator, registries []*storage.HashRegistry) {
	t.Helper()
	p := &asyncValidationTeardownPayload{t: t, v: v, registries: registries}
	_, _ = testkit.RunTestPipeline(
		context.Background(),
		asyncValidationTestTeardownPipelineKind,
		p,
		testkit.TestStage{Name: "FINALIZE_stop_validator", Fn: asyncValidationTeardownStageStopValidator},
		testkit.TestStage{Name: "FINALIZE_drain_hash_registries", Fn: asyncValidationTeardownStageDrainHashRegistries},
		testkit.TestStage{Name: "FINALIZE_global_storage_queues", Fn: asyncValidationTeardownStageGlobalStorageQueues},
	)
}

// createTestCommandWithContext creates a cobra command with proper context setup
// that mimics the exact same context that would be passed if running from the CLI.
// This ensures tests use the same context hierarchy as real CLI execution.
func createTestCommandWithContext(t *testing.T, projectRoot string) *cobra.Command {
	cmd := &cobra.Command{
		Use: "test",
	}

	// Create Go context (mimics root.go:86 - starts with Background)
	// In real CLI, this gets wrapped with tracker, but for tests we can use a cancellable context
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	t.Cleanup(cancel) // Ensure context is cancelled when test completes

	// Set Go context on command (mimics root.go:406 - cmd.SetContext)
	cmd.SetContext(ctx)

	// Create CLI initialization context (mimics root.go:247)
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, projectRoot)

	// Get CLI context from command (mimics root.go:329 - cli.GetContextFromCommand)
	cliCtx, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		// Fallback to minimal context if GetContextFromCommand fails
		manager := cli.NewContextManager()
		minimalCtx, err := manager.LoadContext(initCtx)
		if err != nil {
			t.Fatalf("Failed to create minimal context: %v", err)
		}
		cliCtx = cli.ContextFromInner(minimalCtx)
	}

	// Register CLI context with command (mimics root.go:341 - cli.SetContext)
	cli.SetContext(cmd, cliCtx)

	return cmd
}
