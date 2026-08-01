package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestCASPostSyncCallback_CacheUpdate tests that the CAS post-sync callback
// correctly updates the Object ID Cache with the object ID and file path.
// This verifies the fix for cache building issues where CAS objects (like goals)
// were not being added to the cache.
func TestCASPostSyncCallback_CacheUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	goalsDir := filepath.Join(processDir, "goals")

	// Create process directory structure
	if err := os.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create process dir: %v", err)
	}

	// Create object_specs directory (required for validation)
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create specs directory: %v", err)
	}

	// Create storage
	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Register cache operation handler to track cache updates for goals only
	var goalCacheUpdateReceived bool
	var cachedObjectID, cachedKind, cachedFilePath string

	SetCacheOperationHandler(func(cacheCtx *pkgctx.CacheContext) error {
		// Only track updates for goal objects (filter out audit events, etc.)
		if cacheCtx.Operation == pkgctx.CacheOperationUpdate && cacheCtx.Kind == "goal" {
			goalCacheUpdateReceived = true
			cachedObjectID = cacheCtx.NewID
			cachedKind = cacheCtx.Kind
			cachedFilePath = cacheCtx.FilePath
		}
		// Just track the callback - don't actually update cache (avoid import cycle)
		return nil
	})

	// Create a goal object (uses CAS)
	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	objectID := "GOAL-001"
	obj := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Test Goal",
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Create object - this should trigger the CAS post-sync callback
	if err := storage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("failed to create object: %v", err)
	}

	// Verify callback was invoked for goal
	if !goalCacheUpdateReceived {
		t.Fatal("CAS post-sync callback was not invoked for goal object")
	}

	// Verify cache update had correct values
	if cachedObjectID != objectID {
		t.Errorf("cache update had wrong object ID: expected %s, got %s", objectID, cachedObjectID)
	}

	if cachedKind != "goal" {
		t.Errorf("cache update had wrong kind: expected 'goal', got %s", cachedKind)
	}

	// Verify file path is a hash-based filename in the goals directory
	if cachedFilePath == emptyValue {
		t.Fatal("cache update had empty file path")
	}

	if !filepath.IsAbs(cachedFilePath) {
		t.Errorf("cache update file path should be absolute, got: %s", cachedFilePath)
	}

	// Verify file path is in goals directory
	if !filepath.HasPrefix(cachedFilePath, goalsDir) {
		t.Errorf("cache update file path should be in goals directory, got: %s", cachedFilePath)
	}

	// Verify file path has hash-based filename (64-char hex + .yaml)
	fileName := filepath.Base(cachedFilePath)
	if len(fileName) != 69 { // 64 chars hash + 5 chars ".yaml"
		t.Errorf("cache update file path should have hash-based filename (64 chars + .yaml), got: %s", fileName)
	}

	// Verify file actually exists at the path
	if _, err := os.Stat(cachedFilePath); err != nil {
		t.Errorf("file does not exist at cached file path: %s, error: %v", cachedFilePath, err)
	}

	// Best-effort: flush any CAS index activity for goals before TempDir cleanup
	// runs to reduce the chance of background workers keeping files open.
	if err := FlushListingIndexForKind("goal"); err != nil {
		t.Logf("FlushListingIndexForKind(goal) failed during cleanup: %v", err)
	}
}
