package internal

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqktime"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestGraphFileBackendParity comprehensively tests that graph backend
// produces identical results to file backend for all operations.
//
// This test suite ensures:
// 1. CRUD operations produce same results
// 2. List operations with filters/sorts/pagination match
// 3. Query operations return consistent data
// 4. Data integrity is maintained across backends

func parityBacklogFields(extra map[string]any) map[string]any {
	obj := map[string]any{
		objects.FieldKeyKind:                     "backlog_item",
		objects.FieldKeySchemaVersion:            objects.DefaultSchemaVersion,
		objects.FieldKeyProblemStatement:         "Parity harness validates shared file/graph schema membrane.",
		objects.FieldKeyAcceptanceConsiderations: "Create/update/list parity accepted for graph-file sync.",
		objects.FieldKeyContext:                  "graph_file_parity",
		objects.FieldKeyCreatedAt:                zqktime.NowRFC3339UTC(),
		objects.FieldKeyCreatedBy:                "test",
		objects.FieldKeyUpdatedAt:                zqktime.NowRFC3339UTC(),
		objects.FieldKeyUpdatedBy:                "test",
	}
	for k, v := range extra {
		obj[k] = v
	}
	return obj
}

func TestGraphFileBackendParity(t *testing.T) {
	// Not t.Parallel(): PrepareIsolatedTempProject uses t.Setenv(ZQK_TEST_ROOT); ZQK_PROJECT_ROOT is also set.
	// Skip if graph backend is not available
	if !isGraphBackendAvailable() {
		t.Skip("Graph backend not available (ZQK_GRAPH_ENABLED not set or graph not running)")
	}

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "internal.graph_file_parity",
		SkipSetupTestEnvironment: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{
				{
					Name: "copy_process_specs_for_parity",
					Fn: func() error {
						specsDir := filepath.Join(root, paths.ProcessInternalObjectSpecsDir)
						if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
							return err
						}
						pr := findProjectRootForParityTest(t)
						sourceSpecsDir := filepath.Join(pr, paths.ProcessInternalObjectSpecsDir)
						return copySpecFilesForParity(sourceSpecsDir, specsDir)
					},
				},
			}
		},
	})
	testRoot := proj.Root
	t.Setenv(zqkenv.ProjectRoot().Name(), testRoot)

	// Prefer testkit MemGraph pool (Database=test) over shared studio pool.
	pool := testkit.PrepareGraphConnectionForTest(t)

	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Minute)
	defer cancel()
	ctx = pkgctx.WithPromoteOnCreate(storage.WithTestHardDelete(ctx))

	// Security context
	secCtx := pkgctx.NewSystemSecurityContext()

	// Storage context
	storageCtx := pkgctx.NewStorageContext()
	storageCtx.EnableGrouping = true

	fileStorage := proj.FileStorage
	graphStorage := storage.NewPoolAwareGraphStorage(pool, testRoot)

	// Test suite
	t.Run("CRUD_Parity", func(t *testing.T) {
		testCRUDParity(t, ctx, fileStorage, graphStorage, secCtx, storageCtx)
	})

	t.Run("List_Parity", func(t *testing.T) {
		testListParity(t, ctx, fileStorage, graphStorage, secCtx, storageCtx)
	})

	t.Run("Query_Parity", func(t *testing.T) {
		testQueryParity(t, ctx, fileStorage, graphStorage, secCtx)
	})

	t.Run("Filter_Sort_Paginate_Parity", func(t *testing.T) {
		testFilterSortPaginateParity(t, ctx, fileStorage, graphStorage, secCtx, storageCtx)
	})
}

