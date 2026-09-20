package cas_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/storage"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestCAS_Count tests Count operation with CAS
func TestCAS_Count(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestCAS_Count in short mode (countWithFilters can block on WaitGroupManager)")
	}
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-count-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Verify CAS is enabled for backlog_item
	if !fileStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	// Create multiple objects
	testObjs := []map[string]any{
		{
			objects.FieldKeyID:            "BLI-101",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Count Test 1",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "BLI-102",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Count Test 2",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "BLI-103",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Count Test 3",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
	}

	// Create + promote through draft membrane (exploring is preliminary; Count omits drafts).
	for _, obj := range testObjs {
		storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, objects.ObjectStatusValidated)
	}

	// Test Count without filters
	filter := storage.ListFilter{
		Kind: "backlog_item",
	}

	count, err := fileStorage.Count(ctx, secCtx, filter)
	if err != nil {
		t.Fatalf("Failed to count objects: %v", err)
	}

	if count < len(testObjs) {
		t.Errorf("Expected at least %d objects, got %d", len(testObjs), count)
	}

	// Test Count with filter (post-promote status)
	filterWithStatus := storage.ListFilter{
		Kind: "backlog_item",
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusValidated,
		},
	}

	countWithFilter, err := fileStorage.Count(ctx, secCtx, filterWithStatus)
	if err != nil {
		t.Fatalf("Failed to count objects with filter: %v", err)
	}

	if countWithFilter < len(testObjs) {
		t.Errorf("Expected at least %d objects with status 'validated', got %d", len(testObjs), countWithFilter)
	}
}
