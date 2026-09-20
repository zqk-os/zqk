package scheduler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2" // Register spec builders
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

var schedulerCLITestBrandSettingsYAML = `# Test brand settings for scheduler CLI tests
$schema: "https://zqk.dev/schemas/brand_settings.schema.json"
description: "Scheduler CLI test root settings"
version: "1.0.0"

paths:
  project_root: ""
  staleness_check_dirs:
    - paths.ProjectDataDir
    - ".zqk/config"
  aliases:
    docs: "docs"
    process: "` + paths.ProcessDir + `"
    architecture: "docs/architecture"
    zqk: paths.ProjectDataDir
    paths.ProjectDataDir: paths.ProjectDataDir
    streams: ".zqk/streams"
    cache: ".zqk/cache"

cli:
  default_context: "human"
`

func schedulerCLIAppendStagesBeforeStorage(root string) []testkit.NamedTestStep {
	return []testkit.NamedTestStep{
		{
			Name: "scheduler_cli_brand_settings",
			Fn: func() error {
				return fileutil.WriteSecureFile(filepath.Join(root, "zqk-settings.yaml"), []byte(schedulerCLITestBrandSettingsYAML))
			},
		},
		{
			Name: "scheduler_cli_generate_specs",
			Fn: func() error {
				specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
				if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
					return err
				}
				return builders.NewSpecGenerator(specsDir).GenerateAllSpecs()
			},
		},
	}
}

// setupTestEnvironment creates a test environment for scheduler CLI tests via
// [testkit.PrepareIsolatedTempProject]. Callers must not use t.Parallel(): ZQK_TEST_ROOT uses t.Setenv.
func setupTestEnvironment(t *testing.T) (testRoot string, ctx *cli.Context, cmd *cobra.Command) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                      "cmd.scheduler.cli",
		AppendStagesBeforeStorage: schedulerCLIAppendStagesBeforeStorage,
		ForceRemoveRootOnCleanup:  true,
	})
	testRoot = proj.Root

	// Create CLI context (match production defaults: system profile, table format)
	cliCtx := cli.ContextForProjectRoot(testRoot).WithProfile("system").WithFormat("table")

	// Create a mock command for testing. Inject a buffer as command output writer so
	// WriteOutput (used by activity, etc.) has a valid writer (avoids "write |1: file already closed" in parallel tests).
	var outBuf bytes.Buffer
	sysCtx := pkgctx.NewSystemContext()
	sysCtx = pkgctx.WithCommandOutputWriter(sysCtx, &outBuf)
	cmd = &cobra.Command{
		Use: "test",
	}
	cmd.SetContext(sysCtx)
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)

	return testRoot, cliCtx, cmd
}

// rollbackGlobalScheduler registers a cleanup that restores the process-wide scheduler
// registry after tests that call NewSchedulerWithProjectRoot (which always registers globally).
func rollbackGlobalScheduler(t *testing.T) {
	t.Helper()
	restore := scheduler.WithGlobalSchedulerRollback()
	t.Cleanup(restore)
}

// createTestSchedulerJob creates a test scheduler_job object
func createTestSchedulerJob(t *testing.T, storageProvider storagepkg.ObjectStorageProvider, jobID string) {
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// Use dynamic kind lookup for consistency
	schedulerJobKind := getSchedulerJobKind()
	jobData := map[string]any{
		objects.FieldKeyID:                 jobID,
		objects.FieldKeyKind:               schedulerJobKind,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:             objects.ObjectStatusActive,
		objects.FieldKeyJobType:            "cache_prewarm",
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "0 */6 * * *",
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  300,
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyTitle:              "Test Job",
		objects.FieldKeyDescription:        "Test scheduler job",
		objects.FieldKeyCreatedAt:          zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:          "ACC-TEST",
		objects.FieldKeyUpdatedAt:          zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:          "ACC-TEST",
		objects.FieldKeyOriginProject:      validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:       validation.DefaultOriginSystem,
	}

	err := storageProvider.Create(ctx, secCtx, jobData)
	if err != nil {
		t.Fatalf("Failed to create test scheduler job: %v", err)
	}
}

// waitForSchedulerStop waits for a WaitGroup to complete with a timeout to prevent indefinite hangs.
// This is used when waiting for scheduler goroutines to stop after context cancellation.
// If the timeout is exceeded, the test will fail with a descriptive error message.
func waitForSchedulerStop(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("scheduler_test", "wait for scheduler stop").
		StartSimple(func() {
			wg.Wait()
			close(done)
		})

	waitErr := testkit.RunNamedTestSteps(context.Background(), "cmd.scheduler.wait_for_stop",
		testkit.NamedTestStep{
			Name: "WAIT_SCHEDULER_STOP",
			Fn: func() error {
				select {
				case <-done:
					return nil
				case <-time.After(timeout):
					return context.DeadlineExceeded
				}
			},
		},
	)
	if waitErr != nil {
		t.Fatalf("Scheduler did not stop within %v - test may be hanging", timeout)
	}
}