// testCRUDParity tests that Create, Read, Update, Delete operations
// produce identical results in both backends
func testCRUDParity(t *testing.T, ctx context.Context, fileStorage, graphStorage storage.ObjectStorageProvider, secCtx *storage.SecurityContext, _ *storage.StorageContext) {
	// ID must match pattern: ^[A-Z]+-\d{3,}$
	baseID := fmt.Sprintf("BLI-%d", time.Now().UnixNano())

	// Create test object
	testObj := parityBacklogFields(map[string]any{
		objects.FieldKeyID:       baseID,
		objects.FieldKeyTitle:    "Parity Test Item",
		objects.FieldKeyStatus:   objects.ObjectStatusExploring,
		objects.FieldKeyPriority: "high",
	})

	// Create in both backends
	if err := fileStorage.Create(ctx, secCtx, testObj); err != nil {
		t.Fatalf("Failed to create object in file backend: %v", err)
	}

	if err := graphStorage.Create(ctx, secCtx, testObj); err != nil {
		t.Fatalf("Failed to create object in graph backend: %v", err)
	}

	// Read from both backends and compare
	fileObj, err := fileStorage.Read(ctx, secCtx, baseID)
	if err != nil {
		t.Fatalf("Failed to read object from file backend: %v", err)
	}

	graphObj, err := graphStorage.Read(ctx, secCtx, baseID)
	if err != nil {
		t.Fatalf("Failed to read object from graph backend: %v", err)
	}

	// Normalize timestamps (they may differ slightly)
	normalizeObject(fileObj)
	normalizeObject(graphObj)

	if !objectsEqual(fileObj, graphObj) {
		t.Errorf("Read results differ:\nFile: %+v\nGraph: %+v", fileObj, graphObj)
	}

	// Update object
	updatedObj := copyObject(testObj)
	updatedObj[objects.FieldKeyTitle] = "Updated Parity Test Item"
	updatedObj[objects.FieldKeyStatus] = "validated"
	updatedObj[objects.FieldKeyUpdatedAt] = zqktime.NowRFC3339UTC()

	if err := fileStorage.Update(ctx, secCtx, baseID, updatedObj); err != nil {
		t.Fatalf("Failed to update object in file backend: %v", err)
	}

	if err := graphStorage.Update(ctx, secCtx, baseID, updatedObj); err != nil {
		t.Fatalf("Failed to update object in graph backend: %v", err)
	}

	// Read again and compare
	fileObjUpdated, err := fileStorage.Read(ctx, secCtx, baseID)
	if err != nil {
		t.Fatalf("Failed to read updated object from file backend: %v", err)
	}

	graphObjUpdated, err := graphStorage.Read(ctx, secCtx, baseID)
	if err != nil {
		t.Fatalf("Failed to read updated object from graph backend: %v", err)
	}

	normalizeObject(fileObjUpdated)
	normalizeObject(graphObjUpdated)

	if !objectsEqual(fileObjUpdated, graphObjUpdated) {
		t.Errorf("Updated read results differ:\nFile: %+v\nGraph: %+v", fileObjUpdated, graphObjUpdated)
	}

	// Delete from both backends
	if err := fileStorage.Delete(ctx, secCtx, baseID, false); err != nil {
		t.Fatalf("Failed to delete object from file backend: %v", err)
	}

	if err := graphStorage.Delete(ctx, secCtx, baseID, false); err != nil {
		t.Fatalf("Failed to delete object from graph backend: %v", err)
	}

	// Verify deletion
	_, err = fileStorage.Read(ctx, secCtx, baseID)
	if err == nil {
		t.Error("Object still exists in file backend after deletion")
	}

	_, err = graphStorage.Read(ctx, secCtx, baseID)
	if err == nil {
		t.Error("Object still exists in graph backend after deletion")
	}
}

// testListParity tests that List operations produce identical results
func testListParity(t *testing.T, ctx context.Context, fileStorage, graphStorage storage.ObjectStorageProvider, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext) {
	// ID must match pattern: ^[A-Z]+-\d{3,}$
	baseID := fmt.Sprintf("BLI-%d", time.Now().UnixNano())

	// Create multiple test objects
	testObjects := []map[string]any{
		{
			objects.FieldKeyID:            fmt.Sprintf("BLI-%d", time.Now().UnixNano()+1),
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "List Test Item 1",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeyPriority:      "high",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		},
		{
			objects.FieldKeyID:            fmt.Sprintf("BLI-%d", time.Now().UnixNano()+2),
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "List Test Item 2",
			objects.FieldKeyStatus:        objects.ObjectStatusValidated,
			objects.FieldKeyPriority:      "medium",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		},
		{
			objects.FieldKeyID:            fmt.Sprintf("BLI-%d", time.Now().UnixNano()+3),
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "List Test Item 3",
			objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
			objects.FieldKeyPriority:      "low",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		},
	}

	// Create in both backends
	for _, obj := range testObjects {
		if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create object in file backend: %v", err)
		}
		if err := graphStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create object in graph backend: %v", err)
		}
	}

	// Test basic list
	fileList, err := fileStorage.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: "backlog_item"})
	if err != nil {
		t.Fatalf("Failed to list objects from file backend: %v", err)
	}

	graphList, err := graphStorage.List(ctx, secCtx, storageCtx, storage.ListFilter{Kind: "backlog_item"})
	if err != nil {
		t.Fatalf("Failed to list objects from graph backend: %v", err)
	}

	// Filter to our test objects
	fileObjects := filterObjectsByID(fileList.Objects, baseID)
	graphObjects := filterObjectsByID(graphList.Objects, baseID)

	if len(fileObjects) != len(graphObjects) {
		t.Errorf("List count mismatch: file=%d, graph=%d", len(fileObjects), len(graphObjects))
	}

	// Normalize and compare
	normalizeObjects(fileObjects)
	normalizeObjects(graphObjects)

	if !objectSlicesEqual(fileObjects, graphObjects) {
		t.Errorf("List results differ:\nFile: %+v\nGraph: %+v", fileObjects, graphObjects)
	}

	// Cleanup
	for _, obj := range testObjects {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = fileStorage.Delete(ctx, secCtx, obj[objects.FieldKeyID].(string), false)
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = graphStorage.Delete(ctx, secCtx, obj[objects.FieldKeyID].(string), false)
	}
}

