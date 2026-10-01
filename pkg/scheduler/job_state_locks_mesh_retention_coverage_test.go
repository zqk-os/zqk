package scheduler

import (
	"context"
	"os/exec"
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

func TestExtended_JobStateRegistry_MigrationsAndLocks_Wave44(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	reg := NewJobStateRegistry(tmpDir).(*JobStateRegistry)
	if reg == nil {
		t.Fatalf("expected non-nil registry")
	}

	// 1. Helpers
	_ = sanitizeJobIDForPathSegment("SCH/test:123")
	_ = isReservedStateEntry(".hints")
	_ = isReservedStateEntry("locks")
	_ = isReservedStateEntry("normal.yaml")
	_ = reg.lockPathForJobID("SCH-job-1")
	_ = reg.lockPathForExecutionID("EXEC-1")

	// 2. withFileLock
	lockPath := filepath.Join(tmpDir, "test.lock")
	lockErr := withFileLock(lockPath, 100*time.Millisecond, func() error {
		return nil
	})
	if lockErr != nil {
		t.Errorf("expected nil error from withFileLock, got: %v", lockErr)
	}

	// 3. Stale locks cleanup
	cleaned, err := reg.CleanStaleLocks(0)
	if err != nil {
		t.Errorf("CleanStaleLocks failed: %v", err)
	}
	t.Logf("CleanStaleLocks cleaned: %d", cleaned)

	// 4. MigrateLegacyFlatStateFilesBestEffort
	// Create a legacy flat YAML in stateDir
	flatState := JobExecutionState{
		JobID:       "SCH-flat-1",
		ExecutionID: "EXEC-flat-1",
		State:       "running",
		StartedAt:   time.Now(),
	}
	flatBytes, _ := goyaml.Marshal(flatState)
	flatPath := filepath.Join(reg.stateDir, "SCH-flat-1.yaml")
	_ = fileutil.MkdirAll(reg.stateDir, paths.DirPerm755)
	_ = fileutil.WriteFile(flatPath, flatBytes, paths.FilePerm600)

	moved, mErr := reg.MigrateLegacyFlatStateFilesBestEffort()
	if mErr != nil {
		t.Errorf("MigrateLegacyFlatStateFilesBestEffort failed: %v", mErr)
	}
	t.Logf("MigrateLegacyFlatStateFilesBestEffort moved: %d", moved)

	// 5. MigrateUnbucketedJobStateDirsBestEffort
	unbucketedDir := filepath.Join(reg.stateDir, "SCH-unbucketed-1")
	_ = fileutil.MkdirAll(unbucketedDir, paths.DirPerm755)
	unbState := JobExecutionState{
		JobID:       "SCH-unbucketed-1",
		ExecutionID: "EXEC-unb-1",
		State:       "completed",
		StartedAt:   time.Now(),
	}
	unbBytes, _ := goyaml.Marshal(unbState)
	_ = fileutil.WriteFile(filepath.Join(unbucketedDir, "EXEC-unb-1.yaml"), unbBytes, paths.FilePerm600)

	movedUnb, mUnbErr := reg.MigrateUnbucketedJobStateDirsBestEffort()
	if mUnbErr != nil {
		t.Errorf("MigrateUnbucketedJobStateDirsBestEffort failed: %v", mUnbErr)
	}
	t.Logf("MigrateUnbucketedJobStateDirsBestEffort moved: %d", movedUnb)

	// 6. CompleteExecution & DeferExecution
	_ = reg.RegisterExecution("SCH-exec-test", "EXEC-test-1", 12345)
	_ = reg.CompleteExecution("SCH-exec-test", "EXEC-test-1", "success")
	_ = reg.CompleteExecution("SCH-exec-test", "", "failed")

	deferUntil := time.Now().Add(1 * time.Hour)
	_ = reg.DeferExecution("SCH-exec-test", "rate limited", &deferUntil)
	_ = reg.DeferExecution("SCH-exec-test", "no time", nil)

	// 7. Summarize & ListInProgress
	sum, sErr := reg.Summarize(10 * time.Minute)
	if sErr != nil {
		t.Errorf("Summarize failed: %v", sErr)
	}
	if sum == nil {
		t.Fatalf("expected non-nil summary")
	}

	inProg, ipErr := reg.ListInProgress()
	if ipErr != nil {
		t.Errorf("ListInProgress failed: %v", ipErr)
	}
	t.Logf("ListInProgress found %d states", len(inProg))

	// 8. Retention cleanup helpers
	stats := &stateRetentionCleanupStats{}
	reg.maybeRemoveExpiredStateFile(flatPath, time.Now().Add(1*time.Hour), stats)
	_ = reg.cleanupCompletedInDirBestEffort(unbucketedDir)
	_ = reg.cleanupCompletedBestEffort()
	reg.recordStateRetentionCleanup(*stats)
	_ = reg.writeStateHintsFile()
}

func TestExtended_MeshLeaseSupervision_Wave44(t *testing.T) {
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
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	handler := NewMeshLeaseSupervisionHandler(sp, tmpDir, logger)
	if handler == nil {
		t.Fatalf("expected non-nil handler")
	}

	// 1. getFloat helper
	if getFloat(float64(10.5)) != 10.5 || getFloat(float32(5.5)) != 5.5 || getFloat(int(42)) != 42 || getFloat(int64(99)) != 99 || getFloat("invalid") != 0 {
		t.Errorf("unexpected getFloat behavior")
	}

	// 2. reapStaleSubprocesses with dummy cmd
	activeLeases := map[string]bool{"LEASE-active": true}
	cmd := exec.CommandContext(t.Context(), "true")
	_ = cmd.Start()
	leaseSupervisionMutex.Lock()
	activeLeaseSubprocesses["LEASE-stale"] = cmd
	leaseSupervisionMutex.Unlock()

	hImpl := handler.(*MeshLeaseSupervisionHandler)
	hImpl.reapStaleSubprocesses(activeLeases)

	// 3. Execute with empty list (no sessions)
	job := &ScheduledJob{
		ID:       "SCH-mesh-lease-test",
		JobType:  "mesh_lease_supervision",
		Category: CategoryMaintenance,
	}
	_ = handler.Execute(ctx, job)

	// 4. Create revoked and quota-exhausted sessions
	revokedSession := map[string]any{
		objects.FieldKeyID:                "SES-revoked",
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeySessionMode:       "federated_lease",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyProviderKernelRef: "kernel-dummy",
		objects.FieldKeyRevokedAt:         time.Now().Format(time.RFC3339),
	}
	_ = sp.Create(ctx, secCtx, revokedSession)

	quotaSession := map[string]any{
		objects.FieldKeyID:                "SES-quota",
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeySessionMode:       "federated_lease",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyProviderKernelRef: "kernel-dummy",
		objects.FieldKeyTermType:          "fixed",
		objects.FieldKeyMaxUnits:          100.0,
		objects.FieldKeyConsumedUnits:     150.0,
	}
	_ = sp.Create(ctx, secCtx, quotaSession)

	_ = handler.Execute(ctx, job)
}

func TestExtended_RetentionToleranceAndMaxCount_Wave44(t *testing.T) {
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

	h := NewRetentionToleranceHandler(sp, tmpDir).(*RetentionToleranceHandler)
	h.SetProgressFunc(func(msg string) {})
	h.emitProgress("test progress")

	// 1. Static classification helpers
	_ = isHighVolumeKind(objects.KindAuditEvent)
	_ = isHighVolumeKind(objects.KindSchedulerJob)
	_ = isCatalogKind(objects.KindObjectSpec)
	_ = isCatalogKind(objects.KindLifecycle)
	_ = isCatalogKind(objects.KindSchedulerJob)
	_ = skipArchiveForOldestIDsPath(objects.KindAuditEvent, []string{"active"})
	_ = skipArchiveForOldestIDsPath(objects.KindAuditEvent, nil)

	dummyJob := &ScheduledJob{
		ID: "SCH-ret-test",
		EnvironmentVariables: map[string]string{
			EnvKeyBatchSize:         "50",
			EnvKeyMaxBatches:        "2",
			EnvKeyBulkDeleteWorkers: "4",
			EnvKeyKinds:             objects.KindAuditEvent + "," + objects.KindAuditAggregationMetric,
		},
	}
	bSize, mBatches := getBatchConfig(dummyJob)
	if bSize != 50 || mBatches != 2 {
		t.Errorf("unexpected batch config: %d, %d", bSize, mBatches)
	}
	workers := getBulkDeleteWorkers(dummyJob)
	if workers != 4 {
		t.Errorf("unexpected workers: %d", workers)
	}
	kFilter := getKindFilter(dummyJob)
	if !kFilter[objects.KindAuditEvent] {
		t.Errorf("expected kind in filter")
	}
	_ = retentionKindPriority(objects.KindAuditEvent)
	_ = retentionKindPriority(objects.KindAuditAggregationMetric)
	_ = retentionKindPriority("unknown_kind")

	// 2. archiveListedObjects
	objs := []map[string]any{
		{
			objects.FieldKeyID:            "", // empty id branch
			objects.FieldKeyKind:          objects.KindSchedulerJob,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "SCH-reusable-1",
			objects.FieldKeyKind:          objects.KindSchedulerJob,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyExecutionMode: "reusable", // reusable branch
		},
		{
			objects.FieldKeyID:            "PRI-plan-1",
			objects.FieldKeyKind:          objects.KindPriorityPlan,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusComplete, // priority plan branch
		},
	}
	_ = sp.Create(ctx, secCtx, objs[1])
	_ = sp.Create(ctx, secCtx, objs[2])

	archivedCount := h.archiveListedObjects(ctx, secCtx, "SCH-job-ret", objects.KindPriorityPlan, objects.ObjectStatusArchived, objs)
	t.Logf("archiveListedObjects count: %d", archivedCount)

	// 3. archiveOldObjects
	cutoff := time.Now().Add(1 * time.Hour)
	_ = h.archiveOldObjects(ctx, secCtx, nil, "SCH-job-ret", objects.KindPriorityPlan, cutoff, 10, 2)
	_ = h.archiveOldObjects(ctx, secCtx, nil, "SCH-job-ret", "unknown_no_archive_status_kind", cutoff, 10, 2)

	// 4. enforceMaxCountViaHVNoProtect & enforceMaxCountViaHVWithProtect
	_, _ = h.enforceMaxCountViaHVNoProtect(ctx, secCtx, "SCH-job-ret", objects.KindAuditEvent, 5, 2, 2, 10)
	_, _ = h.enforceMaxCountViaHVWithProtect(ctx, secCtx, "SCH-job-ret", objects.KindAuditEvent, 5, 2, 2, 10, []string{"active"})

	// 5. enforceMaxCount
	_, _ = h.enforceMaxCount(ctx, secCtx, nil, "SCH-job-ret", objects.KindAuditEvent, 0, nil, 10, 1, 2)
	_, _ = h.enforceMaxCount(ctx, secCtx, nil, "SCH-job-ret", objects.KindPriorityPlan, 100, []string{"active"}, 10, 1, 2)

	// 6. cleanupOldObjects
	deleted := h.cleanupOldObjects(ctx, secCtx, nil, "SCH-job-ret", objects.KindAuditEvent, cutoff, nil, 10, 2, 2)
	t.Logf("cleanupOldObjects deleted: %d", deleted)

	// 7. Execute handler
	_ = h.Execute(ctx, dummyJob)
}
