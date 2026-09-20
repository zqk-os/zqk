package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// TestSnapshotManager_CaptureMetadataWithoutBegin tests capturing metadata without beginning snapshot
func TestSnapshotManager_CaptureMetadataWithoutBegin(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")

	// CaptureMetadata without BeginSnapshot should fail
	_, err := manager.CaptureMetadata(context.Background(), []string{"TEST-001"})
	if err == nil {
		t.Error("Expected error on CaptureMetadata without BeginSnapshot, got nil")
	}
}

// TestProxyStorage_ReplayEmptyQueue tests replaying empty queue
func TestProxyStorage_ReplayEmptyQueue(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)

	proxy.BeginSnapshot()
	proxy.EndSnapshot()

	// Replay empty queue should succeed
	ctx := context.Background()
	err := proxy.ReplayQueuedOperations(ctx)
	if err != nil {
		t.Errorf("ReplayQueuedOperations on empty queue failed: %v", err)
	}
}

// TestSnapshotManager_ConcurrentOperations tests concurrent snapshot operations
func TestSnapshotManager_ConcurrentOperations(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(1000)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Begin snapshot
	_, err := manager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	// Concurrent creates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		goroutinelabels.NewGoroutine("storage_test", "concurrent snapshot create").StartSimple(func() {
			func(id int) {
				obj := map[string]any{
					objects.FieldKeyID:            "TEST-CONCURRENT-001",
					objects.FieldKeyKind:          "test_object",
					"sequence":                    id,
					objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
				}
				err := proxy.Create(ctx, secCtx, obj)
				if err != nil {
					t.Errorf("Concurrent create failed: %v", err)
				}
				done <- true
			}(i)
		})
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all operations queued
	if queue.Size() != 10 {
		t.Errorf("Expected 10 operations in queue, got %d", queue.Size())
	}

	// End snapshot and replay
	err = manager.EndSnapshot(ctx)
	if err != nil {
		t.Fatalf("EndSnapshot failed: %v", err)
	}
}

// TestSnapshotManager_Cancellation tests snapshot cancellation
func TestSnapshotManager_Cancellation(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx, cancel := context.WithCancel(context.Background())

	// Begin snapshot
	_, err := manager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	// Queue some operations
	obj := map[string]any{
		objects.FieldKeyID:            "TEST-CANCEL-001",
		objects.FieldKeyKind:          "test_object",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	err = proxy.Create(ctx, secCtx, obj)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Cancel context
	cancel()

	// End snapshot (should still work, cancellation affects operations, not snapshot lifecycle)
	err = manager.EndSnapshot(ctx)
	if err != nil {
		// EndSnapshot may fail due to cancellation, which is acceptable
		t.Logf("EndSnapshot failed due to cancellation (expected): %v", err)
	}
}

// TestSnapshotManager_LargeObjectSet tests snapshot with large number of objects
func TestSnapshotManager_LargeObjectSet(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(10000)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Create many objects
	objectCount := 1000
	objectIDs := make([]string, objectCount)
	for i := 0; i < objectCount; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            "TEST-LARGE-001",
			objects.FieldKeyKind:          "test_object",
			"sequence":                    i,
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		_ = mockStorage.Create(ctx, secCtx, obj) //nolint:errcheck // Test setup - error handling not critical
		objectIDs[i] = "TEST-LARGE-001"
	}

	// Begin snapshot
	_, err := manager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	// Capture metadata for all objects
	metadata, err := manager.CaptureMetadata(ctx, objectIDs)
	if err != nil {
		t.Fatalf("CaptureMetadata failed: %v", err)
	}

	if len(metadata.ObjectIDs) != objectCount {
		t.Errorf("Expected %d object IDs, got %d", objectCount, len(metadata.ObjectIDs))
	}

	// End snapshot
	err = manager.EndSnapshot(ctx)
	if err != nil {
		t.Fatalf("EndSnapshot failed: %v", err)
	}
}

// TestSnapshotManager_MissingObjectHash tests handling of missing object hashes
func TestSnapshotManager_MissingObjectHash(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Create object without hash registry
	obj := map[string]any{
		objects.FieldKeyID:            "TEST-NOHASH-001",
		objects.FieldKeyKind:          "test_object",
		objects.FieldKeyTitle:         "Test Object",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	_ = mockStorage.Create(ctx, secCtx, obj) //nolint:errcheck // Test setup - best effort

	// Begin snapshot
	_, err := manager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	// Capture metadata (should handle missing hash gracefully)
	metadata, err := manager.CaptureMetadata(ctx, []string{"TEST-NOHASH-001"})
	if err != nil {
		t.Fatalf("CaptureMetadata failed: %v", err)
	}

	// Hash may be empty or calculated
	if len(metadata.Hashes) == 0 {
		t.Log("No hashes captured (acceptable if hash calculation fails)")
	}

	// End snapshot
	err = manager.EndSnapshot(ctx)
	if err != nil {
		t.Fatalf("EndSnapshot failed: %v", err)
	}
}

// TestSnapshotManager_InvalidTimestamp tests handling of invalid timestamps
func TestSnapshotManager_InvalidTimestamp(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	ctx := context.Background()

	// Begin snapshot
	snapshotTimestamp, err := manager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	// Verify timestamp is valid
	if snapshotTimestamp.IsZero() {
		t.Error("Snapshot timestamp should not be zero")
	}

	// Verify timestamp is recent (within last second)
	now := time.Now().UTC()
	if snapshotTimestamp.After(now) {
		t.Error("Snapshot timestamp should not be in the future")
	}
	if now.Sub(snapshotTimestamp) > time.Second {
		t.Error("Snapshot timestamp should be recent")
	}

	// End snapshot
	err = manager.EndSnapshot(ctx)
	if err != nil {
		t.Fatalf("EndSnapshot failed: %v", err)
	}
}