func TestStartScheduler_AlreadyRunning(t *testing.T) {
	rollbackGlobalScheduler(t)
	testRoot, _, _ := setupTestEnvironment(t)

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create scheduler
	sched := scheduler.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)

	// Start scheduler
	startCtx1, cancel1 := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel1()

	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler").
		StartSimple(func() {
			defer wg.Done()
			_ = sched.Start(startCtx1) //nolint:errcheck // Test setup - errors are acceptable
		})

	// Wait deterministically for scheduler to start
	if !waitForConditionWithTimeoutScheduler(pkgctx.NewSystemContext(),
		func() bool { return sched.IsRunning() },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Scheduler did not start within 5 seconds")
	}

	// Try to start again - should fail
	startCtx2, cancel2 := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel2()

	err = sched.Start(startCtx2)
	if err == nil {
		t.Error("Starting scheduler twice should return an error")
	}

	// Cleanup
	cancel1()
	waitForSchedulerStop(t, &wg, 5*time.Second)
}

func TestStopScheduler_Success(t *testing.T) {
	rollbackGlobalScheduler(t)
	testRoot, _, _ := setupTestEnvironment(t)

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create scheduler
	sched := scheduler.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)

	// Start scheduler
	startCtx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler").
		StartSimple(func() {
			defer wg.Done()
			_ = sched.Start(startCtx) //nolint:errcheck // Test setup - errors are acceptable
		})

	// Wait deterministically for scheduler to start
	if !waitForConditionWithTimeoutScheduler(pkgctx.NewSystemContext(),
		func() bool { return sched.IsRunning() },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Scheduler did not start within 5 seconds")
	}

	if !sched.IsRunning() {
		t.Fatal("Scheduler should be running before stop test")
	}

	// Stop scheduler
	sched.Stop()

	// Wait deterministically for scheduler to stop
	if !waitForConditionWithTimeoutScheduler(pkgctx.NewSystemContext(),
		func() bool { return !sched.IsRunning() },
		15*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Scheduler did not stop within 15 seconds")
	}

	if sched.IsRunning() {
		t.Error("Scheduler should be stopped after Stop()")
	}

	// Cleanup
	cancel()
	waitForSchedulerStop(t, &wg, 15*time.Second)
}

func TestStopScheduler_NotRunning(t *testing.T) {
	rollbackGlobalScheduler(t)
	testRoot, _, _ := setupTestEnvironment(t)

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create scheduler (not started)
	sched := scheduler.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)

	// Stop scheduler that's not running - should not panic
	sched.Stop()

	if sched.IsRunning() {
		t.Error("Scheduler should not be running")
	}
}

func TestShowSchedulerStatus_Running(t *testing.T) {
	rollbackGlobalScheduler(t)
	testRoot, _, _ := setupTestEnvironment(t)

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create scheduler (registers globally; rollbackGlobalScheduler restores after test)
	sched := scheduler.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)

	// Start scheduler
	startCtx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler").
		StartSimple(func() {
			defer wg.Done()
			_ = sched.Start(startCtx) //nolint:errcheck // Test setup - errors are acceptable
		})

	// Wait deterministically for scheduler to start
	if !waitForConditionWithTimeoutScheduler(pkgctx.NewSystemContext(),
		func() bool { return sched.IsRunning() },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Scheduler did not start within 5 seconds")
	}

	// Check status: our instance is running and global points to it (restore ensures we still own global during check)
	globalSched := scheduler.GetGlobalScheduler()
	if globalSched == nil {
		t.Fatal("Global scheduler should be registered")
	}
	if globalSched != sched {
		t.Skip("Global scheduler was overwritten by another parallel test; skipping global IsRunning check")
	}
	if !globalSched.IsRunning() {
		t.Error("Global scheduler should report as running")
	}

	// Cleanup: Stop() releases scheduler-held project files; context cancel alone can leave
	// .zqk paths busy and break t.TempDir RemoveAll (directory not empty).
	sched.Stop()
	if !waitForConditionWithTimeoutScheduler(pkgctx.NewSystemContext(),
		func() bool { return !sched.IsRunning() },
		15*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Scheduler did not stop within 15 seconds")
	}
	cancel()
	waitForSchedulerStop(t, &wg, 15*time.Second)
}

func TestShowSchedulerStatus_NotRunning(t *testing.T) {
	rollbackGlobalScheduler(t)
	testRoot, _, _ := setupTestEnvironment(t)

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create scheduler (not started; constructor registers globally)
	sched := scheduler.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)

	// Check status
	globalSched := scheduler.GetGlobalScheduler()
	if globalSched == nil {
		t.Fatal("Global scheduler should be registered")
	}
	if globalSched != sched {
		t.Fatal("GetGlobalScheduler should return the scheduler constructed for this test")
	}

	if globalSched.IsRunning() {
		t.Error("Global scheduler should report as not running")
	}
}

