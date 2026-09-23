package scheduler

import (
	"context"
	"os"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_LifecycleCoordination_Deep(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-lc-coord-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("storage failed: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	s := &Scheduler{
		storage:        sp,
		logger:         logger,
		secCtx:         secCtx,
		projectRoot:    tmpDir,
		jobs:           make(map[string]*ScheduledJob),
		handlerFactory: NewHandlerFactory(sp, nil, nil, tmpDir, logger, nil, nil, nil, nil),
	}

	// 1. queryJobCompletionEvents fallback to storage
	res, err := s.queryJobCompletionEvents(ctx)
	if err != nil {
		t.Errorf("expected no error from queryJobCompletionEvents fallback, got %v", err)
	}
	if res == nil {
		t.Error("expected non-nil result")
	}

	// 2. TriggerJobByEvent
	job1 := &ScheduledJob{
		ID:          "SCH-evt-1",
		TriggerType: "event",
		EventFilter: "create:agent_task",
		Enabled:     true,
	}
	job2 := &ScheduledJob{
		ID:          "SCH-evt-2",
		TriggerType: "event",
		EventFilter: "*",
		Enabled:     true,
	}
	job3 := &ScheduledJob{
		ID:          "SCH-evt-3",
		TriggerType: "event",
		EventFilter: "delete:priority_plan",
		Enabled:     true,
	}

	s.jobsMu.Lock()
	s.jobs[job1.ID] = job1
	s.jobs[job2.ID] = job2
	s.jobs[job3.ID] = job3
	s.jobsMu.Unlock()

	// A. When s.triggeredPool is nil
	err = s.TriggerJobByEvent(ctx, "create", objects.KindAgentTask, map[string]any{"key": "val"})
	if err != nil {
		t.Errorf("expected nil err when pool is nil, got %v", err)
	}

	// B. When s.triggeredPool is active
	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 100})
	pool := budget.NewPool("test_triggered_pool", "testing", 2, 10)
	pool.Start(ctx)
	defer pool.Stop()
	s.triggeredPool = pool

	err = s.TriggerJobByEvent(ctx, "create", objects.KindAgentTask, map[string]any{"key": "val"})
	if err != nil {
		t.Errorf("expected nil err when pool is active, got %v", err)
	}

	// 3. recoverMissedJob
	timerJob := &ScheduledJob{
		ID:                "SCH-timer-rec-1",
		JobType:           "maintenance",
		TriggerType:       "timer",
		ScheduleExpr:      "0 * * * *",
		MaxRuntimeSeconds: 1,
	}
	s.recoverMissedJob(timerJob)

	// 4. healthMonitor with canceled context
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	s.healthMonitor(canceledCtx)
}
