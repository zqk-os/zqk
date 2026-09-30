package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestReproduceActualBug reproduces the EXACT scenario from system check:
// 1. Spec file has id_prefixes: [DEC-, ADR-]
// 2. Config file has decision: [DEC-, ADR-]
// 3. GetGlobalIDPrefixesConfig() is called during parseSpecFile
// 4. If config load fails, it falls back to default (only DEC-)
// 5. This overwrites the spec's [DEC-, ADR-] with default's [DEC-]
// NOTE: Cannot use t.Parallel() - this test uses os.Chdir() which is incompatible with parallel execution
func TestReproduceActualBug(t *testing.T) {
	restoreIDValidatorGlobals(t)
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

	// Create spec with BOTH prefixes (matching actual project)
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

	// Create config with BOTH prefixes (matching actual project)
	// Config file should be in configs/ subdirectory per ProcessInternalConfigsDir
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

	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Create go.mod marker file to help path discovery
	goModFile := filepath.Join(projectRoot, "go.mod")
	if err := fileutil.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	// Reset global configs to ensure fresh loading (this is what ReloadPatterns does)
	ResetGlobalPathsConfig()
	ResetGlobalIDPrefixesConfig()

	// Trigger config load BEFORE parsing spec file (this ensures config is available)
	// GetGlobalIDPrefixesConfig() will load config from file if not already loaded
	_ = GetGlobalIDPrefixesConfig()

	// Create validator (this is what GetIDValidator does)
	validator := NewIDValidator(specsDir)

	// Simulate what parseSpecFile does:
	// 1. First, it reads id_prefixes from spec: [DEC-, ADR-]
	// 2. Then it calls GetGlobalIDPrefixesConfig() to override
	// 3. If GetGlobalIDPrefixesConfig() returns default (only DEC-), it overwrites!

	// Step 1: Parse spec file (this is what loadPatternsFromSpecsUnlocked does)
	config, err := validator.parseSpecFile(specFile)
	if err != nil {
		t.Fatalf("Failed to parse spec file: %v", err)
	}

	t.Logf("After parseSpecFile, prefixes: %v", config.Prefixes)

	// Check what GetGlobalIDPrefixesConfig returns
	globalConfig := GetGlobalIDPrefixesConfig()
	if globalConfig == nil {
		t.Fatal("GetGlobalIDPrefixesConfig() returned nil")
	}

	configPrefixes := globalConfig.GetPrefixesForKind("decision")
	t.Logf("GetGlobalIDPrefixesConfig() returns prefixes: %v", configPrefixes)

	// The bug: if config has fewer prefixes than spec, we shouldn't override
	// But currently, parseSpecFile always overrides with config, even if config is default
	if len(configPrefixes) < len(config.Prefixes) {
		t.Errorf("BUG REPRODUCED: Config has fewer prefixes (%d) than spec (%d)", len(configPrefixes), len(config.Prefixes))
		t.Errorf("Config prefixes: %v", configPrefixes)
		t.Errorf("Spec prefixes: %v", config.Prefixes)
		t.Error("This means GetGlobalIDPrefixesConfig() returned default config instead of file config")
	}

	// After ensureDefaultPatterns (this is what ReloadPatterns does)
	if err := validator.ReloadPatterns(); err != nil {
		t.Fatalf("ReloadPatterns() failed: %v", err)
	}

	finalPrefixes := validator.GetValidPrefixes("decision")
	t.Logf("After ReloadPatterns, final prefixes: %v", finalPrefixes)

	if len(finalPrefixes) != 2 {
		t.Errorf("Expected 2 prefixes, got %d: %v", len(finalPrefixes), finalPrefixes)
	}

	hasADR := false
	for _, p := range finalPrefixes {
		if p == "ADR-" {
			hasADR = true
			break
		}
	}

	if !hasADR {
		t.Errorf("BUG: Missing ADR- prefix after ReloadPatterns (prefixes: %v)", finalPrefixes)
	}
}
