package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestIDValidatorConfigOverride reproduces the issue where the validator
// uses inference rules (AGE-) instead of config prefixes (AGENT-ARCH-)
// for agent_architecture objects.
// NOTE: Cannot use t.Parallel() - this test changes working directory which conflicts with parallel execution
func TestIDValidatorConfigOverride(t *testing.T) {
	restoreIDValidatorGlobals(t)
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

	// Create a spec file for agent_architecture with id_pattern
	specFile := filepath.Join(specsDir, "agent_architecture.yaml")
	specContent := `ontology: agent_architecture
schema_version: "` + objects.DefaultSchemaVersion + `"
fields:
  id:
    validation:
      pattern: ^AGENT-ARCH-\d{3,}$
id_template: AGENT-ARCH-{sequence}
`
	if err := fileutil.WriteFile(specFile, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write spec file: %v", err)
	}

	// Create id_prefixes_config.yaml with AGENT-ARCH- prefix (in configs/ subdirectory)
	configFile := filepath.Join(configsDir, "id_prefixes_config.yaml")
	configContent := `version: "1.0.0"
kind_to_prefixes:
  agent_architecture:
    - AGENT-ARCH-  # Uses AGENT-ARCH-### format per spec
`
	if err := fileutil.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Create a marker file (go.mod) so findPathByWalkingUp can identify project root
	goModFile := filepath.Join(projectRoot, "go.mod")
	if err := fileutil.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create go.mod marker: %v", err)
	}

	// Reset global configs BEFORE changing directory (they cache working directory)
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Set the working directory to the project root so findIDPrefixesConfig can find it
	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Reset again after directory change to ensure fresh discovery
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Create a validator with the test directory
	validator := NewIDValidator(specsDir)

	// Load patterns - this should load from our test config
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf("Failed to load patterns: %v", err)
	}

	// Test 1: Check that GetValidPrefixes returns AGENT-ARCH-, not AGE-
	prefixes := validator.GetValidPrefixes("agent_architecture")
	if len(prefixes) == 0 {
		t.Fatal("No prefixes found for agent_architecture")
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
		t.Errorf("Expected AGENT-ARCH- in prefixes, got: %v", prefixes)
		t.Log("This indicates the config override is not working correctly")
		t.Logf("Config file location: %s", configFile)
		t.Logf("Working directory: %s", projectRoot)
	}

	// Test 2: Validate that AGENT-ARCH-001 is valid
	valid, err := validator.ValidateID("AGENT-ARCH-001", "agent_architecture")
	if err != nil {
		t.Fatalf("Validation error: %v", err)
	}
	if !valid {
		t.Error("AGENT-ARCH-001 should be valid for agent_architecture, but validator returned false")
		t.Logf("Prefixes available: %v", prefixes)
	}

	// Test 3: Validate that AGE-001 is NOT valid (if config override worked)
	valid, err = validator.ValidateID("AGE-001", "agent_architecture")
	if err != nil {
		t.Fatalf("Validation error: %v", err)
	}
	// If config override worked, AGE-001 should be invalid
	// If inference is still being used, AGE-001 might be valid (which is the bug)
	if valid && foundAGENTARCH {
		// This is inconsistent - we have AGENT-ARCH- in prefixes but AGE-001 is also valid
		t.Errorf("AGE-001 should not be valid if config override worked (prefixes: %v)", prefixes)
	}
}

// TestIDValidatorConfigOverrideDecision tests the same issue for decision objects
// which should accept both DEC- and ADR- prefixes from config
// NOTE: Cannot use t.Parallel() - this test changes working directory which conflicts with parallel execution
func TestIDValidatorConfigOverrideDecision(t *testing.T) {
	restoreIDValidatorGlobals(t)
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

	// Create a spec file for decision
	specFile := filepath.Join(specsDir, "decision.yaml")
	specContent := `ontology: decision
schema_version: "` + objects.DefaultSchemaVersion + `"
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

	// Create a marker file (go.mod) so findPathByWalkingUp can identify project root
	goModFile := filepath.Join(projectRoot, "go.mod")
	if err := fileutil.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create go.mod marker: %v", err)
	}

	// Reset global configs BEFORE changing directory (they cache working directory)
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Set the working directory to the project root so findIDPrefixesConfig can find it
	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Reset again after directory change to ensure fresh discovery
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Create a validator with the test directory
	validator := NewIDValidator(specsDir)

	// Load patterns - this should load from our test config
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf("Failed to load patterns: %v", err)
	}

	// Check that GetValidPrefixes returns both DEC- and ADR-
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
	}

	// Both DEC-001 and ADR-001 should be valid
	valid, err := validator.ValidateID("DEC-001", "decision")
	if err != nil {
		t.Fatalf("Validation error: %v", err)
	}
	if !valid {
		t.Error("DEC-001 should be valid for decision")
	}

	valid, err = validator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf("Validation error: %v", err)
	}
	if !valid {
		t.Errorf("ADR-001 should be valid for decision (prefixes: %v)", prefixes)
	}
}
