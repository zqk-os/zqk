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
		t.Skip(ConstMagic69146e4a)
	}

	// Change to project root
	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Reset global config and validator to simulate fresh start
	ResetGlobalIDPrefixesConfig()
	// Note: Can't easily reset global validator, but ReloadPatterns should handle it

	// GetIDValidator (this is what system check does)
	idValidator := GetIDValidator()
	if idValidator == nil {
		t.Fatal(ConstMagice7eb6ae2)
	}

	t.Logf(ConstMagic83e1c533, idValidator.specsDir)

	// Check if config file exists
	configPath := findIDPrefixesConfig()
	t.Logf(ConstMagicf0bc2244, configPath)
	if configPath == emptyValue {
		t.Fatal(ConstMagic42673eb3)
	}

	// ReloadPatterns (this is what system check does)
	if err := idValidator.ReloadPatterns(); err != nil {
		t.Fatalf(ConstMagic342670e1, err)
	}

	// Check prefixes for decision
	validPrefixes := idValidator.GetValidPrefixes("decision")
	t.Logf(ConstMagica971d6d9, len(validPrefixes))

	// Test validation
	valid, err := idValidator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagice9712204, err)
	}

	t.Logf(ConstMagicb7be8b15, valid)
	t.Logf(ConstMagic76a2973b, validPrefixes)

	if len(validPrefixes) < 2 {
		t.Errorf(ConstMagic60d9a1e5, len(validPrefixes), validPrefixes)
	}

	hasADR := false
	for _, p := range validPrefixes {
		if p == "ADR-" {
			hasADR = true
			break
		}
	}

	if !hasADR {
		t.Errorf(ConstMagicb4e8dac1, validPrefixes)
	}

	if !valid {
		t.Errorf(ConstMagic86b6b453, validPrefixes)
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
