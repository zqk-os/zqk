package scheduler

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"
)

func readStateByExecutionID(t *testing.T, reg JobStateRegistryInterface, jobID, executionID string) *JobExecutionState {
	t.Helper()
	r := reg.(*JobStateRegistry)
	p := r.stateFilePathForJobExecution(jobID, executionID)
	if b, err := fileutil.ReadFile(p); err == nil {
		var st JobExecutionState
		if err := yaml.Unmarshal(b, &st); err == nil && st.ExecutionID == executionID {
			s := st
			return &s
		}
	}
	// Legacy flat layout (still supported by the registry for reads)
	entries, err := fileutil.ReadDir(r.stateDir)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := fileutil.ReadFile(filepath.Join(r.stateDir, e.Name()))
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
	if err := fileutil.MkdirAll(reg.stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := fileutil.WriteFile(flatPath, b, paths.FilePerm644); err != nil {
		t.Fatalf("write flat: %v", err)
	}

	n, err := reg.MigrateLegacyFlatStateFilesBestEffort()
	if err != nil {
		t.Fatalf("MigrateLegacyFlatStateFilesBestEffort: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected moved=1, got %d", n)
	}
	if _, err := fileutil.Stat(flatPath); !fileutil.IsNotExist(err) {
		t.Fatalf("expected flat file removed, stat err=%v", err)
	}
	nested := reg.stateFilePathForJobExecution(jobID, executionID)
	if _, err := fileutil.Stat(nested); err != nil {
		t.Fatalf("expected nested file at %s: %v", nested, err)
	}
}

func TestJobStateRegistry_CleanStaleLocks(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	// Ensure locks directory exists
	if err := fileutil.MkdirAll(reg.locksDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir locks dir: %v", err)
	}

	// 1. Create a stale lock file (modified 1 hour ago)
	stalePath := filepath.Join(reg.locksDir, "stale.lock")
	if err := fileutil.WriteFile(stalePath, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("write stale: %v", err)
	}
	oldTime := time.Now().Add(-1 * time.Hour)
	if err := fileutil.Chtimes(stalePath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// 2. Create a fresh lock file (modified just now)
	freshPath := filepath.Join(reg.locksDir, "fresh.lock")
	if err := fileutil.WriteFile(freshPath, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("write fresh: %v", err)
	}

	// 3. Create a stale lock file but keep it locked
	lockedPath := filepath.Join(reg.locksDir, "locked.lock")
	if err := fileutil.WriteFile(lockedPath, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("write locked: %v", err)
	}
	if err := fileutil.Chtimes(lockedPath, oldTime, oldTime); err != nil {
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
	if _, err := fileutil.Stat(stalePath); !fileutil.IsNotExist(err) {
		t.Fatalf("expected stale lock to be deleted")
	}

	// Verify fresh remains
	if _, err := fileutil.Stat(freshPath); err != nil {
		t.Fatalf("expected fresh lock to remain: %v", err)
	}

	// Verify locked remains
	if _, err := fileutil.Stat(lockedPath); err != nil {
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
	if err := fileutil.MkdirAll(legacyDir, paths.DirPerm755); err != nil {
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
	if err := fileutil.WriteFile(filepath.Join(legacyDir, executionID+".yaml"), b, paths.FilePerm644); err != nil {
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
	if _, err := fileutil.Stat(dest); err != nil {
		t.Fatalf("expected file at %s: %v", dest, err)
	}
	if _, err := fileutil.Stat(legacyDir); !fileutil.IsNotExist(err) {
		t.Fatalf("expected legacy dir removed, stat err=%v", err)
	}
}

func TestJobStateRegistry_scheduleFullRetentionCleanup_RateLimitedSingleFlight(t *testing.T) {
	t.Parallel()

	prev := stateRetentionFullCleanupMinInterval
	stateRetentionFullCleanupMinInterval = time.Hour
	t.Cleanup(func() { stateRetentionFullCleanupMinInterval = prev })

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	var calls int
	var mu sync.Mutex
	reg.fullCleanupTestHook = func() {
		mu.Lock()
		calls++
		mu.Unlock()
	}

	reg.scheduleFullRetentionCleanup()
	reg.scheduleFullRetentionCleanup()
	reg.scheduleFullRetentionCleanup()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := calls
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	n := calls
	mu.Unlock()
	if n != 1 {
		t.Fatalf("expected exactly 1 full cleanup schedule, got %d", n)
	}

	// Wait for in-flight to finish, then still rate-limited.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		reg.cleanupMu.Lock()
		inflight := reg.fullCleanupInFlight
		reg.cleanupMu.Unlock()
		if !inflight {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	reg.scheduleFullRetentionCleanup()
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	n = calls
	mu.Unlock()
	if n != 1 {
		t.Fatalf("expected rate limit to block second schedule, got %d", n)
	}
}

func TestJobStateRegistry_CompleteExecution_LocalCleanupOnlyHotPath(t *testing.T) {
	t.Parallel()

	prev := stateRetentionFullCleanupMinInterval
	stateRetentionFullCleanupMinInterval = time.Hour
	t.Cleanup(func() { stateRetentionFullCleanupMinInterval = prev })

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)
	reg.retention = time.Hour // expired = older than 1h

	var fullCalls int
	reg.fullCleanupTestHook = func() { fullCalls++ }

	const jobA = "SCH-local-a"
	const jobB = "SCH-local-b"
	if err := reg.RegisterExecution(jobA, "exec-a1", 1); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterExecution(jobB, "exec-b1", 1); err != nil {
		t.Fatal(err)
	}
	if err := reg.CompleteExecution(jobA, "exec-a1", "completed"); err != nil {
		t.Fatal(err)
	}
	if err := reg.CompleteExecution(jobB, "exec-b1", "completed"); err != nil {
		t.Fatal(err)
	}

	// Backdate B's completed file so local cleanup on a later B complete would remove it;
	// create a second B execution and complete it — A's file must remain (local-only).
	bPath := reg.stateFilePathForJobExecution(jobB, "exec-b1")
	bRaw, err := fileutil.ReadFile(bPath)
	if err != nil {
		t.Fatal(err)
	}
	var bst JobExecutionState
	if err := yaml.Unmarshal(bRaw, &bst); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour)
	bst.CompletedAt = &old
	out, err := yaml.Marshal(&bst)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(bPath, out, paths.FilePerm600); err != nil {
		t.Fatal(err)
	}

	if err := reg.RegisterExecution(jobB, "exec-b2", 2); err != nil {
		t.Fatal(err)
	}
	if err := reg.CompleteExecution(jobB, "exec-b2", "completed"); err != nil {
		t.Fatal(err)
	}

	if _, err := fileutil.Stat(bPath); !fileutil.IsNotExist(err) {
		t.Fatalf("expected expired B exec-b1 removed by local cleanup, stat err=%v", err)
	}
	aPath := reg.stateFilePathForJobExecution(jobA, "exec-a1")
	if _, err := fileutil.Stat(aPath); err != nil {
		t.Fatalf("job A state must survive B's local cleanup: %v", err)
	}
	if fullCalls < 1 {
		t.Fatalf("expected at least one scheduled full cleanup attempt, got %d", fullCalls)
	}
}
