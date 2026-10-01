package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/zqk-os/zqk/pkg/agentclaim"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_LifecycleCoordination_Filters(t *testing.T) {
	s := &Scheduler{}

	// Test matchesEventFilter
	if !s.matchesEventFilter("", "evType", "evKind") {
		t.Error("expected empty filter to match all")
	}
	if !s.matchesEventFilter("*", "evType", "evKind") {
		t.Error("expected * filter to match all")
	}
	if !s.matchesEventFilter("evType:evKind", "evType", "evKind") {
		t.Error("expected matching type and kind")
	}
	if s.matchesEventFilter("evType:wrongKind", "evType", "evKind") {
		t.Error("expected mismatch on kind")
	}
	if s.matchesEventFilter("wrongType:evKind", "evType", "evKind") {
		t.Error("expected mismatch on type")
	}
	if !s.matchesEventFilter("evType", "evType", "anyKind") {
		t.Error("expected single type filter to match")
	}
	if s.matchesEventFilter("evType", "wrongType", "anyKind") {
		t.Error("expected mismatch on single type")
	}
	if s.matchesEventFilter("a:b:c", "a", "b") {
		t.Error("expected 3-part filter to not match")
	}

	// Test matchesLifecycleFilter
	if !s.matchesLifecycleFilter("", "kind", "s1", "s2") {
		t.Error("expected empty filter to match")
	}
	if !s.matchesLifecycleFilter("*", "kind", "s1", "s2") {
		t.Error("expected * filter to match")
	}
	if !s.matchesLifecycleFilter("backlog_item:draft->active", "backlog_item", "draft", "active") {
		t.Error("expected exact transition match")
	}
	if s.matchesLifecycleFilter("backlog_item:draft->active", "milestone", "draft", "active") {
		t.Error("expected kind mismatch")
	}
	if s.matchesLifecycleFilter("backlog_item:draft->active", "backlog_item", "other", "active") {
		t.Error("expected fromState mismatch")
	}
	if s.matchesLifecycleFilter("backlog_item:draft->active", "backlog_item", "draft", "other") {
		t.Error("expected toState mismatch")
	}
	if !s.matchesLifecycleFilter("*:draft->active", "other_kind", "draft", "active") {
		t.Error("expected kind wildcard match")
	}
	if !s.matchesLifecycleFilter("backlog_item:*->active", "backlog_item", "anything", "active") {
		t.Error("expected fromState wildcard match")
	}
	if !s.matchesLifecycleFilter("backlog_item:draft->*", "backlog_item", "draft", "anything") {
		t.Error("expected toState wildcard match")
	}
	if !s.matchesLifecycleFilter("*:*->*", "any", "any", "any") {
		t.Error("expected full wildcard match")
	}
	// Invalid formats
	if s.matchesLifecycleFilter("invalid_no_colon", "k", "s1", "s2") {
		t.Error("expected invalid format to return false")
	}
	if s.matchesLifecycleFilter("k:invalid_no_arrow", "k", "s1", "s2") {
		t.Error("expected invalid transition format to return false")
	}

	// Test isConcurrentAllowed
	if !s.isConcurrentAllowed(JobTypeCachePrewarm, "") {
		t.Error("expected CachePrewarm to be allowed")
	}
	if !s.isConcurrentAllowed(JobTypeRunWrapper, CategoryTesting) {
		t.Error("expected RunWrapper testing to be allowed")
	}
	if s.isConcurrentAllowed(JobTypeRunWrapper, "other") {
		t.Error("expected RunWrapper non-testing to be false")
	}
	if s.isConcurrentAllowed(JobTypeAuditEventAggregation, "") {
		t.Error("expected AuditAggregation to be false")
	}
}

