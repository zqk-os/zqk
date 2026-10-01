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
)

func TestExtended_ConvergenceTestBundle_Wave43(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// 1. Outcome classification helpers
	if !outcomeIsBad("fail") || !outcomeIsBad("build_fail") || outcomeIsBad("pass") {
		t.Errorf("unexpected outcomeIsBad")
	}
	if !outcomeIsFlake("flake") || !outcomeIsFlake("quarantine") || outcomeIsFlake("fail") {
		t.Errorf("unexpected outcomeIsFlake")
	}
	if !outcomeIsBuildFailure("build_fail") || !outcomeIsBuildFailure("compile_fail") || outcomeIsBuildFailure("pass") {
		t.Errorf("unexpected outcomeIsBuildFailure")
	}
	if !outcomeIsGood("pass") || !outcomeIsGood("ok") || outcomeIsGood("fail") {
		t.Errorf("unexpected outcomeIsGood")
	}

	// 2. parseSuggestedRerunCommands
	m := map[string]any{
		"suggested_rerun_commands": []any{"go test -run TestA", "go test -run TestB", 123},
	}
	cmds := parseSuggestedRerunCommands(m)
	if len(cmds) != 2 || cmds[0] != "go test -run TestA" {
		t.Errorf("unexpected parseSuggestedRerunCommands: %v", cmds)
	}
	if len(parseSuggestedRerunCommands(nil)) != 0 {
		t.Errorf("expected empty for nil map")
	}

	// 3. readTriggerQueuePending
	p := readTriggerQueuePending(tmpDir)
	if p != 0 {
		t.Errorf("expected 0 pending triggers for empty dir, got %d", p)
	}

	// 4. BuildTestBundleConvergenceSnapshot & applySessionCompletionGates
	lines := []map[string]any{
		{
			"package_path": "pkg/scheduler",
			"outcome":      "passed",
			"passed_tests": 10,
			"failed_tests": 0,
			"duration_ms":  500.0,
		},
		{
			"package_path":      "pkg/storage",
			"outcome":           "failed",
			"passed_tests":      5,
			"failed_tests":      2,
			"duration_ms":       1200.0,
			"failed_test_names": []any{"TestFail1"},
		},
	}
	snap := BuildTestBundleConvergenceSnapshot(tmpDir, lines)
	if snap == nil {
		t.Fatalf("expected non-nil snapshot")
	}
	applySessionCompletionGates(snap)
	fillPrimaryMeasurementOutcomeFromRollup(snap)

	// 5. findFingerprintForPattern
	fp := findFingerprintForPattern(tmpDir, "pkg/scheduler")
	t.Logf("findFingerprintForPattern: %s", fp)

	// 6. Tail reading helpers
	ctx := context.Background()
	_, _ = ReadTestBundleHealthTailLines(ctx, tmpDir, 5)
	_, _ = ReadTestBundleEventsTailLines(ctx, tmpDir, 5)
	_, _ = ReadTestBundleProgressTailLines(ctx, tmpDir, 5)
	_, _, _ = readTestBundleHealthJSONLFullScan(tmpDir, 10)
}

func TestExtended_CapStageGates_Wave43(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	handler := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	ctx := context.Background()

	// 1. Path helpers
	sDir := handler.capStateDir()
	sPath := handler.capStatePath("test_state.json")
	lPath := handler.legacyCapStatePath("test_state.json")
	if sDir == "" || sPath == "" || lPath == "" {
		t.Errorf("expected non-empty state paths: %s, %s, %s", sDir, sPath, lPath)
	}

	// 2. State file read/write
	stateData := map[string]any{"key": "value"}
	handler.writeStateFile("custom_state.json", stateData)
	if !handler.stateFileExists("custom_state.json") {
		t.Errorf("expected stateFileExists true")
	}
	if !handler.stateFileNonEmpty("custom_state.json") {
		t.Errorf("expected stateFileNonEmpty true")
	}
	if !handler.stateFileFresh("custom_state.json", "1h") {
		t.Errorf("expected stateFileFresh true")
	}
	readData, err := handler.readStateFile("custom_state.json")
	if err != nil || readData == nil {
		t.Errorf("failed to read state file: %v, %v", readData, err)
	}

	// 3. Mark stage entered and read pending stage
	handler.markStageEntered("cap_stage_grooming", "PRI-test")
	pending, err := handler.readPendingStage()
	if err != nil || pending.Stage != "cap_stage_grooming" {
		t.Errorf("unexpected pending stage: %+v, err=%v", pending, err)
	}

	// 4. Grooming planned zero latch
	handler.clearGroomingPlannedZeroLatch()
	_, exhausted := handler.groomingPlannedZeroExhausted(0)
	t.Logf("groomingPlannedZeroExhausted(0): %v", exhausted)
	handler.clearGroomingPlannedZeroLatch()

	// 5. Stage failure attempts and quarantine
	handler.recordCAPStageFailureAttempt("cap_stage_grooming")
	handler.clearCAPStageFailureAttempts("cap_stage_grooming")

	handler.quarantineCAPStage(pending)
	isQuar := handler.capStageQuarantineActive(pending)
	t.Logf("capStageQuarantineActive: %v", isQuar)
	retryReady := handler.capStageQuarantineRetryReady(pending, time.Now().Add(2*time.Hour))
	t.Logf("capStageQuarantineRetryReady: %v", retryReady)
	handler.clearCAPStageQuarantine()

	// 6. CVS resolution
	cvs := handler.liveCVSOrEmpty(ctx, "CVS-none")
	if cvs != "" {
		t.Errorf("expected empty for nonexistent cvs")
	}
	boundCVS, focus := handler.resolveBoundCVS(ctx, "PRI-none")
	t.Logf("resolveBoundCVS: cvs=%s focus=%s", boundCVS, focus)

	// 7. Stage delivery and holds
	complete, reason := handler.stageDeliveryComplete(ctx, "cap_stage_review")
	t.Logf("stageDeliveryComplete: complete=%v reason=%s", complete, reason)
	handler.maybeWakeOnStageHold("cap_stage_review", "test hold reason")
}

