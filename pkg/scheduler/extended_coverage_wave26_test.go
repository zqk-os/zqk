package scheduler

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_LifecycleCoordination_HealthAndHelpers(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-lifecycle-coord-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	s := &Scheduler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
		secCtx:      secCtx,
		jobs:        make(map[string]*ScheduledJob),
	}

	// 1. recordHealthMetric
	// Calling with all 0s should early-return without recording
	s.recordHealthMetric(ctx, time.Now(), 0, 0, 0, 0, 0, 0, 0, 0, 0)

	// Calling with real values should record a metric object
	s.recordHealthMetric(ctx, time.Now(), 2, 1, 1, 1, 50*time.Millisecond, 10, 5, 1024*1024, 2048*1024)

	// 2. parseCronSchedule
	// 5-field expression
	sched5, err := s.parseCronSchedule("*/5 * * * *")
	if err != nil || sched5 == nil {
		t.Errorf("parseCronSchedule 5-field failed: %v", err)
	}
	// 6-field expression
	sched6, err := s.parseCronSchedule("0 */5 * * * *")
	if err != nil || sched6 == nil {
		t.Errorf("parseCronSchedule 6-field failed: %v", err)
	}
	// Invalid expression
	_, err = s.parseCronSchedule("invalid cron")
	if err == nil {
		t.Errorf("expected error for invalid cron")
	}

	// 3. getStringFromMap helper
	m := map[string]any{"k1": "v1", "k2": 123}
	if getStringFromMap(m, "k1") != "v1" {
		t.Errorf("expected v1, got %s", getStringFromMap(m, "k1"))
	}
	if getStringFromMap(m, "k2") != "" {
		t.Errorf("expected empty string for non-string value")
	}
	if getStringFromMap(m, "missing") != "" {
		t.Errorf("expected empty string for missing key")
	}

	// 4. hasCompletionEventForTime
	now := time.Now()
	// Nil auditResult with zero lastCompletion
	if s.hasCompletionEventForTime("JOB-1", now, 5*time.Minute, nil, time.Time{}) {
		t.Errorf("expected false for zero completion time")
	}
	// Nil auditResult with lastCompletion within grace period
	if !s.hasCompletionEventForTime("JOB-1", now, 5*time.Minute, nil, now.Add(2*time.Minute)) {
		t.Errorf("expected true for completion within grace period")
	}
	// Nil auditResult with lastCompletion outside grace period
	if s.hasCompletionEventForTime("JOB-1", now, 5*time.Minute, nil, now.Add(10*time.Minute)) {
		t.Errorf("expected false for completion outside grace period")
	}

	// With populated auditResult
	auditRes := &storagepkg.QueryResult{
		Objects: []map[string]any{
			{
				"target_id":  "JOB-1",
				"event_type": "scheduler_job_completed",
				"created_at": now.Add(1 * time.Minute).Format(time.RFC3339),
			},
			{
				"target_id":  "JOB-2",
				"event_type": "scheduler_job_failed",
				"created_at": now.Add(1 * time.Minute).Format(time.RFC3339),
			},
		},
	}
	if !s.hasCompletionEventForTime("JOB-1", now, 5*time.Minute, auditRes, time.Time{}) {
		t.Errorf("expected true for matched completed event in audit result")
	}
	if s.hasCompletionEventForTime("JOB-3", now, 5*time.Minute, auditRes, time.Time{}) {
		t.Errorf("expected false for unmatched job in audit result")
	}

	// 5. Job caching and lookup
	testJob := &ScheduledJob{
		ID:          "SCH-test-1",
		JobType:     "test",
		TriggerType: TriggerTypeManual,
		Enabled:     true,
	}
	s.jobs[testJob.ID] = testJob

	if !s.JobInCache("SCH-test-1") {
		t.Errorf("expected SCH-test-1 to be in cache")
	}
	if s.JobInCache("NONEXISTENT") {
		t.Errorf("expected NONEXISTENT to not be in cache")
	}
	jobLookup, ok := s.lookupJobInCache("SCH-test-1")
	if !ok || jobLookup.ID != "SCH-test-1" {
		t.Errorf("lookupJobInCache failed")
	}

	// 6. countMissedJobs
	lastRun := now.Add(-2 * time.Hour)
	missedJob := &ScheduledJob{
		ID:           "SCH-missed-1",
		JobType:      "periodic",
		TriggerType:  TriggerTypeTimer,
		ScheduleExpr: "*/1 * * * *",
		LastRunAt:    &lastRun,
		Enabled:      true,
	}
	missedCount := s.countMissedJobs([]*ScheduledJob{missedJob}, now, 1*time.Hour, map[string]time.Time{}, nil)
	t.Logf("missedCount: %d", missedCount)

	// 7. TriggerJob disabled job path
	disabledJob := &ScheduledJob{ID: "SCH-disabled", Enabled: false}
	s.jobs[disabledJob.ID] = disabledJob
	_ = s.TriggerJob(ctx, "SCH-disabled")
}

