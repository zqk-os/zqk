package storage

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	pkgobjects "github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProjectionProvider struct {
	created map[string]map[string]any
	updated map[string]map[string]any
	deleted map[string]bool
}

func newMockProjectionProvider() *mockProjectionProvider {
	return &mockProjectionProvider{
		created: make(map[string]map[string]any),
		updated: make(map[string]map[string]any),
		deleted: make(map[string]bool),
	}
}

func (m *mockProjectionProvider) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
	id, _ := obj[pkgobjects.FieldKeyID].(string)
	m.created[id] = obj
	return nil
}

func (m *mockProjectionProvider) Read(ctx context.Context, secCtx *SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.created[id]; ok {
		return obj, nil
	}
	return nil, ErrObjectNotFound
}

func (m *mockProjectionProvider) Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error {
	m.updated[id] = updates
	return nil
}

func (m *mockProjectionProvider) Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error {
	m.deleted[id] = true
	delete(m.created, id)
	return nil
}

func (m *mockProjectionProvider) Exists(ctx context.Context, secCtx *SecurityContext, id string) (bool, error) {
	_, ok := m.created[id]
	return ok, nil
}

func (m *mockProjectionProvider) List(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter) (*QueryResult, error) {
	var objects []map[string]any
	for _, obj := range m.created {
		if k, _ := obj[pkgobjects.FieldKeyKind].(string); k == filter.Kind {
			objects = append(objects, obj)
		}
	}
	return &QueryResult{Objects: objects}, nil
}

func (m *mockProjectionProvider) Query(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query Query) (*QueryResult, error) {
	return &QueryResult{}, nil
}

func (m *mockProjectionProvider) Search(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, query SearchQuery) (*SearchResult, error) {
	return &SearchResult{}, nil
}

func (m *mockProjectionProvider) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	return nil, nil
}

func (m *mockProjectionProvider) BulkCreate(ctx context.Context, secCtx *SecurityContext, objects []map[string]any) (*BulkResult, error) {
	for _, obj := range objects {
		_ = m.Create(ctx, secCtx, obj)
	}
	return &BulkResult{SuccessCount: len(objects)}, nil
}

func (m *mockProjectionProvider) BulkUpdate(ctx context.Context, secCtx *SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	return &BulkResult{SuccessCount: len(updates)}, nil
}

func (m *mockProjectionProvider) BulkGet(ctx context.Context, secCtx *SecurityContext, ids []string) (*BulkResult, error) {
	return &BulkResult{}, nil
}

func (m *mockProjectionProvider) BulkDelete(ctx context.Context, secCtx *SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	for _, id := range ids {
		m.deleted[id] = true
		delete(m.created, id)
	}
	return &BulkResult{SuccessCount: len(ids)}, nil
}

func (m *mockProjectionProvider) Count(ctx context.Context, secCtx *SecurityContext, filter ListFilter) (int, error) {
	return len(m.created), nil
}

func (m *mockProjectionProvider) Aggregate(ctx context.Context, secCtx *SecurityContext, storageCtx *StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	return &AggregateResult{}, nil
}

func (m *mockProjectionProvider) GetRelated(ctx context.Context, secCtx *SecurityContext, id string, relationType string, depth int) ([]map[string]any, error) {
	return nil, nil
}

func (m *mockProjectionProvider) GetPath(ctx context.Context, secCtx *SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return nil, nil
}

func (m *mockProjectionProvider) GetNeighbors(ctx context.Context, secCtx *SecurityContext, id string, direction string) ([]map[string]any, error) {
	return nil, nil
}

func (m *mockProjectionProvider) Move(ctx context.Context, secCtx *SecurityContext, id string, newKind string, updateReferences bool) error {
	return nil
}

func (m *mockProjectionProvider) Rename(ctx context.Context, secCtx *SecurityContext, oldID, newID string, updateReferences bool) error {
	if obj, ok := m.created[oldID]; ok {
		delete(m.created, oldID)
		obj[pkgobjects.FieldKeyID] = newID
		m.created[newID] = obj
	}
	return nil
}

func (m *mockProjectionProvider) Shutdown(ctx context.Context) error {
	return nil
}

