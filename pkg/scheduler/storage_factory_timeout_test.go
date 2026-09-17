package scheduler

// Tests using setupSchedulerCompleteTestEnvironment must not use t.Parallel(): ZQK_TEST_ROOT is process-global.
// BLI-177483 inventory: GetTestCleanup → RunProjectTestTeardown (scheduler_test_layout_helpers_test.go).

import (
	"context"
	"testing"
	"time"

	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

// TestStorageFactory_Create_DoesNotHang verifies that NewStorageFactory doesn't hang
// This is called by scheduler submit before creating the job
func TestStorageFactory_Create_DoesNotHang(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	t.Cleanup(env.Cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})
	testRoot := env.TestRoot

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	// Test NewStorageFactory - this is called early in scheduler submit
	factoryStart := time.Now()
	done := make(chan error, 1)
	var factory *storagepkg.StorageFactory

	goroutinelabels.StartTestGoroutine("test_storage_factory", "creating storage factory in test", func() {
		var factoryErr error
		factory, factoryErr = storagepkg.NewStorageFactory(ctx, testRoot)
		done <- factoryErr
	})

	select {
	case err := <-done:
		factoryDuration := time.Since(factoryStart)
		if err != nil {
			t.Fatalf("failed to create storage factory: %v (took %v)", err, factoryDuration)
		}
		if factoryDuration > 2*time.Second {
			t.Errorf("storage factory creation took too long: %v (expected < 2s)", factoryDuration)
		}
		if factory == nil {
			t.Fatal("storage factory should not be nil")
		}
		t.Logf("Storage factory created successfully in %v", factoryDuration)
	case <-time.After(5 * time.Second):
		t.Fatal("NewStorageFactory() HUNG - this is likely the bug! Operation did not complete within 5 seconds")
	case <-ctx.Done():
		t.Fatal("context cancelled - NewStorageFactory() may be waiting on something that never completes")
	}
}

// TestStorageFactory_GetStorage_DoesNotHang verifies GetStorage() doesn't hang
func TestStorageFactory_GetStorage_DoesNotHang(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	t.Cleanup(env.Cleanup)
	t.Cleanup(func() {
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("scheduler_job", 2*time.Second) //nolint:errcheck // best-effort test cleanup
		_ = caspkg.GetGlobalListingIndexWriteQueue().FlushKind("audit_event", 2*time.Second)   //nolint:errcheck // best-effort test cleanup
	})
	testRoot := env.TestRoot

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	factory, err := storagepkg.NewStorageFactory(ctx, testRoot)
	if err != nil {
		t.Fatalf("failed to create storage factory: %v", err)
	}

	// Test GetStorage() - called right after NewStorageFactory in scheduler submit
	getStorageStart := time.Now()
	done := make(chan bool, 1)
	var storage storagepkg.ObjectStorageProvider

	goroutinelabels.StartTestGoroutine("test_get_storage", "getting storage from factory in test", func() {
		storage = factory.GetStorage()
		done <- true
	})

	select {
	case <-done:
		getStorageDuration := time.Since(getStorageStart)
		if storage == nil {
			t.Fatal("storage should not be nil")
		}
		if getStorageDuration > 1*time.Second {
			t.Errorf("GetStorage() took too long: %v (expected < 1s)", getStorageDuration)
		}
		t.Logf("GetStorage() completed successfully in %v", getStorageDuration)
	case <-time.After(5 * time.Second):
		t.Fatal("GetStorage() HUNG - operation did not complete within 5 seconds")
	case <-ctx.Done():
		t.Fatal("context cancelled - GetStorage() may be waiting on something")
	}
}
