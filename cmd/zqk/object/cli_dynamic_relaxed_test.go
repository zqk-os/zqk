package object

// BLI-177483 inventory: SetupTestEnvironment → testkit.RunStandardTeardown (TempProjectTeardown) in test_helpers.go.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestCreateWithRelaxed(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	// Create an object that references another object that doesn't exist yet
	// With --relaxed, this should succeed (non-blocking reference validation)
	testID := generateTestID(pplanKindBacklogItem)
	referencedID := generateTestID(pplanKindBacklogItem)

	obj := map[string]any{
		objects.FieldKeyID:            testID,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         "Object with forward reference",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		"related_refs":                []string{referencedID}, // Forward reference
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	objFile := filepath.Join(tmpDir, fmt.Sprintf("%s.yaml", testID))
	data, err := yaml.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal object: %v", err)
	}
	if err := fileutil.WriteFile(objFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to write object file: %v", err)
	}

	// Try to create without --relaxed (should fail due to missing reference)
	cmd := execwrap.Command(cliBinary, "object", "create", pplanKindBacklogItem, "--file", objFile)
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		// If it succeeds without --relaxed, that's okay (reference validation might be non-blocking by default)
		// But we should still test that --relaxed works
		t.Logf("Note: Create succeeded without --relaxed (reference validation may be non-blocking by default)")
	} else {
		// Expected to fail without --relaxed
		outputStr := string(output)
		if !strings.Contains(outputStr, "reference") && !strings.Contains(outputStr, "not found") {
			t.Logf("Create failed for unexpected reason: %v\nOutput: %s", err, outputStr)
		}
	}

	// Generate a new ID and write a new file for the relaxed run to prevent "object already exists" collision
	testID2 := generateTestID(pplanKindBacklogItem)
	obj[objects.FieldKeyID] = testID2
	objFile2 := filepath.Join(tmpDir, fmt.Sprintf("%s.yaml", testID2))
	data2, err := yaml.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal object: %v", err)
	}
	if err := fileutil.WriteFile(objFile2, data2, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to write object file: %v", err)
	}

	// Now try with --relaxed (should succeed)
	cmd = execwrap.Command(cliBinary, "object", "create", pplanKindBacklogItem, "--file", objFile2, "--relaxed")
	wireExecForTest(cmd, tmpDir)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Errorf("CLI create with --relaxed failed: %v\nOutput: %s", err, string(output))
		return
	}

	// Verify object was created
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	created, err := storageProvider.Read(pkgctx.NewSystemContext(), pkgctx.NewSystemSecurityContext(), testID2)
	if err != nil {
		t.Errorf("failed to read created object: %v", err)
		return
	}

	if created[objects.FieldKeyID] != testID2 {
		t.Errorf("object ID mismatch: expected %s, got %v", testID2, created[objects.FieldKeyID])
	}
	if created[objects.FieldKeyTitle] != "Object with forward reference" {
		t.Errorf("object title mismatch: expected 'Object with forward reference', got %v", created[objects.FieldKeyTitle])
	}
}

// TestUpdateWithRelaxed tests that object update --relaxed allows forward references
func TestUpdateWithRelaxed(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironment(t)

	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

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

	storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, initialObj, casLeaveStatus(objectStatusExploring))

	// Ensure the CAS index is persisted
	if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(pplanKindBacklogItem, 2*time.Second); err != nil {
		t.Fatalf("failed to flush CAS index queue: %v", err)
	}

	// Update with a forward reference using --relaxed
	referencedID := "CRIT-998" // This object doesn't exist yet
	cmd := execwrap.Command(cliBinary, "object", "update", testID, "--add-ref", fmt.Sprintf("criteria_refs=%s", referencedID), "--relaxed")
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("CLI update with --relaxed failed: %v\nOutput: %s", err, string(output))
		return
	}

	// Verify update
	verifyStorage, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	updated, err := verifyStorage.Read(pkgctx.NewSystemContext(), secCtx, testID)
	if err != nil {
		t.Errorf("failed to read updated object: %v", err)
		return
	}

	// Check that criteria_refs was updated
	if criteriaRefs, ok := updated["criteria_refs"].([]any); ok {
		if len(criteriaRefs) == 0 || criteriaRefs[0] != referencedID {
			t.Errorf("criteria_refs was not updated correctly: expected [%s], got %v", referencedID, criteriaRefs)
		}
	} else if criteriaRefsStr, ok := updated["criteria_refs"].([]string); ok {
		if len(criteriaRefsStr) == 0 || criteriaRefsStr[0] != referencedID {
			t.Errorf("criteria_refs was not updated correctly: expected [%s], got %v", referencedID, criteriaRefsStr)
		}
	} else {
		t.Errorf("criteria_refs field not found or unexpected type: %v", updated["criteria_refs"])
	}
}

