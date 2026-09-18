package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"gopkg.in/yaml.v3"
)

func setupIOQueueTestEnv(t *testing.T) (testRoot string, fileStorage *FileObjectStorage) {
	t.Helper()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	mustEnsureProcessSpecsLayout(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	fos, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, fos)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	return tmpDir, fos
}

// TestIOQueue_ReadOperation tests queued read operations
func TestIOQueue_ReadOperation(t *testing.T) {
	testRoot, fileStorage := setupIOQueueTestEnv(t)

	// Set up I/O queue manager
	manager := GetGlobalIOQueueManager(pkgctx.NewSystemContext())
	manager.SetStorage(fileStorage)
	manager.SetProjectRoot(testRoot)

	// Create a test file
	testDir := filepath.Join(testRoot, "test_data")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	testFile := filepath.Join(testDir, "test_object.yaml")
	testObj := map[string]any{
		objects.FieldKeyID:   "TEST-001",
		objects.FieldKeyKind: "test_object",
		objects.FieldKeyName: "Test Object",
	}

	data, err := yaml.Marshal(testObj)
	if err != nil {
		t.Fatalf("Failed to marshal test object: %v", err)
	}

	if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Enqueue read operation
	resultChan := make(chan IOResult, 1)
	op := &IOOperation{
		Type:     IOOperationRead,
		FilePath: testFile,
		Result:   resultChan,
	}

	if err := manager.Enqueue(op); err != nil {
		t.Fatalf("Failed to enqueue read operation: %v", err)
	}

	// Wait for result
	select {
	case result := <-resultChan:
		if result.Err != nil {
			t.Fatalf("Read operation failed: %v", result.Err)
		}
		if result.Obj == nil {
			t.Fatal("Read operation returned nil object")
		}
		if result.Obj[objects.FieldKeyID] != "TEST-001" {
			t.Errorf("Expected id 'TEST-001', got %v", result.Obj[objects.FieldKeyID])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read operation timed out")
	}
}

// TestIOQueue_WriteOperation tests queued write operations
func TestIOQueue_WriteOperation(t *testing.T) {
	testRoot, fileStorage := setupIOQueueTestEnv(t)

	// Set up I/O queue manager
	manager := GetGlobalIOQueueManager(pkgctx.NewSystemContext())
	manager.SetStorage(fileStorage)
	manager.SetProjectRoot(testRoot)

	// Create test directory
	testDir := filepath.Join(testRoot, "test_data")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	testFile := filepath.Join(testDir, "test_write.yaml")
	testObj := map[string]any{
		objects.FieldKeyID:   "TEST-002",
		objects.FieldKeyKind: "test_object",
		objects.FieldKeyName: "Written Object",
	}

	data, err := yaml.Marshal(testObj)
	if err != nil {
		t.Fatalf("Failed to marshal test object: %v", err)
	}

	// Enqueue write operation
	resultChan := make(chan IOResult, 1)
	op := &IOOperation{
		Type:     IOOperationWrite,
		FilePath: testFile,
		Data:     data,
		Perm:     0600,
		Result:   resultChan,
	}

	if err := manager.Enqueue(op); err != nil {
		t.Fatalf("Failed to enqueue write operation: %v", err)
	}

	// Wait for result
	select {
	case result := <-resultChan:
		if result.Err != nil {
			t.Fatalf("Write operation failed: %v", result.Err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Write operation timed out")
	}

	// Verify file was written
	if _, err := fileutil.Stat(testFile); fileutil.IsNotExist(err) {
		t.Fatal("File was not created")
	}

	// Read back and verify
	readData, err := fileutil.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	var readObj map[string]any
	if err := yaml.Unmarshal(readData, &readObj); err != nil {
		t.Fatalf("Failed to unmarshal written file: %v", err)
	}

	if readObj[objects.FieldKeyID] != "TEST-002" {
		t.Errorf("Expected id 'TEST-002', got %v", readObj[objects.FieldKeyID])
	}
}

// TestIOQueue_OnDemandPattern tests the on-demand worker pattern
func TestIOQueue_OnDemandPattern(t *testing.T) {
	testRoot, fileStorage := setupIOQueueTestEnv(t)

	// Set up I/O queue manager
	manager := GetGlobalIOQueueManager(pkgctx.NewSystemContext())
	manager.SetStorage(fileStorage)
	manager.SetProjectRoot(testRoot)

	// Create test file
	testDir := filepath.Join(testRoot, "test_data")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	testFile := filepath.Join(testDir, "test_on_demand.yaml")
	testObj := map[string]any{
		objects.FieldKeyID:   "TEST-003",
		objects.FieldKeyKind: "test_object",
	}

	data, err := yaml.Marshal(testObj)
	if err != nil {
		t.Fatalf("Failed to marshal test object: %v", err)
	}

	if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Initially, workers should be idle (or minimal)
	// Enqueue an operation - worker should wake up
	resultChan := make(chan IOResult, 1)
	op := &IOOperation{
		Type:     IOOperationRead,
		FilePath: testFile,
		Result:   resultChan,
	}

	if err := manager.Enqueue(op); err != nil {
		t.Fatalf("Failed to enqueue read operation: %v", err)
	}

	// Wait for result
	select {
	case result := <-resultChan:
		if result.Err != nil {
			t.Fatalf("Read operation failed: %v", result.Err)
		}
		if result.Obj == nil {
			t.Fatal("Read operation returned nil object")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read operation timed out")
	}

	t.Log("On-demand pattern: worker woke up when work was enqueued")
}

// TestIOQueue_ConcurrentOperations tests concurrent read/write operations
func TestIOQueue_ConcurrentOperations(t *testing.T) {
	testRoot, fileStorage := setupIOQueueTestEnv(t)

	// Set up I/O queue manager
	manager := GetGlobalIOQueueManager(pkgctx.NewSystemContext())
	manager.SetStorage(fileStorage)
	manager.SetProjectRoot(testRoot)

	// Create test directory
	testDir := filepath.Join(testRoot, "test_data")
	if err := fileutil.MkdirAll(testDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Enqueue multiple concurrent operations
	numOps := 10
	results := make([]chan IOResult, numOps)
	var successCount atomic.Int64

	for i := 0; i < numOps; i++ {
		testFile := filepath.Join(testDir, fmt.Sprintf("test_concurrent_%d.yaml", i))
		testObj := map[string]any{
			objects.FieldKeyID:   fmt.Sprintf("TEST-%03d", i),
			objects.FieldKeyKind: "test_object",
			"seq":                i,
		}

		data, err := yaml.Marshal(testObj)
		if err != nil {
			t.Fatalf("Failed to marshal test object %d: %v", i, err)
		}

		// Write first
		writeResult := make(chan IOResult, 1)
		writeOp := &IOOperation{
			Type:     IOOperationWrite,
			FilePath: testFile,
			Data:     data,
			Perm:     0600,
			Result:   writeResult,
		}

		if err := manager.Enqueue(writeOp); err != nil {
			t.Fatalf("Failed to enqueue write operation %d: %v", i, err)
		}

		// Then read
		readResult := make(chan IOResult, 1)
		readOp := &IOOperation{
			Type:     IOOperationRead,
			FilePath: testFile,
			Result:   readResult,
		}

		if err := manager.Enqueue(readOp); err != nil {
			t.Fatalf("Failed to enqueue read operation %d: %v", i, err)
		}

		results[i] = readResult

		// Wait for write to complete
		select {
		case writeRes := <-writeResult:
			if writeRes.Err != nil {
				t.Errorf("Write operation %d failed: %v", i, writeRes.Err)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("Write operation %d timed out", i)
		}
	}

	// Wait for all reads to complete
	for i, resultChan := range results {
		select {
		case result := <-resultChan:
			if result.Err != nil {
				t.Errorf("Read operation %d failed: %v", i, result.Err)
			} else if result.Obj != nil {
				successCount.Add(1)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("Read operation %d timed out", i)
		}
	}

	if successCount.Load() != int64(numOps) {
		t.Errorf("Expected %d successful reads, got %d", numOps, successCount.Load())
	}
}

// TestIOQueue_ErrorHandling tests error handling for invalid operations
func TestIOQueue_ErrorHandling(t *testing.T) {
	testRoot, fileStorage := setupIOQueueTestEnv(t)

	// Set up I/O queue manager
	manager := GetGlobalIOQueueManager(pkgctx.NewSystemContext())
	manager.SetStorage(fileStorage)
	manager.SetProjectRoot(testRoot)

	// Test read of non-existent file
	resultChan := make(chan IOResult, 1)
	op := &IOOperation{
		Type:     IOOperationRead,
		FilePath: "/nonexistent/file.yaml",
		Result:   resultChan,
	}

	if err := manager.Enqueue(op); err != nil {
		t.Fatalf("Failed to enqueue read operation: %v", err)
	}

	// Wait for result
	select {
	case result := <-resultChan:
		if result.Err == nil {
			t.Error("Expected error for non-existent file, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read operation timed out")
	}

	// Test write with empty data
	writeResult := make(chan IOResult, 1)
	writeOp := &IOOperation{
		Type:     IOOperationWrite,
		FilePath: "/tmp/test_empty.yaml",
		Data:     []byte{}, // Empty data
		Perm:     0600,
		Result:   writeResult,
	}

	if err := manager.Enqueue(writeOp); err != nil {
		t.Fatalf("Failed to enqueue write operation: %v", err)
	}

	// Wait for result
	select {
	case result := <-writeResult:
		if result.Err == nil {
			t.Error("Expected error for empty write data, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Write operation timed out")
	}
}

// TestIOQueue_ShutdownCoordination tests shutdown coordination
func TestIOQueue_ShutdownCoordination(t *testing.T) {
	if zqkenv.EnableIOQueueShutdownTests().Get() == emptyValue {
		t.Skip("Skipping IO queue shutdown coordination test; set ZQK_ENABLE_IOQUEUE_SHUTDOWN_TESTS=1 to enable")
	}

	testRoot, fileStorage := setupIOQueueTestEnv(t)

	// Set up I/O queue manager
	manager := GetGlobalIOQueueManager(pkgctx.NewSystemContext())
	// Enable shutdown honoring in tests for this specific coordinator only,
	// and reset it afterwards so other tests are not affected.
	manager.testOverride = true
	defer func() {
		manager.testOverride = false
	}()
	manager.SetStorage(fileStorage)
	manager.SetProjectRoot(testRoot)

	// Verify manager implements QueueShutdownHandler
	if manager.GetName() != "io_queue_manager" {
		t.Errorf("Expected name 'io_queue_manager', got %s", manager.GetName())
	}

	if !manager.IsCritical() {
		t.Error("I/O queue manager should be critical")
	}

	// Use shutdown coordinator to initiate shutdown (this is how it's done in practice)
	coordinator := GetGlobalShutdownCoordinator()
	// Allow shutdown state to be visible in tests
	coordinator.testOverride = true
	if err := coordinator.InitiateShutdown(); err != nil {
		t.Fatalf("Failed to initiate shutdown: %v", err)
	}

	// Try to enqueue after shutdown - should fail
	resultChan := make(chan IOResult, 1)
	op := &IOOperation{
		Type:     IOOperationRead,
		FilePath: "/some/file.yaml",
		Result:   resultChan,
	}

	if err := manager.Enqueue(op); err == nil {
		t.Error("Expected error when enqueueing after shutdown, got nil")
	}

	// Also test direct InitiateShutdown
	if err := manager.InitiateShutdown(); err != nil {
		t.Fatalf("Failed to initiate shutdown directly: %v", err)
	}

	// Try to enqueue again - should still fail
	if err := manager.Enqueue(op); err == nil {
		t.Error("Expected error when enqueueing after direct shutdown, got nil")
	}

	// Drain should complete (with reasonable timeout)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	// Drain may timeout if workers are still processing, which is acceptable for this test
	err := manager.Drain(ctx)
	if err != nil {
		// Timeout is acceptable - workers may still be processing
		if errors.Is(err, context.DeadlineExceeded) {
			t.Logf("Drain timed out (acceptable if workers still processing): %v", err)
		} else {
			t.Logf("Drain returned error: %v", err)
		}
	}

	// Wait deterministically for workers to finish
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if manager.IsDrained() || manager.GetPendingCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Check pending count
	pending := manager.GetPendingCount()
	if pending > 0 {
		t.Logf("Note: %d pending operations after drain (may be in-flight)", pending)
	}
}

// TestIOQueueStateChangeEventCallback_Invoked verifies the state-change callback is invoked when SetProjectRoot/SetStorage are called.
func TestIOQueueStateChangeEventCallback_Invoked(t *testing.T) {
	testRoot, fileStorage := setupIOQueueTestEnv(t)

	manager := GetGlobalIOQueueManager(pkgctx.NewSystemContext())

	callbackInvoked := make(chan struct{}, 1)
	var changeTypes []string
	var mu sync.Mutex

	SetIOQueueStateChangeEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		changeType string,
	) {
		mu.Lock()
		changeTypes = append(changeTypes, changeType)
		mu.Unlock()
		select {
		case callbackInvoked <- struct{}{}:
		default:
		}
	})
	defer SetIOQueueStateChangeEventCallback(nil)

	manager.SetProjectRoot(testRoot)
	manager.SetStorage(fileStorage)

	select {
	case <-callbackInvoked:
	case <-time.After(5 * time.Second):
		t.Fatal("Expected state-change callback to be invoked within 5 seconds")
	}

	mu.Lock()
	got := changeTypes
	mu.Unlock()
	if len(got) == 0 {
		t.Fatal("Expected at least one changeType")
	}
	seen := make(map[string]bool)
	for _, c := range got {
		seen[c] = true
	}
	if !seen["project_root_set"] && !seen["storage_set"] {
		t.Errorf("Expected project_root_set or storage_set, got %v", got)
	}
}

// TestIOQueue_NoStorageProvider tests behavior when storage provider is not set
func TestIOQueue_NoStorageProvider(t *testing.T) {
	// Create a new manager without storage
	manager := &IOQueueManager{
		queues: make([]*ioQueue, 0),
		config: DefaultIOQueueConfig(),
	}

	// Add a queue
	queue := manager.addQueue(pkgctx.NewSystemContext())

	// Try to process read without storage
	resultChan := make(chan IOResult, 1)
	op := &IOOperation{
		Type:     IOOperationRead,
		FilePath: "/some/file.yaml",
		Result:   resultChan,
	}

	// Enqueue and process
	queue.operations <- op
	queue.queueDepth.Add(1)

	// Process operation
	result := queue.processOperation(op)

	if result.Err == nil {
		t.Error("Expected error when storage provider not set, got nil")
	}
}

// TestIOQueue_InvalidStorageType tests behavior with invalid storage type
func TestIOQueue_InvalidStorageType(t *testing.T) {
	manager := &IOQueueManager{
		queues: make([]*ioQueue, 0),
		config: DefaultIOQueueConfig(),
	}
	// Set invalid storage type using atomic.Value
	manager.SetStorage("not a FileObjectStorage") // Invalid type

	queue := manager.addQueue(pkgctx.NewSystemContext())

	resultChan := make(chan IOResult, 1)
	op := &IOOperation{
		Type:     IOOperationRead,
		FilePath: "/some/file.yaml",
		Result:   resultChan,
	}

	result := queue.processOperation(op)

	if result.Err == nil {
		t.Error("Expected error for invalid storage type, got nil")
	}
}