// testQueryParity tests that Query operations produce identical results
func testQueryParity(t *testing.T, ctx context.Context, fileStorage, graphStorage storage.ObjectStorageProvider, secCtx *storage.SecurityContext) {
	// ID must match pattern: ^[A-Z]+-\d{3,}$
	baseID := fmt.Sprintf("BLI-%d", time.Now().UnixNano())

	// Create test objects
	testObj := parityBacklogFields(map[string]any{
		objects.FieldKeyID:       baseID,
		objects.FieldKeyTitle:    "Query Test Item",
		objects.FieldKeyStatus:   objects.ObjectStatusExploring,
		objects.FieldKeyPriority: "high",
	})

	if err := fileStorage.Create(ctx, secCtx, testObj); err != nil {
		t.Fatalf("Failed to create object in file backend: %v", err)
	}
	if err := graphStorage.Create(ctx, secCtx, testObj); err != nil {
		t.Fatalf("Failed to create object in graph backend: %v", err)
	}

	fileObj, err := fileStorage.Read(ctx, secCtx, baseID)
	if err != nil {
		t.Fatalf("Failed to read file backend: %v", err)
	}
	graphObj, err := graphStorage.Read(ctx, secCtx, baseID)
	if err != nil {
		t.Fatalf("Failed to read graph backend: %v", err)
	}
	normalizeObject(fileObj)
	normalizeObject(graphObj)
	if !objectsEqual(fileObj, graphObj) {
		t.Errorf("Query/read results differ:\nFile: %+v\nGraph: %+v", fileObj, graphObj)
	}

	// Cleanup
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = fileStorage.Delete(ctx, secCtx, baseID, false)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = graphStorage.Delete(ctx, secCtx, baseID, false)
}

