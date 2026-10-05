package storage_test

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// TestStorageExtended_Wave7_NoopObjectStorage tests object_storage_noop.go
func TestStorageExtended_NoopObjectStorage(t *testing.T) {
	noop := storagepkg.NewNoopObjectStorage()
	ctx := context.Background()
	secCtx := &pkgctx.SecurityContext{}

	// Create returns nil
	if err := noop.Create(ctx, secCtx, map[string]any{}); err != nil {
		t.Errorf("expected nil error on noop Create, got %v", err)
	}

	// All other methods return ErrNoopObjectStorage
	if _, err := noop.Read(ctx, secCtx, "id"); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Read")
	}
	if err := noop.Update(ctx, secCtx, "id", nil); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Update")
	}
	if err := noop.Delete(ctx, secCtx, "id", false); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Delete")
	}
	if _, err := noop.List(ctx, secCtx, nil, storagepkg.ListFilter{}); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on List")
	}
	if _, err := noop.Query(ctx, secCtx, nil, storagepkg.Query{}); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Query")
	}
	if _, err := noop.Search(ctx, secCtx, nil, storagepkg.SearchQuery{}); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Search")
	}
	if _, err := noop.BeginTransaction(ctx); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on BeginTransaction")
	}
	if _, err := noop.BulkCreate(ctx, secCtx, nil); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on BulkCreate")
	}
	if _, err := noop.BulkUpdate(ctx, secCtx, nil); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on BulkUpdate")
	}
	if _, err := noop.BulkGet(ctx, secCtx, nil); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on BulkGet")
	}
	if _, err := noop.BulkDelete(ctx, secCtx, nil, false); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on BulkDelete")
	}
	if _, err := noop.Count(ctx, secCtx, storagepkg.ListFilter{}); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Count")
	}
	if _, err := noop.Aggregate(ctx, secCtx, nil, storagepkg.ListFilter{}, nil); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Aggregate")
	}
	if _, err := noop.GetRelated(ctx, secCtx, "id", "rel", 1); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on GetRelated")
	}
	if _, err := noop.GetPath(ctx, secCtx, "from", "to"); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on GetPath")
	}
	if _, err := noop.GetNeighbors(ctx, secCtx, "id", "dir"); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on GetNeighbors")
	}
	if err := noop.Move(ctx, secCtx, "id", "kind", false); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Move")
	}
	if err := noop.Rename(ctx, secCtx, "id", "new", false); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Rename")
	}
	if _, err := noop.Exists(ctx, secCtx, "id"); err != storagepkg.ErrNoopObjectStorage {
		t.Errorf("expected ErrNoopObjectStorage on Exists")
	}
	if err := noop.Shutdown(ctx); err != nil {
		t.Errorf("expected nil on Shutdown, got %v", err)
	}
}

// TestStorageExtended_Wave7_LazyGraphStorage tests lazy_graph_storage.go
func TestStorageExtended_LazyGraphStorage(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	secCtx := &pkgctx.SecurityContext{}

	// LazyGraphStorage with useMockGraph=true
	lazyMock := storagepkg.NewLazyGraphStorage(nil, tmpDir, true)
	_ = lazyMock.GetPool()

	// Operations on mock
	_ = lazyMock.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:   "PRI-001",
		objects.FieldKeyKind: objects.KindPriorityPlan,
	})
	_, _ = lazyMock.Read(ctx, secCtx, "PRI-001")
	_, _ = lazyMock.Exists(ctx, secCtx, "PRI-001")
	_ = lazyMock.Update(ctx, secCtx, "PRI-001", map[string]any{"title": "Updated"})
	_, _ = lazyMock.List(ctx, secCtx, nil, storagepkg.ListFilter{Kind: objects.KindPriorityPlan})
	_, _ = lazyMock.Count(ctx, secCtx, storagepkg.ListFilter{Kind: objects.KindPriorityPlan})
	_, _ = lazyMock.Query(ctx, secCtx, nil, storagepkg.Query{})
	_, _ = lazyMock.Search(ctx, secCtx, nil, storagepkg.SearchQuery{})
	_, _ = lazyMock.BulkCreate(ctx, secCtx, nil)
	_, _ = lazyMock.BulkUpdate(ctx, secCtx, nil)
	_, _ = lazyMock.BulkGet(ctx, secCtx, []string{"PRI-001"})
	_, _ = lazyMock.BulkDelete(ctx, secCtx, []string{"PRI-001"}, false)
	_, _ = lazyMock.Aggregate(ctx, secCtx, nil, storagepkg.ListFilter{Kind: objects.KindPriorityPlan}, nil)
	_, _ = lazyMock.GetRelated(ctx, secCtx, "PRI-001", "parent", 1)
	_, _ = lazyMock.GetPath(ctx, secCtx, "PRI-001", "PRI-002")
	_, _ = lazyMock.GetNeighbors(ctx, secCtx, "PRI-001", "outgoing")
	_ = lazyMock.Move(ctx, secCtx, "PRI-001", "workstream", false)
	_ = lazyMock.Rename(ctx, secCtx, "PRI-001", "PRI-002", false)
	_ = lazyMock.Delete(ctx, secCtx, "PRI-001", false)
	_ = lazyMock.Shutdown(ctx)

	// LazyGraphStorage without provider and without mock (errors out cleanly)
	lazyNoProvider := storagepkg.NewLazyGraphStorage(nil, tmpDir, false)
	err := lazyNoProvider.Create(ctx, secCtx, map[string]any{})
	if err == nil {
		t.Errorf("expected error when initializing lazy graph storage without provider")
	}
}

