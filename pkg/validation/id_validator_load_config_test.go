package validation

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
		t.Skip("Could not find project root - skipping actual file test")
	}

	configPath := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, "id_prefixes_config.yaml")
	if _, err := fileutil.Stat(configPath); err != nil {
		t.Skipf("Config file not found at %s - skipping", configPath)
	}

	oldWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer fileutil.Chdir(oldWd)

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change working directory: %v", err)
	}

	// Reset global config
	ResetGlobalIDPrefixesConfig()

	// Try to load the config file directly
	config, err := LoadIDPrefixesConfig(configPath)
	if err != nil {
		t.Fatalf("LoadIDPrefixesConfig() failed: %v", err)
	}

	if config == nil {
		t.Fatal("LoadIDPrefixesConfig() returned nil")
	}

	// Check decision prefixes
	prefixes := config.GetPrefixesForKind("decision")
	t.Logf("Loaded prefixes from actual file: %v", prefixes)

	if len(prefixes) < 2 {
		t.Errorf("Expected at least 2 prefixes for decision, got %d: %v", len(prefixes), prefixes)
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
		t.Error("Missing DEC- prefix")
	}
	if !hasADR {
		t.Errorf("Missing ADR- prefix (prefixes: %v)", prefixes)
	}

	// Now test GetGlobalIDPrefixesConfig
	ResetGlobalIDPrefixesConfig()
	globalConfig := GetGlobalIDPrefixesConfig()
	if globalConfig == nil {
		t.Fatal("GetGlobalIDPrefixesConfig() returned nil")
	}

	globalPrefixes := globalConfig.GetPrefixesForKind("decision")
	t.Logf("GetGlobalIDPrefixesConfig() returns prefixes: %v", globalPrefixes)

	if len(globalPrefixes) < 2 {
		t.Errorf("GetGlobalIDPrefixesConfig() returned fewer prefixes than expected: %v", globalPrefixes)
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
