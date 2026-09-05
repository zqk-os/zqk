package crud_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

func setupSearchTest(t *testing.T) (string, *storage.FileObjectStorage, *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Not in callers: ZQK_TEST_ROOT is process-global (POL-CODE-006).
	t.Setenv(zqkenv.TestRoot(), tmpDir)

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})

	secCtx := pkgctx.NewSystemSecurityContext()
	return tmpDir, fos, secCtx
}

func TestSearch_Basic(t *testing.T) {
	_, fos, secCtx := setupSearchTest(t)
	ctx := context.Background()

	// Create test backlog items
	items := []map[string]any{
		{objects.FieldKeyID: "BLI-901", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Implement search feature", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "BLI-902", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Add aggregation support", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "BLI-903", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Search implementation", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
	}

	for _, item := range items {
		storage.CreateCASVisible(t, fos, ctx, secCtx, item, "")
	}

	// Test basic search
	query := storage.SearchQuery{
		Query: "search",
		Kinds: []string{"backlog_item"},
		Limit: 10,
	}

	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Search(ctx, secCtx, storageCtx, query)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if result.TotalCount < 2 {
		t.Errorf("Expected at least 2 matches for 'search', got %d", result.TotalCount)
	}

	// Verify results contain "search" in title
	for _, match := range result.Objects {
		title, ok := match.Object[objects.FieldKeyTitle].(string)
		if !ok {
			continue
		}
		if !containsIgnoreCase(title, "search") {
			t.Errorf("Result title '%s' does not contain 'search'", title)
		}
	}
}

func TestSearch_MultipleTerms(t *testing.T) {
	_, fos, secCtx := setupSearchTest(t)
	ctx := context.Background()

	// Create test backlog items
	items := []map[string]any{
		{objects.FieldKeyID: "BLI-901", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Implement search feature", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
		{objects.FieldKeyID: "BLI-902", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Add aggregation support", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development"},
	}

	for _, item := range items {
		storage.CreateCASVisible(t, fos, ctx, secCtx, item, "")
	}

	// Test search with multiple terms
	query := storage.SearchQuery{
		Query: "implement feature",
		Kinds: []string{"backlog_item"},
		Limit: 10,
	}

	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Search(ctx, secCtx, storageCtx, query)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if result.TotalCount < 1 {
		t.Errorf("Expected at least 1 match for 'implement feature', got %d", result.TotalCount)
	}
}

func TestSearch_EmptyResult(t *testing.T) {
	_, fos, secCtx := setupSearchTest(t)
	ctx := context.Background()

	// Test search with no matches
	query := storage.SearchQuery{
		Query: "nonexistent term",
		Kinds: []string{"backlog_item"},
		Limit: 10,
	}

	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Search(ctx, secCtx, storageCtx, query)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if result.TotalCount != 0 {
		t.Errorf("Expected 0 matches for 'nonexistent term', got %d", result.TotalCount)
	}
}

func TestSearch_Highlighting(t *testing.T) {
	_, fos, secCtx := setupSearchTest(t)
	ctx := context.Background()

	// Create test backlog item
	item := map[string]any{
		objects.FieldKeyID: "BLI-901", objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Implement search feature", objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyCategory: "development",
	}

	storage.CreateCASVisible(t, fos, ctx, secCtx, item, "")

	// Test search with highlighting
	query := storage.SearchQuery{
		Query:     "search",
		Kinds:     []string{"backlog_item"},
		Highlight: true,
		Limit:     10,
	}

	storageCtx := pkgctx.GetStorageContext()
	result, err := fos.Search(ctx, secCtx, storageCtx, query)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(result.Objects) == 0 {
		t.Fatal("Expected at least one result")
	}

	match := result.Objects[0]
	if len(match.Highlights) == 0 {
		t.Error("Expected highlights but got none")
	}
}

// Helper function
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
