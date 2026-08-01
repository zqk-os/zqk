package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestEnsureBundledObjectSpecsMigrated_CreatesObjectSpec(t *testing.T) {
	root := t.TempDir()
	specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(specsDir, "migration_test_kind.yaml")
	content := `
ontology: migration_test_kind
description: Migration test spec one line.
schema_version: "` + objects.DefaultSchemaVersion + `"
visibility: internal
`
	if err := os.WriteFile(specPath, []byte(content), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	store, err := NewFileObjectStorageForTest(root)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer func() { _ = store.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(root, store)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	stats, err := EnsureBundledObjectSpecsMigrated(ctx, root, logger)
	if err != nil {
		t.Fatalf("EnsureBundledObjectSpecsMigrated: %v", err)
	}
	if stats.Created != 1 || stats.Updated != 0 || stats.Errors != 0 {
		t.Fatalf("stats: %+v", stats)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	obj, err := store.Read(ctx, secCtx, "OBJ-migration_test_kind")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if obj[objects.FieldKeyKind] != "object_spec" {
		t.Fatalf("kind: %v", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyOntology] != "migration_test_kind" {
		t.Fatalf("ontology: %v", obj[objects.FieldKeyOntology])
	}

	stats2, err := EnsureBundledObjectSpecsMigrated(ctx, root, logger)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if stats2.Created != 0 || stats2.Updated != 0 || stats2.Skipped != 1 {
		t.Fatalf("expected skip on second run, got %+v", stats2)
	}
}
