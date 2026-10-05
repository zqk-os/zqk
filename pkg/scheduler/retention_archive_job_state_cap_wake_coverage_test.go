package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_RetentionTolerance_ArchiveDeep(t *testing.T) {
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

	rth := &RetentionToleranceHandler{
		storage:     sp,
		projectRoot: tmpDir,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// 1. isCatalogKind
	if !isCatalogKind("risk_blocker") || !isCatalogKind("workflow") || isCatalogKind("scheduler_job") {
		t.Errorf("unexpected isCatalogKind result")
	}

	// 2. archiveOldObjects
	cutoff := time.Now().Add(-1 * time.Hour)
	archived := rth.archiveOldObjects(ctx, secCtx, storageCtx, "SCH-job-1", objects.KindPriorityPlan, cutoff, 5, 2)
	t.Logf("archiveOldObjects archived: %d", archived)

	// Cancellation check
	ctxCancelled, cancel := context.WithCancel(ctx)
	cancel()
	if rth.archiveOldObjects(ctxCancelled, secCtx, storageCtx, "SCH-job-1", objects.KindPriorityPlan, cutoff, 5, 2) != 0 {
		t.Errorf("expected 0 for cancelled context")
	}

	// Kind with no archive status
	if rth.archiveOldObjects(ctx, secCtx, storageCtx, "SCH-job-1", "unknown_no_archive_status_kind", cutoff, 5, 2) != 0 {
		t.Errorf("expected 0 for kind without archive status")
	}
}

func TestExtended_JobStateRegistry_CleanupAndMigrate(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	// Create test state files in job directory
	jobDir := filepath.Join(reg.stateDir, "sc", "SCH-test-cleanup")
	_ = os.MkdirAll(jobDir, 0755)

	oldStateFile := filepath.Join(jobDir, "exec-old.yaml")
	_ = os.WriteFile(oldStateFile, []byte("state: completed\nexecution_id: exec-old\ncompleted_at: 2020-01-01T00:00:00Z\n"), 0644)

	stats := &stateRetentionCleanupStats{}
	reg.maybeRemoveExpiredStateFile(oldStateFile, time.Now().Add(1*time.Hour), stats)
	if stats.RemovedStateFiles == 0 {
		t.Errorf("expected expired state file removed")
	}

	_ = reg.cleanupCompletedInDirBestEffort(jobDir)

	// Migrate unbucketed dirs
	unbucketedDir := filepath.Join(reg.stateDir, "SCH-unbucketed")
	_ = os.MkdirAll(unbucketedDir, 0755)
	_ = os.WriteFile(filepath.Join(unbucketedDir, "state.yaml"), []byte("state: running\njob_id: SCH-unbucketed\n"), 0644)
	moved, err := reg.MigrateUnbucketedJobStateDirsBestEffort()
	if err != nil {
		t.Errorf("MigrateUnbucketedJobStateDirsBestEffort failed: %v", err)
	}
	t.Logf("moved unbucketed dirs: %d", moved)
}

func TestExtended_CapOrchestrator_WakeAndTopBLIs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	mockStore := &mockCapStorage{
		listed: map[string][]map[string]any{
			objects.KindPriorityPlan: {
				{
					objects.FieldKeyID:     "PRI-active-1",
					objects.FieldKeyStatus: objects.ObjectStatusActive,
				},
			},
			"backlog_item": {
				{
					objects.FieldKeyID:              "BLI-1",
					objects.FieldKeyPriorityPlanRef: "PRI-active-1",
					objects.FieldKeyTitle:           "Top P0 BLI",
					objects.FieldKeyStatus:          "active",
				},
			},
		},
	}

	h := &CapOrchestratorHandler{
		storage:     mockStore,
		projectRoot: tmpDir,
		logger:      logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}

	ctx := context.Background()

	// 1. resolveWakePlanID
	idDirect := h.resolveWakePlanID(ctx, "PRI-direct")
	if idDirect != "PRI-direct" {
		t.Errorf("expected PRI-direct, got %s", idDirect)
	}
	idFromStorage := h.resolveWakePlanID(ctx, "ATK-task-1")
	if idFromStorage != "PRI-active-1" {
		t.Errorf("expected PRI-active-1 from active priority plan, got %s", idFromStorage)
	}

	// 2. topOpenPlanBLIs
	blis := h.topOpenPlanBLIs(ctx, "PRI-active-1", 3)
	t.Logf("topOpenPlanBLIs found: %d", len(blis))
	if h.topOpenPlanBLIs(ctx, "", 3) != nil {
		t.Errorf("expected nil for empty planID")
	}
	if h.topOpenPlanBLIs(ctx, "PRI-1", 0) != nil {
		t.Errorf("expected nil for n=0")
	}

	// 3. wakeAgentAndScheduleHourglass
	h.wakeAgentAndScheduleHourglass("ATK-wake-1", "architect")
	h.wakeAgentAndScheduleHourglass("ATK-wake-2", "tpm")
}
