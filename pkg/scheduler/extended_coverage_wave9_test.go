package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_RetentionTolerance_ArchiveAndConfig(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-retention-archive-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
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

	// 1. isCatalogKind
	if !isCatalogKind("roadmap") || !isCatalogKind("milestone") || !isCatalogKind("question") {
		t.Error("expected catalog kinds to return true")
	}
	if isCatalogKind("agent_task") || isCatalogKind("audit_event") {
		t.Error("expected operational kinds to return false")
	}

	// 2. Seed agent tasks for archiveOldObjects
	for i := 0; i < 5; i++ {
		taskID := fmt.Sprintf("TASK-arc-%d", i)
		taskObj := map[string]any{
			objects.FieldKeyID:            taskID,
			objects.FieldKeyKind:          objects.KindAgentTask,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "completed",
			objects.FieldKeyCreatedAt:     time.Now().Add(-10 * time.Hour).Format(time.RFC3339),
			objects.FieldKeyTitle:         fmt.Sprintf("Task %d", i),
		}
		_ = sp.Create(ctx, secCtx, taskObj)
	}

	// 3. archiveOldObjects on agent_task
	archived := h.archiveOldObjects(ctx, secCtx, storageCtx, "SCH-job-arc", objects.KindAgentTask, time.Now().Add(time.Hour), 2, 5)
	t.Logf("archiveOldObjects archived: %d", archived)

	// 4. archiveOldObjects on priority_plan
	planObj := map[string]any{
		objects.FieldKeyID:            "PRI-arc-1",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusComplete,
		objects.FieldKeyCreatedAt:     time.Now().Add(-10 * time.Hour).Format(time.RFC3339),
		objects.FieldKeyTitle:         "Plan Arc 1",
	}
	_ = sp.Create(ctx, secCtx, planObj)
	_ = h.archiveOldObjects(ctx, secCtx, storageCtx, "SCH-job-arc", objects.KindPriorityPlan, time.Now().Add(time.Hour), 2, 5)

	// 5. mergeStrategyTolerance
	cfg := h.mergeStrategyTolerance(ctx, &config.RetentionToleranceConfig{}, "SCH-job-cfg")
	t.Logf("mergeStrategyTolerance loaded %d effective kinds", len(cfg))
}

func TestExtended_CVSPipelineTickSync_Comprehensive(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Helpers
	_ = pipelineTickAutoRepointEnabled(nil)
	_ = pipelineTickAutoRepointEnabled(map[string]any{EnvKeyPipelineTickAutoRepoint: "false"})
	_ = pipelineTickAutoRepointEnabled(map[string]any{EnvKeyPipelineTickAutoRepoint: "true"})
	ll := objects.GetGlobalLifecycleLoader()
	_ = shouldRepointOnTerminalTransition(ll, "completed")
	_ = shouldRepointOnTerminalTransition(ll, "active")

	// 2. Create SCH-cvs-pipeline-tick job in storage
	tickJob := map[string]any{
		objects.FieldKeyID:            CVSPipelineTickJobID,
		objects.FieldKeyKind:          objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyJobType:       JobTypeConvergenceSessionTick,
		objects.FieldKeyEnvironmentVariables: map[string]any{
			EnvKeyPipelineTickAutoRepoint:    "true",
			EnvKeyConvergenceSessionID:       "CVS-prior",
			EnvKeyPipelineTickTitleSubstring: "core",
		},
	}
	_ = sp.Create(ctx, secCtx, tickJob)

	// 3. Create target and replacement convergence sessions
	priorCvs := map[string]any{
		objects.FieldKeyID:            "CVS-prior",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "completed",
		objects.FieldKeyTitle:         "Core Convergence Prior",
	}
	_ = sp.Create(ctx, secCtx, priorCvs)

	activeCvs := map[string]any{
		objects.FieldKeyID:            "CVS-new-active",
		objects.FieldKeyKind:          objects.KindConvergenceSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyTitle:         "Core Convergence New Active",
	}
	_ = sp.Create(ctx, secCtx, activeCvs)

	// 4. Test terminal transition trigger
	_ = SyncCVSPipelineTickJobOnLifecycle(ctx, sp, "active", "completed", priorCvs)

	// 5. Test activate transition trigger
	_ = SyncCVSPipelineTickJobOnLifecycle(ctx, sp, "draft", "active", activeCvs)
}

