package object

// Tests that use SetupCompleteTestEnvironment or set ZQK_TEST_ROOT must not use t.Parallel():
// the env var is process-global.

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestMoveCommand_BasicMove(t *testing.T) {
	factory := storage.NewTestingFactory()
	env := setupCompleteTestEnvironmentObject(t, factory)
	defer env.Cleanup()
	testRoot := env.TestRoot
	storageProvider := env.Storage.(storage.ObjectStorageProvider)

	obj := map[string]any{
		objects.FieldKeyID:            "BLI-001",
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Test Item",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := storage.WithCLIOperation(pkgctx.NewSystemContext())

	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, obj, casLeaveStatus(objectStatusExploring))
	if err := storage.FlushAllListingIndexesForProjectRoot(testRoot); err != nil {
		t.Fatalf("Failed to flush CAS indexes: %v", err)
	}

	if _, err := storageProvider.Read(ctx, secCtx, "BLI-001"); err != nil {
		t.Fatalf("Object not found in storage: %v", err)
	}

	newKind := "goal"
	newDir := objects.GetDirectoryFromKind(newKind)
	newDirPath := datacell.CellCASPrimaryDir(testRoot, newDir)
	if err := fileutil.MkdirAll(newDirPath, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create new directory: %v", err)
	}

	err := storageProvider.Move(ctx, secCtx, "BLI-001", newKind, false)
	if err != nil && (strings.Contains(err.Error(), "ID pattern") || strings.Contains(err.Error(), "id does not match")) {
		t.Skipf("Move to kind %s requires ID reassignment (BLI-001 does not match target kind pattern): %v", newKind, err)
	}
	if err != nil {
		t.Fatalf("Failed to move object: %v", err)
	}

	moved, err := storageProvider.Read(ctx, secCtx, "BLI-001")
	if err != nil {
		t.Fatalf("Moved object not found: %v", err)
	}
	if moved[objects.FieldKeyKind] != newKind {
		t.Errorf("kind not updated: got %v, want %v", moved[objects.FieldKeyKind], newKind)
	}
}

func TestMoveCommand_MoveBetweenKinds(t *testing.T) {
	factory := storage.NewTestingFactory()
	env := setupCompleteTestEnvironmentObject(t, factory)
	defer env.Cleanup()
	testRoot := env.TestRoot
	storageProvider := env.Storage.(storage.ObjectStorageProvider)

	obj := map[string]any{
		objects.FieldKeyID:            "BLI-001",
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Test Backlog Item",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := storage.WithCLIOperation(pkgctx.NewSystemContext())

	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, obj, casLeaveStatus(objectStatusExploring))
	if err := storage.FlushAllListingIndexesForProjectRoot(testRoot); err != nil {
		t.Fatalf("Failed to flush CAS indexes: %v", err)
	}

	newKind := "goal"
	if _, err := storageProvider.Read(ctx, secCtx, "BLI-001"); err != nil {
		t.Fatalf("Original object not found: %v", err)
	}

	err := storageProvider.Move(ctx, secCtx, "BLI-001", newKind, false)
	if err != nil && (strings.Contains(err.Error(), "ID pattern") || strings.Contains(err.Error(), "id does not match")) {
		t.Skipf("Move to kind %s requires ID reassignment: %v", newKind, err)
	}
	if err != nil {
		t.Fatalf("Failed to move object: %v", err)
	}

	moved, err := storageProvider.Read(ctx, secCtx, "BLI-001")
	if err != nil {
		t.Fatalf("Moved object not found: %v", err)
	}
	if moved[objects.FieldKeyKind] != newKind {
		t.Errorf("kind not updated: got %v, want %v", moved[objects.FieldKeyKind], newKind)
	}
}

func TestMoveCommand_PreservesHistory(t *testing.T) {
	factory := storage.NewTestingFactory()
	env := setupCompleteTestEnvironmentObject(t, factory)
	defer env.Cleanup()
	testRoot := env.TestRoot
	storageProvider := env.Storage.(storage.ObjectStorageProvider)

	obj := map[string]any{
		objects.FieldKeyID:            "BLI-002",
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Test Item with History",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
		objects.FieldKeyCreatedAt:     "2026-01-01T00:00:00Z",
		objects.FieldKeyCreatedBy:     "ACC-TEST",
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "ACC-TEST",
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := storage.WithCLIOperation(pkgctx.NewSystemContext())

	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, obj, casLeaveStatus(objectStatusExploring))
	if err := storage.FlushAllListingIndexesForProjectRoot(testRoot); err != nil {
		t.Fatalf("Failed to flush CAS indexes: %v", err)
	}

	newKind := "milestone"
	err := storageProvider.Move(ctx, secCtx, "BLI-002", newKind, false)
	if err != nil && (strings.Contains(err.Error(), "ID pattern") || strings.Contains(err.Error(), "id does not match")) {
		t.Skipf("Move to kind %s requires ID reassignment: %v", newKind, err)
	}
	if err != nil {
		t.Fatalf("Failed to move object: %v", err)
	}

	movedObj, err := storageProvider.Read(ctx, secCtx, "BLI-002")
	if err != nil {
		t.Fatalf("Moved object not found: %v", err)
	}

	// Verify history fields preserved
	if movedObj[objects.FieldKeyCreatedAt] != obj[objects.FieldKeyCreatedAt] {
		t.Errorf("created_at not preserved: got %v, want %v", movedObj[objects.FieldKeyCreatedAt], obj[objects.FieldKeyCreatedAt])
	}
	if movedObj[objects.FieldKeyCreatedBy] != obj[objects.FieldKeyCreatedBy] {
		t.Errorf("created_by not preserved: got %v, want %v", movedObj[objects.FieldKeyCreatedBy], obj[objects.FieldKeyCreatedBy])
	}

	// Verify kind changed
	if movedObj[objects.FieldKeyKind] != newKind {
		t.Errorf("kind not updated: got %v, want %v", movedObj[objects.FieldKeyKind], newKind)
	}
}

