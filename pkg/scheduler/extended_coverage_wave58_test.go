package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

func TestExtended_CapDispatch_Helpers_Wave58(t *testing.T) {
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

	// 1. mintAgentTaskID
	id1 := mintAgentTaskID()
	id2 := mintAgentTaskID()
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Errorf("expected unique minted task IDs, got %s, %s", id1, id2)
	}

	// 2. decodeCAPPlanOutput
	var dst map[string]any
	err = decodeCAPPlanOutput([]byte(`{"plan_id":"PRI-123","status":"active"}`), &dst)
	if err != nil || dst["plan_id"] != "PRI-123" {
		t.Errorf("decodeCAPPlanOutput failed: %v, dst=%v", err, dst)
	}

	// 3. capDispatchPlanIDs
	plans := []whatsnext.WhatsNextPriorityPlan{
		{ID: "PRI-B"},
		{ID: "PRI-A"}, // duplicate of primary
		{ID: "PRI-C"},
	}
	ids := capDispatchPlanIDs("PRI-A", plans)
	if len(ids) != 3 || ids[0] != "PRI-A" || ids[1] != "PRI-B" || ids[2] != "PRI-C" {
		t.Errorf("unexpected capDispatchPlanIDs: %v", ids)
	}

	// 4. nestPlanWorkstreams
	mockStore := &mockCapStorage{
		listed: map[string][]map[string]any{
			"workstream": {
				{
					objects.FieldKeyID:              "WS-1",
					objects.FieldKeyPriorityPlanRef: "PRI-1",
					objects.FieldKeyTitle:           "WS Title",
				},
			},
			"backlog_item": {
				{
					objects.FieldKeyID:              "BLI-1",
					objects.FieldKeyPriorityPlanRef: "PRI-1",
					objects.FieldKeyWorkstreamRef:   "WS-1",
					objects.FieldKeyTags:            []any{"tag1"},
				},
				{
					objects.FieldKeyID:              "BLI-2",
					objects.FieldKeyPriorityPlanRef: "PRI-1",
					objects.FieldKeyWorkstreamRef:   "",
				},
			},
		},
	}
	h := &CapOrchestratorHandler{
		storage:     mockStore,
		projectRoot: tmpDir,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	planObj := map[string]any{"id": "PRI-1"}
	bliMap := h.nestPlanWorkstreams(ctx, "PRI-1", planObj)
	if len(bliMap) < 2 {
		t.Errorf("expected at least 2 backlog items in nestPlanWorkstreams, got %d", len(bliMap))
	}
}

func TestExtended_Hourglass_Deep_Wave58(t *testing.T) {
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

	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. newMissedDeadlineRiskBlockerID & actionForExpiredTimer
	risID := newMissedDeadlineRiskBlockerID()
	if risID == "" {
		t.Errorf("expected non-empty risk blocker ID")
	}

	if actionForExpiredTimer("checkin") != actionWakeOrchestrator {
		t.Errorf("expected actionWakeOrchestrator")
	}
	if actionForExpiredTimer("deadline") != actionEscalateDeadline {
		t.Errorf("expected actionEscalateDeadline")
	}
	if actionForExpiredTimer("kill") != actionKillStuckProcess {
		t.Errorf("expected actionKillStuckProcess")
	}
	if actionForExpiredTimer("other") != actionKillStuckProcess {
		t.Errorf("expected default actionKillStuckProcess")
	}

	// 2. refsContainID
	if !refsContainID([]string{"a", "b", "target"}, "target") {
		t.Errorf("expected refsContainID true for []string")
	}
	if !refsContainID([]any{"a", "target"}, "target") {
		t.Errorf("expected refsContainID true for []any")
	}
	if refsContainID([]string{"a", "b"}, "target") {
		t.Errorf("expected refsContainID false when missing")
	}
	if refsContainID(12345, "target") {
		t.Errorf("expected refsContainID false for non-slice")
	}

	// 3. hasOpenMissedDeadlineEscalation
	if hasOpenMissedDeadlineEscalation(ctx, sp, secCtx, "ATK-none") {
		t.Errorf("expected false for missing escalation")
	}
	if hasOpenMissedDeadlineEscalation(ctx, nil, secCtx, "ATK-none") {
		t.Errorf("expected false for nil storage")
	}

	// 4. Scheduler hourglass methods
	sched := NewScheduler(sp, objects.GetGlobalSpecLoader(), objects.GetGlobalLifecycleLoader()).(*Scheduler)
	sched.projectRoot = tmpDir

	sched.checkHourglassTimers(ctx)
	sched.sweepStaleAgentTasks(ctx)

	// Create a dummy timer file and test handleMissedCheckin
	timerDir := filepath.Join(tmpDir, ".zqk", "state", "hourglass")
	_ = os.MkdirAll(timerDir, 0755)
	timerFile := filepath.Join(timerDir, "test.timer")
	_ = os.WriteFile(timerFile, []byte(`{"task_id":"ATK-sample","timer_type":"checkin"}`), 0644)
	sched.handleMissedCheckin(ctx, secCtx, timerFile)

	sched.recordSilentClaimBlocker(ctx, secCtx, "ATK-silent")
	sched.escalateMissedDeadline(ctx, secCtx, "ATK-missed", "agent_task", "Task title")
}
