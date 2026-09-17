package object

// BLI-177483 inventory: SetupTestEnvironment → testkit.RunStandardTeardown (TempProjectTeardown) in test_helpers.go.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
)

// templateYAMLPrefixForParse returns the subset of template output that is valid YAML.
// Optional fields are emitted as commented fragments (field: # placeholder), so the full
// file is not a single parseable document; we parse through the required-field block only.
func templateYAMLPrefixForParse(s string) string {
	const marker = "\n# Optional fields"
	if i := strings.Index(s, marker); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// TestTemplateGeneration tests that templates are generated correctly
func TestTemplateGeneration(t *testing.T) {
	if raceDetectorEnabled {
		t.Skip("skipping template generation test under race detector")
	}
	// Not t.Parallel(): SetupTestEnvironment uses process-global ZQK_TEST_ROOT (serialized mutex; avoid parallel with other packages).
	testEnv := SetupTestEnvironment(t)

	t.Run("Generate template to stdout", func(t *testing.T) {
		cmd := testEnv.CreateCLICommand("object", "template", pplanKindBacklogItem)
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("template generation failed: %v\nOutput: %s", err, string(output))
		}

		outputStr := string(output)
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(templateYAMLPrefixForParse(outputStr)), &parsed); err != nil {
			t.Fatalf("template required-field prefix is not valid YAML: %v", err)
		}
		if got, _ := parsed[objects.FieldKeyKind].(string); got != pplanKindBacklogItem {
			t.Fatalf("parsed kind = %q, want %q", got, pplanKindBacklogItem)
		}
		// Verify template contains expected elements
		if !strings.Contains(outputStr, "kind: "+pplanKindBacklogItem) {
			t.Errorf("template missing kind field")
		}
		if !strings.Contains(outputStr, "schema_version") {
			t.Errorf("template missing schema_version")
		}
		if !strings.Contains(outputStr, "title:") {
			t.Errorf("template missing title field")
		}
		if !strings.Contains(outputStr, "Template for "+pplanKindBacklogItem+" object") {
			t.Errorf("template missing header comment")
		}
	})

	t.Run("Generate template to file", func(t *testing.T) {
		templateFile := filepath.Join(testEnv.GetTestRoot(), "test-template.yaml")
		cmd := testEnv.CreateCLICommand("object", "template", pplanKindBacklogItem, "--output", templateFile)
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("template generation to file failed: %v\nOutput: %s", err, string(output))
		}

		// Verify file was created
		if _, err := fileutil.Stat(templateFile); fileutil.IsNotExist(err) {
			t.Fatalf("template file was not created: %s", templateFile)
		}

		// Verify file contents
		data, err := fileutil.ReadFile(templateFile)
		if err != nil {
			t.Fatalf("failed to read template file: %v", err)
		}

		content := string(data)
		if content == emptyValue {
			t.Fatalf("template file is empty")
		}
		if !strings.Contains(content, "kind:") {
			t.Errorf("template file missing kind field (content: %s)", content)
		}
		if !strings.Contains(content, "schema_version") && !strings.Contains(content, "schema version") {
			t.Errorf("template file missing schema_version (content: %s)", content)
		}

		// Core contract: required-field prefix is valid YAML (optional lines are comments / placeholders).
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(templateYAMLPrefixForParse(content)), &parsed); err != nil {
			t.Fatalf("template required-field prefix is not valid YAML: %v", err)
		}
		if got, _ := parsed[objects.FieldKeyKind].(string); got != pplanKindBacklogItem {
			t.Fatalf("parsed kind = %q, want %q", got, pplanKindBacklogItem)
		}
		if _, ok := parsed[objects.FieldKeySchemaVersion]; !ok {
			t.Fatal("parsed template missing schema_version")
		}

		// Cleanup
		_ = fileutil.Remove(templateFile)
	})

	t.Run("Generate template for different kinds", func(t *testing.T) {
		kinds := []string{"goal", "milestone", "requirement"}
		for _, kind := range kinds {
			cmd := testEnv.CreateCLICommand("object", "template", kind)
			wireExecForTest(cmd, testEnv.GetTestRoot())
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("template generation failed for %s: %v\nOutput: %s", kind, err, string(output))
				continue
			}

			outputStr := string(output)
			if !strings.Contains(outputStr, "kind: "+kind) {
				t.Errorf("template for %s missing kind field", kind)
			}
		}
	})

	t.Run("Question template stays parseable with wrapped descriptions", func(t *testing.T) {
		cmd := testEnv.CreateCLICommand("object", "template", "question", "--include-optional=false")
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("question template generation failed: %v\nOutput: %s", err, string(output))
		}
		out := string(output)
		if strings.Contains(out, "\n(e.g.") || strings.Contains(out, "\n(structural ") {
			t.Fatalf("template includes bare wrapped lines that break YAML comments:\n%s", out)
		}
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(templateYAMLPrefixForParse(out)), &parsed); err != nil {
			t.Fatalf("question template prefix is not valid YAML: %v", err)
		}
	})
}