func TestExtended_HandlersRetentionMaxCount_DeepCoverage(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-retention-maxcount-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	h := &RetentionToleranceHandler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	// 1. isHighVolumeKind and skipArchiveForOldestIDsPath
	_ = isHighVolumeKind(objects.KindAgentTask)
	_ = skipArchiveForOldestIDsPath(objects.KindAgentTask, nil)
	_ = skipArchiveForOldestIDsPath(objects.KindAgentTask, []string{"active"})

	// 2. enforceMaxCountViaHVNoProtect when no IDs
	delHV, handled := h.enforceMaxCountViaHVNoProtect(ctx, secCtx, "JOB-hv", objects.KindAgentTask, 5, 2, 2, 10)
	if handled || delHV != 0 {
		t.Errorf("expected not handled when no IDs, got handled=%v del=%d", handled, delHV)
	}

	// 3. enforceMaxCountViaHVWithProtect when no cache
	delProt, handledProt := h.enforceMaxCountViaHVWithProtect(ctx, secCtx, "JOB-hv-prot", objects.KindAgentTask, 5, 2, 2, 10, []string{"active"})
	if handledProt || delProt != 0 {
		t.Errorf("expected not handled when no cache, got handled=%v del=%d", handledProt, delProt)
	}

	// 4. Seed some agent_task objects with non-protected status
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-max-count")
	bgCtx = pkgctx.WithPromoteOnCreate(bgCtx)

	for i := 1; i <= 6; i++ {
		taskID := fmt.Sprintf("ATK-maxcount-del-%d", i)
		if err := sp.Create(bgCtx, secCtx, map[string]any{
			objects.FieldKeyID:                 taskID,
			objects.FieldKeyKind:               objects.KindAgentTask,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:             objects.ObjectStatusError,
			objects.FieldKeyAssigneePersonaRef: "software_engineer",
			objects.FieldKeyTitle:              fmt.Sprintf("Deletable Task %d", i),
			objects.FieldKeyDescription:        "Description for max count deletion",
			objects.FieldKeyCreatedAt:          time.Now().Add(-time.Duration(10-i) * time.Hour).Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("create task failed: %v", err)
		}
	}

	// 5. enforceMaxCount with maxCount = 2 (should delete 4 oldest tasks)
	delCount, err := h.enforceMaxCount(ctx, secCtx, storageCtx, "JOB-enforce-max", objects.KindAgentTask, 2, nil, 2, 5, 2)
	if err != nil {
		t.Fatalf("enforceMaxCount failed: %v", err)
	}
	t.Logf("enforceMaxCount deleted: %d", delCount)

	// 6. enforceMaxCount with count <= maxCount (early return 0)
	delCountZero, err := h.enforceMaxCount(ctx, secCtx, storageCtx, "JOB-enforce-max-zero", objects.KindAgentTask, 100, nil, 2, 5, 2)
	if err != nil || delCountZero != 0 {
		t.Errorf("expected 0 deletes, got %d, err: %v", delCountZero, err)
	}

	// 7. enforceMaxCount with batch limits (maxBatches = 1, batchSize = 1)
	// Seed 4 more tasks
	for i := 10; i <= 13; i++ {
		taskID := fmt.Sprintf("ATK-maxcount-batch-%d", i)
		if err := sp.Create(bgCtx, secCtx, map[string]any{
			objects.FieldKeyID:                 taskID,
			objects.FieldKeyKind:               objects.KindAgentTask,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:             objects.ObjectStatusError,
			objects.FieldKeyAssigneePersonaRef: "software_engineer",
			objects.FieldKeyTitle:              fmt.Sprintf("Batch Task %d", i),
			objects.FieldKeyDescription:        "Description for batch task",
			objects.FieldKeyCreatedAt:          time.Now().Add(-time.Duration(20-i) * time.Hour).Format(time.RFC3339),
		}); err != nil {
			t.Fatalf("create batch task failed: %v", err)
		}
	}

	delBatch, err := h.enforceMaxCount(ctx, secCtx, storageCtx, "JOB-enforce-batch", objects.KindAgentTask, 1, nil, 1, 1, 1)
	if err != nil {
		t.Fatalf("enforceMaxCount with batch limit failed: %v", err)
	}
	t.Logf("enforceMaxCount with batch limit deleted: %d", delBatch)
}
