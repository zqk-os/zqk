package storage

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestCAS_Count tests Count operation with CAS
func TestCAS_Count(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestCAS_Count in short mode (countWithFilters can block on WaitGroupManager)")
	}
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-count-test")
	mustEnsureProcessSpecsLayout(t, testRoot)

	fileStorage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}
	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(testRoot, fileStorage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Verify CAS is enabled for backlog_item
	if !fileStorage.usesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "account:system",
	}

	// Create multiple objects
	testObjs := []map[string]any{
		{
			objects.FieldKeyID:            "ITEM-101",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Count Test 1",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "ITEM-102",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Count Test 2",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
		{
			objects.FieldKeyID:            "ITEM-103",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Count Test 3",
			objects.FieldKeyStatus:        "exploring",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		},
	}

	// Create objects
	for _, obj := range testObjs {
		if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
			t.Fatalf("Failed to create object %s: %v", obj[objects.FieldKeyID], err)
		}
	}

	// Test Count without filters
	filter := ListFilter{
		Kind: "backlog_item",
	}

	count, err := fileStorage.Count(ctx, secCtx, filter)
	if err != nil {
		t.Fatalf("Failed to count objects: %v", err)
	}

	if count < len(testObjs) {
		t.Errorf("Expected at least %d objects, got %d", len(testObjs), count)
	}

	// Test Count with filter
	filterWithStatus := ListFilter{
		Kind: "backlog_item",
		Filters: map[string]any{
			objects.FieldKeyStatus: "exploring",
		},
	}

	countWithFilter, err := fileStorage.Count(ctx, secCtx, filterWithStatus)
	if err != nil {
		t.Fatalf("Failed to count objects with filter: %v", err)
	}

	if countWithFilter < len(testObjs) {
		t.Errorf("Expected at least %d objects with status 'exploring', got %d", len(testObjs), countWithFilter)
	}
}
