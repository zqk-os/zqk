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

func setupRuntimeDeltaPipelineTest(t *testing.T) (testRoot string, str storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext) {
	t.Helper()
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
	secCtx = pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	str = fos
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	return tmpDir, str, secCtx
}

func TestRuntimeDeltaOnlyUpdate_SkipsCASRewrite_UsesOverlay(t *testing.T) {
	testRoot, str, secCtx := setupRuntimeDeltaPipelineTest(t)
	storage.WriteRuntimeDeltaKindsConfigForTest(t, testRoot)

	ctx := pkgctx.NewSystemContext()

	jobID := "SCH-RTDELTA-001"
	job := map[string]any{
		objects.FieldKeyID: jobID, objects.FieldKeyKind: objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "Runtime delta",
		objects.FieldKeyStatus: "active", objects.FieldKeyJobType: "cache_prewarm", objects.FieldKeyTriggerType: "timer",
		objects.FieldKeyScheduleExpression: "*/5 * * * *", objects.FieldKeyCategory: "maintenance",
		objects.FieldKeyExecutionMode: "reusable", objects.FieldKeyEnabled: true,
		objects.FieldKeyCreatedAt: "2030-01-01T00:00:00Z", objects.FieldKeyCreatedBy: "account:system",
		objects.FieldKeyOriginProject: "zqk", objects.FieldKeyOriginSystem: "zqk",
	}
	if err := str.Create(ctx, secCtx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := str.Update(ctx, secCtx, jobID, map[string]any{objects.FieldKeyLastRunAt: "2030-03-20T12:00:00Z"}); err != nil {
		t.Fatalf("Update runtime delta: %v", err)
	}
	overlay := filepath.Join(testRoot, paths.ProjectDataDir, "state", storage.RuntimeDeltaCurrentDirNameForTest, "scheduler_job", jobID+".yaml")
	if _, err := os.Stat(overlay); err != nil {
		t.Fatalf("runtime-delta overlay missing: %v", err)
	}
	readObj, err := str.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Read after runtime-delta update: %v", err)
	}
	if got, _ := readObj[objects.FieldKeyLastRunAt].(string); got != "2030-03-20T12:00:00Z" {
		t.Fatalf("expected last_run_at from overlay, got %q", got)
	}

	listRes, err := str.List(ctx, secCtx, &pkgctx.StorageContext{}, storage.ListFilter{
		Kind:    "scheduler_job",
		Filters: map[string]any{objects.FieldKeyID: jobID},
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("List scheduler_job: %v", err)
	}
	if len(listRes.Objects) != 1 {
		t.Fatalf("List: want 1 object, got %d", len(listRes.Objects))
	}
	gotLR, _ := listRes.Objects[0][objects.FieldKeyLastRunAt].(string)
	if gotLR != "2030-03-20T12:00:00Z" {
		t.Fatalf("List should merge runtime_delta overlay into last_run_at; got %q", gotLR)
	}
}

func TestRuntimeDeltaStructuralUpdate_ClearsOverlay(t *testing.T) {
	testRoot, str, secCtx := setupRuntimeDeltaPipelineTest(t)
	storage.WriteRuntimeDeltaKindsConfigForTest(t, testRoot)
	ctx := pkgctx.NewSystemContext()
	jobID := "SCH-RTDELTA-002"
	job := map[string]any{
		objects.FieldKeyID: jobID, objects.FieldKeyKind: objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "Runtime delta",
		objects.FieldKeyStatus: "active", objects.FieldKeyJobType: "cache_prewarm", objects.FieldKeyTriggerType: "timer",
		objects.FieldKeyScheduleExpression: "*/5 * * * *", objects.FieldKeyCategory: "maintenance",
		objects.FieldKeyExecutionMode: "reusable", objects.FieldKeyEnabled: true,
		objects.FieldKeyCreatedAt: "2030-01-01T00:00:00Z", objects.FieldKeyCreatedBy: "account:system",
		objects.FieldKeyOriginProject: "zqk", objects.FieldKeyOriginSystem: "zqk",
	}
	if err := str.Create(ctx, secCtx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := str.Update(ctx, secCtx, jobID, map[string]any{objects.FieldKeyLastRunAt: "2030-03-20T12:00:00Z"}); err != nil {
		t.Fatalf("runtime update: %v", err)
	}
	if err := str.Update(ctx, secCtx, jobID, map[string]any{objects.FieldKeyTitle: "Structural update"}); err != nil {
		t.Fatalf("structural update: %v", err)
	}
	overlay := filepath.Join(testRoot, paths.ProjectDataDir, "state", storage.RuntimeDeltaCurrentDirNameForTest, "scheduler_job", jobID+".yaml")
	if _, err := os.Stat(overlay); !os.IsNotExist(err) {
		t.Fatalf("expected overlay removed after structural update")
	}
}

func TestRuntimeDeltaFullPayload_WithOnlyRuntimeFieldChange_SkipsCASRewrite(t *testing.T) {
	testRoot, str, secCtx := setupRuntimeDeltaPipelineTest(t)
	storage.WriteRuntimeDeltaKindsConfigForTest(t, testRoot)

	ctx := pkgctx.NewSystemContext()

	jobID := "SCH-RTDELTA-003"
	job := map[string]any{
		objects.FieldKeyID:                 jobID,
		objects.FieldKeyKind:               objects.KindSchedulerJob,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:              "Runtime delta full payload",
		objects.FieldKeyStatus:             "active",
		objects.FieldKeyJobType:            "cache_prewarm",
		objects.FieldKeyTriggerType:        "timer",
		objects.FieldKeyScheduleExpression: "*/5 * * * *",
		objects.FieldKeyCategory:           "maintenance",
		objects.FieldKeyExecutionMode:      "reusable",
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyCreatedAt:          "2030-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:          "account:system",
		objects.FieldKeyOriginProject:      "zqk",
		objects.FieldKeyOriginSystem:       "zqk",
	}
	if err := str.Create(ctx, secCtx, job); err != nil {
		t.Fatalf("Create: %v", err)
	}

	current, err := str.Read(ctx, secCtx, jobID)
	if err != nil {
		t.Fatalf("Read before update: %v", err)
	}
	fullPayload := make(map[string]any, len(current))
	for k, v := range current {
		fullPayload[k] = v
	}
	fullPayload[objects.FieldKeyLastRunAt] = "2030-03-21T12:00:00Z"

	if err := str.Update(ctx, secCtx, jobID, fullPayload); err != nil {
		t.Fatalf("Update full payload runtime change: %v", err)
	}

	overlay := filepath.Join(testRoot, paths.ProjectDataDir, "state", storage.RuntimeDeltaCurrentDirNameForTest, "scheduler_job", jobID+".yaml")
	if _, err := os.Stat(overlay); err != nil {
		t.Fatalf("runtime-delta overlay missing after full payload update: %v", err)
	}
}