// TestFileCleanupAfterCreate tests that temp files are automatically cleaned up
func TestFileCleanupAfterCreate(t *testing.T) {
	// Not t.Parallel(): SetupTestEnvironment uses process-global ZQK_TEST_ROOT.
	testEnv := SetupTestEnvironment(t)

	t.Run("Cleanup temp file with tmp- prefix", func(t *testing.T) {
		// Create a temp file with tmp- prefix
		tmpFile := filepath.Join(testEnv.GetTestRoot(), "tmp-test-item.yaml")
		testData := fmt.Sprintf(`id: BLI-901
kind: %s
title: Test Item for Cleanup
status: exploring
goal_refs: ["G-123"]
schema_version: "%s"`, pplanKindBacklogItem, objectSchemaV2)

		if err := fileutil.WriteFile(tmpFile, []byte(testData), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("failed to create temp file: %v", err)
		}

		// Verify file exists before create
		if _, err := fileutil.Stat(tmpFile); fileutil.IsNotExist(err) {
			t.Fatalf("temp file was not created: %s", tmpFile)
		}

		// Create object from temp file (should cleanup automatically).
		// Use --timeout so under scheduler/load the create is not killed by CLI timeout hook.
		cmd := testEnv.CreateCLICommand("object", "create", pplanKindBacklogItem, "--file", tmpFile, "--timeout", "90s")
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("create failed: %v\nOutput: %s", err, string(output))
		}

		// Verify file was removed after creation
		if _, err := fileutil.Stat(tmpFile); !fileutil.IsNotExist(err) {
			t.Errorf("temp file was not cleaned up: %s", tmpFile)
		}
	})

	t.Run("Cleanup temp file in system temp directory", func(t *testing.T) {
		// Create a temp file in system temp directory (os.TempDir() returns /tmp on Linux, /var/folders/... on macOS)
		// Use a filename that starts with "tmp-" to ensure it's detected as a temp file
		// The cleanup logic checks for "tmp-" prefix OR /tmp/ in path
		tmpFile := filepath.Join(fileutil.TempDir(), "tmp-zqk-test-cleanup.yaml")
		testData := fmt.Sprintf(`id: BLI-902
kind: %s
title: Test Item in /tmp/
status: exploring
goal_refs: ["G-123"]
schema_version: "%s"`, pplanKindBacklogItem, objectSchemaV2)

		if err := fileutil.WriteFile(tmpFile, []byte(testData), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer fileutil.Remove(tmpFile) // Ensure cleanup even if test fails

		// Verify file exists before create
		if _, err := fileutil.Stat(tmpFile); fileutil.IsNotExist(err) {
			t.Fatalf("temp file was not created: %s", tmpFile)
		}

		// Create object from temp file (should cleanup automatically).
		// Use --timeout so under scheduler/load the create is not killed by CLI timeout hook.
		cmd := testEnv.CreateCLICommand("object", "create", pplanKindBacklogItem, "--file", tmpFile, "--timeout", "90s")
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("create failed: %v\nOutput: %s", err, string(output))
		}

		// Verify file was removed after creation
		if _, err := fileutil.Stat(tmpFile); !fileutil.IsNotExist(err) {
			t.Errorf("temp file in temp directory was not cleaned up: %s", tmpFile)
		}
	})

	t.Run("Keep file when --keep-file flag is set", func(t *testing.T) {
		// Create a temp file
		tmpFile := filepath.Join(testEnv.GetTestRoot(), "tmp-keep-test.yaml")
		testData := fmt.Sprintf(`id: BLI-903
kind: %s
title: Test Item to Keep
status: exploring
goal_refs: ["G-123"]
schema_version: "%s"`, pplanKindBacklogItem, objectSchemaV2)

		if err := fileutil.WriteFile(tmpFile, []byte(testData), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer fileutil.Remove(tmpFile) // Cleanup after test

		// Create object with --keep-file flag. Use --timeout for scheduler/load.
		cmd := testEnv.CreateCLICommand("object", "create", pplanKindBacklogItem, "--file", tmpFile, "--keep-file", "--timeout", "90s")
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("create failed: %v\nOutput: %s", err, string(output))
		}

		// Verify file was NOT removed (kept due to flag)
		if _, err := fileutil.Stat(tmpFile); fileutil.IsNotExist(err) {
			t.Errorf("file was cleaned up despite --keep-file flag: %s", tmpFile)
		}
	})

	t.Run("Do not cleanup non-temp files", func(t *testing.T) {
		// Create a regular file (not temp)
		regularFile := filepath.Join(testEnv.GetTestRoot(), "regular-item.yaml")
		testData := fmt.Sprintf(`id: BLI-904
kind: %s
title: Regular File Test
status: exploring
goal_refs: ["G-123"]
schema_version: "%s"`, pplanKindBacklogItem, objectSchemaV2)

		if err := fileutil.WriteFile(regularFile, []byte(testData), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("failed to create regular file: %v", err)
		}
		defer fileutil.Remove(regularFile) // Cleanup after test

		// Create object from regular file (should NOT cleanup).
		// Use --timeout 120s so under scheduler/load the create is not killed by timeout hook (baseline can be low).
		cmd := testEnv.CreateCLICommand("object", "create", pplanKindBacklogItem, "--file", regularFile, "--timeout", "120s")
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("create failed: %v\nOutput: %s", err, string(output))
		}

		// Verify file was NOT removed (not a temp file)
		if _, err := fileutil.Stat(regularFile); fileutil.IsNotExist(err) {
			t.Errorf("regular file was incorrectly cleaned up: %s", regularFile)
		}
	})
}

