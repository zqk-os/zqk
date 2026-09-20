package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// BenchmarkSnapshotManager_BeginSnapshot benchmarks snapshot initiation
func BenchmarkSnapshotManager_BeginSnapshot(b *testing.B) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(10000)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.BeginSnapshot()
		if err != nil {
			b.Fatalf("BeginSnapshot failed: %v", err)
		}
		_ = manager.EndSnapshot(context.Background()) //nolint:errcheck // Benchmark cleanup - error handling not critical
	}
}

// BenchmarkSnapshotManager_CaptureMetadata benchmarks metadata capture
func BenchmarkSnapshotManager_CaptureMetadata(b *testing.B) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(10000)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Create test objects
	objectIDs := make([]string, 100)
	for i := 0; i < 100; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            "TEST-BENCH-001",
			objects.FieldKeyKind:          "test_object",
			objects.FieldKeyTitle:         "Test Object",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		_ = mockStorage.Create(ctx, secCtx, obj) //nolint:errcheck // Benchmark setup - error handling not critical
		objectIDs[i] = "TEST-BENCH-001"
	}

	_, _ = manager.BeginSnapshot() //nolint:errcheck // Benchmark setup - error handling not critical

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.CaptureMetadata(ctx, objectIDs)
		if err != nil {
			b.Fatalf("CaptureMetadata failed: %v", err)
		}
	}

	_ = manager.EndSnapshot(ctx) //nolint:errcheck // Benchmark cleanup - error handling not critical
}

// BenchmarkProxyStorage_QueueOperations benchmarks operation queuing
func BenchmarkProxyStorage_QueueOperations(b *testing.B) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(100000)
	proxy := NewProxyStorage(mockStorage, queue)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	proxy.BeginSnapshot()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            "TEST-BENCH-002",
			objects.FieldKeyKind:          "test_object",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		err := proxy.Create(ctx, secCtx, obj)
		if err != nil {
			b.Fatalf("Create failed: %v", err)
		}
	}

	proxy.EndSnapshot()
}

// BenchmarkSnapshotOperationQueue_Enqueue benchmarks queue enqueue operations
func BenchmarkSnapshotOperationQueue_Enqueue(b *testing.B) {
	queue := NewSnapshotOperationQueue(100000)
	secCtx := pkgctx.NewSystemSecurityContext()

	op := &SnapshotOperation{
		Type:     SnapshotOpCreate,
		ObjectID: "TEST-BENCH-003",
		Kind:     "test_object",
		Data:     map[string]any{objects.FieldKeyID: "TEST-BENCH-003", objects.FieldKeyKind: "test_object"},
		SecCtx:   secCtx,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := queue.Enqueue(op)
		if err != nil {
			b.Fatalf("Enqueue failed: %v", err)
		}
	}
}

// BenchmarkReconstructStateAtTimestamp benchmarks state reconstruction
func BenchmarkReconstructStateAtTimestamp(b *testing.B) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Create object
	obj := map[string]any{
		objects.FieldKeyID:            "TEST-BENCH-004",
		objects.FieldKeyKind:          "test_object",
		objects.FieldKeyTitle:         "Original",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	_ = mockStorage.Create(ctx, secCtx, obj) //nolint:errcheck // Test setup - best effort

	snapshotTimestamp := time.Now().UTC()

	// Create some change journal entries
	for i := 0; i < 10; i++ {
		entry := map[string]any{
			objects.FieldKeyID:            "CJE-BENCH-001",
			objects.FieldKeyKind:          "change_journal_entry",
			objects.FieldKeyObjectRef:     "test_object:TEST-BENCH-004",
			objects.FieldKeyChangeType:    "update",
			objects.FieldKeyPreviousState: map[string]any{objects.FieldKeyTitle: "Previous"},
			objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		_ = mockStorage.Create(ctx, secCtx, entry) //nolint:errcheck // Test setup - best effort
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ReconstructStateAtTimestamp(
			ctx,
			mockStorage,
			obj,
			"TEST-BENCH-004",
			"test_object",
			snapshotTimestamp,
			logger,
		)
		if err != nil {
			b.Fatalf("ReconstructStateAtTimestamp failed: %v", err)
		}
	}
}

// BenchmarkSnapshotManager_EndToEnd benchmarks complete snapshot workflow
func BenchmarkSnapshotManager_EndToEnd(b *testing.B) {
	mockStorage := &MockObjectStorage{objects: make(map[string]map[string]any)}
	queue := NewSnapshotOperationQueue(10000)
	proxy := NewProxyStorage(mockStorage, queue)
	manager := NewSnapshotManager(proxy, queue, mockStorage, "/tmp/test")
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := context.Background()

	// Create test objects
	objectIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		obj := map[string]any{
			objects.FieldKeyID:            "TEST-BENCH-005",
			objects.FieldKeyKind:          "test_object",
			objects.FieldKeyTitle:         "Test Object",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		_ = mockStorage.Create(ctx, secCtx, obj) //nolint:errcheck // Benchmark setup - error handling not critical
		objectIDs[i] = "TEST-BENCH-005"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Begin snapshot
		_, err := manager.BeginSnapshot()
		if err != nil {
			b.Fatalf("BeginSnapshot failed: %v", err)
		}

		// Capture metadata
		_, err = manager.CaptureMetadata(ctx, objectIDs)
		if err != nil {
			b.Fatalf("CaptureMetadata failed: %v", err)
		}

		// End snapshot
		err = manager.EndSnapshot(ctx)
		if err != nil {
			b.Fatalf("EndSnapshot failed: %v", err)
		}
	}
}

func (m *MockObjectStorage) Shutdown(ctx context.Context) error {
	return nil
}
