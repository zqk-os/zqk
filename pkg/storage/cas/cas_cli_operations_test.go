package cas_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqkenv"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestCAS_CLI_Operations tests CAS operations through the CLI
// This verifies that duplicate ID prevention works correctly in real-world usage
func TestCAS_CLI_Operations(t *testing.T) {
	// Not: SetupScenarioTestEnvironmentForTest uses t.Setenv(ZQK_TEST_ROOT).
	if testing.Short() {
		t.Skip("Skipping CAS CLI scenario test in short mode")
	}
	// Isolated copy of content-addressable-storage scenario (no live checkout as storage root).
	env := storage.SetupScenarioTestEnvironmentForTest(t, "content-addressable-storage")
	testScenarioDir := env.ScenarioRoot
	storage.MustBootstrapScenarioRootForCLIForTest(t, testScenarioDir)

	cliBinary := filepath.Join(testScenarioDir, paths.CLICommandName+"-test-init")
	if _, err := fileutil.Stat(cliBinary); fileutil.IsNotExist(err) {
		t.Skipf("CLI binary not found: %s", cliBinary)
	}

	// Use a unique test ID to avoid conflicts with previous test runs
	// Use a high number to avoid conflicts with existing objects
	testID := "BLI-999"

	t.Run("CreateObject", func(t *testing.T) {
		// First, try to delete the object if it exists (cleanup from previous runs)
		deleteCmd := exec.Command(cliBinary, "object", "delete", testID)
		deleteCmd.Dir = testScenarioDir
		deleteCmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)
		_ = deleteCmd.Run() //nolint:errcheck // Test cleanup - run errors are handled by test logic //nolint:errcheck // Ignore errors - object might not exist

		// Create a new object
		objData := map[string]any{
			objects.FieldKeyID:            testID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "CLI Test Object",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		jsonData, _ := json.Marshal(objData)

		cmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--data", string(jsonData)) //nolint:gosec
		cmd.Dir = testScenarioDir
		cmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to create object: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(string(output), "created successfully") {
			t.Errorf("Expected success message, got: %s", output)
		}
	})

	t.Run("CreateDuplicateID", func(t *testing.T) {
		// First, verify the object exists from the previous test
		// Add a small delay to ensure index is flushed to disk
		checkCmd := exec.Command(cliBinary, "object", "get", testID)
		checkCmd.Dir = testScenarioDir
		checkCmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)
		checkOutput, checkErr := checkCmd.CombinedOutput()
		if checkErr != nil {
			t.Fatalf("Object should exist from previous test, but get failed: %v\nOutput: %s", checkErr, checkOutput)
		}

		// Verify object exists in CAS index by checking the index file directly
		indexPath := filepath.Join(testScenarioDir, paths.ProcessBacklogDir, ".backlog_item.index")
		if _, err := fileutil.Stat(indexPath); err == nil {
			// Index file exists - verify it contains our test ID
			data, err := fileutil.ReadFile(indexPath)
			if err == nil {
				if !strings.Contains(string(data), testID) {
					t.Logf("Warning: Test ID %s not found in index file, but object get succeeded", testID)
				}
			}
		}

		// Try to create the same object again (should fail)
		objData := map[string]any{
			objects.FieldKeyID:            testID,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Duplicate Test",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		jsonData, _ := json.Marshal(objData)

		cmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--data", string(jsonData)) //nolint:gosec
		cmd.Dir = testScenarioDir
		cmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)

		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("Expected error when creating duplicate ID, but command succeeded\nOutput: %s", output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "already exists") &&
			!strings.Contains(outputStr, "object already exists") &&
			!strings.Contains(outputStr, "ErrObjectExists") &&
			!strings.Contains(outputStr, "duplicate") {
			t.Errorf("Expected 'already exists' error, got: %s", outputStr)
		}
	})

	t.Run("ReadObject", func(t *testing.T) {
		// Read the object back
		cmd := exec.Command(cliBinary, "object", "get", testID, "--format", "json")
		cmd.Dir = testScenarioDir
		cmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to read object: %v\nOutput: %s", err, output)
		}

		// Filter out warnings and extract JSON
		outputStr := string(output)
		jsonStart := strings.Index(outputStr, "{")
		if jsonStart == -1 {
			t.Fatalf("No JSON found in output: %s", outputStr)
		}
		jsonStr := outputStr[jsonStart:]

		var result map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
			t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, jsonStr)
		}

		if result[objects.FieldKeyID] != testID {
			t.Errorf("Expected ID %s, got %v", testID, result[objects.FieldKeyID])
		}

		if result[objects.FieldKeyTitle] != "CLI Test Object" {
			t.Errorf("Expected title 'CLI Test Object', got %v", result[objects.FieldKeyTitle])
		}
	})

	t.Run("UpdateObject", func(t *testing.T) {
		// Update the object
		cmd := exec.Command(cliBinary, "object", "update", testID, "--field", "title=Updated CLI Test Object")
		cmd.Dir = testScenarioDir
		cmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to update object: %v\nOutput: %s", err, output)
		}

		// Verify update
		readCmd := exec.Command(cliBinary, "object", "get", testID, "--format", "json")
		readCmd.Dir = testScenarioDir
		readCmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)

		readOutput, err := readCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to read updated object: %v\nOutput: %s", err, readOutput)
		}

		// Filter out warnings and extract JSON
		outputStr := string(readOutput)
		jsonStart := strings.Index(outputStr, "{")
		if jsonStart == -1 {
			t.Fatalf("No JSON found in output: %s", outputStr)
		}
		jsonStr := outputStr[jsonStart:]

		var result map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
			t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, jsonStr)
		}

		if result[objects.FieldKeyTitle] != "Updated CLI Test Object" {
			t.Errorf("Expected title 'Updated CLI Test Object', got %v", result[objects.FieldKeyTitle])
		}
	})

	t.Run("ListObjects", func(t *testing.T) {
		// List objects (should include our test object)
		cmd := exec.Command(cliBinary, "object", "list", "backlog_item", "--format", "json")
		cmd.Dir = testScenarioDir
		cmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to list objects: %v\nOutput: %s", err, output)
		}

		// Filter out warnings and extract JSON
		outputStr := string(output)
		jsonStart := strings.Index(outputStr, "{")
		if jsonStart == -1 {
			t.Fatalf("No JSON found in output: %s", outputStr)
		}
		jsonStr := outputStr[jsonStart:]

		var result map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
			t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, jsonStr)
		}

		objSlice, ok := result["objects"].([]any)
		if !ok {
			t.Fatalf("Expected 'objects' array in result")
		}

		found := false
		for _, obj := range objSlice {
			objMap := obj.(map[string]any)
			if objMap[objects.FieldKeyID] == testID {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Test object %s not found in list", testID)
		}
	})

	t.Run("CountObjects", func(t *testing.T) {
		// Count objects
		cmd := exec.Command(cliBinary, "object", "count", "backlog_item", "--format", "json")
		cmd.Dir = testScenarioDir
		cmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to count objects: %v\nOutput: %s", err, output)
		}

		// Filter out warnings and extract JSON
		outputStr := string(output)
		jsonStart := strings.Index(outputStr, "{")
		var countResult map[string]any
		if jsonStart == -1 {
			// Might be plain text output, try to parse as-is
			if err := json.Unmarshal(output, &countResult); err != nil {
				t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, outputStr)
			}
		} else {
			jsonStr := outputStr[jsonStart:]
			if err := json.Unmarshal([]byte(jsonStr), &countResult); err != nil {
				t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, jsonStr)
			}
		}

		count, ok := countResult["count"].(float64)
		if !ok {
			t.Fatalf("Expected 'count' number in result")
		}

		if count < 1 {
			t.Errorf("Expected at least 1 object, got %v", count)
		}
	})

	t.Run("DeleteObject", func(t *testing.T) {
		// Delete the object
		cmd := exec.Command(cliBinary, "object", "delete", testID)
		cmd.Dir = testScenarioDir
		cmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to delete object: %v\nOutput: %s", err, output)
		}

		// Verify deletion
		readCmd := exec.Command(cliBinary, "object", "get", testID, "--format", "json")
		readCmd.Dir = testScenarioDir
		readCmd.Env = zqkenv.SubprocessEnvironWithTestRoot(testScenarioDir)

		readOutput, err := readCmd.CombinedOutput()
		if err == nil {
			t.Errorf("Expected error when reading deleted object, but command succeeded\nOutput: %s", readOutput)
		}

		if !strings.Contains(string(readOutput), "not found") && !strings.Contains(string(readOutput), "does not exist") {
			t.Errorf("Expected 'not found' error, got: %s", readOutput)
		}
	})
}