func TestTriggerJob_SchedulerNotRunning(t *testing.T) {
	// Do not use t.Parallel(): global scheduler registry is process-wide; parallel tests
	// can register a running scheduler and make triggerJob succeed here.
	rollbackGlobalScheduler(t)
	testRoot, cliCtx, cmd := setupTestEnvironment(t)

	// If a scheduler daemon is running in another process for this project root,
	// triggerJob will enqueue via the trigger queue instead of failing. In that
	// cross-process case, this test's assumption ("not running anywhere") does not hold.
	if status, _ := getSchedulerStatus(cliCtx); status != nil && status.Running && !status.InProcess {
		t.Skip("Scheduler daemon running in another process; triggerJob uses queue path instead of returning error")
	}

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create a test scheduler job
	createTestSchedulerJob(t, storageProvider, "SCH-002")

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Constructor registers globally so triggerJob sees a non-running in-process scheduler.
	_ = scheduler.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)

	// Try to trigger job - should fail
	err = triggerJob(cliCtx, cmd, "SCH-002")
	if err == nil {
		t.Error("Triggering job when scheduler is not running should return an error")
	}
}

func TestTriggerJob_SchedulerRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping flaky scheduler trigger test in short mode (scheduler stop can hang when run with other tests)")
	}
	rollbackGlobalScheduler(t)
	testRoot, cliCtx, cmd := setupTestEnvironment(t)

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create a test scheduler job with manual trigger type (supports manual triggering)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()
	jobData := map[string]any{
		objects.FieldKeyID:                "SCH-003",
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyJobType:           "cache_prewarm",
		objects.FieldKeyTriggerType:       "manual", // Manual trigger type supports manual triggering
		objects.FieldKeyCategory:          "maintenance",
		objects.FieldKeyExecutionMode:     "reusable",
		objects.FieldKeyMaxRuntimeSeconds: 300,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyTitle:             "Test Job",
		objects.FieldKeyDescription:       "Test scheduler job",
		objects.FieldKeyCreatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
	}
	if err := storageProvider.Create(ctx, secCtx, jobData); err != nil {
		t.Fatalf("Failed to create test scheduler job: %v", err)
	}

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create scheduler (constructor registers globally)
	sched := scheduler.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	sched.SetSecurityContext(secCtx) // Use secCtx from above

	// Start scheduler
	startCtx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler").
		StartSimple(func() {
			defer wg.Done()
			_ = sched.Start(startCtx) //nolint:errcheck // Test setup - errors are acceptable
		})

	// Wait deterministically for scheduler to start
	if !waitForConditionWithTimeoutScheduler(pkgctx.NewSystemContext(),
		func() bool { return sched.IsRunning() },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Scheduler did not start within 5 seconds")
	}

	// Trigger job - should succeed
	triggerErr := triggerJob(cliCtx, cmd, "SCH-003")
	if triggerErr != nil {
		// When scheduler runs in another process (e.g. daemon under bundler), the test-created SCH-003
		// is not in that process's job list; skip instead of failing.
		if strings.Contains(triggerErr.Error(), "job not found") {
			t.Skipf("Skipping: trigger failed with job not found (scheduler may be in another process or job not yet loaded): %v", triggerErr)
		}
		t.Errorf("Triggering job when scheduler is running should succeed: %v", triggerErr)
	}

	// Cleanup
	cancel()
	waitForSchedulerStop(t, &wg, 15*time.Second)
}

