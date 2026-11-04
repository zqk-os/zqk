package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type cycle30MockTx struct {
	commitCalls   int
	rollbackCalls int
	commitErr     error
	rollbackErr   error
	created       []map[string]any
	readResult    map[string]any
	readErr       error
	updated       []map[string]any
	deleted       []string
}

func (m *cycle30MockTx) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.created = append(m.created, obj)
	return nil
}

func (m *cycle30MockTx) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	return m.readResult, nil
}

func (m *cycle30MockTx) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.updated = append(m.updated, updates)
	return nil
}

func (m *cycle30MockTx) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	m.deleted = append(m.deleted, id)
	return nil
}

func (m *cycle30MockTx) Commit(ctx context.Context) error {
	m.commitCalls++
	return m.commitErr
}

func (m *cycle30MockTx) Rollback(ctx context.Context) error {
	m.rollbackCalls++
	return m.rollbackErr
}

type cycle30MockUnderlying struct {
	NoopObjectStorage
	listCalled       bool
	queryCalled      bool
	countCalled      bool
	existsCalled     bool
	aggregateCalled  bool
	relatedCalled    bool
	pathCalled       bool
	neighborsCalled  bool
	searchCalled     bool
	bulkCreateCalled bool
	bulkUpdateCalled bool
	bulkGetCalled    bool
	bulkDeleteCalled bool
	moveCalled       bool
	renameCalled     bool
	shutdownCalled   bool
}

func (m *cycle30MockUnderlying) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	m.listCalled = true
	return &QueryResult{Objects: []map[string]any{{"id": "A"}}}, nil
}

func (m *cycle30MockUnderlying) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	m.queryCalled = true
	return &QueryResult{Objects: []map[string]any{{"id": "A"}}}, nil
}

func (m *cycle30MockUnderlying) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	m.countCalled = true
	return 1, nil
}

func (m *cycle30MockUnderlying) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	m.existsCalled = true
	return true, nil
}

func (m *cycle30MockUnderlying) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	m.aggregateCalled = true
	return &AggregateResult{}, nil
}

func (m *cycle30MockUnderlying) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, relationshipType string, depth int) ([]map[string]any, error) {
	m.relatedCalled = true
	return []map[string]any{{"id": id}}, nil
}

func (m *cycle30MockUnderlying) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	m.pathCalled = true
	return []map[string]any{{"from": fromID, "to": toID}}, nil
}

func (m *cycle30MockUnderlying) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, direction string) ([]map[string]any, error) {
	m.neighborsCalled = true
	return []map[string]any{{"id": id}}, nil
}

func (m *cycle30MockUnderlying) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	m.searchCalled = true
	return &SearchResult{TotalCount: 1}, nil
}

func (m *cycle30MockUnderlying) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	m.bulkCreateCalled = true
	return &BulkResult{TotalCount: len(objects), SuccessCount: len(objects)}, nil
}

func (m *cycle30MockUnderlying) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	m.bulkUpdateCalled = true
	return &BulkResult{TotalCount: len(updates), SuccessCount: len(updates)}, nil
}

func (m *cycle30MockUnderlying) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	m.bulkGetCalled = true
	return &BulkResult{TotalCount: len(ids), SuccessCount: len(ids)}, nil
}

func (m *cycle30MockUnderlying) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	m.bulkDeleteCalled = true
	return &BulkResult{TotalCount: len(ids), SuccessCount: len(ids)}, nil
}

func (m *cycle30MockUnderlying) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, newKind string, updateReferences bool) error {
	m.moveCalled = true
	return nil
}

func (m *cycle30MockUnderlying) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	m.renameCalled = true
	return nil
}

func (m *cycle30MockUnderlying) Shutdown(ctx context.Context) error {
	m.shutdownCalled = true
	return nil
}

type cycle30CustomDecorator struct {
	NoopObjectStorage
	inner ObjectStorageProvider
}

func (d *cycle30CustomDecorator) UnderlyingObjectStorageProvider() ObjectStorageProvider {
	return d.inner
}

