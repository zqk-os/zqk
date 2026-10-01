package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/observability"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockObservabilityRecorder struct {
	observability.Recorder
	records []string
}

func (m *mockObservabilityRecorder) IsEnabled() bool {
	return true
}

func (m *mockObservabilityRecorder) Record(name string, builder observability.Builder) error {
	m.records = append(m.records, name)
	return nil
}

func TestExtended_JobStateRegistry_CompleteAndRetention(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-jsr-complete-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)
	rec := &mockObservabilityRecorder{}
	reg.SetObservabilityRecorder(rec)

	jobID := "SCH-run-bundle-test1"
	execID := "exec-bundle-1"

	// 1. RegisterExecution
	if err := reg.RegisterExecution(jobID, execID, 9999); err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}

	// 2. GetState / GetExecutionState
	st, err := reg.GetState(jobID)
	if err != nil || st == nil {
		t.Fatalf("GetState failed: st=%v, err=%v", st, err)
	}
	if st.State != jobExecutionStateInProgress {
		t.Errorf("expected in_progress, got %s", st.State)
	}

	// 3. ListStates / ListInProgress
	states, err := reg.ListStates()
	if err != nil || len(states) == 0 {
		t.Fatalf("ListStates failed: len=%d, err=%v", len(states), err)
	}

	// 4. Summarize
	summary, err := reg.Summarize(time.Millisecond)
	if err != nil || summary == nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary.TotalFiles == 0 {
		t.Errorf("expected > 0 files in summary")
	}

	// 5. CompleteExecution - success
	if err := reg.CompleteExecution(jobID, execID, "success"); err != nil {
		t.Fatalf("CompleteExecution success failed: %v", err)
	}

	// 6. CompleteExecution - failed
	execIDFail := "exec-bundle-fail"
	_ = reg.RegisterExecution(jobID, execIDFail, 10000)
	if err := reg.CompleteExecution(jobID, execIDFail, "job failed with error"); err != nil {
		t.Fatalf("CompleteExecution failed error failed: %v", err)
	}

	// 7. CompleteExecution - skipped
	execIDSkip := "exec-bundle-skip"
	_ = reg.RegisterExecution(jobID, execIDSkip, 10001)
	if err := reg.CompleteExecution(jobID, execIDSkip, "skip this run"); err != nil {
		t.Fatalf("CompleteExecution skip failed: %v", err)
	}

	// 8. CompleteExecution without jobID (scans by executionID)
	execIDScan := "exec-bundle-scan"
	_ = reg.RegisterExecution(jobID, execIDScan, 10002)
	if err := reg.CompleteExecution("", execIDScan, "completed"); err != nil {
		t.Fatalf("CompleteExecution without jobID failed: %v", err)
	}

	// 9. CompleteExecution with nonexistent executionID
	if err := reg.CompleteExecution(jobID, "nonexistent-exec-id", "success"); err != nil {
		t.Errorf("expected nil on nonexistent executionID, got %v", err)
	}

	// 10. UpdateState directly
	st.State = "custom_state"
	if err := reg.UpdateState(jobID, st); err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	// 11. Cleanup expired state file
	reg.retention = 1 * time.Nanosecond
	time.Sleep(5 * time.Millisecond)
	var cleanupStats stateRetentionCleanupStats
	targetPath := reg.stateFilePathForJobExecution(jobID, execID)
	reg.maybeRemoveExpiredStateFile(targetPath, time.Now().Add(time.Hour), &cleanupStats)
	t.Logf("cleanupStats: %+v", cleanupStats)

	// 12. cleanupCompletedBestEffort
	_ = reg.cleanupCompletedBestEffort()

	// 13. scheduleFullRetentionCleanup with test hook
	hookCalled := false
	reg.fullCleanupTestHook = func() { hookCalled = true }
	reg.scheduleFullRetentionCleanup()
	// Call again immediately to hit the rate-limit/in-flight branch
	reg.scheduleFullRetentionCleanup()
	t.Logf("hookCalled: %v", hookCalled)

	// 14. recordStateRetentionCleanup with stats
	reg.recordStateRetentionCleanup(stateRetentionCleanupStats{
		RemovedStateFiles: 3,
		RemovedEmptyDirs:  1,
		RemoveErrors:      0,
	})

	// 15. sanitizeJobIDForPathSegment branches
	_ = sanitizeJobIDForPathSegment("")
	_ = sanitizeJobIDForPathSegment("normal/path/segment")
	longJobID := string(make([]byte, 300))
	_ = sanitizeJobIDForPathSegment(longJobID)

	// 16. Empty jobID / executionID error checks
	if err := reg.RegisterExecution("", "e", 1); err == nil {
		t.Errorf("expected error for empty jobID")
	}
	if err := reg.RegisterExecution("j", "", 1); err == nil {
		t.Errorf("expected error for empty executionID")
	}
	if _, err := reg.GetExecutionState(""); err == nil {
		t.Errorf("expected error for empty jobID")
	}
	if err := reg.CompleteExecution("j", "", "res"); err == nil {
		t.Errorf("expected error for empty executionID")
	}
	if err := reg.DeferExecution("", "reason", nil); err == nil {
		t.Errorf("expected error for empty jobID")
	}

	// 17. MigrateLegacyFlatStateFilesBestEffort
	// Put a flat legacy state file in stateDir
	flatPath := filepath.Join(reg.stateDir, "flat-legacy-exec.yaml")
	flatData := []byte("job_id: SCH-flat-1\nexecution_id: exec-flat-1\nstate: completed\n")
	_ = fileutil.WriteFile(flatPath, flatData, paths.FilePerm600)
	movedFlat, err := reg.MigrateLegacyFlatStateFilesBestEffort()
	t.Logf("MigrateLegacyFlatStateFilesBestEffort: moved=%d, err=%v", movedFlat, err)
}
