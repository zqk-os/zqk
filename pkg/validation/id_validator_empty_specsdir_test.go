package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestGetIDValidatorWithEmptySpecsDir tests the exact scenario in system check:
// GetIDValidator() creates validator with empty specsDir, then ReloadPatterns()
func TestGetIDValidatorWithEmptySpecsDir(t *testing.T) {
	restoreIDValidatorGlobals(t)
	// No t.Parallel(): this test calls os.Chdir, which mutates the working directory of the
	// whole test binary. Running it concurrently makes every relative path read by another
	// parallel test resolve against an arbitrary directory.
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

	// Reset global config, paths config, and global validator
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()
	defer ResetGlobalPathsConfig()
	ResetGlobalIDValidatorForTest()

	// GetIDValidator with empty specsDir (this is what system check does)
	idValidator := GetIDValidator()
	if idValidator == nil {
		t.Fatal("GetIDValidator() returned nil")
	}

	t.Logf("Validator specsDir: %s", idValidator.specsDir)

	// ReloadPatterns (this is what system check does)
	if err := idValidator.ReloadPatterns(); err != nil {
		t.Fatalf("ReloadPatterns() failed: %v", err)
	}

	// Check prefixes
	validPrefixes := idValidator.GetValidPrefixes("decision")
	t.Logf("Valid prefixes: %v", validPrefixes)

	if len(validPrefixes) != 2 {
		t.Errorf("Expected 2 prefixes, got %d: %v", len(validPrefixes), validPrefixes)
	}

	// Test validation
	valid, err := idValidator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf("ValidateID() error: %v", err)
	}
	if !valid {
		t.Errorf("ADR-001 should be valid (prefixes: %v)", validPrefixes)
	}
}