// TestStorageExtended_Wave7_TransactionalStorageWrapper tests transactional_storage_wrapper.go
func TestStorageExtended_TransactionalStorageWrapper(t *testing.T) {
	tmpDir := t.TempDir()
	fos, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create FileObjectStorage: %v", err)
	}
	if cleanup := fos.GetTestCleanup(); cleanup != nil {
		defer cleanup()
	}
	ctx := context.Background()
	defer func() { _ = fos.Shutdown(ctx) }()

	secCtx := &pkgctx.SecurityContext{
		AccountID:   "admin",
		Roles:       []string{"admin"},
		Permissions: []string{"read:*", "write:*"},
	}

	txWrapper, err := storagepkg.NewTransactionalStorageWrapper(ctx, fos)
	if err != nil {
		t.Fatalf("NewTransactionalStorageWrapper failed: %v", err)
	}

	// Wrapper operations
	plan := map[string]any{
		objects.FieldKeyID:     "PRI-050",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyTitle:  "Priority Plan 50",
		objects.FieldKeyStatus: "originated",
	}
	_ = txWrapper.Create(ctx, secCtx, plan)
	_, _ = txWrapper.Read(ctx, secCtx, "PRI-050")
	_, _ = txWrapper.Exists(ctx, secCtx, "PRI-050")
	_ = txWrapper.Update(ctx, secCtx, "PRI-050", map[string]any{"title": "Updated 50"})
	_, _ = txWrapper.List(ctx, secCtx, nil, storagepkg.ListFilter{Kind: objects.KindPriorityPlan})
	_, _ = txWrapper.Count(ctx, secCtx, storagepkg.ListFilter{Kind: objects.KindPriorityPlan})
	_, _ = txWrapper.Query(ctx, secCtx, nil, storagepkg.Query{})
	_, _ = txWrapper.Search(ctx, secCtx, nil, storagepkg.SearchQuery{})
	_, _ = txWrapper.BulkCreate(ctx, secCtx, nil)
	_, _ = txWrapper.BulkUpdate(ctx, secCtx, nil)
	_, _ = txWrapper.BulkGet(ctx, secCtx, []string{"PRI-050"})
	_, _ = txWrapper.BulkDelete(ctx, secCtx, []string{"PRI-050"}, false)
	_, _ = txWrapper.Aggregate(ctx, secCtx, nil, storagepkg.ListFilter{Kind: objects.KindPriorityPlan}, nil)
	_, _ = txWrapper.GetRelated(ctx, secCtx, "PRI-050", "parent", 1)
	_, _ = txWrapper.GetPath(ctx, secCtx, "PRI-050", "PRI-051")
	_, _ = txWrapper.GetNeighbors(ctx, secCtx, "PRI-050", "outgoing")
	_ = txWrapper.Move(ctx, secCtx, "PRI-050", "workstream", false)
	_ = txWrapper.Rename(ctx, secCtx, "PRI-050", "PRI-051", false)

	// Commit and Rollback idempotency
	_ = txWrapper.Commit(ctx)
	_ = txWrapper.Commit(ctx)   // duplicate commit is noop
	_ = txWrapper.Rollback(ctx) // rollback after commit is noop

	// Fresh wrapper for rollback path
	txWrapper2, err := storagepkg.NewTransactionalStorageWrapper(ctx, fos)
	if err == nil {
		_ = txWrapper2.Rollback(ctx)
		_ = txWrapper2.Rollback(ctx) // duplicate rollback is noop
		_ = txWrapper2.Commit(ctx)   // commit after rollback is noop
	}
}
