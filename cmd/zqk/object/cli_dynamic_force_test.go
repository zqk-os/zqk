package object

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestCreateWithForce(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()

	// Create an initial object
	testID := generateTestID(pplanKindBacklogItem)
	initialObj := map[string]any{
		objects.FieldKeyID:            testID,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	// Promote off draft plane so CAS index / List see the object.
	// TRACK: BLI-REDACTED
	storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, initialObj, objects.ObjectStatusValidated)

	// Ensure the CAS index is persisted
	if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(pplanKindBacklogItem, 2*time.Second); err != nil {
		t.Fatalf("failed to flush CAS index queue: %v", err)
	}

	// Try to create the same object again with --force (should update instead of fail)
	updatedObj := map[string]any{
		objects.FieldKeyID:            testID,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Updated Title",
		objects.FieldKeyStatus:        objects.ObjectStatusValidated,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	objFile := filepath.Join(tmpDir, fmt.Sprintf("%s-updated.yaml", testID))
	data, err := yaml.Marshal(updatedObj)
	if err != nil {
		t.Fatalf("failed to marshal object: %v", err)
	}
	if err := fileutil.WriteFile(objFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to write object file: %v", err)
	}

	cmd := execwrap.Command(cliBinary, "object", "create", pplanKindBacklogItem, "--file", objFile, "--force")
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("CLI create with --force failed: %v\nOutput: %s", err, string(output))
		return
	}

	// Wait for write-behind operations to complete (CLI updates go through WAL)
	if err := storage.WaitForWALProcessing(tmpDir, 5*time.Second); err != nil {
		t.Logf("Warning: WAL processing wait failed (may be OK if write-behind disabled): %v", err)
	}

	// Verify object was updated (not just created)
	verifyStorage, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, verifyStorage)
	updated, err := verifyStorage.Read(pkgctx.NewSystemContext(), secCtx, testID)
	if err != nil {
		t.Errorf("failed to read updated object: %v", err)
		return
	}

	if updated[objects.FieldKeyTitle] != "Updated Title" {
		t.Errorf("object was not updated: expected title 'Updated Title', got %v", updated[objects.FieldKeyTitle])
	}
}

// TestUpdateWithForce tests that object update --force creates missing objects
func TestUpdateWithForce(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	// Try to update a non-existent object with --force (should create instead of fail)
	testID := generateTestID(pplanKindBacklogItem)
	newObj := map[string]any{
		objects.FieldKeyID:            testID,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Created via Update",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	objFile := filepath.Join(tmpDir, fmt.Sprintf("%s-new.yaml", testID))
	data, err := yaml.Marshal(newObj)
	if err != nil {
		t.Fatalf("failed to marshal object: %v", err)
	}
	if err := fileutil.WriteFile(objFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to write object file: %v", err)
	}

	cmd := execwrap.Command(cliBinary, "object", "update", testID, "--file", objFile, "--force")
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("CLI update with --force failed: %v\nOutput: %s", err, string(output))
		return
	}

	// Wait for write-behind operations to complete (CLI creates/updates go through WAL)
	if err := storage.WaitForWALProcessing(tmpDir, 5*time.Second); err != nil {
		t.Logf("Warning: WAL processing wait failed (may be OK if write-behind disabled): %v", err)
	}

	// Flush CAS index for project root so Read finds the object (async under bundler)
	if q := caspkg.GetListingIndexWriteQueueForProjectRoot(tmpDir); q != nil {
		_ = q.FlushKind(pplanKindBacklogItem, 5*time.Second)
	}

	// Verify object was created
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()
	created, err := storageProvider.Read(pkgctx.NewSystemContext(), secCtx, testID)
	if err != nil {
		t.Errorf("failed to read created object: %v", err)
		return
	}

	if created[objects.FieldKeyTitle] != "Created via Update" {
		t.Errorf("object was not created correctly: expected title 'Created via Update', got %v", created[objects.FieldKeyTitle])
	}
}

