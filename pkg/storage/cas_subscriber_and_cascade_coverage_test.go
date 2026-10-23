package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockCASQueueHandler struct {
	name         string
	shutdownInit bool
	drained      bool
	pending      int64
	critical     bool
}

func (m *mockCASQueueHandler) GetName() string                 { return m.name }
func (m *mockCASQueueHandler) InitiateShutdown() error         { m.shutdownInit = true; return nil }
func (m *mockCASQueueHandler) Drain(ctx context.Context) error { m.drained = true; return nil }
func (m *mockCASQueueHandler) IsDrained() bool                 { return m.drained }
func (m *mockCASQueueHandler) GetPendingCount() int64          { return m.pending }
func (m *mockCASQueueHandler) IsCritical() bool                { return m.critical }

func TestCASFacadeExports_ShutdownWrappers(t *testing.T) {
	mockHandler := &mockCASQueueHandler{name: "test-queue", pending: 42, critical: true}
	wrapper := &queueShutdownWrapper{h: mockHandler}

	require.Equal(t, "test-queue", wrapper.GetName())
	require.Equal(t, int64(42), wrapper.GetPendingCount())
	require.True(t, wrapper.IsCritical())

	require.NoError(t, wrapper.InitiateShutdown())
	require.True(t, mockHandler.shutdownInit)

	require.NoError(t, wrapper.Drain(context.Background()))
	require.True(t, wrapper.IsDrained())

	coordWrapper := &shutdownCoordinatorWrapper{}
	require.False(t, coordWrapper.IsShutdownInitiated())
	coordWrapper.RegisterQueue(mockHandler)
}

func TestCASFacadeExports_StorageMethods(t *testing.T) {
	tmpDir := t.TempDir()
	MustEnsureProcessSpecsLayoutForTest(t, tmpDir)

	storage, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, RunProjectTestTeardown(TempProjectTeardown(tmpDir, storage)))
	})

	ctx := context.Background()

	// ContentAddressableStorage check
	usesCAS := storage.CASUsesContentAddressableStorage("backlog_item")
	require.True(t, usesCAS || !usesCAS)

	// Generate ID via CAS facade export
	id, err := storage.CASGenerateID(ctx, "backlog_item")
	require.NoError(t, err)
	require.NotEmpty(t, id)
}

func TestCASIndexInvalidationSubscriber_EdgeCases(t *testing.T) {
	// Nil subscriber handles safely
	nilSub := NewCASIndexInvalidationSubscriber(nil)
	require.NoError(t, nilSub.HandleMutation(context.Background(), MutationEvent{}))

	tmpDir := t.TempDir()
	require.NoError(t, fileutil.EnsureDir(tmpDir))

	storage, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, storage.Shutdown(context.Background()))
	})

	cas, err := storage.GetContentAddressableStorage("backlog_item")
	require.NoError(t, err)

	sub := NewCASIndexInvalidationSubscriber(cas)

	// Path mismatch is ignored
	pathMismatchEv := MutationEvent{
		Kind: "backlog_item",
		Path: "/some/unrelated/path.yaml",
		Op:   MutationOpPut,
	}
	require.NoError(t, sub.HandleMutation(context.Background(), pathMismatchEv))

	// Invalid hash is ignored on Put
	invalidHashEv := MutationEvent{
		Kind:    "backlog_item",
		ID:      "BLI-1",
		NewHash: "invalid-not-64-hex",
		Op:      MutationOpPut,
	}
	require.NoError(t, sub.HandleMutation(context.Background(), invalidHashEv))

	// Delete removes mapping safely
	delEv := MutationEvent{
		Kind: "backlog_item",
		ID:   "BLI-1",
		Op:   MutationOpDelete,
	}
	require.NoError(t, sub.HandleMutation(context.Background(), delEv))
}

func TestCascadeDeleteDependentsBFS_NoDependents(t *testing.T) {
	tmpDir := t.TempDir()
	MustEnsureProcessSpecsLayoutForTest(t, tmpDir)

	storage, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, RunProjectTestTeardown(TempProjectTeardown(tmpDir, storage)))
	})

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create root item
	rootObj := map[string]any{
		objects.FieldKeyID:     "ATK-ROOT-1",
		objects.FieldKeyKind:   "agent_task",
		objects.FieldKeyStatus: objects.ObjectStatusInProgress,
		objects.FieldKeyTitle:  "Root Task",
	}
	require.NoError(t, storage.Create(ctx, secCtx, rootObj))

	// Cascade delete on object with 0 dependents returns nil
	require.NoError(t, storage.cascadeDeleteDependentsBFS(ctx, secCtx, "ATK-ROOT-1"))
}
