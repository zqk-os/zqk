package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestReproduceActualBug reproduces the EXACT scenario from system check:
// 1. Spec file has id_prefixes: [DEC-, ADR-]
// 2. Config file has decision: [DEC-, ADR-]
// 3. GetGlobalIDPrefixesConfig() is called during parseSpecFile
// 4. If config load fails, it falls back to default (only DEC-)
// 5. This overwrites the spec's [DEC-, ADR-] with default's [DEC-]
// NOTE: Cannot use t.Parallel() - this test uses os.Chdir() which is incompatible with parallel execution
func TestReproduceActualBug(t *testing.T) {
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

	// Create spec with BOTH prefixes (matching actual project)
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

	// Create config with BOTH prefixes (matching actual project)
	// Config file should be in configs/ subdirectory per ProcessInternalConfigsDir
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

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Create go.mod marker file to help path discovery
	goModFile := filepath.Join(projectRoot, "go.mod")
	if err := os.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic920f5712, err)
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
		t.Fatalf(ConstMagicece4de57, err)
	}

	t.Logf(ConstMagic77dd3f03, config.Prefixes)

	// Check what GetGlobalIDPrefixesConfig returns
	globalConfig := GetGlobalIDPrefixesConfig()
	if globalConfig == nil {
		t.Fatal(ConstMagica6b206af)
	}

	configPrefixes := globalConfig.GetPrefixesForKind("decision")
	t.Logf(ConstMagic391e9945, configPrefixes)

	// The bug: if config has fewer prefixes than spec, we shouldn't override
	// But currently, parseSpecFile always overrides with config, even if config is default
	if len(configPrefixes) < len(config.Prefixes) {
		t.Errorf(ConstMagic9f91b93b, len(configPrefixes), len(config.Prefixes))
		t.Errorf(ConstMagicda49d6a6, configPrefixes)
		t.Errorf(ConstMagic888df74e, config.Prefixes)
		t.Error(ConstMagic51a4e370)
	}

	// After ensureDefaultPatterns (this is what ReloadPatterns does)
	if err := validator.ReloadPatterns(); err != nil {
		t.Fatalf(ConstMagic342670e1, err)
	}

	finalPrefixes := validator.GetValidPrefixes("decision")
	t.Logf(ConstMagic5d994681, finalPrefixes)

	if len(finalPrefixes) != 2 {
		t.Errorf(ConstMagicfdb60e48, len(finalPrefixes), finalPrefixes)
	}

	hasADR := false
	for _, p := range finalPrefixes {
		if p == "ADR-" {
			hasADR = true
			break
		}
	}

	if !hasADR {
		t.Errorf(ConstMagic770197c6, finalPrefixes)
	}
}
