package cas_test

import (
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"context"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
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
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create process dir: %v", err)
	}

	// Create object_specs directory (required for validation)
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create specs directory: %v", err)
	}

	// Create storage
	storageProv, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	defer func() { _ = storageProv.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, storageProv)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Register cache operation handler to track cache updates for goals only
	var goalCacheUpdateReceived bool
	var cachedObjectID, cachedKind, cachedFilePath string

	storage.SetCacheOperationHandler(func(cacheCtx *pkgctx.CacheContext) error {
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

	// Create a goal object (uses CAS). Draft-first create parks preliminary origin on draft;
	// promote to active so CAS PostSyncCallback runs with the durable hash path.
	// TRACK: BLI-REDACTED — draft-plane create / promote membrane.
	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()

	objectID := "GOAL-001"
	obj := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Test Goal",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, storageProv, ctx, secCtx, obj, objects.ObjectStatusActive)

	// Verify callback was invoked for goal (on create and/or promote sync)
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
	if cachedFilePath == "" {
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
	if _, err := fileutil.Stat(cachedFilePath); err != nil {
		t.Errorf("file does not exist at cached file path: %s, error: %v", cachedFilePath, err)
	}

	// Best-effort: flush any CAS index activity for goals before TempDir cleanup
	// runs to reduce the chance of background workers keeping files open.
	if err := storage.FlushListingIndexForKind("goal"); err != nil {
		t.Logf("storage.FlushListingIndexForKind(goal) failed during cleanup: %v", err)
	}
}

// TestCASPostSyncCallback_UpdateRefreshesHashPath verifies Update fires post-sync
// with the new hash path so object-id-cache cannot keep a deleted blob name.
// TRACK: BLI-REDACTED
func TestCASPostSyncCallback_UpdateRefreshesHashPath(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(filepath.Join(processDir, "_internal", "object_specs"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(filepath.Join(processDir, "goals"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	store, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = store.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, store))
	})

	var lastPath string
	var updateCalls int
	storage.SetCacheOperationHandler(func(cacheCtx *pkgctx.CacheContext) error {
		if cacheCtx.Operation == pkgctx.CacheOperationUpdate && cacheCtx.Kind == "goal" {
			updateCalls++
			lastPath = cacheCtx.FilePath
		}
		return nil
	})
	t.Cleanup(func() { storage.SetCacheOperationHandler(nil) })

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()
	objectID := "GOAL-UPDATE-CACHE-001"
	obj := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Cache update path",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, obj, objects.ObjectStatusActive)
	pathAfterCreate := lastPath
	if pathAfterCreate == "" {
		t.Fatal("expected post-sync path after create")
	}
	callsAfterCreate := updateCalls

	if err := store.Update(ctx, secCtx, objectID, map[string]any{
		objects.FieldKeyTitle: "Cache update path v2",
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	_ = storage.FlushListingIndexForKind("goal")

	if updateCalls <= callsAfterCreate {
		t.Fatalf("expected post-sync on update; calls create=%d total=%d", callsAfterCreate, updateCalls)
	}
	if lastPath == "" || lastPath == pathAfterCreate {
		t.Fatalf("expected new hash path after update; create=%q update=%q", pathAfterCreate, lastPath)
	}
	if _, err := fileutil.Stat(lastPath); err != nil {
		t.Fatalf("live path missing: %v", err)
	}
	// Old blob cleanup is best-effort/async; durable contract is post-sync points at newHash.
}

func TestCASCreateFailsWhenPostSyncErrors(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(filepath.Join(processDir, "_internal", "object_specs"), paths.DirPerm755); err != nil {
		t.Fatalf("specs dir: %v", err)
	}
	if err := fileutil.MkdirAll(filepath.Join(processDir, "goals"), paths.DirPerm755); err != nil {
		t.Fatalf("goals dir: %v", err)
	}
	store, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Shutdown(context.Background()) }()

	prev := storage.GetCacheOperationHandler()
	t.Cleanup(func() { storage.SetCacheOperationHandler(prev) })
	storage.SetCacheOperationHandler(func(cacheCtx *pkgctx.CacheContext) error {
		if cacheCtx.Kind == "goal" {
			return errfmt.Errorf("injected post-sync failure")
		}
		return nil
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()
	obj := map[string]any{
		objects.FieldKeyID:            "GOAL-POSTSYNC-FAIL-001",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "PostSync fail",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := store.Create(ctx, secCtx, obj); err == nil {
		t.Fatal("Create must fail when identity cache post-sync errors")
	}
}
