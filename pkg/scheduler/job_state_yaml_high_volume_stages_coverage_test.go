package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	goyaml "gopkg.in/yaml.v3"
)

func TestExtended_JobStateRegistry_StateAndEachYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)

	// 1. Register and GetExecutionState
	jobID := "SCH-deep-reg-1"
	execID := "EXEC-deep-1"
	_ = reg.RegisterExecution(jobID, execID, 9999)

	st, err := reg.GetExecutionState(jobID)
	if err != nil || st == nil {
		t.Errorf("GetExecutionState failed: %v, state: %v", err, st)
	}

	// 2. GetState, UpdateState, ListStates
	legacySt, lErr := reg.GetState(jobID)
	if lErr != nil || legacySt == nil {
		t.Errorf("GetState failed: %v", lErr)
	}
	legacySt.State = "in_progress"
	_ = reg.UpdateState(jobID, legacySt)

	allStates, listErr := reg.ListStates()
	if listErr != nil || len(allStates) == 0 {
		t.Errorf("ListStates failed: %v, states: %d", listErr, len(allStates))
	}

	// 3. DeferExecution with policy decisions
	deferUntil := time.Now().Add(30 * time.Minute)
	_ = reg.DeferExecution(jobID, "exceeded rate limit", &deferUntil)

	// 4. CompleteExecution with failed and completed
	_ = reg.CompleteExecution(jobID, execID, "failed")
	_ = reg.CompleteExecution(jobID, execID, "success")

	// 5. forEachStateYAML
	countYAML := 0
	_ = reg.forEachStateYAML(func(path string) error {
		countYAML++
		return nil
	})
	t.Logf("forEachStateYAML visited: %d files", countYAML)

	// 6. MigrateUnbucketedJobStateDirs with destination collision
	srcDir := filepath.Join(reg.stateDir, "SCH-collide-1")
	b := schedulerStateBucket("SCH-collide-1")
	dstDir := filepath.Join(reg.stateDir, b, "SCH-collide-1")
	_ = fileutil.MkdirAll(srcDir, paths.DirPerm755)
	_ = fileutil.MkdirAll(dstDir, paths.DirPerm755)

	collState := JobExecutionState{
		JobID:       "SCH-collide-1",
		ExecutionID: "EXEC-coll-1",
		State:       "completed",
		StartedAt:   time.Now(),
	}
	collBytes, _ := goyaml.Marshal(collState)
	_ = fileutil.WriteFile(filepath.Join(srcDir, "EXEC-coll-1.yaml"), collBytes, paths.FilePerm600)
	_ = fileutil.WriteFile(filepath.Join(dstDir, "EXEC-coll-1.yaml"), collBytes, paths.FilePerm600)

	moved, mErr := reg.MigrateUnbucketedJobStateDirsBestEffort()
	if mErr != nil {
		t.Errorf("MigrateUnbucketedJobStateDirs collision failed: %v", mErr)
	}
	t.Logf("MigrateUnbucketedJobStateDirs collision moved: %d", moved)
}

func TestExtended_HighVolumeFastPathRetention(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	secCtx := pkgctx.NewSystemSecurityContext()

	// Create audit events (high-volume kind)
	for i := 0; i < 5; i++ {
		ev := map[string]any{
			objects.FieldKeyID:            filepath.Join("AUD-hv-", time.Now().Format("150405000000"), string(rune('a'+i))),
			objects.FieldKeyKind:          objects.KindAuditEvent,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusCompleted,
			objects.FieldKeyCreatedAt:     time.Now().Add(-10 * time.Hour).Format(time.RFC3339),
		}
		_ = sp.Create(ctx, secCtx, ev)
	}

	// Build HighVolumeEventCache for project
	cache := storagepkg.GetGlobalHighVolumeEventCache()
	if cache != nil {
		_ = cache.BuildCache(ctx, tmpDir, sp)
	}

	h := NewRetentionToleranceHandler(sp, tmpDir).(*RetentionToleranceHandler)
	cutoff := time.Now().Add(1 * time.Hour)

	// 1. cleanupOldObjects using populated fast path
	deletedFast := h.cleanupOldObjects(ctx, secCtx, nil, "SCH-hv-ret", objects.KindAuditEvent, cutoff, []string{"draft"}, 10, 2, 2)
	t.Logf("cleanupOldObjects with HV cache deleted: %d", deletedFast)

	// 2. enforceMaxCount using populated HV cache with protect
	delMaxCount, _ := h.enforceMaxCount(ctx, secCtx, nil, "SCH-hv-ret", objects.KindAuditEvent, 0, []string{"draft"}, 10, 2, 2)
	t.Logf("enforceMaxCount with HV cache deleted: %d", delMaxCount)
}

func TestExtended_CapOrchestrator_Stages(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	if cleanup := sp.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	defer func() { _ = sp.Shutdown(ctx) }()

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	capHandler := NewCapOrchestratorHandler(sp, tmpDir, logger).(*CapOrchestratorHandler)

	// Execute stage helpers directly
	_ = capHandler.executeReviewStage(ctx, "echo")
	_ = capHandler.executeMetricsStage(ctx, "echo")
	_ = capHandler.executeSelfImprovementStage(ctx, "echo")
	_ = capHandler.executeSentinelStage(ctx, "echo")
	_ = capHandler.executeGroomingStage(ctx, "echo", "PRI-plan-1", 0, "Test Plan", "active", nil)
}
