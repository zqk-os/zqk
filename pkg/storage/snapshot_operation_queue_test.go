package storage

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestSnapshotOperationQueue_Enqueue(t *testing.T) {
	queue := NewSnapshotOperationQueue(100)
	secCtx := pkgctx.NewSystemSecurityContext()

	op := &SnapshotOperation{
		Type:     SnapshotOpCreate,
		ObjectID: "TEST-001",
		Kind:     "test_object",
		Data:     map[string]any{objects.FieldKeyID: "TEST-001", objects.FieldKeyKind: "test_object"},
		SecCtx:   secCtx,
	}

	err := queue.Enqueue(op)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if queue.Size() != 1 {
		t.Errorf("Expected queue size 1, got %d", queue.Size())
	}

	if op.Sequence != 0 {
		t.Errorf("Expected sequence 0, got %d", op.Sequence)
	}
}

func TestSnapshotOperationQueue_FIFOOrdering(t *testing.T) {
	queue := NewSnapshotOperationQueue(100)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Enqueue multiple operations
	for i := 0; i < 5; i++ {
		op := &SnapshotOperation{
			Type:     SnapshotOpCreate,
			ObjectID: "TEST-001",
			Kind:     "test_object",
			Data:     map[string]any{objects.FieldKeyID: "TEST-001", "sequence": i},
			SecCtx:   secCtx,
		}
		if err := queue.Enqueue(op); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	if queue.Size() != 5 {
		t.Errorf("Expected queue size 5, got %d", queue.Size())
	}
}

func TestSnapshotOperationQueue_PendingWriteTracking(t *testing.T) {
	queue := NewSnapshotOperationQueue(100)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create operation should track pending write
	op := &SnapshotOperation{
		Type:     SnapshotOpCreate,
		ObjectID: "TEST-001",
		Kind:     "test_object",
		Data:     map[string]any{objects.FieldKeyID: "TEST-001"},
		SecCtx:   secCtx,
	}

	if err := queue.Enqueue(op); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if !queue.HasPendingWrite("TEST-001") {
		t.Error("Expected pending write for TEST-001")
	}

	if queue.HasPendingWrite("TEST-002") {
		t.Error("Should not have pending write for TEST-002")
	}
}

func TestSnapshotOperationQueue_GetDataFromPendingWrite(t *testing.T) {
	queue := NewSnapshotOperationQueue(100)
	secCtx := pkgctx.NewSystemSecurityContext()

	expectedData := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
		objects.FieldKeyName: "Test Object",
	}

	op := &SnapshotOperation{
		Type:     SnapshotOpCreate,
		ObjectID: "TEST-001",
		Kind:     "test_object",
		Data:     expectedData,
		SecCtx:   secCtx,
	}

	if err := queue.Enqueue(op); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	data, err := queue.GetDataFromPendingWrite("TEST-001")
	if err != nil {
		t.Fatalf("GetDataFromPendingWrite failed: %v", err)
	}

	if data[objects.FieldKeyID] != expectedData[objects.FieldKeyID] {
		t.Errorf("Expected id %v, got %v", expectedData[objects.FieldKeyID], data[objects.FieldKeyID])
	}

	if data[objects.FieldKeyName] != expectedData[objects.FieldKeyName] {
		t.Errorf("Expected name %v, got %v", expectedData[objects.FieldKeyName], data[objects.FieldKeyName])
	}
}

func TestSnapshotOperationQueue_Replay(t *testing.T) {
	queue := NewSnapshotOperationQueue(100)
	secCtx := pkgctx.NewSystemSecurityContext()

	mockStorage := &MockObjectStorage{
		objects: make(map[string]map[string]any),
	}

	// Enqueue create operation
	op1 := &SnapshotOperation{
		Type:     SnapshotOpCreate,
		ObjectID: "TEST-001",
		Kind:     "test_object",
		Data:     map[string]any{objects.FieldKeyID: "TEST-001", objects.FieldKeyKind: "test_object", objects.FieldKeyName: "Test 1"},
		SecCtx:   secCtx,
	}
	if err := queue.Enqueue(op1); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Enqueue update operation
	op2 := &SnapshotOperation{
		Type:     SnapshotOpUpdate,
		ObjectID: "TEST-001",
		Data:     map[string]any{objects.FieldKeyName: "Test 1 Updated"},
		SecCtx:   secCtx,
	}
	if err := queue.Enqueue(op2); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// Replay operations
	ctx := context.Background()
	if err := queue.Replay(ctx, mockStorage); err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	// Verify operations were executed
	if len(mockStorage.objects) != 1 {
		t.Errorf("Expected 1 object, got %d", len(mockStorage.objects))
	}

	obj := mockStorage.objects["TEST-001"]
	if obj == nil {
		t.Fatal("Object TEST-001 not found")
	}

	if obj[objects.FieldKeyName] != "Test 1 Updated" {
		t.Errorf("Expected name 'Test 1 Updated', got %v", obj[objects.FieldKeyName])
	}

	// Queue should be cleared after replay
	if queue.Size() != 0 {
		t.Errorf("Expected queue size 0 after replay, got %d", queue.Size())
	}
}

