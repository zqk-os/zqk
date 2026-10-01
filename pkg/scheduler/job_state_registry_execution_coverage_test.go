package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_JobStateRegistry_DeepCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	regInterface := NewJobStateRegistry(tmpDir)
	reg, ok := regInterface.(*JobStateRegistry)
	if !ok {
		t.Fatalf("expected *JobStateRegistry")
	}

	reg.SetObservabilityRecorder(nil)

	// 1. Path helpers
	lJob := reg.lockPathForJobID("SCH-job-1")
	lExec := reg.lockPathForExecutionID("exec-1")
	sFile := reg.stateFilePathForJobExecution("SCH-job-1", "exec-1")
	if lJob == "" || lExec == "" || sFile == "" {
		t.Errorf("expected non-empty path helpers: %s, %s, %s", lJob, lExec, sFile)
	}

	// 2. RegisterExecution
	err := reg.RegisterExecution("SCH-job-1", "exec-1", 12345)
	if err != nil {
		t.Fatalf("failed to register execution: %v", err)
	}

	// 3. GetState & GetExecutionState
	st, err := reg.GetState("SCH-job-1")
	if err != nil || st == nil || st.ExecutionID != "exec-1" {
		t.Errorf("unexpected state: %+v, err=%v", st, err)
	}
	stExec, err := reg.GetExecutionState("SCH-job-1")
	if err != nil || stExec == nil {
		t.Errorf("unexpected execution state: %+v, err=%v", stExec, err)
	}

	// 4. ListInProgress & ListStates
	inProg, err := reg.ListInProgress()
	if err != nil || len(inProg) == 0 {
		t.Errorf("expected at least 1 in-progress job: len=%d, err=%v", len(inProg), err)
	}

	states, err := reg.ListStates()
	if err != nil || len(states) == 0 {
		t.Errorf("expected at least 1 state: len=%d, err=%v", len(states), err)
	}

	// 5. DeferExecution
	future := time.Now().Add(1 * time.Hour)
	err = reg.DeferExecution("SCH-job-1", "resource ceiling", &future)
	if err != nil {
		t.Errorf("failed to defer execution: %v", err)
	}

	// 6. UpdateState
	st.State = jobExecutionStateInProgress
	err = reg.UpdateState("SCH-job-1", st)
	if err != nil {
		t.Errorf("failed to update state: %v", err)
	}

	// 7. CompleteExecution (completed, failed, skipped)
	err = reg.CompleteExecution("SCH-job-1", "exec-1", jobExecutionStateCompleted)
	if err != nil {
		t.Errorf("failed to complete execution: %v", err)
	}

	// Register another and complete as failed
	_ = reg.RegisterExecution("SCH-job-2", "exec-2", 12346)
	err = reg.CompleteExecution("SCH-job-2", "exec-2", jobExecutionStateFailed)
	if err != nil {
		t.Errorf("failed to complete failed execution: %v", err)
	}

	// Register another and complete as skipped
	_ = reg.RegisterExecution("SCH-job-3", "exec-3", 12347)
	err = reg.CompleteExecution("SCH-job-3", "exec-3", jobExecutionStateSkipped)
	if err != nil {
		t.Errorf("failed to complete skipped execution: %v", err)
	}

	// 8. findExecutionStateByExecutionID
	filePath, foundSt, err := reg.findExecutionStateByExecutionID("exec-1")
	if err != nil || filePath == "" || foundSt == nil || foundSt.JobID != "SCH-job-1" {
		t.Errorf("failed to find state by execution id: filePath=%s, foundSt=%+v, err=%v", filePath, foundSt, err)
	}

	// 9. Summarize
	summary, err := reg.Summarize(10 * time.Minute)
	if err != nil || summary == nil {
		t.Errorf("failed to summarize: summary=%+v, err=%v", summary, err)
	}

	// 10. Hints and cleanup
	_ = reg.writeStateHintsFile()
	reg.scheduleFullRetentionCleanup()
	_ = reg.cleanupCompletedBestEffort()

	// 11. Stale locks
	locksDir := filepath.Join(tmpDir, paths.ProjectDataDir, "scheduler", "locks")
	_ = fileutil.MkdirAll(locksDir, 0755)
	oldLock := filepath.Join(locksDir, "SCH-old.lock")
	_ = os.WriteFile(oldLock, []byte("pid"), 0644)
	oldTime := time.Now().Add(-1 * time.Hour)
	_ = os.Chtimes(oldLock, oldTime, oldTime)

	cleared, err := reg.CleanStaleLocks(10 * time.Minute)
	t.Logf("CleanStaleLocks cleared: %d locks, err=%v", cleared, err)

	// 12. Migrations
	movedFlat, err := reg.MigrateLegacyFlatStateFilesBestEffort()
	t.Logf("MigrateLegacyFlatStateFilesBestEffort moved: %d, err=%v", movedFlat, err)

	movedUnbucketed, err := reg.MigrateUnbucketedJobStateDirsBestEffort()
	t.Logf("MigrateUnbucketedJobStateDirsBestEffort moved: %d, err=%v", movedUnbucketed, err)
}

func TestExtended_JobExecution_DeepCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	// 1. WriteJobOutcome & WriteJobProgress
	WriteJobOutcome(tmpDir, "SCH-out-1", "timer", map[string]any{"events": 10})
	WriteJobProgress(tmpDir, "SCH-out-1", map[string]any{"status": "running"})

	// Empty guards
	WriteJobOutcome("", "", "", nil)
	WriteJobProgress("", "", nil)

	// Test-bundle job ID branch
	WriteJobOutcome(tmpDir, "SCH-run-bundle-test", "test", map[string]any{"ok": true})

	// 2. trimJobLogFileIfNeeded
	logFile := filepath.Join(tmpDir, "test.log")
	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test log: %v", err)
	}

	trimJobLogFileIfNeeded(logFile, 2)
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read trimmed log: %v", err)
	}
	expected := "line 4\nline 5\n"
	if string(data) != expected {
		t.Errorf("expected trimmed content %q, got %q", expected, string(data))
	}

	// Edge cases for trimJobLogFileIfNeeded
	trimJobLogFileIfNeeded(logFile, 0)
	trimJobLogFileIfNeeded(logFile, -1)
	trimJobLogFileIfNeeded("/nonexistent/file.log", 5)

	// 3. dispatchContextForScheduledJob
	ctx0, cancel0 := dispatchContextForScheduledJob(&ScheduledJob{MaxRuntimeSeconds: 0})
	defer cancel0()
	if ctx0 == nil {
		t.Errorf("expected non-nil context for max runtime 0")
	}

	ctx600, cancel600 := dispatchContextForScheduledJob(&ScheduledJob{MaxRuntimeSeconds: 600})
	defer cancel600()
	if ctx600 == nil {
		t.Errorf("expected non-nil context for max runtime 600")
	}

	// 4. shouldInvokeConfiguredJobCallback
	jobNoCallback := &ScheduledJob{ID: "SCH-nocb"}
	if shouldInvokeConfiguredJobCallback(context.Background(), jobNoCallback) {
		t.Errorf("expected false for job without callback")
	}

	jobWithCallback := &ScheduledJob{
		ID: "SCH-withcb",
		Metadata: map[string]any{
			"callback_url": "http://localhost:8080/callback",
		},
	}
	_ = shouldInvokeConfiguredJobCallback(context.Background(), jobWithCallback)
}
