package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_RetentionCleanupAndTolerance(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	rth := NewRetentionToleranceHandler(sp, tmpDir).(*RetentionToleranceHandler)

	var msgs []string
	rth.SetProgressFunc(func(m string) {
		msgs = append(msgs, m)
	})

	// 1. Helpers in handlers_retention_tolerance.go
	if getBulkDeleteWorkers(nil) != defaultBulkDeleteWorkers {
		t.Errorf("expected default bulk delete workers")
	}
	jobWithWorkers := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyBulkDeleteWorkers: "100",
		},
	}
	if getBulkDeleteWorkers(jobWithWorkers) != maxBulkDeleteWorkers {
		t.Errorf("expected workers to be capped at maxBulkDeleteWorkers")
	}
	jobWithWorkersNorm := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyBulkDeleteWorkers: "10",
		},
	}
	if getBulkDeleteWorkers(jobWithWorkersNorm) != 10 {
		t.Errorf("expected 10 workers")
	}

	// Batch config
	bSize, maxB := getBatchConfig(nil)
	if bSize != defaultRetentionToleranceBatchSize || maxB != defaultRetentionToleranceMaxBatches {
		t.Errorf("unexpected default batch config")
	}
	jobWithBatches := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyBatchSize:  "100",
			EnvKeyMaxBatches: "-1",
		},
	}
	bSize, maxB = getBatchConfig(jobWithBatches)
	if bSize != 100 || maxB != retentionToleranceUnlimitedMaxBatches {
		t.Errorf("unexpected batch config: %d, %d", bSize, maxB)
	}

	jobWithBatchesPos := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyMaxBatches: "5",
		},
	}
	_, maxB = getBatchConfig(jobWithBatchesPos)
	if maxB != 5 {
		t.Errorf("expected 5 max batches, got %d", maxB)
	}

	// retentionKindPriority
	if retentionKindPriority(objects.KindAuditEvent) != 0 {
		t.Errorf("expected audit_event to have priority 0")
	}
	if retentionKindPriority(objects.KindMcpSession) != 1 {
		t.Errorf("expected mcp_session to have priority 1")
	}
	if retentionKindPriority("request_metric") != 2 {
		t.Errorf("expected metric to have priority 2")
	}
	if retentionKindPriority("other_kind") != 3 {
		t.Errorf("expected other to have priority 3")
	}

	// getKindFilter
	if getKindFilter(nil) != nil {
		t.Errorf("expected nil filter for nil job")
	}
	jobFilter := &ScheduledJob{
		EnvironmentVariables: map[string]string{
			EnvKeyKinds: "audit_event, backlog_item, ",
		},
	}
	f := getKindFilter(jobFilter)
	if f == nil || !f["audit_event"] || !f["backlog_item"] || f[""] {
		t.Errorf("unexpected kind filter: %v", f)
	}

	// 2. cleanupOldObjects & cleanupOldObjectsSlowPath
	ctx := context.Background()
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-reason")
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Seed some agent tasks
	for i := 0; i < 5; i++ {
		task := map[string]any{
			objects.FieldKeyID:                 fmt.Sprintf("ATK-clean-w35-%d", i),
			objects.FieldKeyKind:               objects.KindAgentTask,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:          time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
			objects.FieldKeyStatus:             objects.ObjectStatusError,
			objects.FieldKeyTitle:              fmt.Sprintf("Clean Task %d", i),
			objects.FieldKeyDescription:        "Clean task description",
			objects.FieldKeyAssigneePersonaRef: "software_engineer",
		}
		if err := sp.Create(bgCtx, secCtx, task); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	// Test cleanupOldObjects with storage (slow path fallback)
	cutoff := time.Now().Add(-1 * time.Hour)
	deleted := rth.cleanupOldObjects(ctx, secCtx, storageCtx, "SCH-clean", objects.KindAgentTask, cutoff, []string{objects.ObjectStatusInProgress}, 2, 2, 2)
	t.Logf("cleanupOldObjects deleted %d items", deleted)

	// Test cleanupOldObjects with unlimited max batches (-1)
	deletedUnlimited := rth.cleanupOldObjects(ctx, secCtx, storageCtx, "SCH-clean", objects.KindAgentTask, cutoff, nil, 2, -1, 2)
	t.Logf("cleanupOldObjects unlimited deleted: %d", deletedUnlimited)

	// Test with cancelled context to trigger interrupt checker
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_ = rth.cleanupOldObjects(cancelCtx, secCtx, storageCtx, "SCH-clean", objects.KindAgentTask, cutoff, nil, 2, 2, 2)
	_ = rth.cleanupOldObjectsSlowPath(cancelCtx, secCtx, storageCtx, "SCH-clean", objects.KindAgentTask, nil, 2, 2, 2, cutoff.Format(time.RFC3339))

	// Test executeRetentionToleranceCore with kind filter resulting in empty
	jobFilteredOut := &ScheduledJob{
		ID: "SCH-tol",
		EnvironmentVariables: map[string]string{
			EnvKeyKinds: "nonexistent_kind_xyz",
		},
	}
	err = rth.executeRetentionToleranceCore(ctx, jobFilteredOut)
	if err != nil {
		t.Errorf("expected no error for empty filtered kinds, got: %v", err)
	}

	// Test executeRetentionToleranceCore with invalid config file path
	rthBad := NewRetentionToleranceHandler(sp, "/nonexistent/path/that/fails").(*RetentionToleranceHandler)
	_ = rthBad.executeRetentionToleranceCore(ctx, &ScheduledJob{ID: "SCH-tol2"})
}