func TestSnapshotOperationQueue_Overflow(t *testing.T) {
	queue := NewSnapshotOperationQueue(5) // Small max size
	secCtx := pkgctx.NewSystemSecurityContext()

	// Fill queue to capacity
	for i := 0; i < 5; i++ {
		op := &SnapshotOperation{
			Type:     SnapshotOpCreate,
			ObjectID: "TEST-001",
			Kind:     "test_object",
			Data:     map[string]any{objects.FieldKeyID: "TEST-001"},
			SecCtx:   secCtx,
		}
		if err := queue.Enqueue(op); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	// Next enqueue should fail
	op := &SnapshotOperation{
		Type:     SnapshotOpCreate,
		ObjectID: "TEST-002",
		Kind:     "test_object",
		Data:     map[string]any{objects.FieldKeyID: "TEST-002"},
		SecCtx:   secCtx,
	}

	err := queue.Enqueue(op)
	if err == nil {
		t.Error("Expected overflow error, got nil")
	}

	if queue.Size() != 5 {
		t.Errorf("Expected queue size 5, got %d", queue.Size())
	}
}

func TestSnapshotOperationQueue_ConcurrentEnqueue(t *testing.T) {
	queue := NewSnapshotOperationQueue(1000)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Concurrent enqueue operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		goroutinelabels.NewGoroutine("storage_test", "concurrent snapshot enqueue").StartSimple(func() {
			func(id int) {
				for j := 0; j < 10; j++ {
					op := &SnapshotOperation{
						Type:     SnapshotOpCreate,
						ObjectID: "TEST-001",
						Kind:     "test_object",
						Data:     map[string]any{objects.FieldKeyID: "TEST-001", "goroutine": id, "iteration": j},
						SecCtx:   secCtx,
					}
					if err := queue.Enqueue(op); err != nil {
						t.Errorf("Concurrent enqueue failed: %v", err)
					}
				}
				done <- true
			}(i)
		})
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all operations were enqueued
	if queue.Size() != 100 {
		t.Errorf("Expected queue size 100, got %d", queue.Size())
	}
}

func TestSnapshotOperationQueue_Clear(t *testing.T) {
	queue := NewSnapshotOperationQueue(100)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Enqueue some operations
	for i := 0; i < 5; i++ {
		op := &SnapshotOperation{
			Type:     SnapshotOpCreate,
			ObjectID: "TEST-001",
			Kind:     "test_object",
			Data:     map[string]any{objects.FieldKeyID: "TEST-001"},
			SecCtx:   secCtx,
		}
		if err := queue.Enqueue(op); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	if queue.Size() != 5 {
		t.Errorf("Expected queue size 5, got %d", queue.Size())
	}

	queue.Clear()

	if queue.Size() != 0 {
		t.Errorf("Expected queue size 0 after clear, got %d", queue.Size())
	}

	if queue.HasPendingWrite("TEST-001") {
		t.Error("Should not have pending write after clear")
	}
}

// MockObjectStorage for testing
type MockObjectStorage struct {
	objects map[string]map[string]any
}

func (m *MockObjectStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id, _ := obj[objects.FieldKeyID].(string)
	m.objects[id] = obj
	return nil
}

func (m *MockObjectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	obj, ok := m.objects[id]
	if !ok {
		return nil, nil
	}
	return obj, nil
}

func (m *MockObjectStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	obj, ok := m.objects[id]
	if !ok {
		// Create if doesn't exist
		obj = make(map[string]any)
		obj[objects.FieldKeyID] = id
		m.objects[id] = obj
	}
	for k, v := range updates {
		obj[k] = v
	}
	return nil
}

func (m *MockObjectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	delete(m.objects, id)
	return nil
}

// Implement other required methods with minimal implementations
//
//nolint:gocritic // Interface requires value semantics for ListFilter
func (m *MockObjectStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	return nil, nil
}

func (m *MockObjectStorage) Query(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query Query) (*QueryResult, error) {
	return nil, nil
}

//nolint:gocritic // Interface requires value semantics for SearchQuery
func (m *MockObjectStorage) Search(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, query SearchQuery) (*SearchResult, error) {
	return nil, nil
}

func (m *MockObjectStorage) BeginTransaction(ctx context.Context) (ObjectTransaction, error) {
	return nil, nil
}

func (m *MockObjectStorage) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objects []map[string]any) (*BulkResult, error) {
	return nil, nil
}

func (m *MockObjectStorage) BulkUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, updates []BulkUpdateItem) (*BulkResult, error) {
	return nil, nil
}

func (m *MockObjectStorage) BulkGet(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string) (*BulkResult, error) {
	return nil, nil
}

func (m *MockObjectStorage) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	return nil, nil
}

func (m *MockObjectStorage) GetRelated(ctx context.Context, secCtx *pkgctx.SecurityContext, id, relationshipType string, depth int) ([]map[string]any, error) {
	return nil, nil
}

func (m *MockObjectStorage) GetPath(ctx context.Context, secCtx *pkgctx.SecurityContext, fromID, toID string) ([]map[string]any, error) {
	return nil, nil
}

func (m *MockObjectStorage) GetNeighbors(ctx context.Context, secCtx *pkgctx.SecurityContext, id, direction string) ([]map[string]any, error) {
	return nil, nil
}

func (m *MockObjectStorage) Move(ctx context.Context, secCtx *pkgctx.SecurityContext, id, newKind string, updateReferences bool) error {
	return nil
}

func (m *MockObjectStorage) Rename(ctx context.Context, secCtx *pkgctx.SecurityContext, oldID, newID string, updateReferences bool) error {
	return nil
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (m *MockObjectStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) {
	return nil, nil
}

func (m *MockObjectStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	_, ok := m.objects[id]
	return ok, nil
}

//nolint:gocritic // Interface requires value semantics for ListFilter
func (m *MockObjectStorage) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter ListFilter) (int, error) {
	return len(m.objects), nil
}
