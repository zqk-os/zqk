package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func TestSnapshotManager_BeginSnapshot(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")

	if manager.IsActive() {
		t.Error("Snapshot should not be active initially")
	}

	timestamp, err := manager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	if timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}

	if !manager.IsActive() {
		t.Error("Snapshot should be active after BeginSnapshot")
	}

	if !proxy.IsSnapshotActive() {
		t.Error("Proxy should be active after BeginSnapshot")
	}
}

func TestSnapshotManager_EndSnapshot(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	secCtx := pkgctx.NewSystemSecurityContext()

	_, _ = manager.BeginSnapshot() //nolint:errcheck // Test setup - error handling not critical

	// Queue some operations
	obj := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
	}
	if err := proxy.Create(context.Background(), secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	ctx := context.Background()
	if err := manager.EndSnapshot(ctx); err != nil {
		t.Fatalf("EndSnapshot failed: %v", err)
	}

	if manager.IsActive() {
		t.Error("Snapshot should not be active after EndSnapshot")
	}

	if proxy.IsSnapshotActive() {
		t.Error("Proxy should not be active after EndSnapshot")
	}

	// Operations should be replayed
	if len(mockStorage.objects) != 1 {
		t.Errorf("Expected 1 object in storage after replay, got %d", len(mockStorage.objects))
	}
}

func TestSnapshotManager_CaptureMetadata(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create objects in storage
	obj1 := map[string]any{
		objects.FieldKeyID:        "TEST-001",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyUpdatedAt: zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
	}
	obj2 := map[string]any{
		objects.FieldKeyID:        "TEST-002",
		objects.FieldKeyKind:      "test_object",
		objects.FieldKeyUpdatedAt: zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
	}

	if err := mockStorage.Create(context.Background(), secCtx, obj1); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := mockStorage.Create(context.Background(), secCtx, obj2); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	_, _ = manager.BeginSnapshot() //nolint:errcheck // Test setup - error handling not critical

	objectIDs := []string{"TEST-001", "TEST-002"}
	ctx := context.Background()
	metadata, err := manager.CaptureMetadata(ctx, objectIDs)
	if err != nil {
		t.Fatalf("CaptureMetadata failed: %v", err)
	}

	if metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	if len(metadata.ObjectIDs) != 2 {
		t.Errorf("Expected 2 object IDs, got %d", len(metadata.ObjectIDs))
	}

	if metadata.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}

	// Verify hashes were captured
	if len(metadata.Hashes) == 0 {
		t.Error("Expected hashes to be captured")
	}
}

func TestSnapshotManager_DoubleBeginSnapshot(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")

	_, err := manager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	// Second BeginSnapshot should fail
	_, err = manager.BeginSnapshot()
	if err == nil {
		t.Error("Expected error on second BeginSnapshot")
	}
}

func TestSnapshotManager_EndSnapshotWithoutBegin(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")

	ctx := context.Background()
	err := manager.EndSnapshot(ctx)
	if err == nil {
		t.Error("Expected error on EndSnapshot without BeginSnapshot")
	}
}

func TestSnapshotManager_GetSnapshotTimestamp(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")

	// Before BeginSnapshot, timestamp should be zero
	timestamp := manager.GetSnapshotTimestamp()
	if !timestamp.IsZero() {
		t.Error("Timestamp should be zero before BeginSnapshot")
	}

	beginTimestamp, err := manager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	// After BeginSnapshot, timestamp should match
	timestamp = manager.GetSnapshotTimestamp()
	if !timestamp.Equal(beginTimestamp) {
		t.Error("Timestamp should match BeginSnapshot return value")
	}
}

func TestSnapshotManager_ConcurrentCapture(t *testing.T) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create objects
	for i := 0; i < 10; i++ {
		obj := map[string]any{
			objects.FieldKeyID:        "TEST-001",
			objects.FieldKeyKind:      "test_object",
			objects.FieldKeyUpdatedAt: zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		}
		if err := mockStorage.Create(context.Background(), secCtx, obj); err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	}

	_, _ = manager.BeginSnapshot() //nolint:errcheck // Test setup - error handling not critical

	// Concurrent metadata capture should not cause deadlock
	ctx := context.Background()
	objectIDs := []string{"TEST-001"}

	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		i := i
		goroutinelabels.StartTestGoroutine(fmt.Sprintf("test_capture_metadata_%d", i), fmt.Sprintf("capturing metadata %d in snapshot test", i), func() {
			_, err := manager.CaptureMetadata(ctx, objectIDs)
			if err != nil {
				t.Errorf("CaptureMetadata failed: %v", err)
			}
			done <- true
		})
	}

	// Wait for all captures
	for i := 0; i < 5; i++ {
		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Deadlock detected in concurrent CaptureMetadata")
		}
	}
}
