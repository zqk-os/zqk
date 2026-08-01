package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestSystemCheckScenario simulates exactly what system check does:
// 1. GetIDValidator() (gets global singleton)
// 2. ReloadPatterns()
// 3. ValidateID()
// 4. GetValidPrefixes() for error message
func TestSystemCheckScenario(t *testing.T) {
	// Not parallel: uses ResetGlobalIDPrefixesConfig + global ID validator singleton; parallel tests race and empty GetValidPrefixes.
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	internalDir := datacell.CellCASPrimaryDir(projectRoot, "_internal")
	if err := os.MkdirAll(internalDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagicf379db0a, err)
	}

	specsDir := filepath.Join(internalDir, "object_specs")
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagice9cc074e, err)
	}

	// Create configs directory (config file should be in configs/ subdirectory)
	configsDir := filepath.Join(internalDir, "configs")
	if err := os.MkdirAll(configsDir, paths.DirPerm755); err != nil {
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
	if err := os.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
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
	if err := os.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic8e96c016, err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Reset global config to simulate fresh start
	ResetGlobalIDPrefixesConfig()

	// Step 1: GetIDValidator() - this is what system check does
	idValidator := GetIDValidator()
	if idValidator == nil {
		t.Fatal(ConstMagice7eb6ae2)
	}

	// Step 2: ReloadPatterns() - this is what system check does
	if err := idValidator.ReloadPatterns(); err != nil {
		t.Fatalf(ConstMagic342670e1, err)
	}

	// Step 3: ValidateID() - this is what system check does
	valid, err := idValidator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagice9712204, err)
	}

	// Step 4: GetValidPrefixes() - this is what system check does for error message
	validPrefixes := idValidator.GetValidPrefixes("decision")

	t.Logf(ConstMagic11fe312a, valid)
	t.Logf(ConstMagicf77c052d, validPrefixes)

	if !valid {
		t.Errorf(ConstMagic86b6b453, validPrefixes)
	}

	if len(validPrefixes) != 2 {
		t.Errorf(ConstMagic03464b04, len(validPrefixes), validPrefixes)
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
		t.Error(ConstMagic7ad19585)
	}
	if !hasADR {
		t.Error(ConstMagicfeddf376)
	}
}
