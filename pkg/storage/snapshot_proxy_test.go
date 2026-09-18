package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestProxyStorage_BeginEndSnapshot(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)

	if proxy.IsSnapshotActive() {
		t.Error("Snapshot should not be active initially")
	}

	proxy.BeginSnapshot()

	if !proxy.IsSnapshotActive() {
		t.Error("Snapshot should be active after BeginSnapshot")
	}

	proxy.EndSnapshot()

	if proxy.IsSnapshotActive() {
		t.Error("Snapshot should not be active after EndSnapshot")
	}
}

func TestProxyStorage_CreateDuringSnapshot(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()

	proxy.BeginSnapshot()

	obj := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
		objects.FieldKeyName: "Test Object",
	}

	// Create should be queued, not executed
	err := proxy.Create(context.Background(), secCtx, obj)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Object should not be in storage yet
	if len(mockStorage.objects) != 0 {
		t.Errorf("Expected 0 objects in storage, got %d", len(mockStorage.objects))
	}

	// Operation should be queued
	if queue.Size() != 1 {
		t.Errorf("Expected 1 operation in queue, got %d", queue.Size())
	}

	proxy.EndSnapshot()

	// Replay operations
	ctx := context.Background()
	if err := proxy.ReplayQueuedOperations(ctx); err != nil {
		t.Fatalf("ReplayQueuedOperations failed: %v", err)
	}

	// After replay, object should be in storage
	if len(mockStorage.objects) != 1 {
		t.Errorf("Expected 1 object in storage after replay, got %d", len(mockStorage.objects))
	}

	if mockStorage.objects["TEST-001"] == nil {
		t.Fatal("Object TEST-001 not found in storage")
	}
}

func TestProxyStorage_ReadAfterWriteConsistency(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()

	proxy.BeginSnapshot()

	// Create object
	obj := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
		objects.FieldKeyName: "Test Object",
	}

	if err := proxy.Create(context.Background(), secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Read should return data from queued write
	readObj, err := proxy.Read(context.Background(), secCtx, "TEST-001")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if readObj == nil {
		t.Fatal("Read returned nil")
	}

	if readObj[objects.FieldKeyName] != "Test Object" {
		t.Errorf("Expected name 'Test Object', got %v", readObj[objects.FieldKeyName])
	}

	// Object should not be in storage yet
	if len(mockStorage.objects) != 0 {
		t.Errorf("Expected 0 objects in storage, got %d", len(mockStorage.objects))
	}
}

func TestProxyStorage_UpdateDuringSnapshot(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create object first (not in snapshot)
	obj := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
		objects.FieldKeyName: "Original Name",
	}
	if err := mockStorage.Create(context.Background(), secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	proxy.BeginSnapshot()

	// Update should be queued
	updates := map[string]any{objects.FieldKeyName: "Updated Name"}
	if err := proxy.Update(context.Background(), secCtx, "TEST-001", updates); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Original name should still be in storage
	if mockStorage.objects["TEST-001"][objects.FieldKeyName] != "Original Name" {
		t.Errorf("Expected name 'Original Name', got %v", mockStorage.objects["TEST-001"][objects.FieldKeyName])
	}

	proxy.EndSnapshot()

	// Replay operations
	ctx := context.Background()
	if err := proxy.ReplayQueuedOperations(ctx); err != nil {
		t.Fatalf("ReplayQueuedOperations failed: %v", err)
	}

	// After replay, name should be updated
	if mockStorage.objects["TEST-001"][objects.FieldKeyName] != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got %v", mockStorage.objects["TEST-001"][objects.FieldKeyName])
	}
}

func TestProxyStorage_DeleteDuringSnapshot(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create object first
	obj := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
	}
	if err := mockStorage.Create(context.Background(), secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	proxy.BeginSnapshot()

	// Delete should be queued
	if err := proxy.Delete(context.Background(), secCtx, "TEST-001", false); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Object should still exist
	if len(mockStorage.objects) != 1 {
		t.Errorf("Expected 1 object in storage, got %d", len(mockStorage.objects))
	}

	proxy.EndSnapshot()

	// Replay operations
	ctx := context.Background()
	if err := proxy.ReplayQueuedOperations(ctx); err != nil {
		t.Fatalf("ReplayQueuedOperations failed: %v", err)
	}

	// After replay, object should be deleted
	if len(mockStorage.objects) != 0 {
		t.Errorf("Expected 0 objects in storage after delete, got %d", len(mockStorage.objects))
	}
}