func TestCycle30_StaticFloor_TransactionalWrapper_ConstructorAndNilSafety(t *testing.T) {
	require.Nil(t, UnwrapToFileObjectStorage((*TransactionalStorageWrapper)(nil)))
	require.Nil(t, UnwrapToFileFirstProjection((*TransactionalStorageWrapper)(nil)))

	uninit := &TransactionalStorageWrapper{}
	require.Nil(t, UnwrapToFileObjectStorage(uninit))
	require.Nil(t, UnwrapToFileFirstProjection(uninit))
	require.Nil(t, uninit.GetUnderlying())

	ctx := context.Background()
	wrapper, err := NewTransactionalStorageWrapper(ctx, NoopObjectStorage{})
	require.Nil(t, wrapper)
	require.ErrorIs(t, err, ErrNoopObjectStorage)
}

func TestCycle30_StaticFloor_TransactionalWrapper_CommitLifecycle(t *testing.T) {
	ctx := context.Background()
	mockTx := &cycle30MockTx{}
	wrapper := &TransactionalStorageWrapper{
		tx:  mockTx,
		ctx: ctx,
	}

	require.NoError(t, wrapper.Commit(ctx))
	require.True(t, wrapper.committed)
	require.Equal(t, 1, mockTx.commitCalls)

	require.NoError(t, wrapper.Commit(ctx))
	require.Equal(t, 1, mockTx.commitCalls)

	require.NoError(t, wrapper.Rollback(ctx))
	require.Equal(t, 0, mockTx.rollbackCalls)

	expectedErr := errors.New("underlying commit error")
	failingTx := &cycle30MockTx{commitErr: expectedErr}
	failingWrapper := &TransactionalStorageWrapper{tx: failingTx, ctx: ctx}
	err := failingWrapper.Commit(ctx)
	require.ErrorIs(t, err, expectedErr)
	require.False(t, failingWrapper.committed)
}

func TestCycle30_StaticFloor_TransactionalWrapper_RollbackLifecycle(t *testing.T) {
	ctx := context.Background()
	mockTx := &cycle30MockTx{}
	wrapper := &TransactionalStorageWrapper{
		tx:  mockTx,
		ctx: ctx,
	}

	require.NoError(t, wrapper.Rollback(ctx))
	require.True(t, wrapper.rolledBack)
	require.Equal(t, 1, mockTx.rollbackCalls)

	require.NoError(t, wrapper.Rollback(ctx))
	require.Equal(t, 1, mockTx.rollbackCalls)

	require.NoError(t, wrapper.Commit(ctx))
	require.Equal(t, 0, mockTx.commitCalls)

	expectedErr := errors.New("underlying rollback error")
	failingTx := &cycle30MockTx{rollbackErr: expectedErr}
	failingWrapper := &TransactionalStorageWrapper{tx: failingTx, ctx: ctx}
	err := failingWrapper.Rollback(ctx)
	require.ErrorIs(t, err, expectedErr)
	require.False(t, failingWrapper.rolledBack)
}

func TestCycle30_StaticFloor_TransactionalWrapper_TxOpsDelegation(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	mockTx := &cycle30MockTx{
		readResult: map[string]any{"id": "GOAL-001", "kind": "goal"},
	}
	wrapper := &TransactionalStorageWrapper{
		tx:  mockTx,
		ctx: ctx,
	}

	obj := map[string]any{"id": "GOAL-001", "kind": "goal"}
	require.NoError(t, wrapper.Create(ctx, secCtx, obj))
	require.Len(t, mockTx.created, 1)

	readObj, err := wrapper.Read(ctx, secCtx, "GOAL-001")
	require.NoError(t, err)
	require.Equal(t, "GOAL-001", readObj["id"])

	updates := map[string]any{"title": "Updated Goal"}
	require.NoError(t, wrapper.Update(ctx, secCtx, "GOAL-001", updates))
	require.Len(t, mockTx.updated, 1)

	require.NoError(t, wrapper.Delete(ctx, secCtx, "GOAL-001", false))
	require.Equal(t, []string{"GOAL-001"}, mockTx.deleted)

	tx, err := wrapper.BeginTransaction(ctx)
	require.NoError(t, err)
	require.Equal(t, mockTx, tx)
}

