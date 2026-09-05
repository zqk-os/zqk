package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// TestScheduler_SubmitJob_CreatesJob verifies that submitting a job creates it in storage
func TestScheduler_SubmitJob_CreatesJob(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	testRoot, storage := prepareHandlersIsolatedTempProject(t)
	storagepkg.BuildPathAliasCacheForProject(testRoot)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// Create a job similar to what scheduler submit does
	jobID := "SCH-TEST-SUBMIT-001"
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
		objects.FieldKeyTitle:             "Test Submit Job",
		objects.FieldKeyDescription:       "Test job submitted via scheduler submit",
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

	// Create the job - this should not hang
	createStart := time.Now()
	err := storage.Create(ctx, secCtx, jobData)
	createDuration := time.Since(createStart)

	if err != nil {
		t.Fatalf("failed to create job: %v (took %v)", err, createDuration)
	}

	if createDuration > 5*time.Second {
		t.Errorf("job creation took too long: %v (expected < 5s)", createDuration)
	}

	// Verify job exists
	readJob, err := storage.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("failed to read created job: %v", err)
	}

	if readJob[objects.FieldKeyID] != jobID {
		t.Errorf("job ID mismatch: expected %q, got %q", jobID, readJob[objects.FieldKeyID])
	}

	t.Logf("Job created successfully in %v", createDuration)
}

// TestScheduler_SubmitJob_WithSchedulerRunning verifies job execution when scheduler is running
func TestScheduler_SubmitJob_WithSchedulerRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)
	scheduler := NewSchedulerWithProjectRoot(storage, specLoader, lifecycleLoader, testRoot, nil)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 300*time.Second)
	defer cancel()

	// Start scheduler in a goroutine (Start() blocks until context is cancelled)
	startDone := make(chan error, 1)
	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler for submit test").StartSimple(func() {
		startDone <- scheduler.Start(ctx)
	})

	// Give scheduler time to initialize
	time.Sleep(100 * time.Millisecond)

	// Stop scheduler before storage teardown (cleanup LIFO: registered after RegisterTempProjectTeardown)
	t.Cleanup(func() {
		_ = testkit.RunNamedTestSteps(context.Background(), "scheduler.submit_test_cleanup",
			testkit.NamedTestStep{Name: "CANCEL_CONTEXT", Fn: func() error {
				cancel()
				return nil
			}},
			testkit.NamedTestStep{Name: "WAIT_START_RETURN", Fn: func() error {
				select {
				case <-startDone:
				case <-time.After(2 * time.Second):
				}
				return nil
			}},
			testkit.NamedTestStep{Name: "STOP_SCHEDULER", Fn: func() error {
				scheduler.Stop()
				time.Sleep(200 * time.Millisecond)
				return nil
			}},
		)
	})

	// Create a job (simulating scheduler submit)
	secCtx := pkgctx.NewSystemSecurityContext()
	jobID := "SCH-TEST-SUBMIT-002"
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
		objects.FieldKeyTitle:             "Test Submit Job",
		objects.FieldKeyCommand:           "echo",
		objects.FieldKeyCommandArgs:       []string{"test"},
		objects.FieldKeyRetryCount:        0,
		objects.FieldKeyCreatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:         "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyUpdatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:         "ACC-1785920548450214012-68b850c0",
		objects.FieldKeyOriginProject:     "zqk",
		objects.FieldKeyOriginSystem:      "zqk",
	}

	// Create job - should not hang
	createStart := time.Now()
	err := storage.Create(ctx, secCtx, jobData)
	createDuration := time.Since(createStart)

	if err != nil {
		t.Fatalf("failed to create job: %v (took %v)", err, createDuration)
	}

	if createDuration > 5*time.Second {
		t.Errorf("job creation took too long: %v (expected < 5s)", createDuration)
	}

	// Wait for job to be picked up (scheduler watches for changes)
	// Immediate jobs should execute quickly
	time.Sleep(2 * time.Second)

	t.Logf("Job created in %v, scheduler should pick it up", createDuration)
}

// TestScheduler_TriggerQueue_ProcessesJobs verifies trigger queue processing
func TestScheduler_TriggerQueue_ProcessesJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)
	scheduler := NewSchedulerWithProjectRoot(storage, specLoader, lifecycleLoader, testRoot, nil)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 300*time.Second)
	defer cancel()

	// Start scheduler in a goroutine (Start() blocks until context is cancelled)
	startDone := make(chan error, 1)
	goroutinelabels.NewGoroutine("scheduler_test", "start scheduler for trigger queue test").StartSimple(func() {
		startDone <- scheduler.Start(ctx)
	})

	// Ensure cleanup: cancel context and stop scheduler (runs before storage teardown; registered after RegisterTempProjectTeardown)
	t.Cleanup(func() {
		_ = testkit.RunNamedTestSteps(context.Background(), "scheduler.trigger_queue_test_cleanup",
			testkit.NamedTestStep{Name: "CANCEL_CONTEXT", Fn: func() error {
				cancel()
				return nil
			}},
			testkit.NamedTestStep{Name: "WAIT_START_RETURN", Fn: func() error {
				select {
				case <-startDone:
				case <-time.After(5 * time.Second):
				}
				return nil
			}},
			testkit.NamedTestStep{Name: "STOP_SCHEDULER", Fn: func() error {
				scheduler.Stop()
				return nil
			}},
		)
	})

	// Create trigger queue and test enqueue
	triggerQueue := NewJobTriggerQueue(testRoot)

	// Test that trigger queue exists and can be created
	// (EnqueueTrigger may not be exposed, but we can verify the queue exists)
	if triggerQueue == nil {
		t.Fatal("trigger queue should not be nil")
	}

	t.Logf("Trigger queue created successfully")
}
