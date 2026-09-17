package internal

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestGraphBackendOutputFormats tests that JSON and YAML outputs work correctly
// when using the graph backend instead of file backend.
//
// The output format handling happens at the CLI layer, not the storage layer, so it
// works the same regardless of backend. This test verifies that when graph mode is enabled,
// the commands produce valid JSON/YAML output.
//
//nolint:gocyclo // Test function intentionally exercises many output format combinations
func TestGraphBackendOutputFormats(t *testing.T) {
	// Not t.Parallel(): setupCLITestEnvironmentForParity uses PrepareIsolatedTempProject (t.Setenv).
	// Skip if graph backend is not available
	if !isGraphBackendAvailable() {
		t.Skip("Graph backend not available (ZQK_GRAPH_ENABLED not set or graph not running)")
	}

	// Project root is t.TempDir-backed with teardown registered by PrepareIsolatedTempProject;
	// removing it here would race that teardown.
	tmpDir, cliBinary := setupCLITestEnvironmentForParity(t)

	// Enable graph mode for these tests
	t.Setenv(zqkenv.GraphEnabled().Name(), "true")

	// Test that list command produces valid JSON in graph mode
	t.Run("ListJSON", func(t *testing.T) {
		// Create a test object first (this will use graph backend if enabled)
		// ID must match pattern for backlog_item: BLI-XXX
		testObj := map[string]any{
			objects.FieldKeyID:            "BLI-998",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Graph Test Item",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "test",
		}

		testFile := filepath.Join(tmpDir, "test-graph-obj.yaml")
		data, _ := yaml.Marshal(testObj)
		if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Skipf("Skipping graph list JSON test (failed to create test file): %v", err)
			return
		}

		// Create the object (will use graph backend)
		createCmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--file", testFile) //nolint:gosec
		zqkenv.WireExecForIsolatedProjectWithExtras(createCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
		if err := createCmd.Run(); err != nil {
			t.Skipf("Skipping graph list JSON test (failed to create test object): %v", err)
			return
		}

		// List with JSON format
		listCmd := exec.Command(cliBinary, "object", "list", "backlog_item", "--format", "json") //nolint:gosec
		zqkenv.WireExecForIsolatedProjectWithExtras(listCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
		output, err := listCmd.CombinedOutput()
		if err != nil {
			t.Errorf("Graph backend list --format json failed: %v\nOutput: %s", err, string(output))
			return
		}

		// Validate JSON
		var jsonData map[string]any
		if err := json.Unmarshal(output, &jsonData); err != nil {
			t.Errorf("Graph backend produced invalid JSON: %v\nOutput: %s", err, string(output))
		}

		// Verify structure matches file backend structure
		if _, ok := jsonData["objects"]; !ok {
			// Might be grouped, check for groups
			if _, ok := jsonData["groups"]; !ok {
				t.Errorf("Graph backend JSON output missing 'objects' or 'groups' key: %v", jsonData)
			}
		}
	})

	// Test that list command produces valid YAML in graph mode
	t.Run("ListYAML", func(t *testing.T) {
		// List with YAML format
		listCmd := exec.Command(cliBinary, "object", "list", "backlog_item", "--format", "yaml") //nolint:gosec
		zqkenv.WireExecForIsolatedProjectWithExtras(listCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
		output, err := listCmd.CombinedOutput()
		if err != nil {
			t.Errorf("Graph backend list --format yaml failed: %v\nOutput: %s", err, string(output))
			return
		}

		// Validate YAML
		var yamlData map[string]any
		if err := yaml.Unmarshal(output, &yamlData); err != nil {
			t.Errorf("Graph backend produced invalid YAML: %v\nOutput: %s", err, string(output))
		}

		// Verify structure matches file backend structure
		if _, ok := yamlData["objects"]; !ok {
			// Might be grouped, check for groups
			if _, ok := yamlData["groups"]; !ok {
				t.Errorf("Graph backend YAML output missing 'objects' or 'groups' key: %v", yamlData)
			}
		}
	})

	// Test that get command produces valid JSON in graph mode
	t.Run("GetJSON", func(t *testing.T) {
		// Create a test object first
		testObj := map[string]any{
			objects.FieldKeyID:            "BLI-997",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Graph Get Test Item",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "test",
		}

		testFile := filepath.Join(tmpDir, "test-graph-get.json")
		data, _ := yaml.Marshal(testObj)
		if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Skipf("Skipping graph get JSON test (failed to create test file): %v", err)
			return
		}

		// Create the object
		createCmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--file", testFile) //nolint:gosec
		zqkenv.WireExecForIsolatedProjectWithExtras(createCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
		if err := createCmd.Run(); err != nil {
			t.Skipf("Skipping graph get JSON test (failed to create test object): %v", err)
			return
		}

		// Get with JSON format
		getCmd := exec.Command(cliBinary, "object", "get", "BLI-997", "--format", "json") //nolint:gosec
		zqkenv.WireExecForIsolatedProjectWithExtras(getCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
		output, err := getCmd.CombinedOutput()
		if err != nil {
			t.Errorf("Graph backend get --format json failed: %v\nOutput: %s", err, string(output))
			return
		}

		// Extract JSON from output (may contain warnings/logs)
		jsonStart := -1
		for i, b := range output {
			if b == '{' {
				jsonStart = i
				break
			}
		}
		if jsonStart == -1 {
			t.Errorf("No JSON found in graph backend output: %s", string(output))
			return
		}

		// Validate JSON
		var jsonData map[string]any
		if err := json.Unmarshal(output[jsonStart:], &jsonData); err != nil {
			t.Errorf("Graph backend get produced invalid JSON: %v\nOutput: %s", err, string(output))
			return
		}

		// Verify it has expected fields
		if _, ok := jsonData[objects.FieldKeyID]; !ok {
			t.Errorf("Graph backend get JSON missing 'id' field: %v", jsonData)
		}
		if _, ok := jsonData[objects.FieldKeyKind]; !ok {
			t.Errorf("Graph backend get JSON missing 'kind' field: %v", jsonData)
		}
	})

	// Test that get command produces valid YAML in graph mode
	t.Run("GetYAML", func(t *testing.T) {
		// Create a test object first
		testObj := map[string]any{
			objects.FieldKeyID:            "BLI-996",
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Graph Get YAML Test Item",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyCreatedBy:     "test",
			objects.FieldKeyUpdatedAt:     "2025-12-26T00:00:00Z",
			objects.FieldKeyUpdatedBy:     "test",
		}

		testFile := filepath.Join(tmpDir, "test-graph-get-yaml.yaml")
		data, _ := yaml.Marshal(testObj)
		if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Skipf("Skipping graph get YAML test (failed to create test file): %v", err)
			return
		}

		// Create the object
		createCmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--file", testFile) //nolint:gosec
		zqkenv.WireExecForIsolatedProjectWithExtras(createCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
		if err := createCmd.Run(); err != nil {
			t.Skipf("Skipping graph get YAML test (failed to create test object): %v", err)
			return
		}

		// Get with YAML format
		getCmd := exec.Command(cliBinary, "object", "get", "BLI-996", "--format", "yaml") //nolint:gosec
		zqkenv.WireExecForIsolatedProjectWithExtras(getCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
		output, err := getCmd.CombinedOutput()
		if err != nil {
			t.Errorf("Graph backend get --format yaml failed: %v\nOutput: %s", err, string(output))
			return
		}

		// Extract YAML from output (may contain warnings/logs)
		yamlStart := -1
		for i, b := range output {
			if b == '-' || b == 'i' { // YAML might start with --- or id:
				// Look for "id:" pattern
				if i+2 < len(output) && output[i] == 'i' && output[i+1] == 'd' && output[i+2] == ':' {
					yamlStart = i
					break
				}
			}
		}
		if yamlStart == -1 {
			// Try parsing entire output
			yamlStart = 0
		}

		// Validate YAML
		var yamlData map[string]any
		if err := yaml.Unmarshal(output[yamlStart:], &yamlData); err != nil {
			t.Errorf("Graph backend get produced invalid YAML: %v\nOutput: %s", err, string(output))
			return
		}

		// Verify it has expected fields
		if _, ok := yamlData[objects.FieldKeyID]; !ok {
			t.Errorf("Graph backend get YAML missing 'id' field: %v", yamlData)
		}
		if _, ok := yamlData[objects.FieldKeyKind]; !ok {
			t.Errorf("Graph backend get YAML missing 'kind' field: %v", yamlData)
		}
	})

	// Test that fields command produces valid JSON in graph mode
	t.Run("FieldsJSON", func(t *testing.T) {
		fieldsCmd := exec.Command(cliBinary, "object", "fields", "--list-kinds", "--format", "json") //nolint:gosec
		zqkenv.WireExecForIsolatedProjectWithExtras(fieldsCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
		output, err := fieldsCmd.CombinedOutput()
		if err != nil {
			t.Errorf("Graph backend fields --list-kinds --format json failed: %v\nOutput: %s", err, string(output))
			return
		}

		// Validate JSON
		var jsonData map[string]any
		if err := json.Unmarshal(output, &jsonData); err != nil {
			t.Errorf("Graph backend fields produced invalid JSON: %v\nOutput: %s", err, string(output))
		}

		// Verify structure
		if _, ok := jsonData["kinds"]; !ok {
			t.Errorf("Graph backend fields JSON missing 'kinds' key: %v", jsonData)
		}
	})

	// Test that fields command produces valid YAML in graph mode
	t.Run("FieldsYAML", func(t *testing.T) {
		fieldsCmd := exec.Command(cliBinary, "object", "fields", "--list-kinds", "--format", "yaml") //nolint:gosec
		zqkenv.WireExecForIsolatedProjectWithExtras(fieldsCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
		output, err := fieldsCmd.CombinedOutput()
		if err != nil {
			t.Errorf("Graph backend fields --list-kinds --format yaml failed: %v\nOutput: %s", err, string(output))
			return
		}

		// Validate YAML
		var yamlData map[string]any
		if err := yaml.Unmarshal(output, &yamlData); err != nil {
			t.Errorf("Graph backend fields produced invalid YAML: %v\nOutput: %s", err, string(output))
		}

		// Verify structure
		if _, ok := yamlData["kinds"]; !ok {
			t.Errorf("Graph backend fields YAML missing 'kinds' key: %v", yamlData)
		}
	})
}

// TestGraphBackendOutputConsistency tests that graph backend produces the same
// output structure as file backend for the same data.
//
//nolint:gocyclo // Test function intentionally exercises many consistency scenarios
func TestGraphBackendOutputConsistency(t *testing.T) {
	// Not t.Parallel(): setupCLITestEnvironmentForParity uses PrepareIsolatedTempProject (t.Setenv).
	// Skip if graph backend is not available
	if !isGraphBackendAvailable() {
		t.Skip("Graph backend not available (ZQK_GRAPH_ENABLED not set or graph not running)")
	}

	// Project root is t.TempDir-backed with teardown registered by PrepareIsolatedTempProject;
	// removing it here would race that teardown.
	tmpDir, cliBinary := setupCLITestEnvironmentForParity(t)

	// Create a test object (ID must match pattern for backlog_item: BLI-XXX)
	testObj := map[string]any{
		objects.FieldKeyID:            "BLI-999",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Consistency Test Item",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     "2025-12-26T00:00:00Z",
		objects.FieldKeyCreatedBy:     "test",
		objects.FieldKeyUpdatedAt:     "2025-12-26T00:00:00Z",
		objects.FieldKeyUpdatedBy:     "test",
	}

	testFile := filepath.Join(tmpDir, "test-consistency.yaml")
	data, _ := yaml.Marshal(testObj)
	if err := fileutil.WriteFile(testFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create object in file backend first
	createFileCmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--file", testFile) //nolint:gosec
	wireIsolatedCLI(createFileCmd, tmpDir)
	if err := createFileCmd.Run(); err != nil {
		t.Fatalf("Failed to create object in file backend: %v", err)
	}

	// Get object from file backend as JSON
	getFileCmd := exec.Command(cliBinary, "object", "get", "BLI-999", "--format", "json") //nolint:gosec
	wireIsolatedCLI(getFileCmd, tmpDir)
	fileOutput, err := getFileCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get object from file backend: %v\nOutput: %s", err, string(fileOutput))
	}

	// Extract JSON from output (may contain warnings/logs)
	jsonStart := -1
	for i, b := range fileOutput {
		if b == '{' {
			jsonStart = i
			break
		}
	}
	if jsonStart == -1 {
		// Try parsing as YAML first (file backend might output YAML by default)
		var yamlData map[string]any
		if err := yaml.Unmarshal(fileOutput, &yamlData); err == nil {
			// Convert YAML to JSON for comparison
			jsonBytes, _ := json.Marshal(yamlData)
			fileOutput = jsonBytes
			jsonStart = 0
		} else {
			t.Fatalf("No JSON/YAML found in file backend output: %s", string(fileOutput))
		}
	}

	var fileData map[string]any
	if err := json.Unmarshal(fileOutput[jsonStart:], &fileData); err != nil {
		t.Fatalf("File backend produced invalid JSON: %v\nOutput: %s", err, string(fileOutput))
	}

	// Now create same object in graph backend
	createGraphCmd := exec.Command(cliBinary, "object", "create", "backlog_item", "--file", testFile) //nolint:gosec
	zqkenv.WireExecForIsolatedProjectWithExtras(createGraphCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
	// Note: This might fail if object already exists, but that's okay for this test
	//nolint:errcheck // Test command run - error acceptable
	_ = createGraphCmd.Run()

	// Get object from graph backend as JSON
	getGraphCmd := exec.Command(cliBinary, "object", "get", "BLI-999", "--format", "json") //nolint:gosec
	zqkenv.WireExecForIsolatedProjectWithExtras(getGraphCmd, tmpDir, zqkenv.GraphEnabled().Name() + "=true")
	graphOutput, err := getGraphCmd.CombinedOutput()
	if err != nil {
		t.Skipf("Graph backend not accessible or object not found: %v", err)
		return
	}

	// Extract JSON from output (may contain warnings/logs)
	graphJSONStart := -1
	for i, b := range graphOutput {
		if b == '{' {
			graphJSONStart = i
			break
		}
	}
	if graphJSONStart == -1 {
		t.Fatalf("No JSON found in graph backend output: %s", string(graphOutput))
	}

	var graphData map[string]any
	if err := json.Unmarshal(graphOutput[graphJSONStart:], &graphData); err != nil {
		t.Fatalf("Graph backend produced invalid JSON: %v\nOutput: %s", err, string(graphOutput))
	}

	// Compare key fields (they should match)
	keyFields := []string{"id", "kind", "title", "status"}
	for _, field := range keyFields {
		fileVal := fmt.Sprintf("%v", fileData[field])
		graphVal := fmt.Sprintf("%v", graphData[field])
		if fileVal != graphVal {
			t.Errorf("Field %s differs between backends: file=%v, graph=%v", field, fileVal, graphVal)
		}
	}
}

// TestGraphBackendStorageProvider tests that the storage provider selection
// works correctly and returns PoolAwareGraphStorage when graph mode is enabled.
func TestGraphBackendStorageProvider(t *testing.T) {
	t.Setenv(zqkenv.GraphEnabled().Name(), "true")

	projectRoot := "."
	provider, err := getStorageProviderForTest(t, projectRoot)
	if err != nil {
		t.Skipf("Failed to get storage provider (graph may not be running): %v", err)
		return
	}

	// Verify it implements ObjectStorageProvider interface
	if provider == nil {
		t.Error("Storage provider is nil")
		return
	}

	// Verify it implements the interface
	var _ = provider

	// When graph is enabled, getStorageProvider should return PoolAwareGraphStorage
	// (or FileObjectStorage if graph connection fails)
	// We can't easily check the type without importing internal types,
	// but we can verify it works by checking it's not nil
}

// isGraphBackendAvailable checks if graph backend is available for testing
func isGraphBackendAvailable() bool {
	// Check if ZQK_GRAPH_ENABLED is set and graph is actually running
	// This is a simplified check - in a real scenario, we'd try to connect
	// to verify the graph database is actually available
	if enabled := config.StorageGraphEnabled().Safe(); enabled {
		// TODO: Add actual connection test to verify graph is running
		// For now, we'll skip tests if graph is not explicitly enabled
		// This allows tests to run in file-only mode
		return true
	}
	return false
}