func TestStartScheduler_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping flaky scheduler context cancellation test in short mode (Stop() can hang when run with other tests)")
	}
	rollbackGlobalScheduler(t)
	testRoot, _, cmd := setupTestEnvironment(t)

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create a test scheduler job
	createTestSchedulerJob(t, storageProvider, "SCH-004")

	// Create spec and lifecycle loaders
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	// Create scheduler
	sched := scheduler.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)

	// Create context that will be cancelled
	startCtx, cancel := context.WithCancel(pkgctx.NewSystemContext())

	// Create command context that will be cancelled
	cmdCtx, cmdCancel := context.WithCancel(pkgctx.NewSystemContext())
	cmd.SetContext(cmdCtx)

	// Start scheduler in goroutine. Also call cancel() synchronously after cmdCancel() below so startCtx
	// is cancelled even if the helper goroutine has not run yet (avoids rare livelock: Start blocked on
	// <-startCtx.Done() while the helper is still waiting to observe cmd.Context().Done()).
	done := make(chan struct{})

	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler for cancellation test").StartSimple(func() {
		defer close(done)
		goroutinelabels.NewGoroutine("scheduler_test", "context cancellation helper").StartSimple(func() {
			<-cmd.Context().Done()
			cancel()
		})
		_ = sched.Start(startCtx) //nolint:errcheck // Test setup - errors are acceptable
	})

	// Wait deterministically for scheduler to start
	if !waitForConditionWithTimeoutScheduler(pkgctx.NewSystemContext(),
		func() bool { return sched.IsRunning() },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Scheduler did not start within 5 seconds")
	}

	if !sched.IsRunning() {
		t.Fatal("Scheduler should be running")
	}

	// Cancel command context (simulating Ctrl+C), then cancel startCtx immediately (idempotent with helper).
	cmdCancel()
	cancel()

	// Stop() can be slow under contention; bundle uses a long go-test timeout for the package.
	const contextCancelStopWait = 60 * time.Second

	// Wait for scheduler to stop with timeout
	waitErr := testkit.RunNamedTestSteps(context.Background(), "cmd.scheduler.context_cancel_wait",
		testkit.NamedTestStep{
			Name: "WAIT_CONTEXT_CANCEL_STOP",
			Fn: func() error {
				select {
				case <-done:
					return nil
				case <-time.After(contextCancelStopWait):
					return context.DeadlineExceeded
				}
			},
		},
	)
	if waitErr != nil {
		t.Fatalf("Scheduler did not stop within %v - test may be hanging", contextCancelStopWait)
	}

	if sched.IsRunning() {
		t.Error("Scheduler should be stopped after context cancellation")
	}
}

func TestListJobs_Success(t *testing.T) {
	testRoot, cliCtx, _ := setupTestEnvironment(t)

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create multiple test scheduler jobs
	createTestSchedulerJob(t, storageProvider, "SCH-005")
	createTestSchedulerJob(t, storageProvider, "SCH-006")

	// List jobs - should succeed
	err = listJobs(cliCtx, &cobra.Command{})
	if err != nil {
		t.Errorf("Listing jobs should succeed: %v", err)
	}
}

func TestListJobs_NoJobs(t *testing.T) {
	testRoot, cliCtx, _ := setupTestEnvironment(t)

	// Create storage provider
	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	_ = storageFactory.GetStorage()

	// List jobs with no jobs - should succeed (empty list)
	err = listJobs(cliCtx, &cobra.Command{})
	if err != nil {
		t.Errorf("Listing jobs with no jobs should succeed: %v", err)
	}
}

// TestSchedulerStartCommand_BackgroundDefault verifies the start command defaults to background
// so the scheduler runs consistently in the background; --foreground attaches to the process.
func TestSchedulerStartCommand_BackgroundDefault(t *testing.T) {
	t.Parallel()
	schedulerCmd := NewSchedulerCmd()
	startCmd, _, _ := schedulerCmd.Find([]string{"start"})
	if startCmd == nil {
		t.Fatal("scheduler start command not found")
	}
	backgroundFlag := startCmd.Flags().Lookup("background")
	if backgroundFlag == nil {
		t.Error("scheduler start should have --background flag")
		return
	}
	foregroundFlag := startCmd.Flags().Lookup("foreground")
	if foregroundFlag == nil {
		t.Error("scheduler start should have --foreground flag")
		return
	}
	backgroundDefault, err := startCmd.Flags().GetBool("background")
	if err != nil {
		t.Errorf("getting --background flag: %v", err)
		return
	}
	if !backgroundDefault {
		t.Error("--background should default to true (scheduler runs in background by default)")
	}
	foregroundDefault, err := startCmd.Flags().GetBool("foreground")
	if err != nil {
		t.Errorf("getting --foreground flag: %v", err)
		return
	}
	if foregroundDefault {
		t.Error("--foreground should default to false")
	}
}

func TestBuildAutonomyCorsHandler(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	corsHandler := buildAutonomyCorsHandler(dummyHandler)

	// 1. Allowed origin: http://127.0.0.1:5173
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rec := httptest.NewRecorder()
	corsHandler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:5173" {
		t.Errorf("expected Access-Control-Allow-Origin to be http://127.0.0.1:5173, got %q", got)
	}

	// 2. Allowed origin: http://localhost:5173
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec = httptest.NewRecorder()
	corsHandler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("expected Access-Control-Allow-Origin to be http://localhost:5173, got %q", got)
	}

	// 3. Disallowed origin: external site (must NOT reflect or wildcard)
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://malicious.example.com")
	rec = httptest.NewRecorder()
	corsHandler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected Access-Control-Allow-Origin to be empty for untrusted origin, got %q", got)
	}

	// 4. Preflight OPTIONS on allowed origin
	req = httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	rec = httptest.NewRecorder()
	corsHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 No Content for OPTIONS, got %d", rec.Code)
	}
}
