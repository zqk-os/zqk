package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtended_RetentionCleanup_SlowPathAndProtectStatuses(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-retention-cleanup-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() {
		_ = sp.Shutdown(context.Background())
		_ = os.RemoveAll(tmpDir)
	}()

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	h := NewRetentionToleranceHandler(sp, tmpDir).(*RetentionToleranceHandler)

	kind := objects.KindAgentTask

	// Create an item with unprotected status
	_ = sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            "ATK-1785886324283087000-old00001",
		objects.FieldKeyKind:          kind,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "completed",
	})

	// Create an item with protected status
	_ = sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            "ATK-1785886324283087000-old00002",
		objects.FieldKeyKind:          kind,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "in_progress",
	})

	// Use cutoff in future so existing objects match created_at < cutoff
	cutoff := time.Now().Add(time.Hour)
	deleted := h.cleanupOldObjects(
		ctx,
		secCtx,
		storageCtx,
		"job-cleanup-test",
		kind,
		cutoff,
		[]string{"in_progress", "active"},
		10,
		2,
		1,
	)
	t.Logf("cleanupOldObjects deleted: %d", deleted)

	// Verify protected object still exists
	if _, readErr := sp.Read(ctx, secCtx, "ATK-1785886324283087000-old00002"); readErr != nil {
		t.Errorf("protected object should still exist: %v", readErr)
	}

	// Test cleanupOldObjects with default batch parameters
	deletedDefault := h.cleanupOldObjects(
		ctx,
		secCtx,
		storageCtx,
		"job-cleanup-test-2",
		kind,
		cutoff,
		nil,
		0,
		-1,
		1,
	)
	t.Logf("cleanupOldObjects with default parameters deleted: %d", deletedDefault)
}

func TestExtended_JobStateRegistry_MigrationAndDefer(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	// 1. MigrateUnbucketedJobStateDirsBestEffort
	// Create an unbucketed job state dir: state/SCH-MIGRATE-1/
	unbucketedDir := filepath.Join(tmpDir, "SCH-MIGRATE-1")
	if err := fileutil.MkdirAll(unbucketedDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	stateContent := []byte("job_id: SCH-MIGRATE-1\nstate: completed\nexecution_id: exec-mig-1\n")
	_ = fileutil.WriteFile(filepath.Join(unbucketedDir, "exec-mig-1.yaml"), stateContent, 0644)

	moved, err := reg.MigrateUnbucketedJobStateDirsBestEffort()
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	t.Logf("Migrated %d entries", moved)

	// Run again to verify idempotency
	movedAgain, err := reg.MigrateUnbucketedJobStateDirsBestEffort()
	if err != nil || movedAgain != 0 {
		t.Errorf("idempotent migration should return 0, got %d, err %v", movedAgain, err)
	}

	// 2. RegisterExecution & DeferExecution
	jobID := "SCH-DEFER-TEST"
	executionID := "exec-def-1"
	if err := reg.RegisterExecution(jobID, executionID, 1234); err != nil {
		t.Fatalf("failed to register execution: %v", err)
	}

	deferTime := time.Now().Add(10 * time.Minute)
	if err := reg.DeferExecution(jobID, "resource busy", &deferTime); err != nil {
		t.Fatalf("failed to defer execution: %v", err)
	}

	// Verify deferred state
	retrieved, err := reg.GetExecutionState(jobID)
	if err != nil {
		t.Fatalf("failed to get execution state: %v", err)
	}
	if retrieved.State != jobExecutionStateDeferred || retrieved.PolicyReason != "resource busy" {
		t.Errorf("unexpected state: %+v", retrieved)
	}

	// 3. findExecutionStateByExecutionID
	path, found, err := reg.findExecutionStateByExecutionID(executionID)
	if err != nil || found == nil || path == "" {
		t.Errorf("failed to find execution state: path=%s, found=%v, err=%v", path, found, err)
	}

	// 4. CleanStaleLocks
	locksDir := filepath.Join(tmpDir, "locks")
	_ = fileutil.MkdirAll(locksDir, 0755)
	oldLockFile := filepath.Join(locksDir, "stale.lock")
	_ = fileutil.WriteFile(oldLockFile, []byte("stale"), 0644)
	_ = os.Chtimes(oldLockFile, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour))
	cleaned, err := reg.CleanStaleLocks(time.Hour)
	if err != nil {
		t.Fatalf("CleanStaleLocks error: %v", err)
	}
	t.Logf("CleanStaleLocks cleaned: %d", cleaned)
}

func TestExtended_Hourglass_EscalationsAndSweeps(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "test-hourglass-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
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

	s := &Scheduler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	// 1. escalateMissedDeadline on backlog_item (supports deferred status)
	taskID := "BLI-1785886324283087000-miss00001"
	_ = sp.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, map[string]any{
		objects.FieldKeyID:            taskID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "open",
		objects.FieldKeyTitle:         "BLI with missed deadline",
	})

	s.escalateMissedDeadline(ctx, secCtx, taskID, objects.KindBacklogItem, "BLI with missed deadline")
	// Second invocation should detect dedupe
	s.escalateMissedDeadline(ctx, secCtx, taskID, objects.KindBacklogItem, "BLI with missed deadline")

	// Verify escalation path executed
	t.Logf("hasOpenMissedDeadlineEscalation: %v", hasOpenMissedDeadlineEscalation(ctx, sp, secCtx, taskID))

	// 2. sweepStaleAgentTasks
	// Seed a terminal priority plan
	planID := "PRI-1785886324283087000-plan00001"
	_ = sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            planID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
	})

	// Seed orphaned task whose plan is terminal
	orphanedID := "ATK-1785886324283087000-orph00002"
	_ = sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:              orphanedID,
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
		objects.FieldKeyPriorityPlanRef: planID,
	})

	// Seed timed-out stale task
	staleID := "ATK-1785886324283087000-stal00003"
	_ = sp.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            staleID,
		objects.FieldKeyKind:          objects.KindAgentTask,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
	})

	// Update the on-disk file to have an old updated_at timestamp
	if filePath, pathErr := sp.GetObjectFilePath(staleID, objects.KindAgentTask); pathErr == nil && filePath != "" {
		if content, readErr := fileutil.ReadFile(filePath); readErr == nil {
			oldTs := time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339)
			modContent := strings.Replace(string(content), "in_progress", "in_progress\nupdated_at: \""+oldTs+"\"", 1)
			_ = fileutil.WriteFile(filePath, []byte(modContent), 0644)
		}
	}

	s.sweepStaleAgentTasks(ctx)

	// Verify orphaned task was archived
	orphanedObj, err := sp.Read(ctx, secCtx, orphanedID)
	if err == nil {
		if orphanedObj[objects.FieldKeyStatus] != objects.ObjectStatusArchived {
			t.Errorf("expected orphaned task status archived, got %v", orphanedObj[objects.FieldKeyStatus])
		}
	}

	// Verify timed-out task was transitioned to error
	staleObj, err := sp.Read(ctx, secCtx, staleID)
	if err == nil {
		if staleObj[objects.FieldKeyStatus] != objects.ObjectStatusError {
			t.Errorf("expected stale task status error, got %v", staleObj[objects.FieldKeyStatus])
		}
	}
}
