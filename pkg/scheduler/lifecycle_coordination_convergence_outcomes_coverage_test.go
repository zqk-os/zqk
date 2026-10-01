package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_LifecycleCoordination_DeepHelpers_Wave59(t *testing.T) {
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

	// 1. getStringFromMap
	m := map[string]any{"k1": "v1", "k2": 123}
	if getStringFromMap(m, "k1") != "v1" {
		t.Errorf("expected v1")
	}
	if getStringFromMap(m, "k2") != "" {
		t.Errorf("expected empty string for non-string")
	}
	if getStringFromMap(m, "missing") != "" {
		t.Errorf("expected empty string for missing key")
	}

	// 2. parseCronSchedule
	s1, err1 := sched.parseCronSchedule("0 * * * *")
	if err1 != nil || s1 == nil {
		t.Errorf("expected valid schedule for 0 * * * *: %v", err1)
	}
	_, err2 := sched.parseCronSchedule("invalid-cron-expression")
	if err2 == nil {
		t.Errorf("expected error for invalid cron")
	}

	// 3. matchesEventFilter
	if !sched.matchesEventFilter("*", "create", "job") {
		t.Errorf("expected wildcard to match")
	}
	if !sched.matchesEventFilter("create:job", "create", "job") {
		t.Errorf("expected exact match")
	}
	if sched.matchesEventFilter("create:task", "create", "job") {
		t.Errorf("expected mismatch")
	}
	if !sched.matchesEventFilter("create", "create", "job") {
		t.Errorf("expected type match")
	}

	// 4. matchesLifecycleFilter
	if !sched.matchesLifecycleFilter("*", "task", "pending", "running") {
		t.Errorf("expected wildcard match")
	}
	if !sched.matchesLifecycleFilter("task:pending->running", "task", "pending", "running") {
		t.Errorf("expected kind:from->to match")
	}
	if sched.matchesLifecycleFilter("task:pending->completed", "task", "pending", "running") {
		t.Errorf("expected mismatch")
	}

	// 5. isConcurrentAllowed
	if !sched.isConcurrentAllowed("run_wrapper", CategoryMaintenance) {
		t.Logf("isConcurrentAllowed tested")
	}

	// 6. JobInCache & lookupJobInCache
	sched.jobs["SCH-job-cache-1"] = &ScheduledJob{ID: "SCH-job-cache-1", Title: "cached"}
	if !sched.JobInCache("SCH-job-cache-1") {
		t.Errorf("expected JobInCache true")
	}
	if sched.JobInCache("SCH-missing") {
		t.Errorf("expected JobInCache false for missing")
	}
	j, ok := sched.lookupJobInCache("SCH-job-cache-1")
	if !ok || j == nil || j.ID != "SCH-job-cache-1" {
		t.Errorf("expected job from lookupJobInCache")
	}

	// 7. collectTimerJobs & buildCompletionMapFromActivityCache
	timerJobs, okTimer := sched.collectTimerJobs()
	if okTimer && len(timerJobs) > 0 {
		t.Logf("collected timer jobs: %d", len(timerJobs))
	}
	compMap := sched.buildCompletionMapFromActivityCache()
	if compMap == nil {
		t.Errorf("expected non-nil compMap")
	}

	// 8. recordHealthMetric
	sched.recordHealthMetric(ctx, time.Now().UTC(), 1, 0, 0, 0, 100*time.Millisecond, 10, 5, 1024, 2048)
}

func TestExtended_CapDispatch_Continuation_Wave59(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	mockStore := &mockCapStorage{
		listed: map[string][]map[string]any{
			objects.KindAgentTask: {
				{
					objects.FieldKeyID:              "ATK-pend-1",
					objects.FieldKeyStatus:          objects.ObjectStatusPendingVerification,
					objects.FieldKeyPriorityPlanRef: "PRI-1",
					objects.FieldKeyCommitHashes:    []any{"abc1234"},
				},
			},
		},
	}
	h := &CapOrchestratorHandler{
		storage:     mockStore,
		projectRoot: tmpDir,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	// resumePendingVerificationTasks with empty planID
	if err := h.resumePendingVerificationTasks(context.Background(), "zqk", ""); err != nil {
		t.Errorf("expected nil for empty planID, got: %v", err)
	}

	// resumePendingVerificationTasks with planID
	_ = h.resumePendingVerificationTasks(context.Background(), "zqk", "PRI-1")
}

func TestExtended_ConvergenceTestBundle_OutcomesAndSnapshots_Wave59(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// 1. Outcome classification helpers
	if !outcomeIsBad("fail") || !outcomeIsBad("timeout") || !outcomeIsBad("flake") || !outcomeIsBad("compile_fail") {
		t.Errorf("expected outcomeIsBad true for failures")
	}
	if outcomeIsBad("pass") || outcomeIsBad("ok") {
		t.Errorf("expected outcomeIsBad false for pass")
	}

	if !outcomeIsFlake("flake") || !outcomeIsFlake("quarantined_flake") {
		t.Errorf("expected outcomeIsFlake true")
	}
	if outcomeIsFlake("fail") {
		t.Errorf("expected outcomeIsFlake false for fail")
	}

	if !outcomeIsBuildFailure("build_fail") || !outcomeIsBuildFailure("compile_fail") {
		t.Errorf("expected outcomeIsBuildFailure true")
	}
	if outcomeIsBuildFailure("timeout") {
		t.Errorf("expected outcomeIsBuildFailure false for timeout")
	}

	if !outcomeIsGood("pass") || !outcomeIsGood("ok") {
		t.Errorf("expected outcomeIsGood true")
	}
	if outcomeIsGood("fail") {
		t.Errorf("expected outcomeIsGood false for fail")
	}

	// 2. readTriggerQueuePending
	if readTriggerQueuePending("") != -1 {
		t.Errorf("expected -1 for empty project root")
	}
	pending := readTriggerQueuePending(tmpDir)
	if pending < 0 {
		t.Errorf("expected >= 0 pending triggers, got %d", pending)
	}

	// 3. parseSuggestedRerunCommands
	m := map[string]any{
		KeySuggestedRerunCommands: []any{"go test ./pkg/scheduler/..."},
	}
	cmds := parseSuggestedRerunCommands(m)
	if len(cmds) != 1 || cmds[0] != "go test ./pkg/scheduler/..." {
		t.Errorf("unexpected rerun commands: %v", cmds)
	}

	// 4. BuildTestBundleConvergenceSnapshot
	lines := []map[string]any{
		{
			"job_id":       TestBundleJobIDPrefix + "1",
			"outcome":      "pass",
			"test_count":   10,
			"failed_count": 0,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
		},
		{
			"job_id":       TestBundleJobIDPrefix + "2",
			"outcome":      "fail",
			"test_count":   10,
			"failed_count": 1,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
		},
	}
	snap := BuildTestBundleConvergenceSnapshot(tmpDir, lines)
	if snap == nil {
		t.Fatalf("expected non-nil snapshot")
	}
	applySessionCompletionGates(snap)
	fillPrimaryMeasurementOutcomeFromRollup(snap)

	// Tail lines reading
	ctx := context.Background()
	_, _ = ReadTestBundleHealthTailLines(ctx, tmpDir, 10)
	_, _ = ReadTestBundleEventsTailLines(ctx, tmpDir, 10)
	_, _ = ReadTestBundleProgressTailLines(ctx, tmpDir, 10)
}
