package validation

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestConfigFileDiscovery tests if findIDPrefixesConfig can find the config file
// from different working directories (simulating actual project usage)
// NOTE: Cannot use t.Parallel() - this test changes working directory which conflicts with parallel execution
func TestConfigFileDiscovery(t *testing.T) {
	restoreIDValidatorGlobals(t)
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	configsDir := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagic2967e378, err)
	}

	// Create a marker file (go.mod) so findPathByWalkingUp can identify project root
	goModFile := filepath.Join(projectRoot, "go.mod")
	if err := fileutil.WriteFile(goModFile, []byte("module test\n"), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagica76bb0bc, err)
	}

	// Config file should be in configs/ subdirectory per ProcessInternalConfigsDir
	configFile := filepath.Join(configsDir, ConstMagic014a7ae7)
	configContent := `version: "1.0.0"
kind_to_prefixes:
  decision:
    - DEC-
    - ADR-
`
	if err := fileutil.WriteFile(configFile, []byte(configContent), paths.FilePerm644); err != nil {
		t.Fatalf(ConstMagic8e96c016, err)
	}

	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer fileutil.Chdir(oldWd)

	// Test 1: From project root
	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()
	foundPath := findIDPrefixesConfig()
	t.Logf(ConstMagic7b4155cb, foundPath)
	if foundPath == emptyValue {
		t.Error(ConstMagic50785345)
	}

	// Test 2: From a subdirectory (simulating system check running from different location)
	subDir := filepath.Join(projectRoot, "some", "sub", "directory")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf(ConstMagicbc403569, err)
	}

	if err := fileutil.Chdir(subDir); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()
	foundPath2 := findIDPrefixesConfig()
	t.Logf(ConstMagic425ba67b, foundPath2)
	if foundPath2 == emptyValue {
		t.Error(ConstMagic8f630751)
	}

	// Test 3: Load config and verify it has both prefixes
	ResetGlobalIDPrefixesConfig()
	ResetGlobalPathsConfig()
	config := GetGlobalIDPrefixesConfig()
	if config == nil {
		t.Fatal(ConstMagica6b206af)
	}

	prefixes := config.GetPrefixesForKind("decision")
	t.Logf(ConstMagic3e6eb5b5, prefixes)

	if len(prefixes) != 2 {
		t.Errorf(ConstMagicfdb60e48, len(prefixes), prefixes)
	}
}
