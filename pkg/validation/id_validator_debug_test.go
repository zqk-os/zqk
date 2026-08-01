package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestStep1_ConfigLoadsBothPrefixes tests if the config file loads both DEC- and ADR- correctly
func TestStep1_ConfigLoadsBothPrefixes(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	internalDir := datacell.CellCASPrimaryDir(tmpDir, "_internal")
	if err := os.MkdirAll(internalDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagicf379db0a, err)
	}

	// Create configs directory (config file should be in configs/ subdirectory)
	configsDir := filepath.Join(internalDir, "configs")
	if err := os.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic2967e378, err)
	}

	// Config file should be in configs/ subdirectory per ProcessInternalConfigsDir
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

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Reset global config
	ResetGlobalIDPrefixesConfig()

	// Load config
	config := GetGlobalIDPrefixesConfig()
	if config == nil {
		t.Fatal(ConstMagica6b206af)
	}

	prefixes := config.GetPrefixesForKind("decision")
	t.Logf(ConstMagic1e58e3ca, prefixes)

	if len(prefixes) != 2 {
		t.Errorf(ConstMagicfdb60e48, len(prefixes), prefixes)
	}

	hasDEC := false
	hasADR := false
	for _, p := range prefixes {
		if p == "DEC-" {
			hasDEC = true
		}
		if p == "ADR-" {
			hasADR = true
		}
	}

	if !hasDEC {
		t.Error(ConstMagic1065eb8d)
	}
	if !hasADR {
		t.Error(ConstMagicc52897f1)
	}
}

// TestStep2_ParseSpecFileOverridesWithConfig tests if parseSpecFile correctly overrides spec prefixes with config
func TestStep2_ParseSpecFileOverridesWithConfig(t *testing.T) {
	t.Parallel()
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

	// Reset global config
	ResetGlobalIDPrefixesConfig()

	// Create validator and parse spec file
	validator := NewIDValidator(specsDir)
	config, err := validator.parseSpecFile(specFile)
	if err != nil {
		t.Fatalf(ConstMagicece4de57, err)
	}

	t.Logf(ConstMagicc3864760, config.Prefixes)

	if len(config.Prefixes) != 2 {
		t.Errorf(ConstMagic4d240d1c, len(config.Prefixes), config.Prefixes)
	}

	hasDEC := false
	hasADR := false
	for _, p := range config.Prefixes {
		if p == "DEC-" {
			hasDEC = true
		}
		if p == "ADR-" {
			hasADR = true
		}
	}

	if !hasDEC {
		t.Error(ConstMagic6c57eb89)
	}
	if !hasADR {
		t.Error(ConstMagic8892eac1)
	}
}

// TestStep3_EnsureDefaultPatternsOverrides tests if ensureDefaultPatterns correctly overrides patterns
func TestStep3_EnsureDefaultPatternsOverrides(t *testing.T) {
	t.Parallel()
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

	// Reset global config
	ResetGlobalIDPrefixesConfig()

	// Create validator and load patterns
	validator := NewIDValidator(specsDir)
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf(ConstMagic82cf76ae, err)
	}

	// Check prefixes after ensureDefaultPatterns
	prefixes := validator.GetValidPrefixes("decision")
	t.Logf(ConstMagic9f3edb13, prefixes)

	if len(prefixes) != 2 {
		t.Errorf(ConstMagic6308c4fe, len(prefixes), prefixes)
	}

	hasDEC := false
	hasADR := false
	for _, p := range prefixes {
		if p == "DEC-" {
			hasDEC = true
		}
		if p == "ADR-" {
			hasADR = true
		}
	}

	if !hasDEC {
		t.Error(ConstMagic570bc43b)
	}
	if !hasADR {
		t.Error(ConstMagic890b7800)
	}
}

// TestStep4_ValidateIDAcceptsADR tests if ValidateID actually accepts ADR-001
func TestStep4_ValidateIDAcceptsADR(t *testing.T) {
	t.Parallel()
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

	// Reset global config
	ResetGlobalIDPrefixesConfig()

	// Create validator and load patterns
	validator := NewIDValidator(specsDir)
	if err := validator.LoadPatterns(); err != nil {
		t.Fatalf(ConstMagic82cf76ae, err)
	}

	// Test validation
	valid, err := validator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}

	prefixes := validator.GetValidPrefixes("decision")
	t.Logf(ConstMagic76a2973b, prefixes)
	t.Logf(ConstMagicb7be8b15, valid)

	if !valid {
		t.Errorf(ConstMagica604e41b, prefixes)
	}

	// Also test DEC-001
	valid, err = validator.ValidateID("DEC-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	if !valid {
		t.Error(ConstMagicd670938b)
	}
}

// TestStep5_ReloadPatternsResetsConfig tests if ReloadPatterns correctly resets and reloads config
func TestStep5_ReloadPatternsResetsConfig(t *testing.T) {
	t.Parallel()
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

	// Reset global config
	ResetGlobalIDPrefixesConfig()

	// Create validator
	validator := NewIDValidator(specsDir)

	// Reload patterns (this should reset config and reload)
	if err := validator.ReloadPatterns(); err != nil {
		t.Fatalf(ConstMagic48b58fba, err)
	}

	// Check prefixes
	prefixes := validator.GetValidPrefixes("decision")
	t.Logf(ConstMagicd1f8dd2c, prefixes)

	if len(prefixes) != 2 {
		t.Errorf(ConstMagicc18be18d, len(prefixes), prefixes)
	}

	// Test validation
	valid, err := validator.ValidateID("ADR-001", "decision")
	if err != nil {
		t.Fatalf(ConstMagicbcc473f4, err)
	}
	if !valid {
		t.Errorf(ConstMagic294e4e64, prefixes)
	}
}
