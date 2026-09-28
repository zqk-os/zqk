package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/agentclaim"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	riskstatus "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/shared_risk_blockers"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_PIDFile_DeepLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-pid-file-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// 1. Initially no PID file
	pid, err := readPIDFile(tmpDir)
	if err == nil {
		t.Errorf("expected error for non-existent PID file, got pid %d", pid)
	}
	if !isPIDFileNotExist(err) {
		t.Errorf("expected isPIDFileNotExist to be true, got false for: %v", err)
	}

	// 2. Write and Read PID file
	if err := WritePIDFile(tmpDir); err != nil {
		t.Fatalf("WritePIDFile failed: %v", err)
	}
	pid, err = readPIDFile(tmpDir)
	if err != nil {
		t.Fatalf("readPIDFile failed: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), pid)
	}

	// 3. Process running / alive checks
	if !IsDaemonProcessAlive(os.Getpid()) {
		t.Errorf("expected current process to be alive")
	}
	if IsDaemonProcessAlive(99999999) {
		t.Errorf("expected non-existent process to not be alive")
	}
	if isZombieProcess(os.Getpid()) {
		t.Errorf("current process should not be a zombie")
	}
	_ = isSchedulerDaemonProcess(os.Getpid())
	_ = processBelongsToProjectRoot(os.Getpid(), tmpDir)
	_ = processBelongsToProjectRoot(os.Getpid(), "")

	// 4. Keep-alive operations
	alive, lastKA, err := IsSchedulerAlive(tmpDir)
	if err != nil {
		t.Errorf("IsSchedulerAlive err: %v", err)
	}
	// PID is running (current process), but keep-alive file doesn't exist yet
	if alive {
		t.Errorf("expected alive=false before writeKeepAlive")
	}
	_ = lastKA

	if err := writeKeepAlive(tmpDir); err != nil {
		t.Fatalf("writeKeepAlive failed: %v", err)
	}
	kaTime, err := readKeepAlive(tmpDir)
	if err != nil {
		t.Fatalf("readKeepAlive failed: %v", err)
	}
	if time.Since(kaTime) > 10*time.Second {
		t.Errorf("keepalive time too old: %v", kaTime)
	}

	alive, _, err = IsSchedulerAlive(tmpDir)
	if err != nil || !alive {
		t.Errorf("expected alive=true after writeKeepAlive, got alive=%v err=%v", alive, err)
	}

	// Remove KeepAlive
	if err := RemoveKeepAlive(tmpDir); err != nil {
		t.Errorf("RemoveKeepAlive failed: %v", err)
	}
	// Removing again should succeed (no-op)
	if err := RemoveKeepAlive(tmpDir); err != nil {
		t.Errorf("RemoveKeepAlive non-existent should succeed, got: %v", err)
	}

	// 5. IsSchedulerRunning
	running, runPID, err := IsSchedulerRunning(tmpDir)
	if err != nil || !running || runPID != os.Getpid() {
		t.Errorf("IsSchedulerRunning expected true, got %v (pid=%d, err=%v)", running, runPID, err)
	}

	// 6. Remove PID file
	if err := RemovePIDFile(tmpDir); err != nil {
		t.Errorf("RemovePIDFile failed: %v", err)
	}
	if err := RemovePIDFile(tmpDir); err != nil {
		t.Errorf("RemovePIDFile non-existent should succeed: %v", err)
	}

	// 7. Timeout reader helper
	data, err := readFileWithTimeout("/dev/null", 100*time.Millisecond)
	if err != nil {
		t.Errorf("readFileWithTimeout /dev/null failed: %v", err)
	}
	_ = data

	// 8. ResolveProjectRootFromCWD
	_ = ResolveProjectRootFromCWD()
}

