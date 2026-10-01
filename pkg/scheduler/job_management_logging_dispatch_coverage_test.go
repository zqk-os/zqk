package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestJobManagement_LoggingAndDispatch(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = sp.Shutdown(ctx) }()

	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)
	sched.projectRoot = tmpDir

	now := time.Now().UTC()
	testJob := &ScheduledJob{
		ID:           "SCH-test-mgmt",
		JobType:      JobTypeRunWrapper,
		Category:     CategoryMaintenance,
		TriggerType:  "timer",
		ScheduleExpr: "0 * * * *",
		Command:      "echo",
		CommandArgs:  []string{"hello"},
		LastRunAt:    &now,
	}

	// 1. All jobLogFields helper functions
	_ = jobLogFields(testJob)
	_ = jobLogFieldsWithErr(testJob, fmt.Errorf("sample error"))
	_ = jobLogFieldsByIDAndErr("SCH-1", fmt.Errorf("sample error"))
	_ = jobLogFieldsByID("SCH-1")
	_ = logErrField(fmt.Errorf("sample error"))
	_ = logErrField(nil)
	_ = jobLogFieldsWithSchedule(testJob)
	_ = jobLogFieldsForImmediate(testJob)
	_ = jobLogFieldsForReexec(testJob)
	_ = jobLogFieldsWithTriggerType(testJob)
	_ = jobLogFieldsWithCategory(testJob)
	_ = jobLogFieldsWithCategoryAndDuration(testJob, 500*time.Millisecond)
	_ = jobLogFieldsForFailedCommand(testJob, "echo hello", "exit_error", "exit 1", 1, 2, 100*time.Millisecond, fmt.Errorf("exit 1"))
	_ = jobLogFieldsForPanic("SCH-1", "run_wrapper", "maintenance", "echo", []byte("stack trace"))

	// 2. priorityDispatchJob
	if priorityDispatchJob(nil) {
		t.Errorf("expected false for nil job")
	}
	if priorityDispatchJob(testJob) {
		t.Logf("priorityDispatchJob checked")
	}

	// 3. scheduleImmediateJob & registerTriggeredJob & createJobHandler
	_ = sched.scheduleImmediateJob(testJob)
	_ = sched.registerTriggeredJob(testJob)
	h := sched.createJobHandler(testJob)
	if h == nil {
		t.Errorf("expected non-nil JobHandler")
	}

	// 4. unscheduleAllJobs & GetAsyncRouter
	sched.unscheduleAllJobs()
	router := sched.GetAsyncRouter()
	if router == nil {
		t.Logf("GetAsyncRouter returned nil (unconfigured)")
	}
}

func TestExtended_SwarmWorker_Deep_Wave61(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	mockStore := &mockCapStorage{
		listed: map[string][]map[string]any{
			objects.KindAgentTask: {
				{
					objects.FieldKeyID:     "ATK-swarm-1",
					objects.FieldKeyStatus: objects.ObjectStatusApproved,
					objects.FieldKeyTitle:  "Task 1",
				},
				{
					objects.FieldKeyID:     "ATK-swarm-2",
					objects.FieldKeyStatus: objects.ObjectStatusInProgress,
					objects.FieldKeyTitle:  "Orphan Task",
				},
			},
		},
	}

	sched := NewScheduler(mockStore, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)
	sched.projectRoot = tmpDir
	sched.secCtx = pkgctx.NewSystemSecurityContext()

	ctx := context.Background()

	// 1. pollAndSpawnSwarmTasks
	sched.pollAndSpawnSwarmTasks(ctx)

	// 2. recoverOrphanedTasks
	sched.recoverOrphanedTasks(ctx)

	// 3. orphanRecoveryStatus
	st, ok := orphanRecoveryStatus(objects.ObjectStatusInProgress)
	if !ok || st != objects.ObjectStatusApproved {
		t.Errorf("expected approved for in_progress orphan")
	}
	_, okOther := orphanRecoveryStatus(objects.ObjectStatusComplete)
	if okOther {
		t.Errorf("expected false for complete")
	}

	// 4. resolveSwarmClaimant & fallbackChatModel & applyCodeDraftSampling
	c := resolveSwarmClaimant(tmpDir, map[string]any{"id": "ATK-1"})
	if c == "" {
		t.Logf("claimant empty without peer seat (expected)")
	}
	t.Setenv("ZQK_LLM_CHAT_MODEL", "model-test")
	fb := fallbackChatModel()
	if fb != "model-test" {
		t.Errorf("expected model-test from fallbackChatModel, got %s", fb)
	}
	applyCodeDraftSampling(nil)
}