func TestCycle30_StaticFloor_TransactionalWrapper_UnderlyingDelegation(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	mockTx := &cycle30MockTx{}
	underlying := &cycle30MockUnderlying{}
	wrapper := &TransactionalStorageWrapper{
		underlying: underlying,
		tx:         mockTx,
		ctx:        ctx,
	}

	require.Equal(t, underlying, wrapper.GetUnderlying())

	qr, err := wrapper.List(ctx, secCtx, nil, ListFilter{})
	require.NoError(t, err)
	require.NotNil(t, qr)
	require.True(t, underlying.listCalled)

	qr, err = wrapper.Query(ctx, secCtx, nil, Query{})
	require.NoError(t, err)
	require.NotNil(t, qr)
	require.True(t, underlying.queryCalled)

	cnt, err := wrapper.Count(ctx, secCtx, ListFilter{})
	require.NoError(t, err)
	require.Equal(t, 1, cnt)
	require.True(t, underlying.countCalled)

	exists, err := wrapper.Exists(ctx, secCtx, "GOAL-001")
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, underlying.existsCalled)

	agg, err := wrapper.Aggregate(ctx, secCtx, nil, ListFilter{}, nil)
	require.NoError(t, err)
	require.NotNil(t, agg)
	require.True(t, underlying.aggregateCalled)

	rel, err := wrapper.GetRelated(ctx, secCtx, "GOAL-001", "parent", 1)
	require.NoError(t, err)
	require.Len(t, rel, 1)
	require.True(t, underlying.relatedCalled)

	path, err := wrapper.GetPath(ctx, secCtx, "A", "B")
	require.NoError(t, err)
	require.Len(t, path, 1)
	require.True(t, underlying.pathCalled)

	neighbors, err := wrapper.GetNeighbors(ctx, secCtx, "A", "outgoing")
	require.NoError(t, err)
	require.Len(t, neighbors, 1)
	require.True(t, underlying.neighborsCalled)

	sr, err := wrapper.Search(ctx, secCtx, nil, SearchQuery{})
	require.NoError(t, err)
	require.NotNil(t, sr)
	require.True(t, underlying.searchCalled)

	br, err := wrapper.BulkCreate(ctx, secCtx, []map[string]any{{"id": "A"}})
	require.NoError(t, err)
	require.Equal(t, 1, br.TotalCount)
	require.True(t, underlying.bulkCreateCalled)

	br, err = wrapper.BulkUpdate(ctx, secCtx, []BulkUpdateItem{{ID: "A"}})
	require.NoError(t, err)
	require.Equal(t, 1, br.TotalCount)
	require.True(t, underlying.bulkUpdateCalled)

	br, err = wrapper.BulkGet(ctx, secCtx, []string{"A"})
	require.NoError(t, err)
	require.Equal(t, 1, br.TotalCount)
	require.True(t, underlying.bulkGetCalled)

	br, err = wrapper.BulkDelete(ctx, secCtx, []string{"A"}, false)
	require.NoError(t, err)
	require.Equal(t, 1, br.TotalCount)
	require.True(t, underlying.bulkDeleteCalled)

	require.NoError(t, wrapper.Move(ctx, secCtx, "A", "kindB", false))
	require.True(t, underlying.moveCalled)

	require.NoError(t, wrapper.Rename(ctx, secCtx, "A", "A2", false))
	require.True(t, underlying.renameCalled)

	require.NoError(t, wrapper.Shutdown(ctx))
	require.True(t, underlying.shutdownCalled)
}

func TestCycle30_StaticFloor_TransactionalWrapper_EndToEnd_RealStorage(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	fs, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	ctx := context.Background()
	t.Cleanup(func() {
		require.NoError(t, fs.Shutdown(ctx))
		opts := TempProjectTeardown(tmpDir, fs)
		if teardownErr := RunProjectTestTeardown(opts); teardownErr != nil {
			t.Logf("project teardown: %v", teardownErr)
		}
	})

	secCtx := pkgctx.NewSystemSecurityContext()
	txWrapper, err := NewTransactionalStorageWrapper(ctx, fs)
	require.NoError(t, err)

	objA := map[string]any{
		objects.FieldKeyID:     "PRI-CYCLE30-001",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyTitle:  "Cycle 30 Plan A",
		objects.FieldKeyStatus: "originated",
	}
	require.NoError(t, txWrapper.Create(ctx, secCtx, objA))
	require.NoError(t, txWrapper.Commit(ctx))

	persisted, err := fs.Read(ctx, secCtx, "PRI-CYCLE30-001")
	require.NoError(t, err)
	require.Equal(t, "PRI-CYCLE30-001", persisted[objects.FieldKeyID])

	txWrapperRollback, err := NewTransactionalStorageWrapper(ctx, fs)
	require.NoError(t, err)
	objB := map[string]any{
		objects.FieldKeyID:     "PRI-CYCLE30-002",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyTitle:  "Cycle 30 Plan B",
		objects.FieldKeyStatus: "originated",
	}
	require.NoError(t, txWrapperRollback.Create(ctx, secCtx, objB))
	require.NoError(t, txWrapperRollback.Rollback(ctx))

	_, err = fs.Read(ctx, secCtx, "PRI-CYCLE30-002")
	require.Error(t, err)
}

