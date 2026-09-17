package storage

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCASIndexInvalidationSubscriber_Direct(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-cas-sub-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Shutdown(context.Background()) }()

	cas, err := storage.GetContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("failed to get CAS instance: %v", err)
	}

	subscriber := NewCASIndexInvalidationSubscriber(cas)

	// 1. Put mutation event updates in-memory CAS index
	putEvent := MutationEvent{
		Kind:    "backlog_item",
		ID:      "BLI-CAS-SUB-001",
		Op:      MutationOpPut,
		NewHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	if err := subscriber.HandleMutation(context.Background(), putEvent); err != nil {
		t.Fatalf("HandleMutation Put failed: %v", err)
	}

	hash, err := cas.GetHashForID("BLI-CAS-SUB-001")
	if err != nil {
		t.Fatalf("GetHashForID failed after Put: %v", err)
	}
	if hash != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expected hash %q, got %q", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", hash)
	}

	// 2. Put mutation for a different kind is ignored
	diffKindEvent := MutationEvent{
		Kind:    "criteria",
		ID:      "CRIT-DIFF-001",
		Op:      MutationOpPut,
		NewHash: "hash-crit-999",
	}
	if err := subscriber.HandleMutation(context.Background(), diffKindEvent); err != nil {
		t.Fatalf("HandleMutation diff kind failed: %v", err)
	}
	if _, err := cas.GetHashForID("CRIT-DIFF-001"); err == nil {
		t.Fatalf("expected error for unindexed different kind ID, got nil")
	}

	// 3. Delete mutation event removes in-memory CAS index mapping
	delEvent := MutationEvent{
		Kind: "backlog_item",
		ID:   "BLI-CAS-SUB-001",
		Op:   MutationOpDelete,
	}
	if err := subscriber.HandleMutation(context.Background(), delEvent); err != nil {
		t.Fatalf("HandleMutation Delete failed: %v", err)
	}

	if _, err := cas.GetHashForID("BLI-CAS-SUB-001"); err == nil {
		t.Fatalf("expected error after Delete, but hash still resolved")
	}
}

func TestCASIndexInvalidationSubscriber_StorageIntegration(t *testing.T) {
	tmpDir, err := fileutil.MkdirTemp("", "zqk-cas-storage-sub-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer fileutil.RemoveAll(tmpDir)

	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		_ = RunProjectTestTeardown(opts)
	})

	cas, err := storage.GetContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("failed to get CAS: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := WithTestHardDelete(context.Background())

	objID := "BLI-MUTATION-INDEX-SYNC"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "CAS Index Sync Test Item",
		objects.FieldKeyDescription:   "Testing CAS index updates on mutation shockwave",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Create CAS-visible object in planned status
	CreateCASVisible(t, storage, ctx, secCtx, obj, objects.ObjectStatusPlanned)

	// In-memory index map must immediately reflect the object ID
	hash1, err := cas.GetHashForID(objID)
	if err != nil {
		t.Fatalf("GetHashForID failed after Create: %v", err)
	}
	if hash1 == "" {
		t.Fatalf("expected non-empty hash after Create")
	}

	// Update object
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Title for Index Sync",
	}
	if err := storage.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("storage.Update failed: %v", err)
	}

	hash2, err := cas.GetHashForID(objID)
	if err != nil {
		t.Fatalf("GetHashForID failed after Update: %v", err)
	}
	if hash2 == "" || hash2 == hash1 {
		t.Fatalf("expected new hash after Update, got %q (old %q)", hash2, hash1)
	}

	// Delete object
	if err := storage.Delete(ctx, secCtx, objID, false); err != nil {
		t.Fatalf("storage.Delete failed: %v", err)
	}

	if _, err := cas.GetHashForID(objID); err == nil {
		t.Fatalf("expected GetHashForID to fail after Delete")
	}
}
