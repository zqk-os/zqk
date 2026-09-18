package scheduler

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

type listOmittingStorage struct {
	storagepkg.ObjectStorageProvider
	omittedID string
}

func (s *listOmittingStorage) List(ctx context.Context, secCtx *storagepkg.SecurityContext, storageCtx *storagepkg.StorageContext, filter storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	res, err := s.ObjectStorageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}
	filtered := make([]map[string]any, 0, len(res.Objects))
	for _, item := range res.Objects {
		if item[objects.FieldKeyID] != s.omittedID {
			filtered = append(filtered, item)
		}
	}
	return &storagepkg.QueryResult{
		Objects: filtered,
		Groups:  res.Groups,
		Meta:    res.Meta,
	}, nil
}

// TestExpediteOneShot_ReachesExecuteJobWhenCeilingReachedAndListOmitsID verifies that
// immediate one-shot CLI jobs with callbacks reach executeJob even when runtime.NumGoroutine()
// is at or above the concurrency ceiling and storage.List omits the job ID (CAS list lag).
// TRACK: TDE-1789630460110488000-1f2e7aa3
// TRACK: TDE-1789621330341821000-20a78769
func TestExpediteOneShot_ReachesExecuteJobWhenCeilingReachedAndListOmitsID(t *testing.T) {
	sched, testRoot, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"developer"}, []string{"read:scheduler_job", "execute:scheduler_job", "write:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	// Set up pools on sched
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 64})
	goroutinelabels.SetDefaultBudget(budget)
	sched.triggeredPool = budget.NewPool("test_triggered", "test", 4, 16)
	sched.triggeredPriorityPool = budget.NewPool("test_priority", "test", 4, 16)
	sched.triggeredPool.Start(ctx)
	sched.triggeredPriorityPool.Start(ctx)
	t.Cleanup(func() {
		sched.triggeredPool.Stop()
		sched.triggeredPriorityPool.Stop()
	})

	jobID := "SCH-1789645000000000000"
	markerFile := filepath.Join(testRoot, "callback_done.marker")

	// Wrap storage to simulate CAS list lag (List omits jobID)
	sched.storage = &listOmittingStorage{
		ObjectStorageProvider: sched.storage,
		omittedID:             jobID,
	}

	created := time.Now().UTC()
	jobData := map[string]any{
		objects.FieldKeyID:                   jobID,
		objects.FieldKeyKind:                 objects.KindSchedulerJob,
		objects.FieldKeyTitle:                "Expedite one-shot integration",
		objects.FieldKeyStatus:               objects.ObjectStatusActive,
		objects.FieldKeyJobType:              JobTypeRunWrapper,
		objects.FieldKeyTriggerType:          TriggerTypeImmediate,
		objects.FieldKeyCategory:             CategoryManual,
		objects.FieldKeyExecutionMode:        ExecutionModeOneTime,
		objects.FieldKeyMaxRuntimeSeconds:    30,
		objects.FieldKeyEnabled:              true,
		objects.FieldKeyCommand:              "echo expedite_success",
		objects.FieldKeyCallbackOnCompletion: "touch " + markerFile,
		objects.FieldKeyCreatedAt:            created.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:            "ACC-TEST",
		objects.FieldKeyUpdatedAt:            created.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:            "ACC-TEST",
		objects.FieldKeyOriginProject:        validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:         validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:        objects.DefaultSchemaVersion,
	}

	if err := sched.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("create fixture job: %v", err)
	}

	queue := NewJobTriggerQueue(testRoot).(*JobTriggerQueue)
	if err := queue.EnqueueTriggerRequestWithOrigin(jobID, TriggerOriginCLISubmit); err != nil {
		t.Fatalf("EnqueueTriggerRequestWithOrigin: %v", err)
	}

	// Elevate goroutines to above concurrency.DefaultGoroutineCeiling
	hold := make(chan struct{})
	defer close(hold)
	for runtime.NumGoroutine() < concurrency.DefaultGoroutineCeiling+5 {
		goroutinelabels.NewGoroutine("test.elevate_goroutines", "elevate goroutines for test").StartSimple(func() { <-hold })
	}

	if sched.JobInCache(jobID) {
		t.Fatal("job should not be in cache before drain")
	}

	// Drain trigger batch: should admit via storage.Read, bypass goroutine ceiling via priorityDispatchJob,
	// and submit to priority pool for immediate execution.
	queue.drainAndProcessTriggerBatch(ctx, sched)

	// Wait for callback marker file to be created by executeJob completion
	deadline := time.Now().Add(5 * time.Second)
	var executed bool
	for time.Now().Before(deadline) {
		if fileutil.Exists(markerFile) {
			executed = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !executed {
		t.Fatal("expected executeJob to be reached and completion callback invoked despite NumGoroutine at ceiling and List omitting ID")
	}

	// Wait for last_run_at to be populated in storage after completion
	var lastRunRecorded bool
	for time.Now().Before(deadline) {
		updatedJob, err := sched.storage.Read(ctx, secCtx, jobID)
		if err == nil && updatedJob[objects.FieldKeyLastRunAt] != nil {
			lastRunRecorded = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !lastRunRecorded {
		t.Fatal("expected last_run_at to be populated on executed job in storage")
	}
}

// TestDrainAndProcessTriggerBatch_ReenqueuesOnDeadlineExceeded verifies that if TriggerJob
// returns DeadlineExceeded (e.g. from resource contention), the trigger request is re-enqueued
// rather than silently dropped.
// TRACK: TDE-1789630460110488000-1f2e7aa3
func TestDrainAndProcessTriggerBatch_ReenqueuesOnDeadlineExceeded(t *testing.T) {
	setDispatchResourceWaitMaxForTest(50 * time.Millisecond)
	t.Cleanup(func() { setDispatchResourceWaitMaxForTest(0) })

	sched, testRoot, cleanup := setupTestScheduler(t)
	t.Cleanup(cleanup)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"developer"}, []string{"read:scheduler_job", "execute:scheduler_job", "write:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	jobID := "SCH-1789645000000000001"
	created := time.Now().UTC()
	// CategoryTesting with reusable execution mode does NOT bypass ceiling
	jobData := map[string]any{
		objects.FieldKeyID:                jobID,
		objects.FieldKeyKind:              objects.KindSchedulerJob,
		objects.FieldKeyTitle:             "Testing category job",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyJobType:           JobTypeRunWrapper,
		objects.FieldKeyTriggerType:       TriggerTypeImmediate,
		objects.FieldKeyCategory:          CategoryTesting,
		objects.FieldKeyExecutionMode:     "reusable",
		objects.FieldKeyMaxRuntimeSeconds: 30,
		objects.FieldKeyEnabled:           true,
		objects.FieldKeyCommand:           "echo test",
		objects.FieldKeyCreatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyCreatedBy:         "ACC-TEST",
		objects.FieldKeyUpdatedAt:         created.Format(time.RFC3339),
		objects.FieldKeyUpdatedBy:         "ACC-TEST",
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}

	if err := sched.storage.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, jobData); err != nil {
		t.Fatalf("create fixture job: %v", err)
	}

	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 64})
	goroutinelabels.SetDefaultBudget(budget)
	sched.triggeredPool = budget.NewPool("test_triggered", "test", 1, 1)
	sched.triggeredPool.Start(ctx)
	t.Cleanup(func() { sched.triggeredPool.Stop() })

	// Load job into cache
	if err := sched.loadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("load jobs: %v", err)
	}

	queue := NewJobTriggerQueue(testRoot).(*JobTriggerQueue)
	if err := queue.EnqueueTriggerRequestWithOrigin(jobID, TriggerOriginCLISubmit); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// Elevate goroutines to ceiling so WaitUnderGoroutineCeiling times out with DeadlineExceeded
	hold := make(chan struct{})
	for runtime.NumGoroutine() < concurrency.DefaultGoroutineCeiling+5 {
		goroutinelabels.NewGoroutine("test.elevate_goroutines", "elevate goroutines for test").StartSimple(func() { <-hold })
	}

	// Drain: non-priority job will fail WaitUnderGoroutineCeiling with DeadlineExceeded
	queue.drainAndProcessTriggerBatch(ctx, sched)

	// Release elevated goroutines
	close(hold)

	// Verify request was re-enqueued and not dropped
	pending, err := queue.DequeueTriggerRequests(-1)
	if err != nil {
		t.Fatalf("DequeueTriggerRequests: %v", err)
	}
	if len(pending) != 1 || pending[0].JobID != jobID {
		t.Fatalf("expected trigger to be re-enqueued after DeadlineExceeded, got %+v", pending)
	}
}
