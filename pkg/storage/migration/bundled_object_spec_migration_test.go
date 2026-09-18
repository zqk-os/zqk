package migration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestEnsureBundledObjectSpecsMigrated_CreatesObjectSpec(t *testing.T) {
	root := filepath.Join(t.TempDir(), "test-scenarios")
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(specsDir, "migration_test_kind.yaml")
	content := `
ontology: migration_test_kind
description: Migration test spec one line.
schema_version: "` + objects.DefaultSchemaVersion + `"
visibility: internal
`
	if err := fileutil.WriteFile(specPath, []byte(content), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	store, err := storage.NewFileObjectStorageForTest(root)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}

	defer func() { _ = store.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(root, store)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	stats, err := EnsureBundledObjectSpecsMigrated(ctx, root, logger)
	if err != nil {
		t.Fatalf("EnsureBundledObjectSpecsMigrated: %v", err)
	}
	// Hermetic test roots copy the repo spec tree, so more than the fixture
	// YAML is migrated. Require the fixture object and a clean error count.
	if stats.Created < 1 || stats.Errors != 0 {
		t.Fatalf("stats: %+v", stats)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	expectedID := ObjectSpecIDForStem("migration_test_kind")
	obj, err := store.Read(ctx, secCtx, expectedID)
	if err != nil {
		t.Fatalf("read %s: %v", expectedID, err)
	}
	if obj[objects.FieldKeyKind] != "object_spec" {
		t.Fatalf("kind: %v", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyOntology] != "migration_test_kind" {
		t.Fatalf("ontology: %v", obj[objects.FieldKeyOntology])
	}
	if desc, ok := obj[objects.FieldKeyDescription].(string); !ok || len(desc) < 10 {
		t.Fatalf("description: %v", obj[objects.FieldKeyDescription])
	}
	if fp, ok := obj[objects.FieldKeyFilePath].(string); !ok || filepath.IsAbs(fp) {
		t.Fatalf("expected project-relative file_path, got %v", fp)
	}

	stats2, err := EnsureBundledObjectSpecsMigrated(ctx, root, logger)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if stats2.Created != 0 || stats2.Errors != 0 || stats2.Skipped < 1 {
		t.Fatalf("expected skip on second run, got %+v", stats2)
	}
}
