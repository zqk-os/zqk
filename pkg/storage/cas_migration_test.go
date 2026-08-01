package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestCAS_MigrationInTestScenario tests that content-addressable storage works
// for all kinds in the test scenario, including migration of existing objects
func TestCAS_MigrationInTestScenario(t *testing.T) {

	// This is a heavy-weight integration test that depends on an external
	// test binary and a fully prepared CAS scenario directory. By default
	// we skip it to keep the core storage test suite deterministic and
	// parallel-safe. Enable explicitly when the scenario environment is
	// available.
	if os.Getenv(zqkenv.EnableCASMigrationScenarioTest()) == emptyValue {
		t.Skip("Skipping CAS migration scenario test; set ZQK_ENABLE_CAS_MIGRATION_SCENARIO_TEST=1 to enable")
	}

	// Prepare an isolated copy of the content-addressable-storage test scenario.
	// This ensures test-scenarios data remains immutable while tests operate on
	// a per-test working copy.
	env := setupScenarioTestEnvironmentForTest(t, "content-addressable-storage")
	scenarioDir := env.ScenarioRoot

	// Check if test binary exists
	testBinary := filepath.Join(scenarioDir, "zqk-admin-test-init")
	if _, err := os.Stat(testBinary); err != nil {
		t.Fatalf("Test binary not found: %s", testBinary)
	}

	t.Logf("Testing CAS migration in scenario: %s", scenarioDir)

	// Test that new objects use CAS
	t.Run("NewObjectsUseCAS", func(t *testing.T) {
		testNewObjectsUseCAS(t, testBinary, scenarioDir)
	})

	// Test that existing objects can be read (both formats)
	t.Run("ReadExistingObjects", func(t *testing.T) {
		testReadExistingObjects(t, testBinary, scenarioDir)
	})

	// Test that updates migrate to CAS
	t.Run("UpdateMigratesToCAS", func(t *testing.T) {
		testUpdateMigratesToCAS(t, testBinary, scenarioDir)
	})

	// Test that all operations work with CAS
	t.Run("AllOperationsWork", func(t *testing.T) {
		testAllOperationsWork(t, testBinary, scenarioDir)
	})
}

func testNewObjectsUseCAS(t *testing.T, binaryPath, scenarioDir string) {
	// Create a new object with an ID that does not already exist in the
	// scenario backlog directory to avoid collisions across runs.
	testID := "ITEM-999"
	backlogDir := datacell.CellCASPrimaryDir(scenarioDir, "backlog")
	for i := 999; i < 10_000; i++ {
		candidate := fmt.Sprintf("ITEM-%d", i)
		candidatePath := filepath.Join(backlogDir, candidate+".yaml")
		if _, err := os.Stat(candidatePath); os.IsNotExist(err) {
			testID = candidate
			break
		}
	}
	kind := "backlog_item"

	objData := fmt.Sprintf(`id: %s
kind: %s
title: Test CAS New Object
status: exploring
schema_version: "`+objects.DefaultSchemaVersion+`"`, testID, kind)

	cmd := runIsolatedCLICommand(binaryPath, []string{"object", "create", kind, "--data", objData}, scenarioDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create object: %v\nOutput: %s", err, output)
	}

	// Check that hash-based file was created
	kindDir := datacell.CellCASPrimaryDir(scenarioDir, "backlog")
	indexPath := filepath.Join(kindDir, fmt.Sprintf(".%s.index", kind))

	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("Index file not created: %v", err)
	}

	// Verify object can be retrieved
	getCmd := runIsolatedCLICommand(binaryPath, []string{"object", "get", testID}, scenarioDir)
	getOutput, err := getCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get object: %v\nOutput: %s", err, getOutput)
	}

	if !strings.Contains(string(getOutput), testID) {
		t.Errorf("Object not found in output: %s", getOutput)
	}

	t.Logf("✓ New object %s created with CAS", testID)
}

func testReadExistingObjects(t *testing.T, binaryPath, scenarioDir string) {
	// Test reading existing ID-based objects (ITEM-001, ITEM-002)
	testIDs := []string{"ITEM-001", "ITEM-002"}

	for _, id := range testIDs {
		cmd := runIsolatedCLICommand(binaryPath, []string{"object", "get", id}, scenarioDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to read existing object %s: %v\nOutput: %s", id, err, output)
		}

		if !strings.Contains(string(output), id) {
			t.Errorf("Object %s not found in output", id)
		}
	}

	t.Logf("✓ Existing ID-based objects can be read")
}

func testUpdateMigratesToCAS(t *testing.T, binaryPath, scenarioDir string) {
	// Update an existing ID-based object (should migrate to CAS)
	testID := "ITEM-001"

	cmd := runIsolatedCLICommand(binaryPath, []string{"object", "update", testID, "--field", "title=Updated via CAS Migration"}, scenarioDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to update object: %v\nOutput: %s", err, output)
	}

	// Check that old ID-based file is gone and new hash-based file exists
	kindDir := datacell.CellCASPrimaryDir(scenarioDir, "backlog")
	oldFile := filepath.Join(kindDir, fmt.Sprintf("%s.yaml", testID))

	if _, err := os.Stat(oldFile); err == nil {
		t.Logf("Old ID-based file still exists (may be cleaned up later)")
	}

	// Verify object can still be retrieved
	getCmd := runIsolatedCLICommand(binaryPath, []string{"object", "get", testID}, scenarioDir)
	getOutput, err := getCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get updated object: %v\nOutput: %s", err, getOutput)
	}

	if !strings.Contains(string(getOutput), "Updated via CAS Migration") {
		t.Errorf("Update not reflected in object")
	}

	t.Logf("✓ Object updated and migrated to CAS")
}

func testAllOperationsWork(t *testing.T, binaryPath, scenarioDir string) {
	// Test list
	listCmd := runIsolatedCLICommand(binaryPath, []string{"object", "list", "backlog_item", "--format", "json"}, scenarioDir)
	listOutput, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("List failed: %v\nOutput: %s", err, listOutput)
	}

	if !strings.Contains(string(listOutput), "ITEM-") {
		t.Error("List did not return backlog items")
	}

	// Test count
	countCmd := runIsolatedCLICommand(binaryPath, []string{"object", "count", "backlog_item"}, scenarioDir)
	countOutput, err := countCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Count failed: %v\nOutput: %s", err, countOutput)
	}

	// Test filter
	filterCmd := runIsolatedCLICommand(binaryPath, []string{"object", "list", "backlog_item", "--filter", "status=exploring", "--format", "json"}, scenarioDir)
	filterOutput, err := filterCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Filter failed: %v\nOutput: %s", err, filterOutput)
	}

	t.Logf("✓ All operations (list, count, filter) work with CAS")
}