func TestFileFirstProjectionStorage_BasicLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tempDir)

	fileStorage, err := GetFileObjectStorage(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fileStorage.Shutdown(context.Background()) })

	mockProj := newMockProjectionProvider()
	storage := NewFileFirstProjectionStorage(fileStorage, mockProj)

	ctx := WithTestHardDelete(context.Background())
	secCtx := pkgctx.NewSystemSecurityContext()

	objID := "GOAL-PROJ-TEST-001"
	testObj := map[string]any{
		pkgobjects.FieldKeyID:     objID,
		pkgobjects.FieldKeyKind:   "goal",
		pkgobjects.FieldKeyTitle:  "Test Projection Goal",
		pkgobjects.FieldKeyStatus: pkgobjects.ObjectStatusActive,
	}

	// 1. Create (File FIRST, then Projection)
	err = storage.Create(ctx, secCtx, testObj)
	require.NoError(t, err)

	// Verify file SSOT read
	readObj, err := storage.Read(ctx, secCtx, objID)
	require.NoError(t, err)
	assert.Equal(t, "Test Projection Goal", readObj[pkgobjects.FieldKeyTitle])

	// Verify projection received creation
	assert.Contains(t, mockProj.created, objID)

	// 2. Update
	updates := map[string]any{
		pkgobjects.FieldKeyTitle: "Updated Projection Goal Title",
	}
	err = storage.Update(ctx, secCtx, objID, updates)
	require.NoError(t, err)

	updatedRead, err := storage.Read(ctx, secCtx, objID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Projection Goal Title", updatedRead[pkgobjects.FieldKeyTitle])
	assert.Contains(t, mockProj.updated, objID)

	// 3. Delete
	err = storage.Delete(ctx, secCtx, objID, false)
	require.NoError(t, err)

	_, err = storage.Read(ctx, secCtx, objID)
	assert.Error(t, err)
	assert.True(t, mockProj.deleted[objID])
}

// TestFileFirstProjectionStorage_CASMutateKeepsGraphLockstep documents that pending-journal
// work must not bypass FileFirstProjectionStorage: Create/Update/Delete still project.
// TRACK: BLI-1785895580100186000-c5539372
func TestFileFirstProjectionStorage_CASMutateKeepsGraphLockstep(t *testing.T) {
	tempDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tempDir)
	ResetObjectIDCachePendingForTest()
	t.Cleanup(ResetObjectIDCachePendingForTest)

	fileStorage, err := GetFileObjectStorage(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fileStorage.Shutdown(context.Background()) })

	mockProj := newMockProjectionProvider()
	storage := NewFileFirstProjectionStorage(fileStorage, mockProj)

	ctx := WithCLIOperation(pkgctx.WithPromoteOnCreate(WithTestHardDelete(context.Background())))
	secCtx := pkgctx.NewSystemSecurityContext()
	objID := "GOAL-lockstep-1"
	testObj := map[string]any{
		pkgobjects.FieldKeyID:            objID,
		pkgobjects.FieldKeyKind:          "goal",
		pkgobjects.FieldKeyTitle:         "Lockstep Goal",
		pkgobjects.FieldKeyDescription:   "Substantive description for lockstep goal testing.",
		pkgobjects.FieldKeyStatus:        pkgobjects.ObjectStatusActive,
		pkgobjects.FieldKeySchemaVersion: pkgobjects.DefaultSchemaVersion,
	}
	require.NoError(t, storage.Create(ctx, secCtx, testObj))
	assert.Contains(t, mockProj.created, objID, "graph must stay in lockstep on CAS create")
	require.NoError(t, storage.Update(ctx, secCtx, objID, map[string]any{pkgobjects.FieldKeyTitle: "Lockstep Updated"}))
	assert.Contains(t, mockProj.updated, objID, "graph must stay in lockstep on CAS update")
	require.NoError(t, storage.Delete(ctx, secCtx, objID, false))
	assert.True(t, mockProj.deleted[objID], "graph must stay in lockstep on CAS delete")
}

