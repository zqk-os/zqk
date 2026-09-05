package scheduler

// BLI-177483 inventory: setupSchedulerCompleteTestEnvironment → GetTestCleanup → RunProjectTestTeardown (scheduler_test_layout_helpers_test.go).

import (
	"context"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func waitCreateResultWithPipeline(timeout time.Duration, done <-chan error, ctx context.Context) (timedOut bool, canceled bool, err error) {
	_ = testkit.RunNamedTestSteps(context.Background(), "scheduler.storage_create_watchdog",
		testkit.NamedTestStep{
			Name: "WAIT_CREATE_RESULT",
			Fn: func() error {
				select {
				case err = <-done:
				case <-time.After(timeout):
					timedOut = true
				case <-ctx.Done():
					canceled = true
				}
				return nil
			},
		},
	)
	return timedOut, canceled, err
}

// TestStorage_CreateSchedulerJob_DoesNotHang verifies that creating a scheduler job doesn't hang
// This isolates the exact operation that scheduler submit performs
func TestStorage_CreateSchedulerJob_DoesNotHang(t *testing.T) {
	// Not t.Parallel(): setupSchedulerCompleteTestEnvironment sets ZQK_TEST_ROOT via os.Setenv (process-global).
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	t.Cleanup(env.Cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	storagepkg.BuildPathAliasCacheForProject(env.TestRoot)
	secCtx := env.SecurityContext
	// Bundle runs (many packages, -p 10) can delay storage.Create; keep a firm upper bound to catch real hangs.
	//
	// Create pipeline stage listing_index_flush (see storage.create_pipeline.go) calls
	// FlushKindContext with storage.IndexFlushAfterCreateTimeout. The flush can wait until that stage
	// budget (or ctx deadline) while the per-kind queue drains; other tests may enqueue work on the
	// same process-global queue. Total Create latency includes earlier stages plus this flush—so the
	// test watchdog must exceed IndexFlushAfterCreateTimeout by a comfortable margin (not equal to it).
	const createWatchdog = 20 * time.Second
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), createWatchdog)
	defer cancel()

	// Create job data exactly as scheduler submit does
	jobID := "SCH-TEST-TIMEOUT-001"
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              "scheduler_job",
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:            "active",
		objects.FieldKeyJobType:           JobTypeRunWrapper,
		objects.FieldKeyTriggerType:       "immediate",
		objects.FieldKeyCategory:          "manual",
		objects.FieldKeyExecutionMode:     "one_time",
		objects.FieldKeyMaxRuntimeSeconds: 10,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyTitle:             "Test Job",
		objects.FieldKeyDescription:       "Test job for timeout detection",
		objects.FieldKeyCommand:           "echo",
		objects.FieldKeyCommandArgs:       []string{"test"},
		objects.FieldKeyRetryCount:        0,
		objects.FieldKeyRetryDelaySeconds: 5,
		objects.FieldKeyCreatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:         "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:         "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginProject:     "zqk",
		objects.FieldKeyOriginSystem:      "zqk",
	}

	// Create job with timeout - this is where scheduler submit hangs
	createStart := time.Now()
	done := make(chan error, 1)

	goroutinelabels.StartTestGoroutine("test_storage_create", "creating storage object in test", func() {
		done <- storage.Create(ctx, secCtx, jobData)
	})

	timedOut, canceled, err := waitCreateResultWithPipeline(createWatchdog, done, ctx)
	if timedOut {
		t.Fatal("storage.Create() HUNG - this is the bug! Operation did not complete within watchdog")
	}
	if canceled {
		t.Fatal("context cancelled - storage.Create() may be waiting on something that never completes")
	}
	createDuration := time.Since(createStart)
	if err != nil {
		t.Fatalf("failed to create job: %v (took %v)", err, createDuration)
	}
	// CI/load can push creates past a few seconds without a hang; stay under the watchdog.
	if createDuration > createWatchdog {
		t.Errorf("job creation took too long: %v (expected < %v)", createDuration, createWatchdog)
	}
	t.Logf("Job created successfully in %v", createDuration)
}

// TestStorage_CreateSchedulerJob_WithValidation verifies job creation with validation enabled
func TestStorage_CreateSchedulerJob_WithValidation(t *testing.T) {
	// Not t.Parallel(): setupSchedulerCompleteTestEnvironment sets ZQK_TEST_ROOT via os.Setenv (process-global).
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	t.Cleanup(env.Cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	secCtx := env.SecurityContext
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	// Minimal valid job data
	jobID := "SCH-TEST-MINIMAL-001"
	jobData := map[string]any{
		objects.FieldKeyID:            jobID,
		objects.FieldKeyKind:          "scheduler_job",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyJobType:       JobTypeRunWrapper,
		objects.FieldKeyTriggerType:   "immediate",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyCommand:       "echo",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}

	// Test with timeout
	createStart := time.Now()
	done := make(chan error, 1)

	goroutinelabels.StartTestGoroutine("test_storage_create", "creating storage object in test", func() {
		done <- storage.Create(ctx, secCtx, jobData)
	})

	timedOut, canceled, err := waitCreateResultWithPipeline(10*time.Second, done, ctx)
	if timedOut {
		t.Fatal("storage.Create() HUNG with validation - operation did not complete within 10 seconds")
	}
	if canceled {
		t.Fatal("context cancelled - storage.Create() may be waiting on validation that never completes")
	}
	createDuration := time.Since(createStart)
	if err != nil {
		// Validation errors are OK - we're testing for hangs, not validation
		t.Logf("job creation returned error (may be validation): %v (took %v)", err, createDuration)
		if createDuration > 2*time.Second {
			t.Errorf("even with validation error, should return quickly: %v", createDuration)
		}
	} else {
		t.Logf("Job created successfully in %v", createDuration)
	}
}