func TestExtended_LifecycleCoordination_TriggerAndExecution(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	s := &Scheduler{
		storage:     sp,
		logger:      logger,
		secCtx:      secCtx,
		projectRoot: tmpDir,
		jobs:        make(map[string]*ScheduledJob),
		cron:        cron.New(cron.WithSeconds(), cron.WithLocation(time.UTC)),
	}

	// Add event and lifecycle jobs
	eventJob := &ScheduledJob{
		ID:          "SCH-event-1",
		TriggerType: "event",
		EventFilter: "item:created",
		Enabled:     true,
		Command:     "echo",
		CommandArgs: []string{"event-ran"},
	}
	s.jobs[eventJob.ID] = eventJob

	lcJob := &ScheduledJob{
		ID:              "SCH-lc-1",
		TriggerType:     TriggerTypeLifecycle,
		LifecycleFilter: "backlog_item:*->completed",
		Enabled:         true,
		Command:         "echo",
		CommandArgs:     []string{"lc-ran"},
	}
	s.jobs[lcJob.ID] = lcJob

	// Test JobInCache
	if !s.JobInCache("SCH-event-1") {
		t.Error("expected job to be in cache")
	}
	if s.JobInCache("nonexistent") {
		t.Error("expected nonexistent job not to be in cache")
	}

	// TriggerJobByEvent with no triggeredPool
	err = s.TriggerJobByEvent(ctx, "item", "created", map[string]any{"id": "123"})
	if err != nil {
		t.Errorf("TriggerJobByEvent failed: %v", err)
	}

	// TriggerJobByLifecycle with no triggeredPool
	err = s.TriggerJobByLifecycle(ctx, "backlog_item", "active", "completed", map[string]any{
		objects.FieldKeyID: "BLI-123",
	})
	if err != nil {
		t.Errorf("TriggerJobByLifecycle failed: %v", err)
	}

	// EvtDataKeyForTesting
	_ = EvtDataKeyForTesting()

	// Test recordHealthMetric
	s.recordHealthMetric(ctx, time.Now(), 1, 0, 0, 0, 10*time.Millisecond, 10, 5, 1000, 2000)

	// Test autoCompleteMilestonesForBacklogItem
	// Create a milestone object
	mObj := map[string]any{
		objects.FieldKeyID:            "MLS-1",
		objects.FieldKeyKind:          objects.KindMilestone,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Milestone 1",
	}
	_ = sp.Create(ctx, secCtx, mObj)

	// Create a completed backlog item referencing MLS-1
	bliObj := map[string]any{
		objects.FieldKeyID:            "BLI-1",
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusComplete,
		objects.FieldKeyTitle:         "Item 1",
		objects.FieldKeyMilestoneRefs: []any{"MLS-1"},
	}
	_ = sp.Create(ctx, secCtx, bliObj)

	s.autoCompleteMilestonesForBacklogItem(ctx, bliObj)

	// Test checkAndRecoverMissedJobs
	timerJob := &ScheduledJob{
		ID:           "SCH-timer-1",
		TriggerType:  "timer",
		ScheduleExpr: "0 0 * * *", // midnight
		Enabled:      true,
		Command:      "echo",
	}
	s.jobs[timerJob.ID] = timerJob
	s.running = true

	missed, recovered := s.checkAndRecoverMissedJobs(ctx)
	t.Logf("Missed: %d, Recovered: %d", missed, recovered)

	// Test buildCompletionMapFromActivityCache
	completions := s.buildCompletionMapFromActivityCache()
	if completions == nil {
		t.Error("expected non-nil completion map")
	}

	// Test buildCompletionMap with query result
	now := time.Now().Truncate(time.Second)
	qr := &storagepkg.QueryResult{
		Objects: []map[string]any{
			{
				"event_type": "scheduler_job_completed",
				"target_id":  "SCH-timer-1",
				"created_at": now.Add(1 * time.Minute).Format(time.RFC3339),
			},
		},
	}
	cMap := s.buildCompletionMap(qr)
	if _, ok := cMap["SCH-timer-1"]; !ok {
		t.Error("expected SCH-timer-1 in cMap")
	}

	// Test hasCompletionEventForTime
	hasComp := s.hasCompletionEventForTime("SCH-timer-1", now, 5*time.Minute, qr, now.Add(1*time.Minute))
	if !hasComp {
		t.Error("expected true for hasCompletionEventForTime")
	}
	hasCompNil := s.hasCompletionEventForTime("SCH-timer-1", now.Add(-10*time.Minute), 5*time.Minute, nil, now)
	if hasCompNil {
		t.Error("expected false for hasCompletionEventForTime out of grace period")
	}

	// Test watch methods with pre-canceled context
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	s.watchJobChanges(cancelCtx)
	s.watchSchedulerConfig(cancelCtx)
	s.keepAliveHeartbeat(cancelCtx)
	s.healthMonitor(cancelCtx)
	s.maybeReconcileSchedulerJobCASIndexBeforeReload(ctx)
	time.Sleep(500 * time.Millisecond)
}

