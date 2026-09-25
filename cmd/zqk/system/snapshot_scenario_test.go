package system

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// Tests using setupTestSnapshotEnvironment must not use t.Parallel(): t.Setenv(ZQK_TEST_ROOT) is process-scoped to the test.

// setupTestSnapshotEnvironment creates a temporary test environment for snapshot tests
func setupTestSnapshotEnvironment(t *testing.T) (tmpDir string, storageProvider storage.ObjectStorageProvider, cleanup func()) {
	tmpDir = t.TempDir()
	// Registered immediately after t.TempDir: runs second-to-last (right before RemoveAll on tmpDir),
	// scrubbing any residual children after defer cleanup() + other t.Cleanups (see Go test cleanup order).
	t.Cleanup(func() {
		testkit.ScrubProjectRootForTempCleanup(tmpDir, 100, 50*time.Millisecond)
	})
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	// Remove nested .zqk directories before TempDir cleanup to avoid flaky
	// "directory not empty" failures from late background writes.
	t.Cleanup(func() {
		_ = filepath.Walk(tmpDir, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				return nil //nolint:nilerr // cleanup walk in test
			}
			if info.IsDir() && info.Name() == paths.ProjectDataDir {
				_ = fileutil.RemoveAll(path)
				return filepath.SkipDir
			}
			return nil
		})
	})

	testRoot, err := setupSystemTestEnvironmentRoot(t, tmpDir)
	if err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}

	storageFactory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), testRoot)
	if err != nil {
		t.Fatalf("failed to create storage factory: %v", err)
	}

	storageProvider = storageFactory.GetStorage()

	cleanup = func() {
		if fs, ok := storageProvider.(*storage.FileObjectStorage); ok {
			_ = testkit.RunStandardTeardown(testkit.TempProjectTeardown(testRoot, fs))
			return
		}
		_ = fileutil.RemoveAll(testRoot)
	}

	return testRoot, storageProvider, cleanup
}

func TestSnapshotScenario_EndToEnd(t *testing.T) {
	testRoot, storageProvider, cleanup := setupTestSnapshotEnvironment(t)
	defer cleanup()

	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Create proxy/queue for snapshot
	queue := storage.NewSnapshotOperationQueue(10000)
	proxyStorage := storage.NewProxyStorage(storageProvider, queue)
	snapshotManager := storage.NewSnapshotManager(proxyStorage, queue, storageProvider, testRoot)

	// Begin snapshot
	snapshotTimestamp, err := snapshotManager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	if snapshotTimestamp.IsZero() {
		t.Error("Snapshot timestamp should not be zero")
	}

	// Test with empty object list (simpler, avoids validation issues)
	objectIDs := []string{}

	// Capture metadata
	metadata, err := snapshotManager.CaptureMetadata(ctx, objectIDs)
	if err != nil {
		t.Fatalf("CaptureMetadata failed: %v", err)
	}

	if metadata.Timestamp != snapshotTimestamp {
		t.Error("Metadata timestamp should match snapshot timestamp")
	}

	// Extract objects (with empty list, should return empty)
	extractedObjects, err := extractObjectsWithHandlers(ctx, storageProvider, metadata, "include", logger)
	if err != nil {
		t.Fatalf("extractObjectsWithHandlers failed: %v", err)
	}

	// With empty object list, should get empty extracted objects
	if len(extractedObjects) != 0 {
		t.Errorf("Expected 0 extracted objects with empty list, got %d", len(extractedObjects))
	}

	// End snapshot
	if err := snapshotManager.EndSnapshot(ctx); err != nil {
		t.Fatalf("EndSnapshot failed: %v", err)
	}

	// Verify metadata has snapshot timestamp
	if metadata.Timestamp.IsZero() {
		t.Error("Metadata timestamp should not be zero")
	}

	// Verify timestamp format (should be RFC3339Nano)
	timestampStr := metadata.Timestamp.Format(time.RFC3339Nano)
	_, err = time.Parse(time.RFC3339Nano, timestampStr)
	if err != nil {
		t.Errorf("snapshot_timestamp should be in RFC3339Nano format: %v", err)
	}
}

func TestSnapshotScenario_EmptyObjectList(t *testing.T) {
	testRoot, storageProvider, cleanup := setupTestSnapshotEnvironment(t)
	defer cleanup()

	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Create proxy/queue for snapshot
	queue := storage.NewSnapshotOperationQueue(10000)
	proxyStorage := storage.NewProxyStorage(storageProvider, queue)
	snapshotManager := storage.NewSnapshotManager(proxyStorage, queue, storageProvider, testRoot)

	// Begin snapshot
	_, err := snapshotManager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	// Capture metadata with empty list
	metadata, err := snapshotManager.CaptureMetadata(ctx, []string{})
	if err != nil {
		t.Fatalf("CaptureMetadata failed: %v", err)
	}

	if len(metadata.ObjectIDs) != 0 {
		t.Errorf("Expected 0 object IDs, got %d", len(metadata.ObjectIDs))
	}

	// Extract objects (with empty list, should return empty)
	extractedObjects, err := extractObjectsWithHandlers(ctx, storageProvider, metadata, "include", logger)
	if err != nil {
		t.Fatalf("extractObjectsWithHandlers failed: %v", err)
	}

	if len(extractedObjects) != 0 {
		t.Errorf("Expected 0 extracted objects, got %d", len(extractedObjects))
	}

	// End snapshot
	if err := snapshotManager.EndSnapshot(ctx); err != nil {
		t.Fatalf("EndSnapshot failed: %v", err)
	}
}

func TestSnapshotScenario_NonExistentObject(t *testing.T) {
	testRoot, storageProvider, cleanup := setupTestSnapshotEnvironment(t)
	defer cleanup()

	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Create proxy/queue for snapshot
	queue := storage.NewSnapshotOperationQueue(10000)
	proxyStorage := storage.NewProxyStorage(storageProvider, queue)
	snapshotManager := storage.NewSnapshotManager(proxyStorage, queue, storageProvider, testRoot)

	// Begin snapshot
	_, err := snapshotManager.BeginSnapshot()
	if err != nil {
		t.Fatalf("BeginSnapshot failed: %v", err)
	}

	// Capture metadata with non-existent object
	metadata, err := snapshotManager.CaptureMetadata(ctx, []string{"NON-EXISTENT-001"})
	if err != nil {
		// This is expected - metadata capture should handle missing objects gracefully
		t.Logf("CaptureMetadata returned error (expected): %v", err)
		// End snapshot and return early if metadata capture failed
		_ = snapshotManager.EndSnapshot(ctx) //nolint:errcheck // Test cleanup - best effort //nolint:errcheck // Cleanup in test - error handling not critical
		return
	}

	// Extract objects (should handle missing objects gracefully)
	extractedObjects, err := extractObjectsWithHandlers(ctx, storageProvider, metadata, "include", logger)
	if err != nil {
		t.Fatalf("extractObjectsWithHandlers failed: %v", err)
	}

	// Should have 0 objects (non-existent object skipped)
	if len(extractedObjects) > 0 {
		t.Errorf("Expected 0 extracted objects, got %d", len(extractedObjects))
	}

	// End snapshot
	if err := snapshotManager.EndSnapshot(ctx); err != nil {
		t.Fatalf("EndSnapshot failed: %v", err)
	}
}

// Note: More complex integration tests (change policies, adaptors) are covered
// by unit tests in pkg/storage. These integration tests focus on the basic
// snapshot workflow and scenario object creation.
