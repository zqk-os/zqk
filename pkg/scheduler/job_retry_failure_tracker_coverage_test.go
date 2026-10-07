package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"

	cron "github.com/robfig/cron/v3"
)

type mockExecHandler struct {
	fn func(ctx context.Context, job *ScheduledJob) error
}

func (m *mockExecHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	if m.fn != nil {
		return m.fn(ctx, job)
	}
	return nil
}

func TestExtended_JobExecution_RetryAndOutcomes(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	s := &Scheduler{
		cron:                cron.New(cron.WithSeconds(), cron.WithLocation(time.UTC)),
		notificationContext: NewNotificationContext(logger, nil),
		storage:             sp,
		logger:              logger,
		secCtx:              secCtx,
		projectRoot:         tmpDir,
		jobs:                make(map[string]*ScheduledJob),
	}

	job := &ScheduledJob{
		ID:                "SCH-retry-pipe-1",
		JobType:           "test",
		Category:          "testing",
		MaxRuntimeSeconds: 1,
	}

	// 1. Success handler
	hSuccess := &mockExecHandler{fn: func(ctx context.Context, job *ScheduledJob) error { return nil }}
	err = s.runSchedulerJobHandlerWithRetry(ctx, ctx, job, hSuccess, nil, time.Now())
	if err != nil {
		t.Errorf("expected nil err, got %v", err)
	}

	// 2. Retryable error ("hash mismatch")
	attempts := 0
	hRetry := &mockExecHandler{fn: func(ctx context.Context, job *ScheduledJob) error {
		attempts++
		if attempts < 2 {
			return errors.New("hash mismatch error, will retry")
		}
		return nil
	}}
	err = s.runSchedulerJobHandlerWithRetry(ctx, ctx, job, hRetry, nil, time.Now())
	if err != nil {
		t.Errorf("expected success after retry, got %v", err)
	}

	// 3. Panicking handler
	hPanic := &mockExecHandler{fn: func(ctx context.Context, job *ScheduledJob) error {
		panic("simulated panic in job")
	}}
	err = s.runSchedulerJobHandlerWithRetry(ctx, ctx, job, hPanic, nil, time.Now())
	if err == nil {
		t.Error("expected error for panicking handler")
	}

	// 4. Timeout context
	timeoutCtx, cancelTimeout := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancelTimeout()
	hBlock := &mockExecHandler{fn: func(ctx context.Context, job *ScheduledJob) error {
		return context.DeadlineExceeded
	}}
	err = s.runSchedulerJobHandlerWithRetry(ctx, timeoutCtx, job, hBlock, nil, time.Now().Add(-2*time.Second))
	if err == nil {
		t.Error("expected timeout error")
	}

	// 5. Canceled context
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	cancelFunc()
	err = s.runSchedulerJobHandlerWithRetry(ctx, cancelCtx, job, hBlock, nil, time.Now())
	if err == nil {
		t.Error("expected canceled error")
	}

	// 6. executeJobAfterHandlerReturns
	oneTimeJob := &ScheduledJob{
		ID:            "SCH-one-time",
		ExecutionMode: jobExecutionModeOneTime,
		Enabled:       true,
	}
	// Seed job in storage so disableJobInStorage and updateJobInStorage work
	_ = sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            "SCH-one-time",
		objects.FieldKeyKind:          objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		objects.FieldKeyEnabled:       true,
	})

	s.executeJobAfterHandlerReturns(ctx, oneTimeJob, nil, time.Now(), "exec-1", nil, "op-1", func() {})
	s.executeJobAfterHandlerReturns(ctx, oneTimeJob, errors.New("failed"), time.Now(), "exec-2", nil, "op-2", func() {})

	// 7. shouldInvokeConfiguredJobCallback
	preCommitCtx := ContextWithTriggerOrigin(ctx, TriggerOriginPreCommit)
	if !shouldInvokeConfiguredJobCallback(preCommitCtx, job) {
		t.Error("expected true for pre_commit origin")
	}
	if !shouldInvokeConfiguredJobCallback(ctx, oneTimeJob) {
		t.Error("expected true for one_time job")
	}
	perpetualJob := &ScheduledJob{ExecutionMode: "perpetual"}
	if shouldInvokeConfiguredJobCallback(ctx, perpetualJob) {
		t.Error("expected false for perpetual job without pre_commit")
	}
}

func TestExtended_CapOrchestrator_FailureTrackerAndIndex(t *testing.T) {
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, sp)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	h := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	// 1. Failure tracker
	h.recordFailure("review", errors.New("review failure 1"))
	tracker := h.readFailureTracker()
	if tracker.ConsecutiveFailures != 1 || tracker.LastStage != "review" {
		t.Errorf("unexpected tracker: %+v", tracker)
	}

	h.clearFailures()
	trackerAfter := h.readFailureTracker()
	if trackerAfter.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 failures after clear, got %d", trackerAfter.ConsecutiveFailures)
	}

	// 2. Open agent instructions index
	instObj := map[string]any{
		objects.FieldKeyID:               "AGI-test-1",
		objects.FieldKeyKind:             "agent_instruction",
		objects.FieldKeySchemaVersion:    objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:           objects.ObjectStatusProposed,
		objects.FieldKeyInstruction:      "Improve test coverage",
		objects.FieldKeyPriorityPlanRefs: []any{"PRI-test"},
	}
	_ = sp.Create(ctx, secCtx, instObj)

	idx := h.buildOpenAgentInstructionIndex(ctx)
	if idx == nil {
		t.Fatal("expected non-nil instruction index")
	}
	hasInst := h.hasOpenAgentInstruction(ctx, "PRI-test", "", "Improve")
	t.Logf("hasOpenAgentInstruction: %v", hasInst)

	// 3. Open agent task index
	taskObj := map[string]any{
		objects.FieldKeyID:               "TASK-cap-1",
		objects.FieldKeyKind:             objects.KindAgentTask,
		objects.FieldKeySchemaVersion:    objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:           "in_progress",
		objects.FieldKeyPriorityPlanRefs: []any{"PRI-test"},
	}
	_ = sp.Create(ctx, secCtx, taskObj)

	taskIdx := h.buildOpenAgentTaskIndex(ctx)
	if taskIdx == nil {
		t.Fatal("expected non-nil task index")
	}
	hasTask := h.hasOpenTaskForPlan(ctx, "PRI-test", "TASK-cap-1")
	t.Logf("hasOpenTaskForPlan: %v", hasTask)

	// 4. resolveWakePlanID & topOpenPlanBLIs
	_ = h.resolveWakePlanID(ctx, "TASK-cap-1")
	_ = h.topOpenPlanBLIs(ctx, "PRI-test", 5)

}
