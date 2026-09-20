package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// Test0_StartScheduler_Success lives in this file so it runs before other *_test.go files in the
// package: Go orders tests by file path, not by test name. scheduler_00_* sorts before
// scheduler_start_integration_test.go (…/scheduler_00… < …/scheduler_s…), so this executes before
// TestSchedulerStart_EndToEnd. Running after EndToEnd stressed shutdown and caused Stop() to exceed
// the wait budget when the bundle runs sequentially.
func Test0_StartScheduler_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping flaky scheduler start/stop test in short mode (Stop() can hang when run with other tests)")
	}
	rollbackGlobalScheduler(t)
	testRoot, _, _ := setupTestEnvironment(t)

	storageFactory, err := storagepkg.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()

	createTestSchedulerJob(t, storageProvider, "SCH-001")

	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()

	sched := schedpkg.NewSchedulerWithProjectRoot(storageProvider, specLoader, lifecycleLoader, testRoot, nil)
	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)

	if sched.IsRunning() {
		t.Error("Scheduler should not be running initially")
	}

	startCtx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	defer cancel()

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler").
		StartSimple(func() {
			defer close(done)
			_ = sched.Start(startCtx) //nolint:errcheck // Test setup - errors are acceptable
		})

	if !waitForConditionWithTimeoutScheduler(pkgctx.NewSystemContext(),
		func() bool { return sched.IsRunning() },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Fatal("Scheduler did not start within 5 seconds")
	}

	if !sched.IsRunning() {
		t.Error("Scheduler should be running after Start()")
	}

	cancel()

	shutdownBudget := 120 * time.Second

	waitErr := testkit.RunNamedTestSteps(context.Background(), "cmd.scheduler.start_stop_wait",
		testkit.NamedTestStep{
			Name: "WAIT_START_STOP_DONE",
			Fn: func() error {
				select {
				case <-done:
					return nil
				case <-time.After(shutdownBudget):
					return context.DeadlineExceeded
				}
			},
		},
	)
	if waitErr != nil {
		t.Fatalf("Scheduler did not stop within %v - Start() may still be in Stop(); see scheduler_00_* test file order vs integration tests", shutdownBudget)
	}

	if !waitForConditionWithTimeoutScheduler(pkgctx.NewSystemContext(),
		func() bool { return !sched.IsRunning() },
		shutdownBudget,
		10*time.Millisecond,
	) {
		t.Fatalf("Scheduler IsRunning() did not clear within %v", shutdownBudget)
	}

	if sched.IsRunning() {
		t.Error("Scheduler should be stopped after cancellation")
	}
}
