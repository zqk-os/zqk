package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

// TestPopulateScenarioWithAllKinds copies all object specs to the test scenario
// and creates sample objects of each kind using the CLI
// This stages comprehensive test data that can be easily extended
func TestPopulateScenarioWithAllKinds(t *testing.T) {
	if zqkenv.PopulateScenarioTest().Get() != "1" {
		t.Skip("scenario population is opt-in; set ZQK_POPULATE_SCENARIO_TEST=1 to run")
	}
	// Module root for read-only spec/binary paths (not cwd-relative walk)
	projectRoot := moduleRootFromGoEnv(t)
	specPath := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir, "test_audit_aggregation_metric.yaml")
	if _, err := fileutil.Stat(specPath); err != nil {
		t.Fatalf("expected spec at %s: %v", specPath, err)
	}

	// Use test-scenarios directory
	scenarioDir := filepath.Join(projectRoot, "test-scenarios", "content-addressable-storage")

	// Check if scenario directory exists
	if _, err := fileutil.Stat(scenarioDir); err != nil {
		t.Skipf("Test scenario directory does not exist: %s (run TestContentAddressableStorage_ComprehensiveOperations first)", scenarioDir)
	}

	// Check if test binary exists
	testBinary := filepath.Join(scenarioDir, "zqk-admin-test-init")
	if _, err := fileutil.Stat(testBinary); err != nil {
		t.Fatalf("Test binary not found: %s (run: make build TARGET=content-addressable-storage)", testBinary)
	}

	t.Logf("Populating scenario with all object kinds: %s", scenarioDir)

	// Step 1: Copy all object specs to scenario
	t.Run("CopySpecs", func(t *testing.T) {
		if err := copyAllSpecsToScenario(projectRoot, scenarioDir); err != nil {
			t.Fatalf("Failed to copy specs: %v", err)
		}
	})

	// Step 2: Get all discoverable kinds
	registry := objects.GetGlobalFieldRegistry()
	if err := registry.LoadFields(); err != nil {
		t.Fatalf("Failed to load field registry: %v", err)
	}

	kinds, err := registry.GetAllKinds()
	if err != nil {
		t.Fatalf("Failed to get all kinds: %v", err)
	}

	t.Logf("Found %d object kinds to populate", len(kinds))

	// Step 3: Create sample objects for each kind using CLI
	t.Run("CreateObjects", func(t *testing.T) {
		createdCount := 0
		skippedCount := 0

		// Get ID prefix config to generate proper IDs
		idPrefixConfig := validation.GetGlobalIDPrefixesConfig()

		for _, kind := range kinds {
			// Skip internal/system kinds that shouldn't be created via CLI
			if isInternalKind(kind) {
				skippedCount++
				continue
			}

			// Get ID prefix for this kind
			prefixes := idPrefixConfig.GetPrefixesForKind(kind)
			if len(prefixes) == 0 {
				t.Logf("No ID prefix found for %s, skipping", kind)
				skippedCount++
				continue
			}
			prefix := prefixes[0] // Use first prefix

			// Create 2 sample objects of each kind
			for i := 1; i <= 2; i++ {
				// Generate proper ID with correct prefix
				objID := fmt.Sprintf("%s%03d", prefix, i)

				// Create object using CLI
				if err := createObjectViaCLI(t, testBinary, scenarioDir, kind, objID, i); err != nil {
					// Log but don't fail - some objects may have dependency requirements
					// This is expected and the test is designed to create what it can
					t.Logf("Skipped %s %s: %v", kind, objID, err)
					continue
				}
				createdCount++
			}
		}

		t.Logf("Scenario population complete:")
		t.Logf("  - Created: %d objects", createdCount)
		t.Logf("  - Skipped internal kinds: %d", skippedCount)
		t.Logf("  - Some objects may have been skipped due to dependency requirements")
		t.Logf("  - Test scenario data is now staged and can be extended manually")
	})
}

// copyAllSpecsToScenario copies all object specs from project to scenario
func copyAllSpecsToScenario(projectRoot, scenarioDir string) error {
	sourceSpecDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	targetSpecDir := filepath.Join(scenarioDir, paths.ProcessInternalObjectSpecsDir)

	// Ensure target directory exists
	if err := fileutil.MkdirAll(targetSpecDir, paths.DirPerm755); err != nil {
		return fmt.Errorf("failed to create target specs directory: %w", err)
	}

	// Read all spec files
	entries, err := fileutil.ReadDir(sourceSpecDir)
	if err != nil {
		return fmt.Errorf("failed to read source specs directory: %w", err)
	}

	copiedCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		sourcePath := filepath.Join(sourceSpecDir, entry.Name())
		targetPath := filepath.Join(targetSpecDir, entry.Name())

		// Read source file
		data, err := fileutil.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read spec file %s: %w", entry.Name(), err)
		}

		// Write to target
		if err := fileutil.WriteFile(targetPath, data, paths.FilePerm644); err != nil {
			return fmt.Errorf("failed to write spec file %s: %w", entry.Name(), err)
		}

		copiedCount++
	}

	fmt.Fprintf(os.Stdout, "Copied %d spec files to scenario\n", copiedCount)
	return nil
}

// createObjectViaCLI creates an object using the CLI binary
//
//nolint:unparam // t parameter is kept for API consistency with testing helpers
func createObjectViaCLI(_ *testing.T, binaryPath, scenarioDir, kind, objID string, index int) error {
	// Get field registry to build proper test object
	registry := objects.GetGlobalFieldRegistry()
	kindFields, err := registry.GetFieldsForKind(kind)
	if err != nil {
		return fmt.Errorf("failed to get fields for kind %s: %w", kind, err)
	}

	// Build object using test object builder logic
	obj := createTestObjectForCRUD(kind, objID, kindFields, index)

	// Marshal to YAML
	yamlData, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object to YAML: %w", err)
	}

	// Write to temp file
	tempFile := filepath.Join(scenarioDir, fmt.Sprintf(".temp-%s-%s.yaml", kind, objID))
	if err := fileutil.WriteFile(tempFile, yamlData, paths.FilePerm644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	defer fileutil.Remove(tempFile)

	// Use CLI to create object - syntax: zqk object create <kind> --file <file>
	// The ID is in the file, not as a separate argument
	cmd := RunIsolatedCLICommand(binaryPath, []string{"object", "create", kind, "--file", tempFile}, scenarioDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if object already exists (that's okay - test is idempotent)
		if strings.Contains(string(output), "already exists") ||
			strings.Contains(string(output), "Object already exists") {
			return nil
		}
		return fmt.Errorf("CLI create failed: %w\nOutput: %s", err, output)
	}

	return nil
}

// isInternalKind checks if a kind is internal and shouldn't be created via CLI
func isInternalKind(kind string) bool {
	internalKinds := map[string]bool{
		"audit_event":                   true,
		"change_journal_entry":          true,
		"audit_aggregation_metric":      true,
		"test_audit_aggregation_metric": true,
		"object_spec":                   true,
		"lifecycle":                     true,
		"base_object":                   true,
		"base_metric":                   true,
		"auditable":                     true,
	}
	return internalKinds[kind]
}
