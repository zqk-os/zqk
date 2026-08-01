package scheduler

import (
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	storagepkg "github.com/lanceman/zqk/pkg/storage"

	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func readStateByExecutionID(t *testing.T, reg JobStateRegistryInterface, jobID, executionID string) *JobExecutionState {
	t.Helper()
	r := reg.(*JobStateRegistry)
	p := r.stateFilePathForJobExecution(jobID, executionID)
	if b, err := os.ReadFile(p); err == nil {
		var st JobExecutionState
		if err := yaml.Unmarshal(b, &st); err == nil && st.ExecutionID == executionID {
			s := st
			return &s
		}
	}
	// Legacy flat layout (still supported by the registry for reads)
	entries, err := os.ReadDir(r.stateDir)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(r.stateDir, e.Name()))
		if err != nil {
			continue
		}
		var st JobExecutionState
		if err := yaml.Unmarshal(b, &st); err != nil {
			continue
		}
		if st.ExecutionID == executionID {
			s := st
			return &s
		}
	}
	return nil
}

func TestCRIT9037_JobStateRegistry_RegisterAndGetExecutionState(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	const jobID = "SCH-1"
	const executionID = "exec-1"

	if err := reg.RegisterExecution(jobID, executionID, 123); err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}

	st, err := reg.GetExecutionState(jobID)
	if err != nil {
		t.Fatalf("GetExecutionState failed: %v", err)
	}
	if st == nil {
		t.Fatalf("expected non-nil execution state")
	}
	if st.ExecutionID != executionID {
		t.Fatalf("expected ExecutionID=%q, got %q", executionID, st.ExecutionID)
	}
	if st.State != jobExecutionStateInProgress {
		t.Fatalf("expected State=%q, got %q", jobExecutionStateInProgress, st.State)
	}
	if st.StartedAt.IsZero() {
		t.Fatalf("expected StartedAt to be set")
	}
}

func TestCRIT9037_JobStateRegistry_ListInProgressIncludesDeferred(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	const jobID = "SCH-2"
	const executionID = "exec-2"

	if err := reg.RegisterExecution(jobID, executionID, 456); err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}

	deferUntil := time.Now().UTC().Add(10 * time.Minute)
	if err := reg.DeferExecution(jobID, "waiting_for_dependency", &deferUntil); err != nil {
		t.Fatalf("DeferExecution failed: %v", err)
	}

	list, err := reg.ListInProgress()
	if err != nil {
		t.Fatalf("ListInProgress failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 in-progress/deferred state, got %d", len(list))
	}
	if list[0].JobID != jobID {
		t.Fatalf("expected JobID=%q, got %q", jobID, list[0].JobID)
	}
	if list[0].ExecutionID != executionID {
		t.Fatalf("expected ExecutionID=%q, got %q", executionID, list[0].ExecutionID)
	}
	if list[0].State != jobExecutionStateDeferred {
		t.Fatalf("expected State=%q, got %q", jobExecutionStateDeferred, list[0].State)
	}
}

func TestCRIT9037_JobStateRegistry_CompleteExecutionMovesStateOutOfInProgress(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	const jobID = "SCH-3"
	const executionID = "exec-3"

	if err := reg.RegisterExecution(jobID, executionID, 789); err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}
	if err := reg.CompleteExecution(jobID, executionID, "completed"); err != nil {
		t.Fatalf("CompleteExecution failed: %v", err)
	}

	st, err := reg.GetExecutionState(jobID)
	if err != nil {
		t.Fatalf("GetExecutionState failed: %v", err)
	}
	if st != nil {
		t.Fatalf("expected GetExecutionState to return nil after completion (no in_progress/deferred)")
	}

	persisted := readStateByExecutionID(t, reg, jobID, executionID)
	if persisted == nil {
		t.Fatalf("expected persisted execution state file to exist")
	}
	if persisted.State != jobExecutionStateCompleted {
		t.Fatalf("expected persisted State=%q, got %q", jobExecutionStateCompleted, persisted.State)
	}
	if persisted.CompletedAt == nil {
		t.Fatalf("expected persisted CompletedAt to be set")
	}
}

func TestCRIT9037_JobStateRegistry_CompleteExecutionMarksFailed(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	const jobID = "SCH-4"
	const executionID = "exec-4"

	if err := reg.RegisterExecution(jobID, executionID, 999); err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}

	if err := reg.CompleteExecution(jobID, executionID, "execution failed: dependency not met"); err != nil {
		t.Fatalf("CompleteExecution failed: %v", err)
	}

	persisted := readStateByExecutionID(t, reg, jobID, executionID)
	if persisted == nil {
		t.Fatalf("expected persisted execution state file to exist")
	}
	if persisted.State != jobExecutionStateFailed {
		t.Fatalf("expected persisted State=%q, got %q", jobExecutionStateFailed, persisted.State)
	}
	if persisted.CompletedAt == nil {
		t.Fatalf("expected persisted CompletedAt to be set")
	}
}

