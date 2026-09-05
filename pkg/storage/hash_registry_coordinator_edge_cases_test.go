package storage

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TestHashRegistry_CoordinatorIntegration_ConcurrentSaves tests concurrent save operations
func TestHashRegistry_CoordinatorIntegration_ConcurrentSaves(t *testing.T) {
	// Runs sequentially due to shared global callback
	if os.Getenv(zqkenv.EnableHashRegistryCoordinatorTests()) == "" && os.Getenv("STORAGE_TEST_ENABLE_HASH_REGISTRY_COORDINATOR_TESTS") == "" {
		t.Skip("Skipping hash registry coordinator integration test; set ZQK_ENABLE_HASH_REGISTRY_COORDINATOR_TESTS=1 to enable")
	}
	testRoot, fileStorage, _ := setupTestingFactoryCompleteTestEnvironment(t)

	kindDir := datacell.CellCASPrimaryDir(testRoot, "backlog")
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	registry := NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", kindDir)
	registry.SetProjectRoot(testRoot)
	registry.SetStorage(fileStorage)

	var callbackCalls atomic.Int32
	callbackWaiter := newCallbackWaiter()
	SetHashRegistryEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		kind string,
		batchSize int,
		hashCount int,
		duration time.Duration,
		status string,
		err error,
	) {
		// Only count events for this test's registry
		if projectRoot != testRoot {
			return
		}
		callbackCalls.Add(1)
		if status == "complete" || status == "error" {
			callbackWaiter.Invoke()
		}
	})
	defer SetHashRegistryEventCallback(nil)

	// Concurrent saves
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_test", "concurrent hash registry save").StartSimple(func() {
			func(idx int) {
				defer wg.Done()
				registry.SetHash(fmt.Sprintf("file%d.yaml", idx), fmt.Sprintf("hash%d", idx))
				if err := registry.Save(); err != nil {
					t.Errorf("Save() failed: %v", err)
				}
			}(i)
		})
	}
	wg.Wait()

	if !callbackWaiter.WaitForInvocation(pkgctx.NewSystemContext(), 5*time.Second) {
		t.Error("Expected callbacks to be called within 5 seconds, but none were")
	}
}

// TestHashRegistry_CoordinatorIntegration_EmptyRegistry tests with empty registry
func TestHashRegistry_CoordinatorIntegration_EmptyRegistry(t *testing.T) {
	// Runs sequentially due to shared global callback
	if os.Getenv(zqkenv.EnableHashRegistryCoordinatorTests()) == "" && os.Getenv("STORAGE_TEST_ENABLE_HASH_REGISTRY_COORDINATOR_TESTS") == "" {
		t.Skip("Skipping hash registry coordinator integration test; set ZQK_ENABLE_HASH_REGISTRY_COORDINATOR_TESTS=1 to enable")
	}
	testRoot, fileStorage, _ := setupTestingFactoryCompleteTestEnvironment(t)

	kindDir := datacell.CellCASPrimaryDir(testRoot, "backlog")
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	registry := NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", kindDir)
	registry.SetProjectRoot(testRoot)
	registry.SetStorage(fileStorage)

	var callbackCalls atomic.Int32
	callbackWaiter := newCallbackWaiter()
	SetHashRegistryEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		kind string,
		batchSize int,
		hashCount int,
		duration time.Duration,
		status string,
		err error,
	) {
		// Only count events for this test's registry
		if projectRoot != testRoot {
			return
		}
		callbackCalls.Add(1)
		if status == "complete" || status == "error" {
			callbackWaiter.Invoke()
		}
	})
	defer SetHashRegistryEventCallback(nil)

	// Save empty registry
	if err := registry.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	if !callbackWaiter.WaitForInvocation(pkgctx.NewSystemContext(), 5*time.Second) {
		t.Error("Expected callback for empty registry save within 5 seconds")
	}
}

// TestHashRegistry_CoordinatorIntegration_NilStorage tests with nil storage
func TestHashRegistry_CoordinatorIntegration_NilStorage(t *testing.T) {
	// Runs sequentially due to shared global callback
	if os.Getenv(zqkenv.EnableHashRegistryCoordinatorTests()) == "" && os.Getenv("STORAGE_TEST_ENABLE_HASH_REGISTRY_COORDINATOR_TESTS") == "" {
		t.Skip("Skipping hash registry coordinator integration test; set ZQK_ENABLE_HASH_REGISTRY_COORDINATOR_TESTS=1 to enable")
	}
	kindDir := t.TempDir()
	registry := NewHashRegistry(pkgctx.NewSystemContext(), "test_kind", kindDir)
	registry.SetProjectRoot(t.TempDir())
	// Don't set storage - callback should not be called

	var callbackCalls atomic.Int32
	SetHashRegistryEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storage ObjectStorageProvider,
		kind string,
		batchSize int,
		hashCount int,
		duration time.Duration,
		status string,
		err error,
	) {
		callbackCalls.Add(1)
	})
	defer SetHashRegistryEventCallback(nil)

	registry.SetHash("file1.yaml", "hash1")
	if err := registry.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Wait briefly to ensure callback would have been called if it were going to be
	// Since callback should NOT be called, we wait briefly and verify it wasn't
	time.Sleep(50 * time.Millisecond)

	// Callback should not be called without storage
	if callbackCalls.Load() != 0 {
		t.Error("Expected callback not to be called without storage")
	}
}
