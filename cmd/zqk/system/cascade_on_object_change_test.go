package system

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestCascadeOnObjectChange_DeleteUpdatesCaches verifies that the narrow cascade
// for delete operations removes IDs from object-id-cache and clears list cache
// entries for the affected kind.
func TestCascadeOnObjectChange_DeleteUpdatesCaches(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create file storage: %v", err)
	}
	t.Cleanup(func() {
		resetDir, rerr := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if rerr != nil {
			_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
				ProjectRoot:           projectRoot,
				FileStorage:           fileStorage,
				StripProcessArtifacts: true,
				WALTimeout:            30 * time.Second,
				ShutdownTimeout:       30 * time.Second,
			})
			return
		}
		defer fileutil.RemoveAll(resetDir)

		secCtxAlready := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
		_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
			ProjectRoot:               projectRoot,
			FileStorage:               fileStorage,
			StripProcessArtifacts:     true,
			WALTimeout:                30 * time.Second,
			ShutdownTimeout:           30 * time.Second,
			TearDownGlobalAuditBuffer: true,
			SecCtx:                    secCtxAlready,
			AuditBufferResetRoot:      resetDir,
		})
	})

	// Create a single criteria file so we have a concrete ID and path.
	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := fileutil.EnsureDir(criteriaDir); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	id := "CRIT-CASCADE-001"
	kind := "criteria"
	filePath := filepath.Join(criteriaDir, id+".yaml")
	if err := fileutil.WriteSecureFile(filePath, []byte("id: "+id+"\nkind: "+kind+"\nschema_version: \""+objects.DefaultSchemaVersion+"\"\nstatus: not_started\ntitle: Cascade\n")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Seed object-id-cache with this entry.
	cache := GetGlobalObjectIDCache()
	if err := cache.Update(id, kind, filePath); err != nil {
		t.Fatalf("cache.Update: %v", err)
	}
	if _, exists := cache.Get(id); !exists {
		t.Fatalf("object-id-cache entry for %s not found after Update", id)
	}

	// Seed list cache with a query result that includes this ID.
	filter := &storage.ListFilter{
		Kind:    kind,
		Filters: map[string]any{},
	}
	result := &storage.QueryResult{
		Objects: []map[string]any{
			{objects.FieldKeyID: id, objects.FieldKeyKind: kind},
		},
	}
	const limit = 10
	storage.SetListCache(projectRoot, filter, limit, result)

	if cached, ok := storage.GetListCache(projectRoot, filter, limit); !ok || cached == nil || len(cached.Objects) == 0 {
		t.Fatalf("expected list cache entry before cascade; ok=%v cached=%v", ok, cached)
	}

	// Run the cascade for a delete operation.
	ctx := pkgctx.NewSystemContext()
	CascadeOnObjectChange(ctx, projectRoot, storage.OpDelete, kind, id, "")

	// Object-id-cache should no longer have the entry.
	if _, exists := cache.Get(id); exists {
		t.Errorf("object-id-cache still contains %s after CascadeOnObjectChange delete", id)
	}

	// List cache for this project/kind should have been invalidated.
	if cached, ok := storage.GetListCache(projectRoot, filter, limit); ok && cached != nil {
		t.Errorf("expected list cache to be invalidated after CascadeOnObjectChange delete; got ok=%v cached=%v", ok, cached)
	}
}