func TestProxyStorage_NormalOperationWhenNotActive(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Operations should execute normally when snapshot is not active
	obj := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
		objects.FieldKeyName: "Test Object",
	}

	if err := proxy.Create(context.Background(), secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Object should be in storage immediately
	if len(mockStorage.objects) != 1 {
		t.Errorf("Expected 1 object in storage, got %d", len(mockStorage.objects))
	}

	// Queue should be empty
	if queue.Size() != 0 {
		t.Errorf("Expected 0 operations in queue, got %d", queue.Size())
	}
}

func TestProxyStorage_ConcurrentOperations(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(1000)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()

	proxy.BeginSnapshot()

	// Concurrent creates
	var wg sync.WaitGroup
	numGoroutines := 10
	opsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent proxy storage operations").StartSimple(func() {
			func(_ int) {
				defer wg.Done()
				for j := 0; j < opsPerGoroutine; j++ {
					obj := map[string]any{
						objects.FieldKeyID:   "TEST-001",
						objects.FieldKeyKind: "test_object",
						objects.FieldKeyName: "Test Object",
					}
					if err := proxy.Create(context.Background(), secCtx, obj); err != nil {
						t.Errorf("Concurrent create failed: %v", err)
					}
				}
			}(i)
		})
	}

	wg.Wait()

	// All operations should be queued
	expectedSize := numGoroutines * opsPerGoroutine
	if queue.Size() != expectedSize {
		t.Errorf("Expected %d operations in queue, got %d", expectedSize, queue.Size())
	}

	// No deadlock should occur
	done := make(chan bool, 1)
	goroutinelabels.StartTestGoroutine("test_end_snapshot", "ending snapshot in test", func() {
		proxy.EndSnapshot()
		done <- true
	})

	select {
	case <-done:
		// Success - no deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("Deadlock detected - EndSnapshot did not complete")
	}
}

func TestProxyStorage_BulkOperations(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()

	proxy.BeginSnapshot()

	// Bulk create
	bulkObjs := []map[string]any{
		{objects.FieldKeyID: "TEST-001", objects.FieldKeyKind: "test_object"},
		{objects.FieldKeyID: "TEST-002", objects.FieldKeyKind: "test_object"},
		{objects.FieldKeyID: "TEST-003", objects.FieldKeyKind: "test_object"},
	}

	result, err := proxy.BulkCreate(context.Background(), secCtx, bulkObjs)
	if err != nil {
		t.Fatalf("BulkCreate failed: %v", err)
	}

	if result.SuccessCount != 3 {
		t.Errorf("Expected 3 successes, got %d", result.SuccessCount)
	}

	// Operations should be queued
	if queue.Size() != 3 {
		t.Errorf("Expected 3 operations in queue, got %d", queue.Size())
	}

	proxy.EndSnapshot()

	// Replay operations
	ctx := context.Background()
	if err := proxy.ReplayQueuedOperations(ctx); err != nil {
		t.Fatalf("ReplayQueuedOperations failed: %v", err)
	}

	// After replay, all objects should be in storage
	if len(mockStorage.objects) != 3 {
		t.Errorf("Expected 3 objects in storage, got %d", len(mockStorage.objects))
	}
}

func TestProxyStorage_ReplayOrder(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()

	proxy.BeginSnapshot()

	// Create object
	obj := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
		objects.FieldKeyName: "Original",
	}
	if err := proxy.Create(context.Background(), secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update 1
	if err := proxy.Update(context.Background(), secCtx, "TEST-001", map[string]any{objects.FieldKeyName: "Update 1"}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Update 2
	if err := proxy.Update(context.Background(), secCtx, "TEST-001", map[string]any{objects.FieldKeyName: "Update 2"}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	proxy.EndSnapshot()

	// Replay operations
	ctx := context.Background()
	if err := proxy.ReplayQueuedOperations(ctx); err != nil {
		t.Fatalf("ReplayQueuedOperations failed: %v", err)
	}

	// Final name should be from last update
	if mockStorage.objects["TEST-001"] == nil {
		t.Fatal("Object TEST-001 not found in storage")
	}
	if mockStorage.objects["TEST-001"][objects.FieldKeyName] != "Update 2" {
		t.Errorf("Expected name 'Update 2', got %v", mockStorage.objects["TEST-001"][objects.FieldKeyName])
	}
}

func TestProxyStorage_ReadWithoutPendingWrite(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create object in storage (not in snapshot)
	obj := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
		objects.FieldKeyName: "Stored Object",
	}
	if err := mockStorage.Create(context.Background(), secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	proxy.BeginSnapshot()

	// Read should work normally (no pending write)
	readObj, err := proxy.Read(context.Background(), secCtx, "TEST-001")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if readObj == nil {
		t.Fatal("Read returned nil")
	}

	if readObj[objects.FieldKeyName] != "Stored Object" {
		t.Errorf("Expected name 'Stored Object', got %v", readObj[objects.FieldKeyName])
	}
}