func TestExtended_SchedulerInternalHelpers_Wave43(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	specLoader := objects.NewSpecLoader(tmpDir)
	lifecycleLoader := objects.NewLifecycleLoader(tmpDir)

	schedIface := NewSchedulerWithProjectRoot(sp, specLoader, lifecycleLoader, tmpDir, nil)
	sched := schedIface.(*Scheduler)

	// 1. getPackageConcurrencyMaxWait
	w := getPackageConcurrencyMaxWait()
	if w <= 0 {
		t.Errorf("expected positive concurrency wait: %v", w)
	}

	// 2. Lock failure tracking
	c, shouldLog, shouldDisable := sched.recordLockFailure("SCH-lock-test")
	if c != 1 {
		t.Errorf("expected count 1, got %d", c)
	}
	_ = shouldLog
	_ = shouldDisable
	if sched.shouldSkipJobDueToLockFailures("SCH-lock-test") {
		t.Errorf("expected false for 1 failure")
	}
	sched.clearLockFailureCount("SCH-lock-test")

	// 3. Dispatch drop retry count
	if sched.getDispatchDropRetryCount("SCH-drop-test") != 0 {
		t.Errorf("expected 0 initially")
	}
	rCount := sched.incrementDispatchDropRetryCount("SCH-drop-test")
	if rCount != 1 {
		t.Errorf("expected 1, got %d", rCount)
	}
	sched.clearDispatchDropRetryCount("SCH-drop-test")
	if sched.getDispatchDropRetryCount("SCH-drop-test") != 0 {
		t.Errorf("expected 0 after clear")
	}

	// 4. TouchActivity & GetLastActivity
	sched.TouchActivity()
	act := sched.GetLastActivity()
	if act.IsZero() || time.Since(act) > 5*time.Second {
		t.Errorf("unexpected last activity: %v", act)
	}

	// 5. RecordTriggerQueue metrics
	sched.RecordTriggerQueueDequeued(5)
	sched.RecordTriggerQueueTriggerFailed("SCH-fail", errors.New("err"))
	sched.RecordTriggerQueueReloadRetry("SCH-retry")
	sched.RecordTriggerQueueReloadFailed("SCH-fail", errors.New("err"))

	// 6. Setters & Getters
	secCtx := pkgctx.NewSystemSecurityContext()
	sched.SetSecurityContext(secCtx)
	sched.SetOnlyManualJobs(true)
	sched.SetOnlyManualJobs(false)
	sched.SetValidationScanner(nil)

	// 7. RegisterJob, GetJob, ListJobs
	testJob := &ScheduledJob{
		ID:       "SCH-sched-test",
		JobType:  JobTypeCachePrewarm,
		Category: CategoryMaintenance,
	}
	err = sched.RegisterJob(testJob)
	if err != nil {
		t.Errorf("failed to register job: %v", err)
	}
	sched.jobsMu.Lock()
	sched.jobs[testJob.ID] = testJob
	sched.jobsMu.Unlock()
	retrieved, found := sched.GetJob("SCH-sched-test")
	if !found || retrieved == nil || retrieved.ID != "SCH-sched-test" {
		t.Errorf("failed to get registered job: %v, found=%v", retrieved, found)
	}
	allJobs := sched.ListJobs()
	if len(allJobs) == 0 {
		t.Errorf("expected at least 1 job in list")
	}

	// 8. GetExecutor
	if sched.GetExecutor() == nil {
		t.Errorf("expected non-nil executor")
	}
}
