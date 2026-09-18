package object

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestAllKindsFilterSortGroupCount tests filtering, sorting, grouping, and counting for all kinds
func TestAllKindsFilterSortGroupCount(t *testing.T) {
	if testing.Short() {
		t.Skip("comprehensive all-kinds filter/sort/group/count is slow; run without -short")
	}
	tmpDir, cliBinary := setupCLITestEnvironmentForComprehensive(t)
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	kinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		t.Fatalf("failed to get all kinds: %v", err)
	}
	if raceDetectorEnabled {
		kinds = []string{pplanKindBacklogItem, "goal", "system_check"}
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)

	RunComprehensiveKindTests(t, kinds, func(t *testing.T, kind string) {
		testKindFilterSortGroupCount(t, cliBinary, storageProvider, ctx, secCtx, kind, fieldRegistry)
	})
}

// testKindFilterSortGroupCount tests filtering, sorting, grouping, and counting for a specific kind
func testKindFilterSortGroupCount(t *testing.T, cliBinary string, storageProvider storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, kind string, fieldRegistry *objects.FieldRegistry) {
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		t.Skipf("skipping %s: failed to get fields: %v", kind, err)
		return
	}

	projectRoot := ""
	if fs, ok := storageProvider.(*storage.FileObjectStorage); ok {
		projectRoot = fs.GetProjectRoot()
	}
	if projectRoot == "" {
		t.Fatalf("filter/sort/group/count tests require FileObjectStorage with a project root")
	}

	testObjects := createTestObjectsForKindCLI(t, storageProvider, ctx, secCtx, kind, kindFields, 5)
	if len(testObjects) == 0 {
		t.Skipf("skipping %s: no test objects created", kind)
		return
	}

	//nolint:errcheck // Test helper - error acceptable
	filterableFields, _ := objects.GenerateFilterableFields(kind)
	//nolint:errcheck // Test helper - error acceptable
	sortableFields, _ := objects.GenerateSortableFields(kind)
	//nolint:errcheck // Test helper - error acceptable
	groupableFields, _ := objects.GenerateGroupableFields(kind)

	if len(filterableFields) > 0 {
		t.Run("Filtering", func(t *testing.T) {
			testFilteringViaCLI(t, projectRoot, cliBinary, kind, filterableFields, testObjects)
		})
	}

	if len(sortableFields) > 0 {
		t.Run("Sorting", func(t *testing.T) {
			testSortingViaCLI(t, projectRoot, cliBinary, kind, sortableFields)
		})
	}

	if len(groupableFields) > 0 {
		t.Run("Grouping", func(t *testing.T) {
			testGroupingViaCLI(t, projectRoot, cliBinary, kind, groupableFields)
		})
	}

	t.Run("Counting", func(t *testing.T) {
		testCountingViaCLI(t, projectRoot, cliBinary, kind)
	})
}