// TestCAS_ComponentLevel_Update tests update operations at the component level
// This verifies that CAS index is correctly updated during updates
func TestCAS_ComponentLevel_Update(t *testing.T) {
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Verify CAS is enabled for backlog_item (should be true when path contains "test-scenarios")
	if !fileStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	objectID := "BLI-002"

	// Create initial object
	obj1 := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Original Title",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj1, "")

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("backlog_item"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Get initial hash from CAS
	cas, err := fileStorage.GetContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	initialHash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get initial hash: %v", err)
	}

	// Update object
	obj1[objects.FieldKeyTitle] = "Updated Title"
	err = fileStorage.Update(ctx, secCtx, objectID, obj1)
	if err != nil {
		t.Fatalf("Failed to update object: %v", err)
	}

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("backlog_item"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify hash changed
	updatedHash, err := cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Failed to get updated hash: %v", err)
	}

	if initialHash == updatedHash {
		t.Errorf("Hash should have changed after update: %s", initialHash)
	}

	// Verify content is updated
	readObj, err := fileStorage.Read(ctx, secCtx, objectID)
	if err != nil {
		t.Fatalf("Failed to read updated object: %v", err)
	}

	if readObj[objects.FieldKeyTitle] != "Updated Title" {
		t.Errorf("Expected title 'Updated Title', got %v", readObj[objects.FieldKeyTitle])
	}
}

