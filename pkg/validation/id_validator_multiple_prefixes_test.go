package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestIDValidatorMultiplePrefixesFromSpec tests the issue where a spec file
// defines multiple prefixes via id_prefixes, but the validator only recognizes
// the first prefix when generating error messages.
//
// This reproduces the bug where:
// - Spec file has: id_prefixes: [DEC-, ADR-]
// - Config has: decision: [DEC-, ADR-]
// - Validator should accept both DEC-001 and ADR-001
// - But error messages only show [DEC-] instead of [DEC-, ADR-]
func TestIDValidatorMultiplePrefixesFromSpec(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create the internal directory structure
	internalDir := datacell.CellCASPrimaryDir(projectRoot, "_internal")
	if err := os.MkdirAll(internalDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagicf379db0a, err)
	}

	// Create object_specs directory
	specsDir := filepath.Join(internalDir, "object_specs")
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagice9cc074e, err)
	}

	// Create configs directory (config file should be in configs/ subdirectory)
	configsDir := filepath.Join(internalDir, "configs")
	if err := os.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic2967e378, err)
	}

	// Create a spec file for decision with id_prefixes defined (matching real spec)
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

	// Create id_prefixes_config.yaml with both DEC- and ADR- prefixes (in configs/ subdirectory)
	configFile := filepath.Join(configsDir, ConstMagic014a7ae7)
	configContent := `version: "1.0.0"
kind_to_prefixes:
  decision:
    - DEC-
    - ADR-  # Architecture Decision Record format (backward compatibility)
`
	if err := os.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic8e96c016, err)
	}

	// Set the working directory to the project root so findIDPrefixesConfig can find it
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Create a validator with the test directory
	validator := NewIDValidator(specsDir)

	// Load patterns - this should load from our test config
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf(ConstMagic82cf76ae, err)
	}

	// Test 1: Check that GetValidPrefixes returns BOTH DEC- and ADR-
	prefixes := validator.GetValidPrefixes("decision")
	if len(prefixes) == 0 {
		t.Fatal(ConstMagicc1aec599)
	}

	// Both prefixes should be present
	foundDEC := false
	foundADR := false
	for _, prefix := range prefixes {
		if prefix == "DEC-" {
			foundDEC = true
		}
		if prefix == "ADR-" {
			foundADR = true
		}
	}

	if !foundDEC {
		t.Errorf(ConstMagic77520e0f, prefixes)
	}
	if !foundADR {
		t.Errorf(ConstMagicf55860c4, prefixes)
		t.Logf(ConstMagicc0789d77, prefixes)
	}

	// Test 2: Both DEC-001 and ADR-001 should be valid
	valid, err := validator.ValidateID("DEC-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	if !valid {
		t.Errorf(ConstMagic55aab9c2, prefixes)
	}

	valid, err = validator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	if !valid {
		t.Errorf(ConstMagica604e41b, prefixes)
		t.Log(ConstMagic9e4d03a6)
	}
}

// TestIDValidatorMultiplePrefixesEvolutionManagement tests the same issue
// for evolution_management objects which should accept both EVO- and EVOL-
func TestIDValidatorMultiplePrefixesEvolutionManagement(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create the internal directory structure
	internalDir := datacell.CellCASPrimaryDir(projectRoot, "_internal")
	if err := os.MkdirAll(internalDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagicf379db0a, err)
	}

	// Create object_specs directory
	specsDir := filepath.Join(internalDir, "object_specs")
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagice9cc074e, err)
	}

	// Create a spec file for evolution_management with id_prefixes defined
	specFile := filepath.Join(specsDir, ConstMagicfe1eed27)
	specContent := `ontology: evolution_management
schema_version: "` + objects.DefaultSchemaVersion + `"
id_template: EVOL-{sequence}
id_prefixes:
  - EVO-
  - EVOL-
`
	if err := os.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic186b95e3, err)
	}

	// Create configs directory (config file should be in configs/ subdirectory)
	configsDir := filepath.Join(internalDir, "configs")
	if err := os.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic2967e378, err)
	}

	// Create id_prefixes_config.yaml with both EVO- and EVOL- prefixes (in configs/ subdirectory)
	configFile := filepath.Join(configsDir, ConstMagic014a7ae7)
	configContent := `version: "1.0.0"
kind_to_prefixes:
  evolution_management:
    - EVO-
    - EVOL-  # Alternative format (backward compatibility)
`
	if err := os.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic8e96c016, err)
	}

	// Set the working directory to the project root
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Create a validator with the test directory
	validator := NewIDValidator(specsDir)

	// Load patterns
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf(ConstMagic82cf76ae, err)
	}

	// Check that GetValidPrefixes returns BOTH EVO- and EVOL-
	prefixes := validator.GetValidPrefixes(ConstMagicb8e0a828)
	if len(prefixes) == 0 {
		t.Fatal(ConstMagicc55105b2)
	}

	// Both prefixes should be present
	foundEVO := false
	foundEVOL := false
	for _, prefix := range prefixes {
		if prefix == "EVO-" {
			foundEVO = true
		}
		if prefix == "EVOL-" {
			foundEVOL = true
		}
	}

	if !foundEVO {
		t.Errorf(ConstMagic615189e5, prefixes)
	}
	if !foundEVOL {
		t.Errorf(ConstMagic621fdee6, prefixes)
		t.Logf(ConstMagicc0789d77, prefixes)
	}

	// Both EVO-001 and EVOL-001 should be valid
	valid, err := validator.ValidateID("EVO-001", ConstMagicb8e0a828)
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	if !valid {
		t.Errorf(ConstMagic6ce90070, prefixes)
	}

	valid, err = validator.ValidateID("EVOL-001", ConstMagicb8e0a828)
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	if !valid {
		t.Errorf(ConstMagic0becb71d, prefixes)
		t.Log(ConstMagicd3b34758)
	}
}