func TestCycle30_StaticFloor_StorageFactory_LifecycleAndStats(t *testing.T) {
	var nilFactory *StorageFactory
	c, pLookups, dLookups := nilFactory.GetStorageFactoryStats()
	assert.GreaterOrEqual(t, c, int64(0))
	assert.Equal(t, int64(0), pLookups)
	assert.Equal(t, int64(0), dLookups)

	tmpDir := t.TempDir()
	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)
	ctx := context.Background()

	factory, err := NewStorageFactory(ctx, tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, factory.Shutdown(ctx))
	})

	assert.Equal(t, tmpDir, factory.GetProjectRoot())
	assert.NotNil(t, factory.GetStorage())
	assert.Nil(t, factory.GetPool())

	prevProv := factory.providerLookupsTotal.Load()
	prevDef := factory.defaultLookupsTotal.Load()

	sKind := factory.GetStorageForKind(objects.KindGoal)
	assert.NotNil(t, sKind)
	assert.Equal(t, prevProv+1, factory.providerLookupsTotal.Load())

	sUnknown := factory.GetStorageForKind("unregistered_cycle30_kind")
	assert.NotNil(t, sUnknown)
	assert.Equal(t, prevDef+1, factory.defaultLookupsTotal.Load())

	created, provTotal, defTotal := factory.GetStorageFactoryStats()
	assert.Greater(t, created, int64(0))
	assert.GreaterOrEqual(t, provTotal, int64(1))
	assert.GreaterOrEqual(t, defTotal, int64(1))
}

func TestCycle30_StaticFloor_StorageFactory_KindRoutingAndMCPContext(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)
	ctx := context.Background()

	factory, err := NewStorageFactory(ctx, tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, factory.Shutdown(ctx))
	})

	goalObj := map[string]any{objects.FieldKeyKind: objects.KindGoal}
	assert.NotNil(t, factory.GetStorageForObject(goalObj))

	unknownObj := map[string]any{objects.FieldKeyKind: "unknown_custom_kind"}
	assert.Equal(t, factory.GetStorage(), factory.GetStorageForObject(unknownObj))

	emptyObj := map[string]any{}
	assert.Equal(t, factory.GetStorage(), factory.GetStorageForObject(emptyObj))

	activeStewardObj := map[string]any{
		objects.FieldKeyKind:        objects.KindGoal,
		ConstMiscStewardshipProfile: "active",
	}
	assert.NotNil(t, factory.GetStorageForObject(activeStewardObj))

	assert.True(t, factory.IsGraphBackend(objects.KindGoal))
	assert.False(t, factory.IsGraphBackend("unregistered_nonexistent_kind"))
	assert.True(t, factory.IsFileBackend("unregistered_nonexistent_kind"))

	mcpCtx := pkgctx.WithLoggingContext(ctx, &pkgctx.LoggingContext{Profile: pkgctx.ProfileMCP})
	mcpFactory, err := NewStorageFactory(mcpCtx, tmpDir)
	require.NoError(t, err)
	require.NoError(t, mcpFactory.Shutdown(ctx))
}