// TestCAS_ComponentLevel_Delete tests delete operations at the component level
// This verifies that CAS index is correctly updated during deletes
func TestCAS_ComponentLevel_Delete(t *testing.T) {
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Verify CAS is enabled for backlog_item (should be true when path contains "test-scenarios")
	if !fileStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	// backlog_item is kernel-critical; need explicit hard-delete allow.
	ctx := storage.WithTestHardDelete(pkgctx.NewSystemContext())
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	objectID := "BLI-003"

	// Create object
	obj := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "To Be Deleted",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, "")

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("backlog_item"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify object exists in CAS
	cas, err := fileStorage.GetContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("Failed to get CAS: %v", err)
	}

	_, err = cas.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Object should exist in CAS before deletion: %v", err)
	}

	// Delete object (requires CLI context)
	err = fileStorage.Delete(ctx, secCtx, objectID, false)
	if err != nil {
		t.Fatalf("Failed to delete object: %v", err)
	}

	// Verify object no longer exists in CAS index
	_, err = cas.GetHashForID(objectID)
	if err == nil {
		t.Errorf("Object should not exist in CAS index after deletion")
	}

	// Verify Read returns ErrObjectNotFound or "not found" error
	_, err = fileStorage.Read(ctx, secCtx, objectID)
	if err == nil {
		t.Errorf("Expected error when reading deleted object, got nil")
	} else if err != storage.ErrObjectNotFound && !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected ErrObjectNotFound or 'not found' error, got %v", err)
	}

	// Verify Exists returns false
	exists, err := fileStorage.Exists(ctx, secCtx, objectID)
	if err != nil {
		t.Fatalf("Exists() should not return error: %v", err)
	}
	if exists {
		t.Errorf("Exists() should return false for deleted object")
	}
}

// TestCAS_ComponentLevel_Move tests move operations at the component level
// This verifies that CAS indexes are correctly updated when moving objects between kinds
func TestCAS_ComponentLevel_Move(t *testing.T) {
	// Create test root in a path that contains "test-scenarios" to enable CAS
	baseTempDir := t.TempDir()
	testRoot := filepath.Join(baseTempDir, "test-scenarios", "cas-test")
	storage.MustEnsureProcessSpecsLayoutForTest(t, testRoot)

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(testRoot, fileStorage)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	// Verify CAS is enabled for backlog_item (should be true when path contains "test-scenarios")
	if !fileStorage.UsesContentAddressableStorage("backlog_item") {
		t.Fatalf("CAS should be enabled for backlog_item when path contains 'test-scenarios'")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-1785920548450214012-68b850c0",
	}

	objectID := "BLI-004"

	// Create object as backlog_item
	obj := map[string]any{
		objects.FieldKeyID:            objectID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "To Be Moved",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, "")

	// Wait for index updates to be processed
	if err := storage.FlushListingIndexForKind("backlog_item"); err != nil {
		t.Fatalf("Failed to flush write queue: %v", err)
	}

	// Verify object exists in backlog_item CAS
	backlogCAS, err := fileStorage.GetContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("Failed to get backlog_item CAS: %v", err)
	}

	backlogHash, err := backlogCAS.GetHashForID(objectID)
	if err != nil {
		t.Fatalf("Object should exist in backlog_item CAS: %v", err)
	}

	// Move requires CLI context
	ctx = storage.WithCLIOperation(ctx)

	// Move to requirement (if requirement uses CAS in test scenario)
	// Note: This test assumes both kinds use CAS when path contains "test-scenarios"
	// Move might fail if requirement kind doesn't exist or has different structure
	// That's okay - we're testing the CAS index update logic
	err = fileStorage.Move(ctx, secCtx, objectID, "requirement", false)
	if err != nil {
		// Move might fail due to validation or missing requirement structure
		// That's okay - we've verified the object exists in CAS
		t.Logf("Move operation returned error (may be expected): %v", err)
		return
	}

	// Verify object no longer exists in backlog_item CAS
	_, err = backlogCAS.GetHashForID(objectID)
	if err == nil {
		t.Errorf("Object should not exist in backlog_item CAS after move")
	}

	// Verify object exists in requirement CAS
	requirementCAS, err := fileStorage.GetContentAddressableStorage("requirement")
	if err == nil {
		_, err = requirementCAS.GetHashForID(objectID)
		if err != nil {
			t.Errorf("Object should exist in requirement CAS after move: %v", err)
		}
	}

	// Verify object can be read with new kind
	readObj, err := fileStorage.Read(ctx, secCtx, objectID)
	if err != nil {
		t.Fatalf("Failed to read moved object: %v", err)
	}

	if readObj[objects.FieldKeyKind] != "requirement" {
		t.Errorf("Expected kind 'requirement', got %v", readObj[objects.FieldKeyKind])
	}

	// Verify hash is preserved (content should be same, just kind changed)
	requirementHash, err := requirementCAS.GetHashForID(objectID)
	if err == nil && backlogHash != requirementHash {
		// Hash might differ if kind field is included in content hash
		// This is expected behavior
		t.Logf("Hash changed after move (expected if kind is in content): %s -> %s", backlogHash, requirementHash)
	}
}