// TestBulkCreateWithRelaxed tests that bulk create with --relaxed allows forward references
func TestBulkCreateWithRelaxed(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	// Create objects where one references another that comes later in the batch
	testID1 := generateTestID(pplanKindBacklogItem)
	testID2 := "BLI-998" // Different ID

	bulkObjects := []map[string]any{
		{
			objects.FieldKeyID:            testID1,
			objects.FieldKeyKind:          pplanKindBacklogItem,
			objects.FieldKeyTitle:         "Object with forward reference",
			objects.FieldKeyStatus:        objectStatusExploring,
			objects.FieldKeyGoalRefs:      []string{"G-123"},
			"related_refs":                []string{testID2}, // Forward reference to object that comes later
			objects.FieldKeySchemaVersion: objectSchemaV2,
		},
		{
			objects.FieldKeyID:            testID2,
			objects.FieldKeyKind:          pplanKindBacklogItem,
			objects.FieldKeyTitle:         "Referenced object",
			objects.FieldKeyStatus:        objectStatusExploring,
			objects.FieldKeyGoalRefs:      []string{"G-123"},
			objects.FieldKeySchemaVersion: objectSchemaV2,
		},
	}

	bulkFile := filepath.Join(tmpDir, "bulk-create-relaxed.yaml")
	data, err := yaml.Marshal(bulkObjects)
	if err != nil {
		t.Fatalf("failed to marshal objects: %v", err)
	}
	if err := fileutil.WriteFile(bulkFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to write bulk file: %v", err)
	}

	// Bulk create with --relaxed (should succeed even with forward references)
	// Note: bulk operations auto-detect relaxed mode, but we can test explicit flag
	cmd := execwrap.Command(cliBinary, "object", "bulk", "create", pplanKindBacklogItem, "--file", bulkFile, "--relaxed")
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("bulk create with --relaxed failed: %v\nOutput: %s", err, string(output))
		return
	}
	t.Logf("bulk create with --relaxed CLI output:\n%s", string(output))

	// Bulk create enqueues write-behind operations; wait so subsequent reads can
	// see the CAS files and CAS index updates.
	if err := storage.WaitForWALProcessing(tmpDir, 45*time.Second); err != nil {
		t.Logf("WaitForWALProcessing after bulk create: %v", err)
	}
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, pplanKindBacklogItem); err != nil {
		t.Logf("FlushListingIndexForProjectRoot(%s) after bulk create: %v", pplanKindBacklogItem, err)
	}

	// Verify both objects were created
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	flushCtx, cancel := storage.DurabilityFlushContext()
	defer cancel()
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, storageProvider, tmpDir, []string{pplanKindBacklogItem}); err != nil {
		t.Logf("EnsureCLIObjectMutationVisibleForProvider after bulk create: %v", err)
	}
	// In subprocess tests, in-process write-buffer waiting won't observe the CLI's
	// storage instance. Re-scan disk for the kind to ensure the CAS index is current.
	if err := storageProvider.EnsureCASIndexPopulatedFromScan(pkgctx.NewSystemContext(), pplanKindBacklogItem); err != nil {
		t.Logf("EnsureCASIndexPopulatedFromScan(%s) after bulk create: %v", pplanKindBacklogItem, err)
	}

	// Verify first/second objects by polling `zqk object get` via CLI.
	// This avoids in-process CAS snapshot/caching artifacts across subprocesses.
	deadline := time.Now().Add(45 * time.Second)
	var obj1 map[string]any
	var obj2 map[string]any
	var getErr1 error
	var getErr2 error

	for time.Now().Before(deadline) {
		// Try first object
		{
			getCmd := execwrap.Command(cliBinary, "object", "get", testID1, "--format", objectFormatYAML)
			wireExecForTest(getCmd, tmpDir)
			out, err := getCmd.CombinedOutput()
			if err == nil {
				parsed := stripCLIOutputForParse(out)
				_ = yaml.Unmarshal(parsed, &obj1)
				if tTitle, ok := obj1[objects.FieldKeyTitle].(string); ok && tTitle == "Object with forward reference" {
					getErr1 = nil
				} else {
					getErr1 = fmt.Errorf("unexpected title for %s: %v", testID1, obj1[objects.FieldKeyTitle])
				}
			} else {
				getErr1 = err
			}
		}

		// Try second object
		{
			getCmd := execwrap.Command(cliBinary, "object", "get", testID2, "--format", objectFormatYAML)
			wireExecForTest(getCmd, tmpDir)
			out, err := getCmd.CombinedOutput()
			if err == nil {
				parsed := stripCLIOutputForParse(out)
				_ = yaml.Unmarshal(parsed, &obj2)
				if tTitle, ok := obj2[objects.FieldKeyTitle].(string); ok && tTitle == "Referenced object" {
					getErr2 = nil
				} else {
					getErr2 = fmt.Errorf("unexpected title for %s: %v", testID2, obj2[objects.FieldKeyTitle])
				}
			} else {
				getErr2 = err
			}
		}

		if getErr1 == nil && getErr2 == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	if getErr1 != nil {
		t.Errorf("failed to read first object via CLI: %v", getErr1)
	}
	if getErr2 != nil {
		t.Errorf("failed to read second object via CLI: %v", getErr2)
	}
}