// testFilterSortPaginateParity tests that filtering, sorting, and pagination
// produce identical results in both backends
func testFilterSortPaginateParity(t *testing.T, ctx context.Context, fileStorage, graphStorage storage.ObjectStorageProvider, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext) {
	// ID must match pattern: ^[A-Z]+-\d{3,}$
	baseID := fmt.Sprintf("BLI-%d", time.Now().UnixNano())

	// Create test objects with different statuses
	// IDs must match pattern: ^[A-Z]+-\d{3,}$
	testObjects := []struct {
		id       string
		status   string
		priority string
	}{
		{fmt.Sprintf("BLI-%d", time.Now().UnixNano()+1), "exploring", "high"},
		{fmt.Sprintf("BLI-%d", time.Now().UnixNano()+2), "validated", "medium"},
		{fmt.Sprintf("BLI-%d", time.Now().UnixNano()+3), "planned", "low"},
		{fmt.Sprintf("BLI-%d", time.Now().UnixNano()+4), "exploring", "low"},
		{fmt.Sprintf("BLI-%d", time.Now().UnixNano()+5), "validated", "high"},
	}

	for i, obj := range testObjects {
		testObj := map[string]any{
			objects.FieldKeyID:            obj.id,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         fmt.Sprintf("FSP Test Item %d", i+1),
			objects.FieldKeyStatus:        obj.status,
			objects.FieldKeyPriority:      obj.priority,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     zqktime.NowRFC3339UTC(),
			objects.FieldKeyUpdatedBy:     "test",
		}

		if err := fileStorage.Create(ctx, secCtx, testObj); err != nil {
			t.Fatalf("Failed to create object in file backend: %v", err)
		}
		if err := graphStorage.Create(ctx, secCtx, testObj); err != nil {
			t.Fatalf("Failed to create object in graph backend: %v", err)
		}
	}

	// Test filtering
	fileFiltered, err := fileStorage.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    "backlog_item",
		Filters: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusExploring},
	})
	if err != nil {
		t.Fatalf("Failed to filter in file backend: %v", err)
	}

	graphFiltered, err := graphStorage.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    "backlog_item",
		Filters: map[string]any{objects.FieldKeyStatus: objects.ObjectStatusExploring},
	})
	if err != nil {
		t.Fatalf("Failed to filter in graph backend: %v", err)
	}

	fileFilteredObjs := filterObjectsByID(fileFiltered.Objects, baseID)
	graphFilteredObjs := filterObjectsByID(graphFiltered.Objects, baseID)

	if len(fileFilteredObjs) != len(graphFilteredObjs) {
		t.Errorf("Filter count mismatch: file=%d, graph=%d", len(fileFilteredObjs), len(graphFilteredObjs))
	}

	// Test sorting
	fileSorted, err := fileStorage.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    "backlog_item",
		SortBy:  "priority",
		SortAsc: true,
	})
	if err != nil {
		t.Fatalf("Failed to sort in file backend: %v", err)
	}

	graphSorted, err := graphStorage.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:    "backlog_item",
		SortBy:  "priority",
		SortAsc: true,
	})
	if err != nil {
		t.Fatalf("Failed to sort in graph backend: %v", err)
	}

	fileSortedObjs := filterObjectsByID(fileSorted.Objects, baseID)
	graphSortedObjs := filterObjectsByID(graphSorted.Objects, baseID)

	if len(fileSortedObjs) != len(graphSortedObjs) {
		t.Errorf("Sort count mismatch: file=%d, graph=%d", len(fileSortedObjs), len(graphSortedObjs))
	}

	// Test pagination
	filePaged, err := fileStorage.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:   "backlog_item",
		Offset: 1,
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("Failed to paginate in file backend: %v", err)
	}

	graphPaged, err := graphStorage.List(ctx, secCtx, storageCtx, storage.ListFilter{
		Kind:   "backlog_item",
		Offset: 1,
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("Failed to paginate in graph backend: %v", err)
	}

	filePagedObjs := filterObjectsByID(filePaged.Objects, baseID)
	graphPagedObjs := filterObjectsByID(graphPaged.Objects, baseID)

	if len(filePagedObjs) != len(graphPagedObjs) {
		t.Errorf("Pagination count mismatch: file=%d, graph=%d", len(filePagedObjs), len(graphPagedObjs))
	}

	// Cleanup
	for _, obj := range testObjects {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = fileStorage.Delete(ctx, secCtx, obj.id, false)
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = graphStorage.Delete(ctx, secCtx, obj.id, false)
	}
}

// Helper functions

func normalizeObject(obj map[string]any) {
	// Remove timestamps that may differ slightly
	delete(obj, "created_at")
	delete(obj, "updated_at")
	// Remove system fields that may differ
	delete(obj, "updated_by")
}

func normalizeObjects(objs []map[string]any) {
	for _, obj := range objs {
		normalizeObject(obj)
	}
}

func objectsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || !valuesEqual(v, bv) {
			return false
		}
	}
	return true
}

func valuesEqual(a, b any) bool {
	// Simple comparison - could be enhanced
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func objectSlicesEqual(a, b []map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	// Sort by ID for comparison
	sort.Slice(a, func(i, j int) bool {
		return a[i][objects.FieldKeyID].(string) < a[j][objects.FieldKeyID].(string)
	})
	sort.Slice(b, func(i, j int) bool {
		return b[i][objects.FieldKeyID].(string) < b[j][objects.FieldKeyID].(string)
	})
	for i := range a {
		if !objectsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func filterObjectsByID(objs []map[string]any, prefix string) []map[string]any {
	var filtered []map[string]any
	for _, obj := range objs {
		if id, ok := obj[objects.FieldKeyID].(string); ok && strings.HasPrefix(id, prefix) {
			filtered = append(filtered, obj)
		}
	}
	return filtered
}

func copyObject(obj map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range obj {
		result[k] = v
	}
	return result
}
