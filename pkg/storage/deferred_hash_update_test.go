package storage_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupDeferredHashTest(t *testing.T) (testRoot string, fos *storage.FileObjectStorage) {
	t.Helper()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	fos, err = storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	return tmpDir, fos
}

func TestDeferredHashManager_BasicOperations(t *testing.T) {
	testRoot, fos := setupDeferredHashTest(t)

	hashManager := storage.NewDeferredHashManager(fos)

	// Test registering an operation
	objectID := "TEST-001"
	objectKind := "test_object"
	filePath := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), "TEST-001.yaml")
	operationID := "op-001"

	hashManager.RegisterOperation(objectID, objectKind, filePath, operationID)

	// Verify operation is pending
	if hashManager.GetPendingOperations(objectID) != 1 {
		t.Errorf("Expected 1 pending operation, got %d", hashManager.GetPendingOperations(objectID))
	}

	if hashManager.IsObjectReady(objectID) {
		t.Error("Object should not be ready with pending operations")
	}

	// Create the file before completing operation (hash update needs file to exist)
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.EnsureDir(testDir); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	testContent := []byte("id: TEST-001\nkind: test_object\ntitle: Test Object\n")
	if err := fileutil.WriteSecureFile(filePath, testContent); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Complete operation (should trigger hash update)
	err := hashManager.CompleteOperation(objectID, operationID)
	if err != nil {
		t.Errorf("Failed to complete operation: %v", err)
	}

	// Verify no pending operations
	if hashManager.GetPendingOperations(objectID) != 0 {
		t.Errorf("Expected 0 pending operations, got %d", hashManager.GetPendingOperations(objectID))
	}

	if !hashManager.IsObjectReady(objectID) {
		t.Error("Object should be ready after all operations complete")
	}
}

func TestDeferredHashManager_MultipleOperations(t *testing.T) {
	testRoot, fos := setupDeferredHashTest(t)

	hashManager := storage.NewDeferredHashManager(fos)

	objectID := "TEST-002"
	objectKind := "test_object"
	filePath := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), "TEST-002.yaml")

	// Register multiple operations
	op1 := "op-001"
	op2 := "op-002"
	op3 := "op-003"

	hashManager.RegisterOperation(objectID, objectKind, filePath, op1)
	hashManager.RegisterOperation(objectID, objectKind, filePath, op2)
	hashManager.RegisterOperation(objectID, objectKind, filePath, op3)

	// Verify 3 pending operations
	if hashManager.GetPendingOperations(objectID) != 3 {
		t.Errorf("Expected 3 pending operations, got %d", hashManager.GetPendingOperations(objectID))
	}

	// Complete first operation
	err := hashManager.CompleteOperation(objectID, op1)
	if err != nil {
		t.Errorf("Failed to complete operation: %v", err)
	}

	// Verify 2 pending operations
	if hashManager.GetPendingOperations(objectID) != 2 {
		t.Errorf("Expected 2 pending operations, got %d", hashManager.GetPendingOperations(objectID))
	}

	// Complete remaining operations
	_ = hashManager.CompleteOperation(objectID, op2) //nolint:errcheck // Test cleanup - errors are acceptable
	_ = hashManager.CompleteOperation(objectID, op3) //nolint:errcheck // Test cleanup - errors are acceptable

	// Verify no pending operations
	if hashManager.GetPendingOperations(objectID) != 0 {
		t.Errorf("Expected 0 pending operations, got %d", hashManager.GetPendingOperations(objectID))
	}
}