func TestJobStateRegistry_MigrateLegacyFlatStateFilesBestEffort(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	const jobID = "SCH-migrate-1"
	const executionID = "exec-migrate-1"

	st := JobExecutionState{
		JobID:       jobID,
		ExecutionID: executionID,
		State:       jobExecutionStateCompleted,
		ProcessID:   42,
		StartedAt:   time.Now().UTC().Add(-time.Hour),
	}
	b, err := yaml.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	flatName := jobID + "-" + executionID + ".yaml"
	flatPath := filepath.Join(reg.stateDir, flatName)
	if err := os.MkdirAll(reg.stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(flatPath, b, paths.FilePerm644); err != nil {
		t.Fatalf("write flat: %v", err)
	}

	n, err := reg.MigrateLegacyFlatStateFilesBestEffort()
	if err != nil {
		t.Fatalf("MigrateLegacyFlatStateFilesBestEffort: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected moved=1, got %d", n)
	}
	if _, err := os.Stat(flatPath); !os.IsNotExist(err) {
		t.Fatalf("expected flat file removed, stat err=%v", err)
	}
	nested := reg.stateFilePathForJobExecution(jobID, executionID)
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("expected nested file at %s: %v", nested, err)
	}
}

func TestJobStateRegistry_CleanStaleLocks(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	// Ensure locks directory exists
	if err := os.MkdirAll(reg.locksDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir locks dir: %v", err)
	}

	// 1. Create a stale lock file (modified 1 hour ago)
	stalePath := filepath.Join(reg.locksDir, "stale.lock")
	if err := os.WriteFile(stalePath, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("write stale: %v", err)
	}
	oldTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(stalePath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// 2. Create a fresh lock file (modified just now)
	freshPath := filepath.Join(reg.locksDir, "fresh.lock")
	if err := os.WriteFile(freshPath, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("write fresh: %v", err)
	}

	// 3. Create a stale lock file but keep it locked
	lockedPath := filepath.Join(reg.locksDir, "locked.lock")
	if err := os.WriteFile(lockedPath, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("write locked: %v", err)
	}
	if err := os.Chtimes(lockedPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	fl, err := storagepkg.NewFileLock(lockedPath)
	if err != nil {
		t.Fatalf("new file lock: %v", err)
	}
	if err := fl.LockWithTimeout(1 * time.Second); err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer fl.Unlock()
	defer fl.Close()

	// Run CleanStaleLocks
	cleaned, err := reg.CleanStaleLocks(5 * time.Minute)
	if err != nil {
		t.Fatalf("CleanStaleLocks failed: %v", err)
	}

	if cleaned != 1 {
		t.Fatalf("expected 1 file cleaned, got %d", cleaned)
	}

	// Verify stale is gone
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale lock to be deleted")
	}

	// Verify fresh remains
	if _, err := os.Stat(freshPath); err != nil {
		t.Fatalf("expected fresh lock to remain: %v", err)
	}

	// Verify locked remains
	if _, err := os.Stat(lockedPath); err != nil {
		t.Fatalf("expected locked lock to remain: %v", err)
	}
}

func TestJobStateRegistry_MigrateUnbucketedJobStateDirsBestEffort(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	const jobID = "SCH-run-bundle-migrate-bucket"
	const executionID = "exec-bucket-migrate"

	legacySeg := sanitizeJobIDForPathSegment(jobID)
	legacyDir := filepath.Join(reg.stateDir, legacySeg)
	if err := os.MkdirAll(legacyDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	st := JobExecutionState{
		JobID:       jobID,
		ExecutionID: executionID,
		State:       jobExecutionStateCompleted,
		ProcessID:   1,
		StartedAt:   time.Now().UTC().Add(-time.Hour),
	}
	b, err := yaml.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, executionID+".yaml"), b, paths.FilePerm644); err != nil {
		t.Fatalf("write: %v", err)
	}

	n, err := reg.MigrateUnbucketedJobStateDirsBestEffort()
	if err != nil {
		t.Fatalf("MigrateUnbucketedJobStateDirsBestEffort: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected moved=1, got %d", n)
	}

	dest := filepath.Join(reg.stateDir, schedulerStateBucket(jobID), legacySeg, executionID+".yaml")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected file at %s: %v", dest, err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("expected legacy dir removed, stat err=%v", err)
	}
}