func TestExtended_HourglassWatchAndEscalate(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	s := &Scheduler{
		storage:     sp,
		logger:      logger,
		secCtx:      secCtx,
		projectRoot: tmpDir,
	}

	// Test actionForExpiredTimer
	if actionForExpiredTimer(agentclaim.TimerTypeCheckin) != actionWakeOrchestrator {
		t.Error("expected actionWakeOrchestrator for checkin")
	}
	if actionForExpiredTimer(timerTypeDeadline) != actionEscalateDeadline {
		t.Error("expected actionEscalateDeadline for deadline")
	}
	if actionForExpiredTimer("unknown") != actionKillStuckProcess {
		t.Error("expected actionKillStuckProcess for unknown")
	}

	// Setup hourglass directory
	hgDir := filepath.Join(tmpDir, ".zqk", "scheduler", "hourglass")
	if err := os.MkdirAll(hgDir, 0755); err != nil {
		t.Fatalf("failed to create hourglass dir: %v", err)
	}

	// 1. Expired deadline timer file
	deadlineFile := filepath.Join(hgDir, "deadline-test.json")
	dInfo := map[string]any{
		"task_id":    "TASK-deadline-1",
		"pid":        os.Getpid(),
		"expires_at": time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		"type":       timerTypeDeadline,
		"kind":       objects.KindAgentTask,
	}
	dBytes, _ := json.Marshal(dInfo)
	_ = os.WriteFile(deadlineFile, dBytes, 0644)

	// Create corresponding task in storage
	_ = sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            "TASK-deadline-1",
		objects.FieldKeyKind:          objects.KindAgentTask,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Deadline task 1",
	})

	// 2. Expired checkin timer file
	checkinFile := filepath.Join(hgDir, "checkin-test.json")
	cInfo := map[string]any{
		"task_id":        "TASK-checkin-1",
		"expires_at":     time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		"type":           agentclaim.TimerTypeCheckin,
		"claimed_by":     "agent-seat-1",
		"window_seconds": 60,
	}
	cBytes, _ := json.Marshal(cInfo)
	_ = os.WriteFile(checkinFile, cBytes, 0644)

	// Create corresponding task in storage
	_ = sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            "TASK-checkin-1",
		objects.FieldKeyKind:          objects.KindAgentTask,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyClaimedBy:     "agent-seat-1",
		objects.FieldKeyTitle:         "Checkin task 1",
	})

	// 3. Expired stuck process timer file
	stuckFile := filepath.Join(hgDir, "stuck-test.json")
	sInfo := map[string]any{
		"task_id":    "TASK-stuck-1",
		"pid":        999999, // non-existent pid
		"expires_at": time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		"type":       "subagent",
	}
	sBytes, _ := json.Marshal(sInfo)
	_ = os.WriteFile(stuckFile, sBytes, 0644)

	// Create corresponding task in storage with steps
	_ = sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            "TASK-stuck-1",
		objects.FieldKeyKind:          objects.KindAgentTask,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyStatus: "pending_implementation",
			},
		},
	})

	// 4. Corrupted json file
	corruptFile := filepath.Join(hgDir, "corrupt.json")
	_ = os.WriteFile(corruptFile, []byte("invalid json"), 0644)

	// 5. Unparseable time file
	unparseFile := filepath.Join(hgDir, "badtime.json")
	_ = os.WriteFile(unparseFile, []byte(`{"expires_at":"invalid-date"}`), 0644)

	// Run checkHourglassTimers
	s.checkHourglassTimers(ctx)

	// Verify deadline task got processed
	dTask, err := sp.Read(ctx, secCtx, "TASK-deadline-1")
	if err == nil {
		t.Logf("Deadline task read back status: %v", dTask[objects.FieldKeyStatus])
	}

	// Verify stuck task got set to error
	stuckTask, err := sp.Read(ctx, secCtx, "TASK-stuck-1")
	if err == nil {
		if stuckTask[objects.FieldKeyStatus] != objects.ObjectStatusError {
			t.Errorf("expected stuck task status to be error, got: %v", stuckTask[objects.FieldKeyStatus])
		}
	}

	// Test handleMissedCheckin directly with edge cases
	s.handleMissedCheckin(ctx, secCtx, filepath.Join(hgDir, "nonexistent.json"))

	// Test sweepStaleAgentTasks
	staleTask := map[string]any{
		objects.FieldKeyID:            "TASK-stale-old",
		objects.FieldKeyKind:          objects.KindAgentTask,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "in_progress",
		objects.FieldKeyUpdatedAt:     time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
	}
	_ = sp.Create(ctx, secCtx, staleTask)
	s.sweepStaleAgentTasks(ctx)

	// Test startHourglassWatcher with canceled context
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	s.startHourglassWatcher(cancelCtx, nil)
}

