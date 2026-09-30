package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
	// No t.Parallel(): this test calls os.Chdir, which mutates the working directory of the
	// whole test binary. Running it concurrently makes every relative path read by another
	// parallel test resolve against an arbitrary directory.
	// Create a temporary test directory
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create object_specs directory
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs directory: %v", err)
	}

	// Create configs directory (config file should be in configs/ subdirectory)
	configsDir := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create configs directory: %v", err)
	}

	// Create a spec file for decision with id_prefixes defined (matching real spec)
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

	// Create id_prefixes_config.yaml with both DEC- and ADR- prefixes (in configs/ subdirectory)
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

	// Set the working directory to the project root so findIDPrefixesConfig can find it
	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Create a validator with the test directory
	validator := NewIDValidator(specsDir)

	// Load patterns - this should load from our test config
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf("Failed to load patterns: %v", err)
	}

	// Test 1: Check that GetValidPrefixes returns BOTH DEC- and ADR-
	prefixes := validator.GetValidPrefixes("decision")
	if len(prefixes) == 0 {
		t.Fatal("No prefixes found for decision")
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
		t.Errorf("Expected DEC- in prefixes, got: %v", prefixes)
	}
	if !foundADR {
		t.Errorf("Expected ADR- in prefixes, got: %v", prefixes)
		t.Logf("This is the bug: spec has both prefixes, but validator only shows: %v", prefixes)
	}

	// Test 2: Both DEC-001 and ADR-001 should be valid
	valid, err := validator.ValidateID("DEC-001", "decision")
	if err != nil {
		t.Fatalf("Validation error: %v", err)
	}
	if !valid {
		t.Errorf("DEC-001 should be valid for decision (prefixes: %v)", prefixes)
	}

	valid, err = validator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf("Validation error: %v", err)
	}
	if !valid {
		t.Errorf("ADR-001 should be valid for decision (prefixes: %v)", prefixes)
		t.Log("This is the bug: ADR-001 should be valid but validator rejected it")
	}
}

// TestIDValidatorMultiplePrefixesEvolutionManagement tests the same issue
// for evolution_management objects which should accept both EVO- and EVOL-
func TestIDValidatorMultiplePrefixesEvolutionManagement(t *testing.T) {
	// No t.Parallel(): this test calls os.Chdir, which mutates the working directory of the
	// whole test binary. Running it concurrently makes every relative path read by another
	// parallel test resolve against an arbitrary directory.
	// Create a temporary test directory
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create object_specs directory
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs directory: %v", err)
	}

	// Create a spec file for evolution_management with id_prefixes defined
	specFile := filepath.Join(specsDir, "evolution_management.yaml")
	specContent := `ontology: evolution_management
schema_version: "` + objects.DefaultSchemaVersion + `"
id_template: EVOL-{sequence}
id_prefixes:
  - EVO-
  - EVOL-
`
	if err := fileutil.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Create configs directory (config file should be in configs/ subdirectory)
	configsDir := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create configs directory: %v", err)
	}

	// Create id_prefixes_config.yaml with both EVO- and EVOL- prefixes (in configs/ subdirectory)
	configFile := filepath.Join(configsDir, "id_prefixes_config.yaml")
	configContent := `version: "1.0.0"
kind_to_prefixes:
  evolution_management:
    - EVO-
    - EVOL-  # Alternative format (backward compatibility)
`
	if err := fileutil.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set the working directory to the project root
	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Create a validator with the test directory
	validator := NewIDValidator(specsDir)

	// Load patterns
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf("Failed to load patterns: %v", err)
	}

	// Check that GetValidPrefixes returns BOTH EVO- and EVOL-
	prefixes := validator.GetValidPrefixes("evolution_management")
	if len(prefixes) == 0 {
		t.Fatal("No prefixes found for evolution_management")
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
		t.Errorf("Expected EVO- in prefixes, got: %v", prefixes)
	}
	if !foundEVOL {
		t.Errorf("Expected EVOL- in prefixes, got: %v", prefixes)
		t.Logf("This is the bug: spec has both prefixes, but validator only shows: %v", prefixes)
	}

	// Both EVO-001 and EVOL-001 should be valid
	valid, err := validator.ValidateID("EVO-001", "evolution_management")
	if err != nil {
		t.Fatalf("Validation error: %v", err)
	}
	if !valid {
		t.Errorf("EVO-001 should be valid for evolution_management (prefixes: %v)", prefixes)
	}

	valid, err = validator.ValidateID("EVOL-001", "evolution_management")
	if err != nil {
		t.Fatalf("Validation error: %v", err)
	}
	if !valid {
		t.Errorf("EVOL-001 should be valid for evolution_management (prefixes: %v)", prefixes)
		t.Log("This is the bug: EVOL-001 should be valid but validator rejected it")
	}
}