func TestCycle30_StaticFloor_StorageFactory_UnwrapVariants(t *testing.T) {
	assert.Nil(t, UnwrapToFileObjectStorage(nil))
	assert.Nil(t, UnwrapToFileFirstProjection(nil))

	tmpDir := t.TempDir()
	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)
	ctx := context.Background()

	fs, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, fs.Shutdown(ctx))
	})

	assert.Equal(t, fs, UnwrapToFileObjectStorage(fs))

	txWrap, err := NewTransactionalStorageWrapper(ctx, fs)
	require.NoError(t, err)
	assert.Equal(t, fs, UnwrapToFileObjectStorage(txWrap))

	meshWrap := NewMeshObjectStorage(fs, nil)
	assert.Equal(t, fs, UnwrapToFileObjectStorage(meshWrap))

	batchWrap := NewBatchingObjectStorage(fs)
	assert.Equal(t, fs, UnwrapToFileObjectStorage(batchWrap))

	customDec := &cycle30CustomDecorator{inner: fs}
	assert.Equal(t, fs, UnwrapToFileObjectStorage(customDec))

	projStorage := NewFileFirstProjectionStorage(fs, nil)
	assert.Equal(t, fs, UnwrapToFileObjectStorage(projStorage))
	assert.Equal(t, projStorage, UnwrapToFileFirstProjection(projStorage))

	txWrapProj := &TransactionalStorageWrapper{underlying: projStorage}
	assert.Equal(t, projStorage, UnwrapToFileFirstProjection(txWrapProj))

	meshWrapProj := NewMeshObjectStorage(projStorage, nil)
	assert.Equal(t, projStorage, UnwrapToFileFirstProjection(meshWrapProj))

	batchWrapProj := NewBatchingObjectStorage(projStorage)
	assert.Equal(t, projStorage, UnwrapToFileFirstProjection(batchWrapProj))

	customDecProj := &cycle30CustomDecorator{inner: projStorage}
	assert.Equal(t, projStorage, UnwrapToFileFirstProjection(customDecProj))
}

func TestCycle30_StaticFloor_ErrorSentinels_InvariantsAndWrapping(t *testing.T) {
	sentinels := []struct {
		err         error
		expectedMsg string
	}{
		{ErrInvalidID, "invalid object ID"},
		{ErrPermissionDeniedStorage, "permission denied"},
		{ErrInvalidSchema, "invalid schema"},
		{ErrTimeout, "storage operation timed out"},
		{ErrOptimisticLockConflict, "optimistic lock conflict: object modified concurrently"},
		{ErrIntegrityViolation, "referential integrity violation"},
		{ErrCASCorrupted, "content-addressable storage checksum verification failed"},
		{ErrInvalidObjectKind, "unregistered or invalid object kind"},
		{ErrStorageClosed, "storage engine is closed"},
		{ErrNamespaceAccessDenied, "namespace isolation access denied"},
		{ErrNoopObjectStorage, "noop object storage: not implemented"},
	}

	for _, item := range sentinels {
		require.NotNil(t, item.err)
		assert.Equal(t, item.expectedMsg, item.err.Error())
		assert.True(t, errors.Is(item.err, item.err))

		wrapped := fmt.Errorf("operation failed: %w", item.err)
		assert.True(t, errors.Is(wrapped, item.err))

		nested := fmt.Errorf("context trace: %w", wrapped)
		assert.True(t, errors.Is(nested, item.err))
	}

	for i := 0; i < len(sentinels); i++ {
		for j := 0; j < len(sentinels); j++ {
			if i != j {
				assert.False(t, errors.Is(sentinels[i].err, sentinels[j].err))
			}
		}
	}
}

func TestCycle30_StaticFloor_Options_EdgeCases(t *testing.T) {
	opts := &FileObjectStorageOptions{
		SkipGlobalWiring: true,
		InitContext:      context.Background(),
	}
	assert.True(t, opts.SkipGlobalWiring)
	assert.NotNil(t, opts.InitContext)

	audit.ApplySessionEnv(nil)

	existingEvt := &audit.EventOptions{SessionID: "sess-fixed-123"}
	audit.ApplySessionEnv(existingEvt)
	assert.Equal(t, "sess-fixed-123", existingEvt.SessionID)

	t.Setenv(zqkenv.SessionID().Name(), "env-session-456")
	blankEvt := &audit.EventOptions{}
	audit.ApplySessionEnv(blankEvt)
	assert.Equal(t, "env-session-456", blankEvt.SessionID)

	var callbackError error
	bufferedTriggered := false
	cbEvt := &audit.EventOptions{
		OnError: func(err error) {
			callbackError = err
		},
		OnBuffered: func() {
			bufferedTriggered = true
		},
	}
	cbEvt.OnError(ErrTimeout)
	cbEvt.OnBuffered()
	assert.ErrorIs(t, callbackError, ErrTimeout)
	assert.True(t, bufferedTriggered)
}