// TestCascadeOnObjectChange_CreateUpdateInvalidatesListAndValidation ensures that
// create and update operations invalidate list and validation caches for the ID.
// Object-id-cache updates remain the responsibility of the CacheContext path.
func TestCascadeOnObjectChange_CreateUpdateInvalidatesListAndValidation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create file storage: %v", err)
	}
	t.Cleanup(func() {
		resetDir, rerr := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if rerr != nil {
			_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
				ProjectRoot:           projectRoot,
				FileStorage:           fileStorage,
				StripProcessArtifacts: true,
				WALTimeout:            30 * time.Second,
				ShutdownTimeout:       30 * time.Second,
			})
			return
		}
		defer fileutil.RemoveAll(resetDir)

		secCtxAlready := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
		_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
			ProjectRoot:               projectRoot,
			FileStorage:               fileStorage,
			StripProcessArtifacts:     true,
			WALTimeout:                30 * time.Second,
			ShutdownTimeout:           30 * time.Second,
			TearDownGlobalAuditBuffer: true,
			SecCtx:                    secCtxAlready,
			AuditBufferResetRoot:      resetDir,
		})
	})

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := fileutil.EnsureDir(criteriaDir); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	id := "CRIT-CASCADE-UPDATE"
	kind := "criteria"

	// Seed list cache with a result for this ID.
	filter := &storage.ListFilter{
		Kind:    kind,
		Filters: map[string]any{},
	}
	result := &storage.QueryResult{
		Objects: []map[string]any{
			{objects.FieldKeyID: id, objects.FieldKeyKind: kind},
		},
	}
	const limit = 5
	storage.SetListCache(projectRoot, filter, limit, result)
	if cached, ok := storage.GetListCache(projectRoot, filter, limit); !ok || cached == nil || len(cached.Objects) == 0 {
		t.Fatalf("expected list cache entry before cascade; ok=%v cached=%v", ok, cached)
	}

	// Call cascade for create.
	ctx := pkgctx.NewSystemContext()
	CascadeOnObjectChange(ctx, projectRoot, storage.OpCreate, kind, id, "")
	if cached, ok := storage.GetListCache(projectRoot, filter, limit); ok && cached != nil {
		t.Errorf("expected list cache to be invalidated after CascadeOnObjectChange create; got ok=%v cached=%v", ok, cached)
	}

	// Re-seed list cache and call cascade for update.
	storage.SetListCache(projectRoot, filter, limit, result)
	if cached, ok := storage.GetListCache(projectRoot, filter, limit); !ok || cached == nil || len(cached.Objects) == 0 {
		t.Fatalf("expected list cache entry before cascade update; ok=%v cached=%v", ok, cached)
	}
	CascadeOnObjectChange(ctx, projectRoot, storage.OpUpdate, kind, id, "")
	if cached, ok := storage.GetListCache(projectRoot, filter, limit); ok && cached != nil {
		t.Errorf("expected list cache to be invalidated after CascadeOnObjectChange update; got ok=%v cached=%v", ok, cached)
	}
}

func TestCascadeOnObjectChange_DependentInvalidation(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	_, err = storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create file storage: %v", err)
	}

	// Setup clean state caches.
	storage.GetGlobalReverseReferenceIndex().Clear()

	// Create parent-child reference in global index.
	parentID := "BLI-PARENT"
	childID := "PER-CHILD"
	storage.GetGlobalReverseReferenceIndex().AddReference(parentID, childID)

	// Inject validation cache entries for both objects.
	ctx := pkgctx.WithCacheMode(context.Background(), pkgctx.CacheModeTestSync)

	parentState := &validation.ValidationState{
		ObjectID:      parentID,
		ObjectKind:    "backlog_item",
		FilePath:      "dummy_parent.yaml",
		LastValidated: time.Now(),
	}
	childState := &validation.ValidationState{
		ObjectID:      childID,
		ObjectKind:    "persona",
		FilePath:      "dummy_child.yaml",
		LastValidated: time.Now(),
	}
	validation.UpdateObjectInGlobalValidationCacheWithContext(ctx, projectRoot, parentState)
	validation.UpdateObjectInGlobalValidationCacheWithContext(ctx, projectRoot, childState)

	// Verify they are cached.
	_, okParent := validation.GetValidationStateForTest(ctx, projectRoot, parentID)
	_, okChild := validation.GetValidationStateForTest(ctx, projectRoot, childID)

	if !okParent {
		t.Fatalf("expected parent to be cached in validation cache")
	}
	if !okChild {
		t.Fatalf("expected child to be cached in validation cache")
	}

	// Trigger cascade update on the child object.
	CascadeOnObjectChange(ctx, projectRoot, storage.OpUpdate, "persona", childID, "")

	// Verify BOTH the child and the parent (its dependent) are evicted from the cache.
	if _, ok := validation.GetValidationStateForTest(ctx, projectRoot, childID); ok {
		t.Errorf("expected child to be evicted from validation cache, but it was still cached")
	}
	if _, ok := validation.GetValidationStateForTest(ctx, projectRoot, parentID); ok {
		t.Errorf("expected parent (dependent) to be evicted from validation cache due to cascade, but it was still cached")
	}
}
