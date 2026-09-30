package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestGlobalIDPrefixesConfigReload tests that the global config can be reloaded
// This is important because GetGlobalIDPrefixesConfig() memos by file stamp, which means
// a miss before the file exists stays the default until ResetGlobalIDPrefixesConfig or
// the discovered path's stamp moves.
// NOTE: Cannot use t.Parallel() - this test changes working directory which conflicts with parallel execution
func TestGlobalIDPrefixesConfigReload(t *testing.T) {
	restoreIDValidatorGlobals(t)
	// Reset the global config to ensure we start fresh
	ResetGlobalIDPrefixesConfig()

	// First, get the config without a file (should use default)
	config1 := GetGlobalIDPrefixesConfig()
	if config1 == nil {
		t.Fatal("GetGlobalIDPrefixesConfig() returned nil")
	}

	// Check that decision only has DEC- in default config
	decisionPrefixes1 := config1.GetPrefixesForKind("decision")
	if len(decisionPrefixes1) == 0 {
		t.Fatal("Default config should have DEC- for decision")
	}
	hasADR1 := false
	for _, p := range decisionPrefixes1 {
		if p == "ADR-" {
			hasADR1 = true
			break
		}
	}
	if hasADR1 {
		t.Log("Default config has ADR- (unexpected, but OK if default was updated)")
	}

	// Now create a config file with both DEC- and ADR-
	tmpDir := t.TempDir()
	configsDir := filepath.Join(tmpDir, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create configs directory: %v", err)
	}

	// Config file should be in configs/ subdirectory per ProcessInternalConfigsDir
	configFile := filepath.Join(configsDir, "id_prefixes_config.yaml")
	configContent := `version: "1.0.0"
kind_to_prefixes:
  decision:
    - DEC-
    - ADR-  # Architecture Decision Record format (backward compatibility)
`
	if err := fileutil.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Create a marker file (go.mod) so findPathByWalkingUp can identify project root
	goModFile := filepath.Join(tmpDir, "go.mod")
	if err := fileutil.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create go.mod marker: %v", err)
	}

	// Change to the temp directory so findIDPrefixesConfig can find it
	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Reset the global configs to force reload
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Now get the config again - it should load from file
	config2 := GetGlobalIDPrefixesConfig()
	if config2 == nil {
		t.Fatal("GetGlobalIDPrefixesConfig() returned nil after reload")
	}

	// Check that decision now has both DEC- and ADR-
	decisionPrefixes2 := config2.GetPrefixesForKind("decision")
	if len(decisionPrefixes2) < 2 {
		t.Errorf("Expected at least 2 prefixes for decision, got: %v", decisionPrefixes2)
	}

	hasDEC := false
	hasADR := false
	for _, p := range decisionPrefixes2 {
		if p == "DEC-" {
			hasDEC = true
		}
		if p == "ADR-" {
			hasADR = true
		}
	}

	if !hasDEC {
		t.Errorf("Expected DEC- in prefixes, got: %v", decisionPrefixes2)
	}
	if !hasADR {
		t.Errorf("Expected ADR- in prefixes after reload, got: %v", decisionPrefixes2)
		t.Log("This indicates the config reload is not working correctly")
	}
}
