package validation

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestLoadConfigFromActualFile tests loading the actual config file from the project
func TestLoadConfigFromActualFile(t *testing.T) {
	restoreIDValidatorGlobals(t)
	// No t.Parallel(): this test calls os.Chdir, which mutates the working directory of the
	// whole test binary. Running it concurrently makes every relative path read by another
	// parallel test resolve against an arbitrary directory.
	// Find project root
	projectRoot := findProjectRootForTest()
	if projectRoot == emptyValue {
		t.Skip(ConstMagicc4ca6937)
	}

	configPath := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, ConstMagic014a7ae7)
	if _, err := fileutil.Stat(configPath); err != nil {
		t.Skipf(ConstMagic61786fc7, configPath)
	}

	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf(ConstMagic25cbf3ea, err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf(ConstMagic0b7b38b2, err)
	}

	// Reset global config
	ResetGlobalIDPrefixesConfig()

	// Try to load the config file directly
	config, err := LoadIDPrefixesConfig(configPath)
	if err != nil {
		t.Fatalf(ConstMagicbe132181, err)
	}

	if config == nil {
		t.Fatal(ConstMagic6c5ff97e)
	}

	// Check decision prefixes
	prefixes := config.GetPrefixesForKind("decision")
	t.Logf(ConstMagic2609e252, prefixes)

	if len(prefixes) < 2 {
		t.Errorf(ConstMagic134ce651, len(prefixes), prefixes)
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
		t.Errorf(ConstMagicb4e8dac1, prefixes)
	}

	// Now test GetGlobalIDPrefixesConfig
	ResetGlobalIDPrefixesConfig()
	globalConfig := GetGlobalIDPrefixesConfig()
	if globalConfig == nil {
		t.Fatal(ConstMagica6b206af)
	}

	globalPrefixes := globalConfig.GetPrefixesForKind("decision")
	t.Logf(ConstMagic391e9945, globalPrefixes)

	if len(globalPrefixes) < 2 {
		t.Errorf(ConstMagic27683689, globalPrefixes)
	}
}

func findProjectRootForTest() string {
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
