package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSystemCheckScenario simulates exactly what system check does:
// 1. GetIDValidator() (gets global singleton)
// 2. ReloadPatterns()
// 3. ValidateID()
// 4. GetValidPrefixes() for error message
func TestSystemCheckScenario(t *testing.T) {
	restoreIDValidatorGlobals(t)
	// Not parallel: uses ResetGlobalIDPrefixesConfig + global ID validator singleton; parallel tests race and empty GetValidPrefixes.
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs directory: %v", err)
	}

	// Create configs directory (config file should be in configs/ subdirectory)
	configsDir := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create configs directory: %v", err)
	}

	// Create spec with both prefixes
	specFile := filepath.Join(specsDir, "decision.yaml")
	specContent := `ontology: decision
schema_version: "` + objects.DefaultSchemaVersion + `"
id_prefixes:
  - DEC-
  - ADR-
`
	if err := fileutil.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Create config with both prefixes (in configs/ subdirectory)
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

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Reset global config to simulate fresh start
	ResetGlobalIDPrefixesConfig()

	// Step 1: GetIDValidator() - this is what system check does
	idValidator := GetIDValidator()
	if idValidator == nil {
		t.Fatal("GetIDValidator() returned nil")
	}

	// Step 2: ReloadPatterns() - this is what system check does
	if err := idValidator.ReloadPatterns(); err != nil {
		t.Fatalf("ReloadPatterns() failed: %v", err)
	}

	// Step 3: ValidateID() - this is what system check does
	valid, err := idValidator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf("ValidateID() error: %v", err)
	}

	// Step 4: GetValidPrefixes() - this is what system check does for error message
	validPrefixes := idValidator.GetValidPrefixes("decision")

	t.Logf("Validation result for ADR-001: %v", valid)
	t.Logf("Valid prefixes returned: %v", validPrefixes)

	if !valid {
		t.Errorf("ADR-001 should be valid (prefixes: %v)", validPrefixes)
	}

	if len(validPrefixes) != 2 {
		t.Errorf("Expected 2 prefixes in error message, got %d: %v", len(validPrefixes), validPrefixes)
	}

	hasDEC := false
	hasADR := false
	for _, p := range validPrefixes {
		if p == "DEC-" {
			hasDEC = true
		}
		if p == "ADR-" {
			hasADR = true
		}
	}

	if !hasDEC {
		t.Error("Error message missing DEC- prefix")
	}
	if !hasADR {
		t.Error("Error message missing ADR- prefix")
	}
}