// TestBulkCreateWithForce tests that object bulk create --force updates existing objects
func TestBulkCreateWithForce(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()

	// Create initial objects
	testID1 := generateTestID(pplanKindBacklogItem)
	testID2 := generateTestID("goal")
	initialObjs := []map[string]any{
		{
			objects.FieldKeyID:            testID1,
			objects.FieldKeyKind:          pplanKindBacklogItem,
			objects.FieldKeyTitle:         "Original Title 1",
			objects.FieldKeyStatus:        objectStatusExploring,
			objects.FieldKeyGoalRefs:      []string{"G-123"},
			objects.FieldKeySchemaVersion: objectSchemaV2,
		},
		{
			objects.FieldKeyID:            testID2,
			objects.FieldKeyKind:          "goal",
			objects.FieldKeyTitle:         "Original Goal",
			objects.FieldKeyStatus:        objectStatusActive,
			objects.FieldKeySchemaVersion: objectSchemaV2,
		},
	}

	for _, obj := range initialObjs {
		leave := objects.GetString(obj, objects.FieldKeyStatus)
		if leave == objectStatusExploring || leave == emptyValue {
			leave = objects.ObjectStatusValidated
		}
		storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, obj, leave)
	}

	// Ensure the CAS index is persisted
	if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(pplanKindBacklogItem, 2*time.Second); err != nil {
		t.Fatalf("failed to flush CAS index queue: %v", err)
	}

	// Try to bulk create the same objects again with --force (should update instead of fail)
	// Note: bulk create requires all objects to be the same kind
	updatedObjs := []map[string]any{
		{
			objects.FieldKeyID:            testID1,
			objects.FieldKeyKind:          pplanKindBacklogItem,
			objects.FieldKeyTitle:         "Updated Title 1",
			objects.FieldKeyStatus:        objects.ObjectStatusValidated,
			objects.FieldKeyGoalRefs:      []string{"G-123"},
			objects.FieldKeySchemaVersion: objectSchemaV2,
		},
		{
			objects.FieldKeyID:            testID2,
			objects.FieldKeyKind:          pplanKindBacklogItem, // Changed to backlog_item to match bulk create requirement
			objects.FieldKeyTitle:         "Updated Title 2",
			objects.FieldKeyStatus:        objects.ObjectStatusValidated,
			objects.FieldKeyGoalRefs:      []string{"G-123"},
			objects.FieldKeySchemaVersion: objectSchemaV2,
		},
	}

	bulkFile := filepath.Join(tmpDir, "bulk-create-force.yaml")
	data, err := yaml.Marshal(updatedObjs)
	if err != nil {
		t.Fatalf("failed to marshal objects: %v", err)
	}
	if err := fileutil.WriteFile(bulkFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to write bulk file: %v", err)
	}

	cmd := execwrap.Command(cliBinary, "object", "bulk", "create", pplanKindBacklogItem, "--file", bulkFile, "--force")
	cmd.Env = EnvWithTestRoot(tmpDir)
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("bulk create with --force failed: %v\nOutput: %s", err, string(output))
		return
	}

	if err := storage.WaitForWALProcessing(tmpDir, 5*time.Second); err != nil {
		t.Logf("Warning: WAL processing wait failed (may be OK if write-behind disabled): %v", err)
	}
	// Flush CAS so Read sees updated objects (async under bundler)
	if q := caspkg.GetListingIndexWriteQueueForProjectRoot(tmpDir); q != nil {
		_ = q.FlushKind(pplanKindBacklogItem, 5*time.Second)
	}

	// Verify objects were updated
	verifyStorage, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, verifyStorage)

	updated1, err := readTestObjectWithRetry(verifyStorage, secCtx, testID1)
	if err != nil {
		t.Errorf("failed to read updated object 1 after retries: %v", err)
	} else if updated1[objects.FieldKeyTitle] != "Updated Title 1" {
		t.Errorf("object 1 was not updated: expected title 'Updated Title 1', got %v", updated1[objects.FieldKeyTitle])
	}
}

// TestBulkUpdateWithForce tests that object bulk update with --filter/--set updates matching objects.
// The bulk update command now uses --filter and --set (no --file or --force); this test verifies
// that updating by filter applies changes to matching objects.
func TestBulkUpdateWithForce(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()

	testID1 := generateTestID(pplanKindBacklogItem)
	existingObj := map[string]any{
		objects.FieldKeyID:            testID1,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, existingObj, objects.ObjectStatusValidated)

	if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(pplanKindBacklogItem, 2*time.Second); err != nil {
		t.Fatalf("failed to flush CAS index queue: %v", err)
	}

	// Bulk update using new API: kind, --filter id=<id>, --set title=...
	cmd := execwrap.Command(cliBinary, "object", "bulk", "update", pplanKindBacklogItem, "--filter", "id="+testID1, "--set", "title=Updated Title")
	cmd.Env = EnvWithTestRoot(tmpDir)
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("CLI bulk update failed: %v\nOutput: %s", err, string(output))
		return
	}

	verifyStorage, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, verifyStorage)

	updated, err := verifyStorage.Read(pkgctx.NewSystemContext(), secCtx, testID1)
	if err != nil {
		t.Errorf("failed to read updated object: %v", err)
	} else if updated[objects.FieldKeyTitle] != "Updated Title" {
		t.Errorf("existing object was not updated: expected title 'Updated Title', got %v", updated[objects.FieldKeyTitle])
	}
}
