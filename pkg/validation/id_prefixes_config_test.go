package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
)

// TestGlobalIDPrefixesConfigReload tests that the global config can be reloaded
// This is important because GetGlobalIDPrefixesConfig() uses sync.Once, which means
// it only loads once. If the config is loaded before the actual config file exists,
// it will use the default config and never reload.
// NOTE: Cannot use t.Parallel() - this test changes working directory which conflicts with parallel execution
func TestGlobalIDPrefixesConfigReload(t *testing.T) {
	// Reset the global config to ensure we start fresh
	ResetGlobalIDPrefixesConfig()

	// First, get the config without a file (should use default)
	config1 := GetGlobalIDPrefixesConfig()
	if config1 == nil {
		t.Fatal(ConstMagica6b206af)
	}

	// Check that decision only has DEC- in default config
	decisionPrefixes1 := config1.GetPrefixesForKind("decision")
	if len(decisionPrefixes1) == 0 {
		t.Fatal(ConstMagic2925f437)
	}
	hasADR1 := false
	for _, p := range decisionPrefixes1 {
		if p == "ADR-" {
			hasADR1 = true
			break
		}
	}
	if hasADR1 {
		t.Log(ConstMagicea723a87)
	}

	// Now create a config file with both DEC- and ADR-
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
    - ADR-  # Architecture Decision Record format (backward compatibility)
`
	if err := os.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic8e96c016, err)
	}

	// Create a marker file (go.mod) so findPathByWalkingUp can identify project root
	goModFile := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagica76bb0bc, err)
	}

	// Change to the temp directory so findIDPrefixesConfig can find it
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Reset the global configs to force reload
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()

	// Now get the config again - it should load from file
	config2 := GetGlobalIDPrefixesConfig()
	if config2 == nil {
		t.Fatal(ConstMagicc1d54dbd)
	}

	// Check that decision now has both DEC- and ADR-
	decisionPrefixes2 := config2.GetPrefixesForKind("decision")
	if len(decisionPrefixes2) < 2 {
		t.Errorf(ConstMagic8e1db388, decisionPrefixes2)
	}

	hasDEC := false
	hasADR := false
	for _, p := range decisionPrefixes2 {
		if p == "DEC-" {
			hasDEC = true
		}
		if p == "ADR-" {
			hasADR = true
		}
	}

	if !hasDEC {
		t.Errorf(ConstMagic77520e0f, decisionPrefixes2)
	}
	if !hasADR {
		t.Errorf(ConstMagic6d4fd7eb, decisionPrefixes2)
		t.Log(ConstMagice4904ce0)
	}
}
