package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestIDValidatorConfigOverride reproduces the issue where the validator
// uses inference rules (AGE-) instead of config prefixes (AGENT-ARCH-)
// for agent_architecture objects.
// NOTE: Cannot use t.Parallel() - this test changes working directory which conflicts with parallel execution
func TestIDValidatorConfigOverride(t *testing.T) {
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

	// Create a spec file for agent_architecture with id_pattern
	specFile := filepath.Join(specsDir, ConstMagic6ee6fd0e)
	specContent := `ontology: agent_architecture
schema_version: "` + objects.DefaultSchemaVersion + `"
fields:
  id:
    validation:
      pattern: ^AGENT-ARCH-\d{3,}$
id_template: AGENT-ARCH-{sequence}
`
	if err := os.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic186b95e3, err)
	}

	// Create id_prefixes_config.yaml with AGENT-ARCH- prefix (in configs/ subdirectory)
	configFile := filepath.Join(configsDir, ConstMagic014a7ae7)
	configContent := `version: "1.0.0"
kind_to_prefixes:
  agent_architecture:
    - AGENT-ARCH-  # Uses AGENT-ARCH-### format per spec
`
	if err := os.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic8e96c016, err)
	}

	// Create a marker file (go.mod) so findPathByWalkingUp can identify project root
	goModFile := filepath.Join(projectRoot, "go.mod")
	if err := os.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagica76bb0bc, err)
	}

	// Reset global configs BEFORE changing directory (they cache working directory)
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Set the working directory to the project root so findIDPrefixesConfig can find it
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Reset again after directory change to ensure fresh discovery
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Create a validator with the test directory
	validator := NewIDValidator(specsDir)

	// Load patterns - this should load from our test config
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf(ConstMagic82cf76ae, err)
	}

	// Test 1: Check that GetValidPrefixes returns AGENT-ARCH-, not AGE-
	prefixes := validator.GetValidPrefixes(ConstMagic3255806e)
	if len(prefixes) == 0 {
		t.Fatal(ConstMagic218b3979)
	}

	// The config should override the inferred prefix
	// Expected: AGENT-ARCH- (from config)
	// Actual (bug): AGE- (from inference)
	foundAGENTARCH := false
	for _, prefix := range prefixes {
		if prefix == "AGENT-ARCH-" {
			foundAGENTARCH = true
			break
		}
	}

	if !foundAGENTARCH {
		t.Errorf(ConstMagic082c21ff, prefixes)
		t.Log(ConstMagic400c8616)
		t.Logf(ConstMagic69338a88, configFile)
		t.Logf(ConstMagicfda8fd12, projectRoot)
	}

	// Test 2: Validate that AGENT-ARCH-001 is valid
	valid, err := validator.ValidateID("AGENT-ARCH-001", ConstMagic3255806e)
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	if !valid {
		t.Error(ConstMagic21b3fb21)
		t.Logf(ConstMagicc5092f9f, prefixes)
	}

	// Test 3: Validate that AGE-001 is NOT valid (if config override worked)
	valid, err = validator.ValidateID("AGE-001", ConstMagic3255806e)
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	// If config override worked, AGE-001 should be invalid
	// If inference is still being used, AGE-001 might be valid (which is the bug)
	if valid && foundAGENTARCH {
		// This is inconsistent - we have AGENT-ARCH- in prefixes but AGE-001 is also valid
		t.Errorf(ConstMagic0d7e82be, prefixes)
	}
}

// TestIDValidatorConfigOverrideDecision tests the same issue for decision objects
// which should accept both DEC- and ADR- prefixes from config
// NOTE: Cannot use t.Parallel() - this test changes working directory which conflicts with parallel execution
func TestIDValidatorConfigOverrideDecision(t *testing.T) {
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

	// Create a spec file for decision
	specFile := filepath.Join(specsDir, "decision.yaml")
	specContent := `ontology: decision
schema_version: "` + objects.DefaultSchemaVersion + `"
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

	// Create a marker file (go.mod) so findPathByWalkingUp can identify project root
	goModFile := filepath.Join(projectRoot, "go.mod")
	if err := os.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagica76bb0bc, err)
	}

	// Reset global configs BEFORE changing directory (they cache working directory)
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Set the working directory to the project root so findIDPrefixesConfig can find it
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Reset again after directory change to ensure fresh discovery
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Create a validator with the test directory
	validator := NewIDValidator(specsDir)

	// Load patterns - this should load from our test config
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf(ConstMagic82cf76ae, err)
	}

	// Check that GetValidPrefixes returns both DEC- and ADR-
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
	}

	// Both DEC-001 and ADR-001 should be valid
	valid, err := validator.ValidateID("DEC-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	if !valid {
		t.Error(ConstMagicd670938b)
	}

	valid, err = validator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	if !valid {
		t.Errorf(ConstMagica604e41b, prefixes)
	}
}
