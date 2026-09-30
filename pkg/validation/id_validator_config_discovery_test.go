package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestConfigFileDiscovery tests if findIDPrefixesConfig can find the config file
// from different working directories (simulating actual project usage)
// NOTE: Cannot use t.Parallel() - this test changes working directory which conflicts with parallel execution
func TestConfigFileDiscovery(t *testing.T) {
	restoreIDValidatorGlobals(t)
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	configsDir := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create configs directory: %v", err)
	}

	// Create a marker file (go.mod) so findPathByWalkingUp can identify project root
	goModFile := filepath.Join(projectRoot, "go.mod")
	if err := fileutil.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create go.mod marker: %v", err)
	}

	// Config file should be in configs/ subdirectory per ProcessInternalConfigsDir
	configFile := filepath.Join(configsDir, "id_prefixes_config.yaml")
	configContent := `version: "1.0.0"
kind_to_prefixes:
  decision:
    - DEC-
    - ADR-
`
	if err := fileutil.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer fileutil.Chdir(oldWd)

	// Test 1: From project root
	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()
	foundPath := findIDPrefixesConfig()
	t.Logf("From project root, found path: %s", foundPath)
	if foundPath == emptyValue {
		t.Error("Config file not found from project root")
	}

	// Test 2: From a subdirectory (simulating system check running from different location)
	subDir := filepath.Join(projectRoot, "some", "sub", "directory")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	if err := fileutil.Chdir(subDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()
	foundPath2 := findIDPrefixesConfig()
	t.Logf("From subdirectory, found path: %s", foundPath2)
	if foundPath2 == emptyValue {
		t.Error("Config file not found from subdirectory (should walk up to find it)")
	}

	// Test 3: Load config and verify it has both prefixes
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()
	config := GetGlobalIDPrefixesConfig()
	if config == nil {
		t.Fatal("GetGlobalIDPrefixesConfig() returned nil")
	}

	prefixes := config.GetPrefixesForKind("decision")
	t.Logf("Loaded prefixes: %v", prefixes)

	if len(prefixes) != 2 {
		t.Errorf("Expected 2 prefixes, got %d: %v", len(prefixes), prefixes)
	}
}
