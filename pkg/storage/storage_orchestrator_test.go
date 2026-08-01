package storage_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

func setupFileObjectStorageForOrchestratorTest(t *testing.T) *storage.FileObjectStorage {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	return fos
}

func TestStorageOrchestrator_BasicOperations(t *testing.T) {
	fos := setupFileObjectStorageForOrchestratorTest(t)

	orchestrator := storage.NewStorageOrchestrator()
	orchestrator.RegisterBackend("file", fos)

	objectID := "TEST-001"
	operationID := "op-001"
	backendName := "file"

	orchestrator.RegisterOperation(backendName, objectID, operationID)

	if !orchestrator.HasPendingOperations(objectID) {
		t.Error("Object should have pending operations")
	}

	pending := orchestrator.GetPendingOperations(objectID)
	if len(pending[backendName]) != 1 {
		t.Errorf("Expected 1 pending operation for backend, got %d", len(pending[backendName]))
	}

	err := orchestrator.PreventOperation(objectID, storage.OpUpdate)
	if err == nil {
		t.Error("Expected error when preventing operation on object with pending operations")
	}

	err = orchestrator.CompleteOperation(backendName, objectID, operationID)
	if err != nil {
		t.Errorf("Failed to complete operation: %v", err)
	}

	if orchestrator.HasPendingOperations(objectID) {
		t.Error("Object should not have pending operations after completion")
	}

	err = orchestrator.PreventOperation(objectID, storage.OpUpdate)
	if err != nil {
		t.Errorf("Operation should be allowed after pending operations complete: %v", err)
	}
}

func TestStorageOrchestrator_MultipleBackends(t *testing.T) {
	fos := setupFileObjectStorageForOrchestratorTest(t)

	orchestrator := storage.NewStorageOrchestrator()
	orchestrator.RegisterBackend("file", fos)

	objectID := "TEST-002"

	orchestrator.RegisterOperation("file", objectID, "op-file-001")

	if !orchestrator.HasPendingOperations(objectID) {
		t.Error("Object should have pending operations")
	}

	err := orchestrator.CompleteOperation("file", objectID, "op-file-001")
	if err != nil {
		t.Errorf("Failed to complete file operation: %v", err)
	}

	if orchestrator.HasPendingOperations(objectID) {
		t.Error("Object should not have pending operations after all backends complete")
	}
}

func TestStorageOrchestrator_ConcurrentOperations(t *testing.T) {
	fos := setupFileObjectStorageForOrchestratorTest(t)

	orchestrator := storage.NewStorageOrchestrator()
	orchestrator.RegisterBackend("file", fos)

	objectID := "TEST-003"
	backendName := "file"

	numOps := 50
	done := make(chan bool, numOps)

	for i := 0; i < numOps; i++ {
		goroutinelabels.NewGoroutine("storage_test", "concurrent storage orchestrator operation").StartSimple(func() {
			func(opNum int) {
				operationID := fmt.Sprintf("op-%d", opNum)
				orchestrator.RegisterOperation(backendName, objectID, operationID)
				time.Sleep(10 * time.Millisecond)
				_ = orchestrator.CompleteOperation(backendName, objectID, operationID) //nolint:errcheck // Test cleanup - errors are acceptable
				done <- true
			}(i)
		})
	}

	for i := 0; i < numOps; i++ {
		<-done
	}

	if orchestrator.HasPendingOperations(objectID) {
		t.Error("Object should not have pending operations after concurrent operations")
	}
}