func TestExtended_RetentionTolerance_MaxCountAndCleanup(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	h := &RetentionToleranceHandler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	// Test helpers
	_ = isHighVolumeKind("audit_event")
	_ = skipArchiveForOldestIDsPath("audit_event", nil)
	_ = skipArchiveForOldestIDsPath("audit_event", []string{"active"})

	// Create test objects of kind "test_retention_kind"
	testKind := "test_retention_kind"
	for i := 0; i < 6; i++ {
		status := "completed"
		if i == 0 {
			status = "active" // protected
		}
		obj := map[string]any{
			objects.FieldKeyID:            testKind + "-" + string(rune('a'+i)),
			objects.FieldKeyKind:          testKind,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        status,
			objects.FieldKeyCreatedAt:     time.Now().Add(-time.Duration(10-i) * time.Hour).Format(time.RFC3339),
		}
		_ = sp.Create(ctx, secCtx, obj)
	}

	// Run enforceMaxCount with count > maxCount and protectStatuses
	deleted, err := h.enforceMaxCount(ctx, secCtx, storageCtx, "SCH-job-ret", testKind, 2, []string{"active"}, 2, 5, 1)
	if err != nil {
		t.Logf("enforceMaxCount returned error: %v", err)
	}
	t.Logf("enforceMaxCount deleted: %d", deleted)

	// Create more objects to test cleanupOldObjects
	for i := 0; i < 4; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            testKind + "-old-" + string(rune('a'+i)),
			objects.FieldKeyKind:          testKind,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "done",
			objects.FieldKeyCreatedAt:     time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
		}
		_ = sp.Create(ctx, secCtx, obj)
	}

	// Test cleanupOldObjects
	cleaned := h.cleanupOldObjects(ctx, secCtx, storageCtx, "SCH-job-ret", testKind, time.Now().Add(-12*time.Hour), []string{"active"}, 2, 5, 1)
	t.Logf("cleanupOldObjects deleted: %d", cleaned)

	// Test cleanupOldObjectsSlowPath directly
	cleanedSlow := h.cleanupOldObjectsSlowPath(ctx, secCtx, storageCtx, "SCH-job-ret", testKind, []string{"active"}, 2, 5, 1, time.Now().Format(time.RFC3339))
	t.Logf("cleanupOldObjectsSlowPath deleted: %d", cleanedSlow)
}

func TestExtended_JobStateRegistry_Edges(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	// Create sample state files to test Summarize
	_ = reg.RegisterExecution("SCH-test-1", "exec-1", 1234)
	_ = reg.CompleteExecution("SCH-test-1", "exec-1", jobExecutionStateCompleted)
	_ = reg.RegisterExecution("SCH-test-2", "exec-2", 1235)
	_ = reg.CompleteExecution("SCH-test-2", "exec-2", jobExecutionStateFailed)
	_ = reg.DeferExecution("SCH-test-3", "rate limited", nil)

	sum, err := reg.Summarize(1)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	t.Logf("Summary: %+v", sum)

	st, err := reg.GetExecutionState("SCH-test-1")
	if err != nil {
		t.Fatalf("GetExecutionState failed: %v", err)
	}
	t.Logf("State: %+v", st)

	_ = reg.cleanupCompletedBestEffort()
}