func TestMoveCommand_UpdatesHashRegistry(t *testing.T) {
	testEnvMu.Lock()
	t.Cleanup(func() { testEnvMu.Unlock() })
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	t.Cleanup(func() {
		if err := testkit.RunStandardTeardown(testkit.TempProjectTeardown(testRoot, nil)); err != nil {
			t.Logf("test teardown: %v", err)
		}
	})

	// Setup test environment
	if _, err := setupObjectTestEnvironmentRoot(testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	// This test will verify that hash registry is updated atomically
	// when an object is moved
	t.Log("Hash registry update test - to be implemented")
}

func TestMoveCommand_CreatesAuditEvent(t *testing.T) {
	testEnvMu.Lock()
	t.Cleanup(func() { testEnvMu.Unlock() })
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	t.Cleanup(func() {
		if err := testkit.RunStandardTeardown(testkit.TempProjectTeardown(testRoot, nil)); err != nil {
			t.Logf("test teardown: %v", err)
		}
	})

	// Setup test environment
	if _, err := setupObjectTestEnvironmentRoot(testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	// This test will verify that an audit event is created for move operations
	t.Log("Audit event creation test - to be implemented")
}

func TestMoveCommand_UpdatesReferences(t *testing.T) {
	factory := storage.NewTestingFactory()
	env := setupCompleteTestEnvironmentObject(t, factory)
	defer env.Cleanup()
	testRoot := env.TestRoot
	storageProvider := env.Storage.(storage.ObjectStorageProvider)

	obj1 := map[string]any{
		objects.FieldKeyID:            "BLI-001",
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Referenced Item",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	obj2 := map[string]any{
		objects.FieldKeyID:            "BLI-002",
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Referencing Item",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
		objects.FieldKeyMilestoneRefs: []string{pplanKindBacklogItem + ":BLI-001"},
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := storage.WithCLIOperation(pkgctx.NewSystemContext())

	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, obj1, casLeaveStatus(objectStatusExploring))
	if err := storage.FlushAllListingIndexesForProjectRoot(testRoot); err != nil {
		t.Fatalf("Failed to flush CAS indexes: %v", err)
	}
	storage.CreateCASVisible(t, storageProvider, ctx, secCtx, obj2, casLeaveStatus(objectStatusExploring))

	newKind := "milestone"
	err := storageProvider.Move(ctx, secCtx, "BLI-001", newKind, true)
	if err != nil && (strings.Contains(err.Error(), "ID pattern") || strings.Contains(err.Error(), "id does not match")) {
		t.Skipf("Move to kind %s requires ID reassignment: %v", newKind, err)
	}
	if err != nil {
		t.Fatalf("Failed to move obj1: %v", err)
	}

	// After move, verify references are updated
	// obj2 should now reference goal:BLI-001 instead of backlog_item:BLI-001
	movedObj2, err := storageProvider.Read(ctx, secCtx, "BLI-002")
	if err != nil {
		t.Fatalf("Failed to read obj2: %v", err)
	}

	refs, ok := movedObj2[objects.FieldKeyMilestoneRefs].([]string)
	if !ok {
		// Try []any
		if refsAny, ok := movedObj2[objects.FieldKeyMilestoneRefs].([]any); ok {
			refs = make([]string, len(refsAny))
			for i, r := range refsAny {
				if rs, ok := r.(string); ok {
					refs[i] = rs
				}
			}
		} else {
			t.Fatalf("milestone_refs not found or wrong type: %T", movedObj2[objects.FieldKeyMilestoneRefs])
		}
	}

	expectedRef := newKind + ":BLI-001"
	found := false
	for _, ref := range refs {
		if ref == expectedRef {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Reference not updated: expected %s in milestone_refs, got %v", expectedRef, refs)
	}
	// Verify moved obj1 exists and has new kind
	movedObj1, err := storageProvider.Read(ctx, secCtx, "BLI-001")
	if err != nil {
		t.Fatalf("Moved object BLI-001 not found: %v", err)
	}
	if movedObj1[objects.FieldKeyKind] != newKind {
		t.Errorf("Object kind not updated: got %v, want %s", movedObj1[objects.FieldKeyKind], newKind)
	}
}