func TestDeferredHashManager_ConcurrentOperations(t *testing.T) {
	testRoot, fos := setupDeferredHashTest(t)

	hashManager := storage.NewDeferredHashManager(fos)

	objectID := "TEST-003"
	objectKind := "test_object"
	filePath := filepath.Join(datacell.CellCASPrimaryDir(testRoot, "test"), "TEST-003.yaml")

	// Register operations concurrently
	numOps := 100
	done := make(chan bool, numOps)

	for i := 0; i < numOps; i++ {
		goroutinelabels.NewGoroutine("storage_test", "concurrent deferred hash update operation").StartSimple(func() {
			func(opNum int) {
				operationID := fmt.Sprintf("op-%d", opNum)
				hashManager.RegisterOperation(objectID, objectKind, filePath, operationID)
				time.Sleep(10 * time.Millisecond)                        // Simulate work
				_ = hashManager.CompleteOperation(objectID, operationID) //nolint:errcheck // Test cleanup - errors are acceptable
				done <- true
			}(i)
		})
	}

	// Wait for all operations to complete
	for i := 0; i < numOps; i++ {
		<-done
	}

	// Verify no pending operations
	if hashManager.GetPendingOperations(objectID) != 0 {
		t.Errorf("Expected 0 pending operations after concurrent operations, got %d",
			hashManager.GetPendingOperations(objectID))
	}
}

func TestDeferredHashManager_HashUpdate(t *testing.T) {
	testRoot, fos := setupDeferredHashTest(t)

	hashManager := storage.NewDeferredHashManager(fos)

	// Create a test object
	objectID := "TEST-004"
	objectKind := "test_object"
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	if err := fileutil.EnsureDir(testDir); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	filePath := filepath.Join(testDir, "TEST-004.yaml")
	testContent := []byte("id: TEST-004\nkind: test_object\ntitle: Test Object\n")
	if err := fileutil.WriteSecureFile(filePath, testContent); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Register and complete operation
	operationID := "op-001"
	hashManager.RegisterOperation(objectID, objectKind, filePath, operationID)

	// Complete operation (should trigger hash update)
	err := hashManager.CompleteOperation(objectID, operationID)
	if err != nil {
		t.Errorf("Failed to complete operation and update hash: %v", err)
	}

	// Verify hash was updated
	hashRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), objectKind, testDir)
	if err := hashRegistry.Load(); err != nil {
		t.Fatalf("failed to load hash registry: %v", err)
	}

	filename := filepath.Base(filePath)
	hash := hashRegistry.GetHash(filename)
	if hash == "" {
		t.Error("Hash was not updated in registry")
	}

	// Verify hash is correct
	expectedHash := storage.DeferredHashManagerCalculateHashForTest(hashManager, testContent)
	if hash != expectedHash {
		t.Errorf("Hash mismatch: expected %s, got %s", expectedHash, hash)
	}

	reg, proc := hashManager.GetDeferredHashStats()
	if reg != 1 {
		t.Errorf("Expected 1 registered operation, got %d", reg)
	}
	if proc != 1 {
		t.Errorf("Expected 1 processed operation, got %d", proc)
	}
}

func TestDeferredHashManager_LifetimeCounters(t *testing.T) {
	testRoot, fos := setupDeferredHashTest(t)
	hashManager := storage.NewDeferredHashManager(fos)

	objectID := "TEST-005"
	objectKind := "test_object"
	testDir := datacell.CellCASPrimaryDir(testRoot, "test")
	_ = fileutil.EnsureDir(testDir)

	filePath := filepath.Join(testDir, "TEST-005.yaml")
	_ = fileutil.WriteSecureFile(filePath, []byte("id: TEST-005\nkind: test_object\n"))

	hashManager.RegisterOperation(objectID, objectKind, filePath, "op-1")
	hashManager.RegisterOperation(objectID, objectKind, filePath, "op-2")

	reg, proc := hashManager.GetDeferredHashStats()
	if reg != 2 {
		t.Errorf("Expected registered=2, got %d", reg)
	}
	if proc != 0 {
		t.Errorf("Expected processed=0, got %d", proc)
	}

	_ = hashManager.CompleteOperation(objectID, "op-1")
	_ = hashManager.CompleteOperation(objectID, "op-2")

	reg, proc = hashManager.GetDeferredHashStats()
	if reg != 2 {
		t.Errorf("Expected registered=2, got %d", reg)
	}
	if proc != 1 {
		t.Errorf("Expected processed=1, got %d", proc)
	}
}
