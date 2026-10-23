package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

type mockGraphTransactionHandler struct {
	provider.GraphTransaction
	commitCalled   bool
	rollbackCalled bool
}

func (m *mockGraphTransactionHandler) Commit(ctx context.Context) error {
	m.commitCalled = true
	return nil
}

func (m *mockGraphTransactionHandler) Rollback(ctx context.Context) error {
	m.rollbackCalled = true
	return nil
}

type mockStorageCRUD struct {
	ObjectStorageProvider
	createdObj map[string]any
	updatedID  string
	readID     string
}

func (m *mockStorageCRUD) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.createdObj = obj
	return nil
}

func (m *mockStorageCRUD) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.readID = id
	return map[string]any{objects.FieldKeyID: id, objects.FieldKeyKind: "goal"}, nil
}

func (m *mockStorageCRUD) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.updatedID = id
	return nil
}

func TestGraphObjectTransaction_CompleteLifecycle(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	graphTx := &mockGraphTransactionHandler{}
	storage := &mockStorageCRUD{}

	tx := &GraphObjectTransaction{
		storage: storage,
		graphTx: graphTx,
		ctx:     ctx,
	}

	testObj := map[string]any{objects.FieldKeyID: "G-1", objects.FieldKeyKind: "goal"}
	require.NoError(t, tx.Create(ctx, secCtx, testObj))
	require.Equal(t, "G-1", storage.createdObj[objects.FieldKeyID])

	readObj, err := tx.Read(ctx, secCtx, "G-1")
	require.NoError(t, err)
	require.Equal(t, "G-1", readObj[objects.FieldKeyID])

	require.NoError(t, tx.Update(ctx, secCtx, "G-1", map[string]any{"title": "Updated"}))
	require.Equal(t, "G-1", storage.updatedID)

	require.NoError(t, tx.Commit(ctx))
	require.True(t, graphTx.commitCalled)

	require.NoError(t, tx.Rollback(ctx))
	require.True(t, graphTx.rollbackCalled)
}

func TestFileSystemTransaction_LifecycleAndStateGuards(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	adapter := &FileSystemAdapter{
		coordinator: NewFileTransactionCoordinator(t.TempDir(), newMockLockStrategy()),
	}

	tx := adapter.BeginTransaction(ctx)
	require.NotNil(t, tx)

	// AddFile deduplication
	tx.AddFile("/tmp/file1.yaml")
	tx.AddFile("/tmp/file1.yaml")
	require.Len(t, tx.filePaths, 1)

	// Create validation errors
	require.Error(t, tx.Create(ctx, secCtx, map[string]any{}))
	require.Error(t, tx.Create(ctx, secCtx, map[string]any{objects.FieldKeyKind: "goal"}))

	// Rollback active transaction
	require.NoError(t, tx.Rollback(ctx))
	require.True(t, tx.rolledBack)
	require.Empty(t, tx.operations)

	// Subsequent operations on rolled back transaction must fail
	require.Error(t, tx.Create(ctx, secCtx, map[string]any{objects.FieldKeyID: "G-1", objects.FieldKeyKind: "goal"}))
	require.Error(t, tx.Update(ctx, secCtx, "G-1", map[string]any{}))
	require.Error(t, tx.Delete(ctx, secCtx, "G-1"))
	require.Error(t, tx.Commit(ctx, secCtx))

	// Committed transaction guards
	tx2 := adapter.BeginTransaction(ctx)
	require.NoError(t, tx2.Commit(ctx, secCtx))
	require.True(t, tx2.committed)
	require.Error(t, tx2.Commit(ctx, secCtx))
	require.Error(t, tx2.Rollback(ctx))
}
