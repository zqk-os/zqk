package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestActualProjectScenario tests with the actual project structure
// This simulates what happens when system check runs in the real project
// NOTE: Cannot use t.Parallel() - this test changes working directory which conflicts with parallel execution
func TestActualProjectScenario(t *testing.T) {
	restoreIDValidatorGlobals(t)
	// Find the actual project root
	projectRoot := findProjectRoot()
	if projectRoot == emptyValue {
		t.Skip("Could not find project root - skipping actual project test")
	}

	// Change to project root
	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Reset global config and validator to simulate fresh start
	ResetGlobalIDPrefixesConfig()
	// Note: Can't easily reset global validator, but ReloadPatterns should handle it

	// GetIDValidator (this is what system check does)
	idValidator := GetIDValidator()
	if idValidator == nil {
		t.Fatal("GetIDValidator() returned nil")
	}

	t.Logf("Validator specsDir: %s", idValidator.specsDir)

	// Check if config file exists
	configPath := findIDPrefixesConfig()
	t.Logf("Config file path: %s", configPath)
	if configPath == emptyValue {
		t.Fatal("Config file not found")
	}

	// ReloadPatterns (this is what system check does)
	if err := idValidator.ReloadPatterns(); err != nil {
		t.Fatalf("ReloadPatterns() failed: %v", err)
	}

	// Check prefixes for decision
	validPrefixes := idValidator.GetValidPrefixes("decision")
	t.Logf("Valid prefixes for decision: %v", len(validPrefixes))

	// Test validation
	valid, err := idValidator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf("ValidateID() error: %v", err)
	}

	t.Logf("ADR-001 validation result: %v", valid)
	t.Logf("Available prefixes: %v", validPrefixes)

	if len(validPrefixes) < 2 {
		t.Errorf("Expected at least 2 prefixes, got %d: %v", len(validPrefixes), validPrefixes)
	}

	hasADR := false
	for _, p := range validPrefixes {
		if p == "ADR-" {
			hasADR = true
			break
		}
	}

	if !hasADR {
		t.Errorf("Missing ADR- prefix (prefixes: %v)", validPrefixes)
	}

	if !valid {
		t.Errorf("ADR-001 should be valid (prefixes: %v)", validPrefixes)
	}
}

// findProjectRoot finds the project root by looking for go.mod or .zqk/specs
func findProjectRoot() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}

	dir := wd
	for {
		// Check for go.mod
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		// Check for .zqk/specs
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProcessInternalDir)); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}
