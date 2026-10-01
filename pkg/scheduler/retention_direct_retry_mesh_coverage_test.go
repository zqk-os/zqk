package scheduler

import (
	"bytes"
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestExtended_RetentionMaxCount_DirectPaths(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	h := &RetentionToleranceHandler{
		storage:     sp,
		logger:      logger,
		projectRoot: tmpDir,
	}

	// Create objects of kind "audit_event"
	testKind := objects.KindAuditEvent
	var ids []string
	for i := 0; i < 6; i++ {
		id := "audit-mc-" + string(rune('a'+i))
		ids = append(ids, id)
		obj := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          testKind,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "recorded",
			objects.FieldKeyCreatedAt:     time.Now().Add(-time.Duration(10-i) * time.Hour).Format(time.RFC3339),
		}
		_ = sp.Create(ctx, secCtx, obj)
	}

	// 1. Direct call to enforceMaxCountViaHVNoProtect
	td, handled := h.enforceMaxCountViaHVNoProtect(ctx, secCtx, "SCH-job-1", testKind, 2, 2, 1, 4)
	t.Logf("enforceMaxCountViaHVNoProtect: td=%d, handled=%v", td, handled)

	// 2. Direct call to enforceMaxCountViaHVWithProtect
	td2, handled2 := h.enforceMaxCountViaHVWithProtect(ctx, secCtx, "SCH-job-2", testKind, 2, 2, 1, 4, []string{"active"})
	t.Logf("enforceMaxCountViaHVWithProtect: td=%d, handled=%v", td2, handled2)

	// 3. Direct call to enforceMaxCountBatchedList (protected overfill branch)
	td3, err := h.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, "SCH-job-3", testKind, 2, []string{"active"}, 2, 2, 1, 6, 4, ids)
	if err != nil {
		t.Logf("enforceMaxCountBatchedList err: %v", err)
	}
	t.Logf("enforceMaxCountBatchedList: td=%d", td3)

	// 3b. Deletable branch (status completed, not active)
	var delIDs []string
	for i := 0; i < 4; i++ {
		id := "audit-del-" + string(rune('a'+i))
		delIDs = append(delIDs, id)
		obj := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          testKind,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        "completed",
			objects.FieldKeyCreatedAt:     time.Now().Add(-time.Duration(10-i) * time.Hour).Format(time.RFC3339),
		}
		_ = sp.Create(ctx, secCtx, obj)
	}
	tdDel, errDel := h.enforceMaxCountBatchedList(ctx, secCtx, storageCtx, "SCH-job-del", testKind, 2, []string{"active"}, 2, 2, 1, 10, 8, delIDs)
	t.Logf("enforceMaxCountBatchedList deletable: td=%d, err=%v", tdDel, errDel)

	// 4. Test with empty / fallback paths
	_, _ = h.enforceMaxCountViaHVNoProtect(ctx, secCtx, "SCH-job-4", "nonexistent_kind", 2, 2, 1, 4)
	_, _ = h.enforceMaxCountViaHVWithProtect(ctx, secCtx, "SCH-job-5", "nonexistent_kind", 2, 2, 1, 4, []string{"active"})
}

func TestExtended_RunWrapperRetry_Detailed(t *testing.T) {
	ctx := context.Background()
	sp := storagepkg.NewNoopObjectStorage()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	tmpDir := t.TempDir()

	h := NewRunWrapperHandlerWithProjectRoot(sp, logger, nil, nil, tmpDir).(*RunWrapperHandler)

	job := &ScheduledJob{
		ID:                "SCH-test-retry-job",
		Command:           "echo",
		CommandArgs:       []string{"hello"},
		MaxRuntimeSeconds: 30,
	}

	// 1. effectiveRunWrapperTimeoutSeconds
	to := effectiveRunWrapperTimeoutSeconds(10, job)
	if to != 10 {
		t.Errorf("expected 10, got %d", to)
	}
	to = effectiveRunWrapperTimeoutSeconds(0, job)
	if to != 30 {
		t.Errorf("expected 30, got %d", to)
	}
	jobNoMax := &ScheduledJob{ID: "SCH-no-max"}
	to = effectiveRunWrapperTimeoutSeconds(0, jobNoMax)
	if to != DefaultMaxRuntimeSeconds {
		t.Errorf("expected default timeout, got %d", to)
	}

	// 2. getExecutor
	exec := h.getExecutor()
	if exec == nil {
		t.Error("expected non-nil executor")
	}

	// 3. runWrapperCollectTestFailuresFromOutput
	buf := bytes.NewBuffer(nil)
	sw := newStreamingOutputWriter(buf, 1024)
	sw.Write([]byte("--- FAIL: TestFoo (0.01s)\nFAIL\n"))
	fails, summary := h.runWrapperCollectTestFailuresFromOutput(job, false, sw, sw, "")
	t.Logf("Collected test failures: %+v, summary: %+v", fails, summary)

	// 4. emitBundleProgress
	h.emitBundleProgress(ctx, job, runWrapperEventStarted)
	h.emitBundleProgress(ctx, job, runWrapperEventCompleted)
}

func TestExtended_MeshLeaseSupervision_Execute(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sp, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	secCtx := pkgctx.NewSystemSecurityContext()

	h := NewMeshLeaseSupervisionHandler(sp, tmpDir, logger).(*MeshLeaseSupervisionHandler)

	// Seed session object
	sessObj := map[string]any{
		objects.FieldKeyID:            "SESS-123",
		objects.FieldKeyKind:          objects.KindZqkSession,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        "active",
		"lease_state":                 "active",
	}
	_ = sp.Create(ctx, secCtx, sessObj)

	job := &ScheduledJob{
		ID: "SCH-mesh-lease-job",
	}

	_ = h.Execute(ctx, job)
}
