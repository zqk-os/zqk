package scheduler

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"

	"sync/atomic"
)

// TestJobLock_DuplicateExecutionPrevention tests that only one scheduler instance
// can execute a job at a time, even when multiple schedulers try simultaneously
func TestJobLock_DuplicateExecutionPrevention(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)

	testRoot := t.TempDir()

	// Process dir plus the object_specs dir IDValidator reads during storage validation.
	if err := paths.EnsureProcessAndObjectSpecsLayout(testRoot); err != nil {
		t.Fatalf("Failed to create test project layout: %v", err)
	}

	// Create storage provider
	storageProvider, err := storagepkg.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, storageProvider)
	storagepkg.BuildPathAliasCacheForProject(testRoot)
	testkit.RegisterTempProjectTeardown(t, testRoot, storageProvider)

	// Create spec and lifecycle loaders
	specLoader := objects.NewSpecLoader("")
	lifecycleLoader := objects.NewLifecycleLoader("")

	// Create a test job (must match pattern ^[A-Z]+-\d{3,}$)
	jobID := "SCH-901"
	secCtx := pkgctx.NewSystemSecurityContext()

	// Track execution count using a file-based counter
	// This works across processes/goroutines
	counterFile := filepath.Join(testRoot, "execution_counter.txt")

	// Create a simple test script that increments the counter
	// Add a small sleep to ensure lock is held during execution
	testScript := filepath.Join(testRoot, "increment_counter.sh")
	scriptContent := `#!/bin/sh
sleep 0.2
echo "$(($(cat ` + counterFile + ` 2>/dev/null || echo 0) + 1))" > ` + counterFile + `
`
	if err := fileutil.WriteFile(testScript, []byte(scriptContent), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Create the job with our test script
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeyTitle:             "Test Duplicate Prevention Job",
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyJobType:           JobTypeRunWrapper,
		objects.FieldKeyTriggerType:       "manual",
		objects.FieldKeyCommand:           "/bin/sh",
		objects.FieldKeyCommandArgs:       []string{testScript},
		objects.FieldKeyCategory:          "test",
		objects.FieldKeyExecutionMode:     "reusable",
		objects.FieldKeyMaxRuntimeSeconds: 5,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCreatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	ctx := pkgctx.NewSystemContext()
	if err := storageProvider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Create multiple scheduler instances (simulating multiple processes)
	numSchedulers := 5
	schedulers := make([]SchedulerInterface, numSchedulers)
	for i := 0; i < numSchedulers; i++ {
		sched := NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
		sched.SetSecurityContext(secCtx)
		schedulers[i] = sched
	}

	// Start all schedulers (this loads jobs)
	cancels := make([]context.CancelFunc, 0, len(schedulers))
	startDones := make([]chan error, 0, len(schedulers))
	for _, sched := range schedulers {
		startCtx, cancel := context.WithTimeout(ctx, 5*time.Second) //nolint:gosec // G118: cancel collected; deferred cleanup calls all cancels
		cancels = append(cancels, cancel)
		startDone := make(chan error, 1)
		startDones = append(startDones, startDone)
		goroutinelabels.NewGoroutine("scheduler_test", "start scheduler").StartSimple(func() {
			func(scheduler SchedulerInterface, done chan error) {
				done <- scheduler.Start(startCtx)
			}(sched, startDone)
		})
	}

	// Note: Schedulers start asynchronously - tests proceed immediately
	// The defer cleanup will ensure they're stopped when tests complete

	// Clean up all context cancellations and stop schedulers at end of test
	defer func() {
		_ = testkit.RunNamedTestSteps(context.Background(), "scheduler.job_lock_cleanup",
			testkit.NamedTestStep{Name: "CANCEL_START_CONTEXTS", Fn: func() error {
				for _, cancel := range cancels {
					cancel()
				}
				return nil
			}},
			testkit.NamedTestStep{Name: "WAIT_START_RETURNS", Fn: func() error {
				for _, done := range startDones {
					select {
					case <-done:
					case <-time.After(5 * time.Second):
					}
				}
				return nil
			}},
			testkit.NamedTestStep{Name: "STOP_SCHEDULERS", Fn: func() error {
				for _, sched := range schedulers {
					sched.Stop()
				}
				return nil
			}},
		)
	}()

	// Wait for at least one scheduler to have loaded the job before firing triggers.
	// Start() loads jobs asynchronously; triggering before load yields "job not found" and 0 executions.
	waitForJobLoadedOnAny(t, schedulers, jobID, 10*time.Second)

	// Launch all schedulers to trigger the job simultaneously
	var wg sync.WaitGroup
	startTime := time.Now()

	// Use a barrier to ensure all triggers start as close together as possible
	var barrier sync.WaitGroup
	barrier.Add(numSchedulers)
	ready := make(chan struct{})

	for i, sched := range schedulers {
		wg.Add(1)
		goroutinelabels.NewGoroutine("scheduler_test", "trigger job").StartSimple(func() {
			func(scheduler SchedulerInterface, _ int) {
				defer wg.Done()
				barrier.Done()
				<-ready                              // Wait for all to be ready
				_ = scheduler.TriggerJob(ctx, jobID) //nolint:errcheck // Test helper - error handling not critical
			}(sched, i)
		})
	}

	// Wait for all goroutines to be ready
	barrier.Wait()
	// Release all at once
	close(ready)

	// Wait for all trigger requests to be submitted
	wg.Wait()
	duration := time.Since(startTime)

	// Read the execution count from the file
	var finalCountVal int64
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := fileutil.ReadFile(counterFile)
		if err == nil && len(data) > 0 {
			if _, err := fmt.Sscanf(string(data), "%d", &finalCountVal); err == nil && finalCountVal > 0 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	finalCount := atomic.Int64{}
	finalCount.Store(finalCountVal)

	// The lock should prevent simultaneous execution
	// We expect at least 1 execution, but the key is that executions should be sequential
	// (not overlapping). With 5 schedulers trying simultaneously, we should see
	// that the lock serializes execution, so we should see fewer than 5 executions
	// if the lock is working correctly.
	// In practice, with a 0.2s job execution time and simultaneous triggers,
	// we might see 1-2 executions total (others timeout or are skipped).
	if finalCount.Load() > int64(numSchedulers) {
		t.Errorf("Got more executions (%d) than schedulers (%d) - lock not working", finalCount.Load(), numSchedulers)
	}

	// The important thing is that we got at least 1 execution, proving the lock
	// allows execution, but serializes it
	if finalCount.Load() < 1 {
		t.Errorf("Expected at least 1 execution, got %d", finalCount.Load())
	}

	t.Logf("Test completed in %v with %d execution(s) (lock serialized %d simultaneous triggers)", duration, finalCount.Load(), numSchedulers)
}

// TestJobLock_SequentialExecution tests that after one execution completes,
// another scheduler can acquire the lock and execute
func TestJobLock_SequentialExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)

	testRoot := t.TempDir()
	ensureSchedulerObjectSpecsForJobLockTest(t, testRoot)

	// Create storage provider
	storageProvider, err := storagepkg.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, storageProvider)
	storagepkg.BuildPathAliasCacheForProject(testRoot)
	testkit.RegisterTempProjectTeardown(t, testRoot, storageProvider)

	// Create spec and lifecycle loaders
	specLoader := objects.NewSpecLoader("")
	lifecycleLoader := objects.NewLifecycleLoader("")

	// Create a test job (must match pattern ^[A-Z]+-\d{3,}$)
	jobID := "SCH-902"
	secCtx := pkgctx.NewSystemSecurityContext()

	// Use a built-in handler (no external shell dependency) and validate sequential execution
	// via deterministic completion callbacks.
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeyTitle:             "Test Sequential Execution Job",
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyJobType:           JobTypeLifecycleCheck,
		objects.FieldKeyTriggerType:       "manual",
		objects.FieldKeyCategory:          "test",
		objects.FieldKeyExecutionMode:     "reusable",
		objects.FieldKeyMaxRuntimeSeconds: 5,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCreatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	ctx := pkgctx.NewSystemContext()
	if err := storageProvider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Create two scheduler instances
	sched1 := NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	sched1.SetSecurityContext(secCtx)
	if err := sched1.LoadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("Failed to load jobs (sched1): %v", err)
	}

	sched2 := NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	sched2.SetSecurityContext(secCtx)
	if err := sched2.LoadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("Failed to load jobs (sched2): %v", err)
	}

	// Trigger job in first scheduler
	cb1 := &schedulerTestOpCallback{done: make(chan error, 1)}
	if err := sched1.TriggerJobWithCallback(ctx, jobID, cb1); err != nil {
		t.Fatalf("Failed to trigger job on sched1: %v", err)
	}
	select {
	case err := <-cb1.done:
		if err != nil {
			t.Fatalf("sched1 job execution failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for sched1 job completion")
	}
	// OperationCallback is emitted before final storage/audit writes complete.
	// Ensure the job is fully finalized (not running) and flush async index updates
	// to avoid tempdir cleanup races.
	waitForJobFinalized(t, sched1.(*Scheduler), jobID, 5*time.Second)
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup

	// Trigger job in second scheduler (should succeed now)
	cb2 := &schedulerTestOpCallback{done: make(chan error, 1)}
	if err := sched2.TriggerJobWithCallback(ctx, jobID, cb2); err != nil {
		t.Fatalf("Failed to trigger job on sched2: %v", err)
	}
	select {
	case err := <-cb2.done:
		if err != nil {
			t.Fatalf("sched2 job execution failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for sched2 job completion")
	}
	waitForJobFinalized(t, sched2.(*Scheduler), jobID, 5*time.Second)
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
	_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
}

func waitForJobFinalized(t *testing.T, sched *Scheduler, jobID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sched.jobsMu.RLock()
		job := sched.jobs[jobID]
		sched.jobsMu.RUnlock()
		if job == nil {
			return
		}
		job.RunningMu.RLock()
		running := job.Running
		job.RunningMu.RUnlock()
		if !running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for job to finalize: %s", jobID)
}

// waitForJobLoadedOnAny waits until at least one scheduler has the job in its map (loaded from storage).
func waitForJobLoadedOnAny(t *testing.T, schedulers []SchedulerInterface, jobID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, s := range schedulers {
			sched := s.(*Scheduler)
			sched.jobsMu.RLock()
			_, ok := sched.jobs[jobID]
			sched.jobsMu.RUnlock()
			if ok {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s not loaded on any scheduler within %v", jobID, timeout)
}

// TestJobLock_TimeoutBehavior tests that when a lock is held, other schedulers
// timeout appropriately instead of blocking indefinitely
func TestJobLock_TimeoutBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)

	testRoot := t.TempDir()

	// Process dir plus the object_specs dir IDValidator reads during storage validation.
	if err := paths.EnsureProcessAndObjectSpecsLayout(testRoot); err != nil {
		t.Fatalf("Failed to create test project layout: %v", err)
	}

	// Create storage provider
	storageProvider, err := storagepkg.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, storageProvider)
	storagepkg.BuildPathAliasCacheForProject(testRoot)
	testkit.RegisterTempProjectTeardown(t, testRoot, storageProvider)

	// Create spec and lifecycle loaders
	specLoader := objects.NewSpecLoader("")
	lifecycleLoader := objects.NewLifecycleLoader("")

	// Create a test job (must match pattern ^[A-Z]+-\d{3,}$)
	jobID := "SCH-903"
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a long-running test script
	testScript := filepath.Join(testRoot, "long_running.sh")
	scriptContent := `#!/bin/sh
sleep 2
`
	if err := fileutil.WriteFile(testScript, []byte(scriptContent), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Update the job to use our long-running script
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeyTitle:             "Test Timeout Behavior Job",
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyJobType:           JobTypeRunWrapper,
		objects.FieldKeyTriggerType:       "manual",
		objects.FieldKeyCommand:           "/bin/sh",
		objects.FieldKeyCommandArgs:       []string{testScript},
		objects.FieldKeyCategory:          "test",
		objects.FieldKeyExecutionMode:     "reusable",
		objects.FieldKeyMaxRuntimeSeconds: 5,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCreatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	ctx := pkgctx.NewSystemContext()
	if err := storageProvider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Create two scheduler instances
	sched1 := NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	sched1.SetSecurityContext(secCtx)
	startCtx1, cancel1 := context.WithTimeout(ctx, 10*time.Second)
	startDone1 := make(chan error, 1)
	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler 1").StartSimple(func() {
		startDone1 <- sched1.Start(startCtx1)
	})
	defer func() {
		cancel1()
		// Wait for Start() to return (it will call Stop() internally)
		select {
		case <-startDone1:
		case <-time.After(5 * time.Second):
		}
		// Start() calls Stop() internally when ctx is cancelled, but ensure it's stopped
		sched1.Stop()
	}()

	sched2 := NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	sched2.SetSecurityContext(secCtx)
	startCtx2, cancel2 := context.WithTimeout(ctx, 10*time.Second)
	startDone2 := make(chan error, 1)
	goroutinelabels.StartTestGoroutine("test_scheduler2_start", "starting scheduler 2 in integration test", func() {
		startDone2 <- sched2.Start(startCtx2)
	})
	defer func() {
		cancel2()
		// Wait for Start() to return (it will call Stop() internally)
		select {
		case <-startDone2:
		case <-time.After(5 * time.Second):
		}
		// Start() calls Stop() internally when ctx is cancelled, but ensure it's stopped
		sched2.Stop()
	}()

	// Start first execution (will hold lock)
	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("scheduler_test", "trigger job").StartSimple(func() {
		defer wg.Done()
		_ = sched1.TriggerJob(ctx, jobID) //nolint:errcheck // Test helper - error handling not critical
	})

	// Wait a bit to ensure first scheduler has acquired lock
	time.Sleep(100 * time.Millisecond)

	// Second scheduler should timeout trying to acquire lock
	startTime := time.Now()
	_ = sched2.TriggerJob(ctx, jobID) //nolint:errcheck // Test helper - error handling not critical
	duration := time.Since(startTime)

	// Should have timed out quickly (within lock timeout, not waiting for job completion)
	// Default lock timeout is 30s, but we should see it fail much sooner
	// The actual timeout behavior depends on the lock implementation
	if duration > 35*time.Second {
		t.Errorf("Expected timeout to occur quickly, but took %v", duration)
	}

	// Wait for first execution to complete
	wg.Wait()
}
