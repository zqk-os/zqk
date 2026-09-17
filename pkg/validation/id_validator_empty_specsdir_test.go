package validation

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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
		t.Fatalf(ConstMagice9cc074e, err)
	}

	// Create configs directory (config file should be in configs/ subdirectory)
	configsDir := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic2967e378, err)
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
		t.Fatalf(ConstMagic186b95e3, err)
	}

	// Create config with both prefixes (in configs/ subdirectory)
	configFile := filepath.Join(configsDir, ConstMagic014a7ae7)
	configContent := `version: "1.0.0"
kind_to_prefixes:
  decision:
    - DEC-
    - ADR-
`
	if err := fileutil.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic8e96c016, err)
	}

	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Reset global config, paths config, and global validator
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()
	defer ResetGlobalPathsConfig()
	ResetGlobalIDValidatorForTest()

	// GetIDValidator with empty specsDir (this is what system check does)
	idValidator := GetIDValidator()
	if idValidator == nil {
		t.Fatal(ConstMagice7eb6ae2)
	}

	t.Logf(ConstMagic83e1c533, idValidator.specsDir)

	// ReloadPatterns (this is what system check does)
	if err := idValidator.ReloadPatterns(); err != nil {
		t.Fatalf(ConstMagic342670e1, err)
	}

	// Check prefixes
	validPrefixes := idValidator.GetValidPrefixes("decision")
	t.Logf(ConstMagic217e901d, validPrefixes)

	if len(validPrefixes) != 2 {
		t.Errorf(ConstMagicfdb60e48, len(validPrefixes), validPrefixes)
	}

	// Test validation
	valid, err := idValidator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagice9712204, err)
	}
	if !valid {
		t.Errorf(ConstMagic86b6b453, validPrefixes)
	}
}
