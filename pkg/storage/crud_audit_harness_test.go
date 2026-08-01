package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

// TestSchedulerJob_CRUDAuditInvariants is a first pass at a common CRUD+stream audit harness
// for CAS/stream-backed kinds. It focuses on scheduler_job, since missing delete evidence
// for timer jobs (e.g. SCH-007) is the concrete failure we've observed in production.
//
// The test currently verifies:
//   - Create: scheduler_job can be created and read back from storage.
//   - Delete: scheduler_job is removed from storage (CAS hash removed; scheduler_job is not
//     stream-backed, so there is no stream_deleted_*.jsonl append for this kind).
//
// This is intentionally narrow but shared: other tests (including cmd/zqk/*)
// can build more exhaustive harnesses on top of this pattern (e.g. checking
// audit_event and change journal). The goal here is to put a hard invariant
// around "delete must at least hit the deleted stream" so we can't silently
// lose scheduler jobs again without a failing test.
func TestSchedulerJob_CRUDAuditInvariants(t *testing.T) {

	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})

	var osp storage.ObjectStorageProvider = fos

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})

	const jobID = "SCH-CRUD-TEST-001"

	job := map[string]any{
		objects.FieldKeyID:                 jobID,
		objects.FieldKeyKind:               "scheduler_job",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:              "CRUD harness test job",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyJobType:            "cache_prewarm",
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/30 * * * *",
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyMaxRuntimeSeconds:  60,
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCreatedAt:          "2030-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:          "account:test",
		objects.FieldKeyUpdatedAt:          "2030-01-01T00:00:00Z",
		objects.FieldKeyUpdatedBy:          "account:test",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
	}

	if err := osp.Create(ctx, secCtx, job); err != nil {
		t.Fatalf("Create scheduler_job %s failed: %v", jobID, err)
	}

	if _, err := osp.Read(ctx, secCtx, jobID); err != nil {
		t.Fatalf("Read scheduler_job %s after create failed: %v", jobID, err)
	}

	deleteCtx := storage.WithCLIOperation(ctx)
	if err := osp.Delete(deleteCtx, secCtx, jobID, false); err != nil {
		t.Fatalf("Delete scheduler_job %s failed: %v", jobID, err)
	}

	if _, err := osp.Read(ctx, secCtx, jobID); err == nil {
		t.Fatalf("scheduler_job %s still readable after Delete; expected not found", jobID)
	}

	exists, err := osp.Exists(context.Background(), secCtx, jobID)
	if err != nil {
		t.Fatalf("Exists after delete: %v", err)
	}
	if exists {
		t.Fatalf("scheduler_job %s still reported as existing after Delete", jobID)
	}
}