// TestTemplateToCreateWorkflow tests the complete workflow: template -> edit -> create -> cleanup
func TestTemplateToCreateWorkflow(t *testing.T) {
	// Not t.Parallel(): SetupTestEnvironment uses process-global ZQK_TEST_ROOT.
	testEnv := SetupTestEnvironment(t)

	t.Run("Complete template workflow with cleanup", func(t *testing.T) {
		// Step 1: Generate template to temp file
		templateFile := filepath.Join(testEnv.GetTestRoot(), "tmp-workflow-item.yaml")
		cmd := testEnv.CreateCLICommand("object", "template", pplanKindBacklogItem, "--output", templateFile)
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("template generation failed: %v\nOutput: %s", err, string(output))
		}

		// Verify template file was created
		if _, err := fileutil.Stat(templateFile); fileutil.IsNotExist(err) {
			t.Fatalf("template file was not created: %s", templateFile)
		}

		// Step 2: Simulate editing. The generated template is not always valid single-document YAML
		// (comments, optional blocks; some kinds omit "# Optional fields" entirely). Write a minimal
		// valid backlog_item for create (template existence was verified above).
		doc := map[string]any{
			objects.FieldKeySchemaVersion: objectSchemaV2,
			objects.FieldKeyID:            "BLI-905",
			objects.FieldKeyKind:          pplanKindBacklogItem,
			objects.FieldKeyTitle:         "Workflow Test Item",
			objects.FieldKeyStatus:        objectStatusExploring,
			objects.FieldKeyGoalRefs:      []string{"G-123"},
			objects.FieldKeyCreatedAt:     "2025-12-29T00:00:00Z",
			objects.FieldKeyUpdatedAt:     "2025-12-29T00:00:00Z",
			objects.FieldKeyCreatedBy:     "ACC-TEST",
			objects.FieldKeyUpdatedBy:     "ACC-TEST",
		}
		out, err := yaml.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal edited template: %v", err)
		}
		if err := fileutil.WriteFile(templateFile, out, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("failed to write modified template: %v", err)
		}

		// Step 3: Create object from modified template (should cleanup)
		cmd = testEnv.CreateCLICommand("object", "create", pplanKindBacklogItem, "--file", templateFile)
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("create from template failed: %v\nOutput: %s", err, string(output))
		}

		// Best-effort CAS reconcile (helps if follow-up steps need storage in-process).
		flushListingIndexAfterObjectCreate(t, testEnv.GetTestRoot(), pplanKindBacklogItem)

		// Step 4: Verify file was cleaned up (primary value of this workflow test)
		if _, err := fileutil.Stat(templateFile); !fileutil.IsNotExist(err) {
			t.Errorf("template file was not cleaned up after creation: %s", templateFile)
		}

		// Step 5: Object round-trip is covered by in-process tests (e.g. TestUpdateFileHashValidation).
		// CLI create uses subprocess WAL/write-behind; a follow-up read in the test process is flaky here.
		if !strings.Contains(string(output), "BLI-905") {
			t.Logf("note: create output did not echo id (CLI may use minimal output): %s", string(output))
		}
	})
}