func TestExtended_RetentionCleanup_FileStorageCASPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	fs, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create file object storage: %v", err)
	}
	if cleanup := fs.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() {
		_ = fs.Shutdown(context.Background())
	}()

	rth := NewRetentionToleranceHandler(fs, tmpDir).(*RetentionToleranceHandler)

	ctx := context.Background()
	bgCtx := pkgctx.WithLifecycleBreakGlass(pkgctx.WithAllowCoreObjectDelete(ctx), "test-reason")
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Create objects of kind agent_task
	for i := 0; i < 3; i++ {
		task := map[string]any{
			objects.FieldKeyID:                 fmt.Sprintf("ATK-cas-%d", i),
			objects.FieldKeyKind:               objects.KindAgentTask,
			objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:          time.Now().Add(-5 * time.Hour).Format(time.RFC3339),
			objects.FieldKeyStatus:             objects.ObjectStatusError,
			objects.FieldKeyTitle:              fmt.Sprintf("CAS Task %d", i),
			objects.FieldKeyDescription:        "CAS task description",
			objects.FieldKeyAssigneePersonaRef: "software_engineer",
		}
		if err := fs.Create(bgCtx, secCtx, task); err != nil {
			t.Fatalf("failed to create object: %v", err)
		}
	}

	// Run cleanup with FileObjectStorage
	cutoff := time.Now().Add(-1 * time.Hour)
	del := rth.cleanupOldObjectsSlowPath(ctx, secCtx, storageCtx, "SCH-cas", objects.KindAgentTask, []string{objects.ObjectStatusInProgress}, 2, 2, 2, cutoff.Format(time.RFC3339))
	t.Logf("cleanupOldObjectsSlowPath with file storage deleted: %d", del)

	// Run again with unlimited maxBatches
	del2 := rth.cleanupOldObjectsSlowPath(ctx, secCtx, storageCtx, "SCH-cas", objects.KindAgentTask, nil, 2, retentionToleranceUnlimitedMaxBatches, 2, cutoff.Format(time.RFC3339))
	t.Logf("cleanupOldObjectsSlowPath with unlimited deleted: %d", del2)
}

func TestExtended_IdleCleanup_DeepCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
	}()

	handler := NewIdleCleanupHandler(tmpDir, logger, sp)

	ctx := context.Background()
	job := &ScheduledJob{ID: "SCH-idle"}

	// Create run directory with old and new .pid files
	runDir := filepath.Join(tmpDir, paths.ProjectDataDir, "run")
	if err := fileutil.MkdirAll(runDir, 0755); err != nil {
		t.Fatalf("failed to mkdir run dir: %v", err)
	}

	oldPid := filepath.Join(runDir, "old.pid")
	if err := os.WriteFile(oldPid, []byte("12345"), 0644); err != nil {
		t.Fatalf("failed to write old pid: %v", err)
	}
	// Set modtime to 48 hours ago
	oldTime := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(oldPid, oldTime, oldTime)

	newPid := filepath.Join(runDir, "new.pid")
	if err := os.WriteFile(newPid, []byte("67890"), 0644); err != nil {
		t.Fatalf("failed to write new pid: %v", err)
	}

	nonPid := filepath.Join(runDir, "not_a_pid.txt")
	if err := os.WriteFile(nonPid, []byte("ignore"), 0644); err != nil {
		t.Fatalf("failed to write non-pid: %v", err)
	}

	// Create agent worktrees directory
	agentWorktrees := paths.AgentWorktreeContainer(tmpDir)
	_ = fileutil.MkdirAll(filepath.Join(agentWorktrees, "ATK-dummy-1"), 0755)
	_ = fileutil.MkdirAll(filepath.Join(agentWorktrees, "ATK-dummy-2"), 0755)

	legacyWorktrees := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WorktreesSubdir)
	_ = fileutil.MkdirAll(filepath.Join(legacyWorktrees, "ATK-legacy-1"), 0755)

	// Execute idle cleanup
	err = handler.Execute(ctx, job)
	if err != nil {
		t.Errorf("unexpected error in IdleCleanupHandler.Execute: %v", err)
	}

	// Verify oldPid was cleaned up, newPid remains
	if fileutil.Exists(oldPid) {
		t.Errorf("expected old.pid to be removed")
	}
	if !fileutil.Exists(newPid) {
		t.Errorf("expected new.pid to still exist")
	}

	// Direct tests on shouldDropAgentWorktree
	secCtx := pkgctx.NewSystemSecurityContext()
	checker := objects.GetGlobalStatusChecker()

	// Missing task -> drop
	reason, drop := handler.shouldDropAgentWorktree(ctx, secCtx, checker, "ATK-missing", filepath.Join(agentWorktrees, "ATK-missing"))
	if !drop {
		t.Errorf("expected missing task to be dropped")
	}
	if reason == "" {
		t.Errorf("expected non-empty reason")
	}

	// Old directory age check
	staleDir := filepath.Join(agentWorktrees, "ATK-stale")
	_ = fileutil.MkdirAll(staleDir, 0755)
	_ = os.Chtimes(staleDir, oldTime, oldTime)
	reasonStale, dropStale := handler.shouldDropAgentWorktree(ctx, secCtx, checker, "ATK-stale", staleDir)
	if !dropStale {
		t.Errorf("expected stale dir to be dropped")
	}
	if reasonStale == "" {
		t.Errorf("expected non-empty reason for stale dir")
	}
}
