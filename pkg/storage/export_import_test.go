package storage_test

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestExportObjects_Empty(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	st := setupFileStorageForExportImport(t)

	filter := storage.ListFilter{Kind: "backlog_item", Limit: 10}
	data, err := storage.ExportObjects(ctx, st, secCtx, storageCtx, filter, storage.ExportFormatYAML)
	if err != nil {
		t.Fatalf("ExportObjects: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty YAML (e.g. [] or ---)")
	}
	objects, err := storage.UnmarshalImportForTest(data, storage.ExportFormatYAML)
	if err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if len(objects) != 0 {
		t.Errorf("expected 0 objects from empty export, got %d", len(objects))
	}
}

// TestExportImport_EmptyRoundTrip exports an empty list then imports it (create_only).
// Full round-trip with created objects requires a test env with object specs (e.g. backlog_item).
func TestExportImport_EmptyRoundTrip(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	st := setupFileStorageForExportImport(t)

	filter := storage.ListFilter{Kind: "backlog_item", Limit: 100}
	exported, err := storage.ExportObjects(ctx, st, secCtx, storageCtx, filter, storage.ExportFormatYAML)
	if err != nil {
		t.Fatalf("ExportObjects: %v", err)
	}

	result, err := storage.ImportObjects(ctx, st, secCtx, exported, storage.ExportFormatYAML, storage.ImportOptions{Mode: storage.ImportModeCreateOnly})
	if err != nil {
		t.Fatalf("ImportObjects: %v", err)
	}
	if result.Created != 0 || result.Failed != 0 {
		t.Errorf("ImportObjects: created=%d failed=%d (expected 0, 0)", result.Created, result.Failed)
	}
}

func TestImportObjects_ValidateOnly(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	st := setupFileStorageForExportImport(t)

	data := []byte("[]")
	result, err := storage.ImportObjects(ctx, st, secCtx, data, storage.ExportFormatYAML, storage.ImportOptions{ValidateOnly: true})
	if err != nil {
		t.Fatalf("ImportObjects: %v", err)
	}
	if result.Skipped != 0 {
		t.Errorf("ImportObjects validate_only: got Skipped=%d", result.Skipped)
	}
}

// setupFileStorageForExportImport creates storage rooted under a test-scenarios
// path in a temp dir so tests are isolated from actual project data (same
// pattern as cas_check_output_test and scenario_environment).
func setupFileStorageForExportImport(t *testing.T) *storage.FileObjectStorage {
	t.Helper()
	tmpDir := t.TempDir()
	scenarioRoot := filepath.Join(tmpDir, "test-scenarios", "export-import-test")
	storage.SetupTestRootLikeSetupTestEnvironmentForExportTest(t, scenarioRoot)
	testRoot, err := filepath.Abs(scenarioRoot)
	if err != nil {
		t.Fatalf("abs scenario root: %v", err)
	}
	t.Setenv(zqkenv.TestRoot(), testRoot)
	st, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	defer func() { _ = st.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, st)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
	return st
}