func TestExtended_Notifications_FormattingAndDisplay(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	nd := NewNotificationDisplay(logger)

	// 1. formatDuration
	d1 := 500 * time.Millisecond
	if formatDuration(d1) != "500ms" {
		t.Errorf("expected 500ms, got %s", formatDuration(d1))
	}
	d2 := 5 * time.Second
	if formatDuration(d2) != "5s" {
		t.Errorf("expected 5s, got %s", formatDuration(d2))
	}

	// 2. Create completed notification with long description
	longDesc := "This is a long description that spans multiple lines.\nSecond line here.\nThird line here."
	notif1 := CreateJobNotification("SCH-notif-1", "test", "testing", notificationEventCompleted, PriorityHigh, 2*time.Second, nil, map[string]any{
		objects.FieldKeyJobTitle: "Test Notification Job",
		"job_description":        longDesc,
	})
	if notif1 == nil || notif1.Title == "" {
		t.Error("expected non-nil notification with title")
	}

	// 3. Create failed notification with test failures
	notif2 := CreateJobNotification("SCH-notif-2", "test", "testing", notificationEventFailed, PriorityCritical, 5*time.Second, fmt.Errorf("tests failed"), map[string]any{
		"is_test_failure": true,
		"test_failures":   []string{"TestA", "TestB"},
	})
	if notif2 == nil {
		t.Error("expected non-nil failed notification")
	}

	// 4. Create status update notification
	notif3 := CreateJobNotification("SCH-notif-3", "test", "testing", notificationEventStatusUpdate, PriorityLow, 0, nil, nil)
	if notif3 == nil {
		t.Error("expected non-nil status update notification")
	}

	// 5. Display notifications
	nd.Display(notif1)
	nd.Display(notif2)
	nd.Display(notif3)

	// Unknown priority
	notif4 := &JobNotification{
		JobID:    "SCH-unknown",
		Event:    notificationEventCompleted,
		Priority: NotificationPriority("unknown"),
	}
	nd.Display(notif4)
}

func TestExtended_JobTriggerQueue_Advanced(t *testing.T) {
	tmpDir := t.TempDir()
	queueDir := filepath.Join(tmpDir, paths.ProjectDataDir, TriggerQueueDir)
	queueFile := filepath.Join(queueDir, TriggerQueueFile)
	lockFile := filepath.Join(queueDir, TriggerQueueFile+".lock")

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	q := &JobTriggerQueue{
		projectRoot: tmpDir,
		queueFile:   queueFile,
		lockFile:    lockFile,
		logger:      logger,
	}

	// 1. SetCASDebounceInterval & shouldReconcileCASForBatch
	q.SetCASDebounceInterval(10 * time.Millisecond)
	if !q.shouldReconcileCASForBatch(true, true) {
		t.Error("expected true when missing job")
	}
	if !q.shouldReconcileCASForBatch(false, true) {
		t.Error("expected true when needsCAS and not debounced")
	}
	time.Sleep(20 * time.Millisecond)
	if !q.shouldReconcileCASForBatch(false, true) {
		t.Error("expected true after debounce elapsed")
	}

	// 2. Enqueue lifecycle requests
	err := q.EnqueueLifecycleTrigger("backlog_item", "draft", "active", map[string]any{"id": "BLI-1"})
	if err != nil {
		t.Fatalf("EnqueueLifecycleTrigger failed: %v", err)
	}

	// 3. EnqueueTriggerRequest & EnqueueTriggerRequestWithOrigin
	_ = q.EnqueueTriggerRequest("SCH-req-1")
	_ = q.EnqueueTriggerRequestWithOrigin("SCH-req-2", "pre_commit")
	_ = q.EnqueueTriggerRequests([]string{"SCH-req-3", "SCH-req-4"}, "manual")

	// 4. HasPendingTriggerWithOrigin
	has, _ := q.HasPendingTriggerWithOrigin("SCH-req-2", "pre_commit")
	t.Logf("HasPendingTriggerWithOrigin: %v", has)

	// 5. PeekTriggerRequests & DequeueTriggerRequests
	peeked, err := q.PeekTriggerRequests()
	if err != nil || len(peeked) == 0 {
		t.Errorf("expected peeked requests, got %v, err %v", len(peeked), err)
	}

	dequeued, err := q.DequeueTriggerRequests(2)
	if err != nil || len(dequeued) == 0 {
		t.Errorf("expected dequeued requests, got %v, err %v", len(dequeued), err)
	}

	// 6. EnqueueTriggerRequestStructs
	reqs := []JobTriggerRequest{
		{JobID: "SCH-batch-1", RequestedAt: time.Now()},
		{JobID: "SCH-batch-2", RequestedAt: time.Now()},
	}
	_ = q.EnqueueTriggerRequestStructs(reqs)
}