func TestFileFirstProjectionStorage_RebuildProjectionFromSSOT(t *testing.T) {
	tempDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tempDir)

	fileStorage, err := GetFileObjectStorage(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fileStorage.Shutdown(context.Background()) })

	mockProj := newMockProjectionProvider()
	storage := NewFileFirstProjectionStorage(fileStorage, mockProj)

	ctx := WithCLIOperation(context.Background())
	secCtx := pkgctx.NewSystemSecurityContext()

	// Seed directly into file SSOT (simulating offline file edits)
	objID := "GOAL-REBUILD-001"
	seedObj := map[string]any{
		pkgobjects.FieldKeyID:     objID,
		pkgobjects.FieldKeyKind:   "goal",
		pkgobjects.FieldKeyTitle:  "Seeded File Goal",
		pkgobjects.FieldKeyStatus: pkgobjects.ObjectStatusActive,
	}
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	CreateCASVisible(t, fileStorage, ctx, secCtx, seedObj, pkgobjects.ObjectStatusActive)

	// Projection does NOT have it yet (CreateCASVisible goes through file SSOT only)
	assert.NotContains(t, mockProj.created, objID)

	// Rebuild projection from file SSOT
	rebuiltCount, err := storage.RebuildProjectionFromSSOT(ctx, secCtx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, rebuiltCount, 1)

	// Verify projection now has the seeded object
	assert.Contains(t, mockProj.created, objID)
	assert.Equal(t, "Seeded File Goal", mockProj.created[objID][pkgobjects.FieldKeyTitle])
}

func TestFileFirstProjectionStorage_ProjectionFailClosed(t *testing.T) {
	t.Setenv(zqkenv.ProjectionFailClosed().Name(), "true")
	tempDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tempDir)

	fileStorage, err := GetFileObjectStorage(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fileStorage.Shutdown(context.Background()) })

	failing := &failingProjection{err: errfmt.Errorf("projection boom")}
	store := NewFileFirstProjectionStorage(fileStorage, failing)

	ctx := WithCLIOperation(pkgctx.WithPromoteOnCreate(context.Background()))
	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{
		pkgobjects.FieldKeyID:     "GOAL-FAIL-001",
		pkgobjects.FieldKeyKind:   "goal",
		pkgobjects.FieldKeyTitle:  "Fail Closed Goal Title",
		pkgobjects.FieldKeyStatus: pkgobjects.ObjectStatusActive,
	}
	err = store.Create(ctx, secCtx, obj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "projection")
}

func TestFileFirstProjectionStorage_RebuildOrphanPurge(t *testing.T) {
	tempDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tempDir)

	fileStorage, err := GetFileObjectStorage(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fileStorage.Shutdown(context.Background()) })

	mockProj := newMockProjectionProvider()
	orphanID := "GOAL-ORPHAN-001"
	mockProj.created[orphanID] = map[string]any{
		pkgobjects.FieldKeyID:    orphanID,
		pkgobjects.FieldKeyKind:  "goal",
		pkgobjects.FieldKeyTitle: "Orphan Only In Projection",
	}
	store := NewFileFirstProjectionStorage(fileStorage, mockProj)

	ctx := WithCLIOperation(context.Background())
	secCtx := pkgctx.NewSystemSecurityContext()
	res, err := store.RebuildProjectionFromSSOTDetailed(ctx, secCtx, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.GreaterOrEqual(t, res.OrphanDeleted, 1)
	assert.True(t, mockProj.deleted[orphanID])
	_, still := mockProj.created[orphanID]
	assert.False(t, still)
}

type failingProjection struct {
	err error
	mockProjectionProvider
}

func (f *failingProjection) Create(ctx context.Context, secCtx *SecurityContext, obj map[string]any) error {
	return f.err
}

func (f *failingProjection) Update(ctx context.Context, secCtx *SecurityContext, id string, updates map[string]any) error {
	return f.err
}

func (f *failingProjection) Delete(ctx context.Context, secCtx *SecurityContext, id string, cascade bool) error {
	return f.err
}

func (f *failingProjection) Exists(ctx context.Context, secCtx *SecurityContext, id string) (bool, error) {
	return false, nil
}
