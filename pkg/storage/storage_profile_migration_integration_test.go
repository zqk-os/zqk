package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

// TestStorageProfileMigrationChain_IDBasedYAMLToCAS exercises [FileObjectStorage.MigrateObjectToCAS]:
// a legacy ID-named YAML under docs/architecture/<kind-dir>/ is ingested into hash-addressed CAS and the
// legacy file can be removed. This is the concrete on-disk step for adopting CAS-backed storage for a
// process object kind (data-cell profile cas_entity).
//
// The light_file → stream hops use different surfaces (runtime organism JSON under .zqk/config;
// stream segments under .zqk/state/streams/). Promotion of legacy layout into stream storage is
// implemented by cmd/zqk/system migrate-legacy-to-stream (walk legacy YAML → storage Create); that
// command is not duplicated here to keep the test bounded and WAL-free (NewFileObjectStorageForTest).
func TestStorageProfileMigrationChain_IDBasedYAMLToCAS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}
	base := t.TempDir()
	testRoot := filepath.Join(base, "test-scenarios", "storage-profile-migration-chain")
	mustEnsureProcessSpecsLayout(t, testRoot)

	const (
		kind     = "backlog_item"
		objectID = "ITEM-migchain-001"
	)
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		t.Fatal("GetDirectoryFromKind(backlog_item)")
	}
	kindDir := datacell.CellCASPrimaryDir(testRoot, dirName)
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir kind dir: %v", err)
	}
	legacyPath := filepath.Join(kindDir, objectID+".yaml")
	yamlBody := `id: ITEM-migchain-001
kind: backlog_item
title: migration chain probe
status: exploring
schema_version: "2.0.0"
created_at: "2030-04-15T12:00:00Z"
updated_at: "2030-04-15T12:00:00Z"
`
	if err := os.WriteFile(legacyPath, []byte(yamlBody), paths.FilePerm644); err != nil {
		t.Fatalf("write legacy yaml: %v", err)
	}

	f, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	defer func() { _ = f.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, f)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{AccountID: "account:system"}

	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		t.Fatalf("getContentAddressableStorage: %v", err)
	}
	if _, err := cas.GetHashForID(objectID); err == nil {
		t.Fatal("expected no CAS index entry before migration")
	}

	// Pass explicit legacy path: after CAS Create, getObjectFilePath may resolve to the hash file,
	// so removal must target the ID-based YAML we wrote (see cas_migration_utility oldFilePath).
	if err := f.MigrateObjectToCAS(ctx, secCtx, objectID, true, legacyPath); err != nil {
		t.Fatalf("MigrateObjectToCAS: %v", err)
	}
	if err := FlushListingIndexForKind(kind); err != nil {
		t.Fatalf("FlushListingIndexForKind: %v", err)
	}

	hash, err := cas.GetHashForID(objectID)
	if err != nil || hash == "" {
		t.Fatalf("GetHashForID after migrate: hash=%q err=%v", hash, err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy ID-based file should be removed; stat err=%v", err)
	}
	obj, err := f.Read(ctx, secCtx, objectID)
	if err != nil {
		t.Fatalf("Read after migration: %v", err)
	}
	if got, _ := obj[objects.FieldKeyID].(string); got != objectID {
		t.Fatalf("Read id: got %q want %q", got, objectID)
	}
}