func TestExtended_Hourglass_DeepCoverage(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-hourglass-deep-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-hourglass")
	bgCtx = pkgctx.WithPromoteOnCreate(bgCtx)

	s := &Scheduler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
		secCtx:      secCtx,
	}

	// 1. actionForExpiredTimer testing
	if act := actionForExpiredTimer(agentclaim.TimerTypeCheckin); act != actionWakeOrchestrator {
		t.Errorf("expected actionWakeOrchestrator, got %v", act)
	}
	if act := actionForExpiredTimer(timerTypeDeadline); act != actionEscalateDeadline {
		t.Errorf("expected actionEscalateDeadline, got %v", act)
	}
	if act := actionForExpiredTimer("unknown_timer"); act != actionKillStuckProcess {
		t.Errorf("expected actionKillStuckProcess, got %v", act)
	}

	// 2. buildMissedDeadlineRiskBlocker and newMissedDeadlineRiskBlockerID
	id1 := newMissedDeadlineRiskBlockerID()
	if id1 == "" {
		t.Errorf("expected non-empty id")
	}
	blocker, err := buildMissedDeadlineRiskBlocker("", "ATK-100", objects.KindAgentTask, "Test Task")
	if err != nil {
		t.Fatalf("buildMissedDeadlineRiskBlocker failed: %v", err)
	}
	if blocker[objects.FieldKeyStatus] != string(riskstatus.StatusOpen) {
		t.Errorf("expected open status, got %v", blocker[objects.FieldKeyStatus])
	}

	// 3. hourglassSourcePresent & refsContainID
	if hourglassSourcePresent(ctx, sp, secCtx, "NONEXISTENT") {
		t.Errorf("expected false for nonexistent task")
	}
	if !refsContainID([]string{"A", "B", "C"}, "B") {
		t.Errorf("expected true for string slice contain")
	}
	if !refsContainID([]any{"A", "B", "C"}, "C") {
		t.Errorf("expected true for any slice contain")
	}
	if refsContainID([]string{"A"}, "Z") {
		t.Errorf("expected false for non-matching")
	}

	// 4. Seed a backlog_item and escalateMissedDeadline
	itemKind := objects.KindBacklogItem
	taskID := "BLI-hourglass-1"
	if err := sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:            taskID,
		objects.FieldKeyKind:          itemKind,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityTier:  "P0",
		objects.FieldKeyTitle:         "Hourglass Item 1",
		objects.FieldKeyDescription:   "Hourglass item description for testing",
	}); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	if !hourglassSourcePresent(ctx, sp, secCtx, taskID) {
		t.Errorf("expected task to be present")
	}

	// First escalation creates risk blocker and defers item
	s.escalateMissedDeadline(bgCtx, secCtx, taskID, itemKind, "Hourglass Item 1")

	// Verify item was deferred
	taskObj, err := sp.Read(ctx, secCtx, taskID)
	if err != nil {
		t.Fatalf("read task failed: %v", err)
	}
	if taskObj[objects.FieldKeyStatus] != objects.ObjectStatusDeferred {
		t.Errorf("expected deferred status, got %v", taskObj[objects.FieldKeyStatus])
	}

	// Verify hasOpenMissedDeadlineEscalation detects it
	if !hasOpenMissedDeadlineEscalation(ctx, sp, secCtx, taskID) {
		t.Errorf("expected hasOpenMissedDeadlineEscalation to return true")
	}

	// Second escalation should skip duplicate
	s.escalateMissedDeadline(bgCtx, secCtx, taskID, itemKind, "Hourglass Item 1")

	// 5. recordSilentClaimBlocker
	s.recordSilentClaimBlocker(bgCtx, secCtx, taskID)

	// 6. checkHourglassTimers with real timer file in scheduler/hourglass dir
	schedDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.SchedulerDir, "hourglass")
	if err := fileutil.MkdirAll(schedDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to mkdir hourglass: %v", err)
	}

	// Write an expired deadline timer file
	timerContent := map[string]any{
		"task_id":    taskID,
		"pid":        os.Getpid(),
		"expires_at": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"type":       timerTypeDeadline,
		"kind":       itemKind,
	}
	timerBytes, _ := json.Marshal(timerContent)
	timerFile := filepath.Join(schedDir, "timer1.json")
	if err := fileutil.WriteFile(timerFile, timerBytes, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write timer file: %v", err)
	}

	// Also write an expired process timer file
	taskID2 := "ATK-hourglass-2"
	if err := sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 taskID2,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyAssigneePersonaRef: "software_engineer",
		objects.FieldKeyTitle:              "Hourglass Task 2",
		objects.FieldKeyDescription:        "Hourglass task description 2",
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyStatus: "pending_implementation",
			},
		},
	}); err != nil {
		t.Fatalf("create task 2 failed: %v", err)
	}

	timerContent2 := map[string]any{
		"task_id":    taskID2,
		"pid":        0, // avoid killing current process
		"expires_at": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"type":       "process_kill",
		"kind":       objects.KindAgentTask,
	}
	timerBytes2, _ := json.Marshal(timerContent2)
	timerFile2 := filepath.Join(schedDir, "timer2.json")
	_ = fileutil.WriteFile(timerFile2, timerBytes2, paths.FilePerm644)

	// Call checkHourglassTimers
	s.checkHourglassTimers(bgCtx)

	// Verify taskID2 was updated to error
	task2Obj, err := sp.Read(ctx, secCtx, taskID2)
	if err == nil {
		if task2Obj[objects.FieldKeyStatus] != objects.ObjectStatusError {
			t.Errorf("expected task 2 to be in error status, got %v", task2Obj[objects.FieldKeyStatus])
		}
	}

	// 7. sweepStaleAgentTasks
	// Seed a task with terminal parent priority plan
	planID := "PRI-hourglass-term"
	if err := sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:            planID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusComplete,
		objects.FieldKeyTitle:         "Completed Plan",
		objects.FieldKeyDescription:   "Plan that is complete",
	}); err != nil {
		t.Fatalf("create terminal plan failed: %v", err)
	}

	orphanTaskID := "ATK-orphan-1"
	if err := sp.Create(bgCtx, secCtx, map[string]any{
		objects.FieldKeyID:                 orphanTaskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeyAssigneePersonaRef: "software_engineer",
		objects.FieldKeyPriorityPlanRef:    planID,
		objects.FieldKeyTitle:              "Orphan Task",
		objects.FieldKeyDescription:        "Task with completed parent plan",
	}); err != nil {
		t.Fatalf("create orphan task failed: %v", err)
	}

	s.sweepStaleAgentTasks(bgCtx)

	// Verify orphan was archived
	orphanObj, err := sp.Read(ctx, secCtx, orphanTaskID)
	if err == nil && orphanObj[objects.FieldKeyStatus] != objects.ObjectStatusArchived {
		t.Errorf("expected orphan task to be archived, got %v", orphanObj[objects.FieldKeyStatus])
	}
}
