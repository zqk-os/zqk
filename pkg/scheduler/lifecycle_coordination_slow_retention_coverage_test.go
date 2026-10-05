package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_LifecycleCoordination_SlowRetention(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	secCtx := pkgctx.NewSystemSecurityContext()
	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)

	// 1. Cron parsing
	c1, err1 := sched.parseCronSchedule("0 * * * *")
	if err1 != nil || c1 == nil {
		t.Errorf("expected valid cron, got err: %v", err1)
	}
	_, err2 := sched.parseCronSchedule("invalid-cron-expr")
	if err2 == nil {
		t.Errorf("expected error for invalid cron")
	}

	// 2. isConcurrentAllowed
	_ = sched.isConcurrentAllowed(JobTypeCachePrewarm, CategoryMaintenance)
	_ = sched.isConcurrentAllowed(JobTypeCleanup, CategoryMaintenance)
	_ = sched.isConcurrentAllowed("unknown_type", "unknown_category")

	// 3. Event and lifecycle matching filters
	_ = sched.matchesEventFilter("created", "created", "audit_event")
	_ = sched.matchesEventFilter("created:audit_event", "created", "audit_event")
	_ = sched.matchesEventFilter("*", "created", "audit_event")
	_ = sched.matchesEventFilter("other", "created", "audit_event")

	_ = sched.matchesLifecycleFilter("draft:active", "backlog_item", "draft", "active")
	_ = sched.matchesLifecycleFilter("backlog_item:draft:active", "backlog_item", "draft", "active")
	_ = sched.matchesLifecycleFilter("*", "backlog_item", "draft", "active")
	_ = sched.matchesLifecycleFilter("completed:*", "backlog_item", "draft", "active")

	// 4. Job collection & missed jobs logic
	sched.jobsMu.Lock()
	timerJob := &ScheduledJob{
		ID:            "SCH-timer-job-1",
		JobType:       JobTypeCachePrewarm,
		Category:      CategoryMaintenance,
		TriggerType:   "timer",
		ScheduleExpr:  "0 * * * *",
		ExecutionMode: "reusable",
		Enabled:       true,
	}
	sched.jobs[timerJob.ID] = timerJob
	sched.jobsMu.Unlock()

	timerJobs, ok := sched.collectTimerJobs()
	if !ok || len(timerJobs) == 0 {
		t.Errorf("expected timer jobs to be collected")
	}

	auditRes := &storagepkg.QueryResult{
		Objects: []map[string]any{
			{
				"event_type": "scheduler_job_completed",
				"target_id":  timerJob.ID,
				"created_at": time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			},
			{
				"event_type": "scheduler_job_failed",
				"target_id":  "SCH-other",
				"created_at": time.Now().Add(-20 * time.Minute).Format(time.RFC3339),
			},
		},
	}
	_ = sched.buildCompletionMapFromActivityCache()
	compMap := sched.buildCompletionMap(auditRes)
	if compMap == nil {
		t.Fatalf("expected non-nil completion map")
	}

	now := time.Now()
	_ = sched.isJobMissed(timerJob, now, 2*time.Hour, compMap, auditRes)
	_ = sched.hasCompletionEventForTime(timerJob.ID, now, 5*time.Minute, auditRes, time.Time{})
	_ = sched.countMissedJobs(timerJobs, now, 2*time.Hour, compMap, auditRes)

	sched.recoverMissedJob(timerJob)

	// 5. Record health metric
	sched.recordHealthMetric(ctx, now, 1, 0, 0, 0, 10*time.Millisecond, 10, 5, 1024, 2048)

	// 6. checkAndRecoverMissedJobs
	checked, recovered := sched.checkAndRecoverMissedJobs(ctx)
	t.Logf("checkAndRecoverMissedJobs checked: %d, recovered: %d", checked, recovered)

	// 7. maybeReconcileSchedulerJobCASIndexBeforeReload
	sched.maybeReconcileSchedulerJobCASIndexBeforeReload(ctx)

	// 8. Event triggers
	_ = sched.TriggerJobByEvent(ctx, "created", "audit_event", map[string]any{
		objects.FieldKeyID: "AUD-123",
	})

	// 9. Lifecycle triggers & milestone auto-completion
	bliObj := map[string]any{
		objects.FieldKeyID:            "BLI-test-auto",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusComplete,
		objects.FieldKeyMilestoneRef:  "MLS-test-auto",
	}
	_ = sp.Create(ctx, secCtx, bliObj)

	mlsObj := map[string]any{
		objects.FieldKeyID:            "MLS-test-auto",
		objects.FieldKeyKind:          objects.KindMilestone,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
	}
	_ = sp.Create(ctx, secCtx, mlsObj)

	_ = sched.TriggerJobByLifecycle(ctx, objects.KindBacklogItem, "active", objects.ObjectStatusComplete, bliObj)
	sched.handleLifecycleMatrixTriggers(ctx, objects.KindBacklogItem, "active", objects.ObjectStatusComplete, bliObj)
	sched.autoCompleteMilestonesForBacklogItem(ctx, bliObj)
}

func TestExtended_RetentionCleanup_SlowPath(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	secCtx := pkgctx.NewSystemSecurityContext()

	h := NewRetentionToleranceHandler(sp, tmpDir).(*RetentionToleranceHandler)

	// Create objects of a non-high-volume kind (KindMilestone)
	cutoff := time.Now().Add(1 * time.Hour)
	cutoffStr := cutoff.Format(time.RFC3339)

	m1 := map[string]any{
		objects.FieldKeyID:            "MLS-slow-1",
		objects.FieldKeyKind:          objects.KindMilestone,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusArchived,
		objects.FieldKeyCreatedAt:     time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
	}
	m2 := map[string]any{
		objects.FieldKeyID:            "MLS-slow-2",
		objects.FieldKeyKind:          objects.KindMilestone,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
		objects.FieldKeyCreatedAt:     time.Now().Add(-3 * time.Hour).Format(time.RFC3339),
	}
	_ = sp.Create(ctx, secCtx, m1)
	_ = sp.Create(ctx, secCtx, m2)

	// 1. cleanupOldObjectsSlowPath directly
	deletedSlow := h.cleanupOldObjectsSlowPath(ctx, secCtx, nil, "SCH-ret-job", objects.KindMilestone, nil, 10, 2, 2, cutoffStr)
	t.Logf("cleanupOldObjectsSlowPath deleted: %d", deletedSlow)

	// 2. cleanupOldObjects on non-high-volume kind
	deletedClean := h.cleanupOldObjects(ctx, secCtx, nil, "SCH-ret-job", objects.KindDecision, cutoff, nil, 10, 2, 2)
	t.Logf("cleanupOldObjects deleted: %d", deletedClean)
}