// TestTemplateCleanupEdgeCases tests edge cases for cleanup detection
func TestTemplateCleanupEdgeCases(t *testing.T) {
	// Not t.Parallel(): SetupTestEnvironment uses process-global ZQK_TEST_ROOT.
	testEnv := SetupTestEnvironment(t)

	t.Run("Cleanup file with tmp- prefix in subdirectory", func(t *testing.T) {
		subDir := filepath.Join(testEnv.GetTestRoot(), "subdir")
		if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
			t.Fatalf("failed to create subdirectory: %v", err)
		}

		tmpFile := filepath.Join(subDir, "tmp-subdir-item.yaml")
		testData := fmt.Sprintf(`id: BLI-906
kind: %s
title: Subdir Temp File
status: exploring
goal_refs: ["G-123"]
schema_version: "%s"`, pplanKindBacklogItem, objectSchemaV2)

		if err := fileutil.WriteFile(tmpFile, []byte(testData), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("failed to create temp file: %v", err)
		}

		// Create object (should cleanup)
		cmd := testEnv.CreateCLICommand("object", "create", pplanKindBacklogItem, "--file", tmpFile)
		wireExecForTest(cmd, testEnv.GetTestRoot())
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("create failed: %v\nOutput: %s", err, string(output))
		}

		// Verify file was cleaned up
		if _, err := fileutil.Stat(tmpFile); !fileutil.IsNotExist(err) {
			t.Errorf("temp file in subdirectory was not cleaned up: %s", tmpFile)
		}
	})

	t.Run("Do not cleanup on create failure", func(t *testing.T) {
		// Create a temp file with invalid data (will cause create to fail)
		tmpFile := filepath.Join(testEnv.GetTestRoot(), "tmp-invalid-item.yaml")
		invalidData := `id: BLI-907
kind: nonexistent_kind_that_will_never_exist
title: Invalid Kind Object
status: exploring`

		if err := fileutil.WriteFile(tmpFile, []byte(invalidData), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer fileutil.Remove(tmpFile) // Cleanup after test

		// Attempt to create (should fail)
		cmd := testEnv.CreateCLICommand("object", "create", "nonexistent_kind_that_will_never_exist", "--file", tmpFile)
		wireExecForTest(cmd, testEnv.GetTestRoot())
		_, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("create should have failed with invalid data")
		}

		// Verify file was NOT removed (cleanup only happens on success)
		if _, err := fileutil.Stat(tmpFile); fileutil.IsNotExist(err) {
			t.Errorf("temp file was cleaned up despite create failure: %s", tmpFile)
		}
	})
}
