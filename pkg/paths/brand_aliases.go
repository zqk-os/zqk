package paths

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

// DefaultBrandSettingsVersion is the default version used by generated/test brand settings.
const DefaultBrandSettingsVersion = "1.0.0"

type brandSettingsAliasesFile struct {
	Paths struct {
		Aliases map[string]string `yaml:"aliases"`
	} `yaml:"paths"`
}

// BrandSettingsPath returns the absolute path to the brand settings file for the given project root.
func BrandSettingsPath(projectRoot string) string {
	return filepath.Join(projectRoot, ProjectDataDir, ConfigDir, BrandSettingsFilename)
}

// LoadBrandPathAliases reads paths.aliases from the brand (or test) settings file.
// Missing or invalid files return nil (callers merge onto defaults).
func LoadBrandPathAliases(projectRoot string) map[string]string {
	if projectRoot == emptyValue {
		return nil
	}
	path := settingsPathForRoot(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s brandSettingsAliasesFile
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil
	}
	if len(s.Paths.Aliases) == 0 {
		return nil
	}
	return s.Paths.Aliases
}

func settingsPathForRoot(projectRoot string) string {
	testRoot := os.Getenv(zqkenv.TestRoot())
	if testRoot != emptyValue {
		absTest, err1 := filepath.Abs(testRoot)
		absProject, err2 := filepath.Abs(projectRoot)
		if err1 == nil && err2 == nil && absTest == absProject {
			return resolveTestBrandSettingsPath(projectRoot)
		}
	}
	return BrandSettingsPath(projectRoot)
}

func resolveTestBrandSettingsPath(projectRoot string) string {
	candidates := []string{
		filepath.Join(projectRoot, ProjectDataDir, ConfigDir, TestSettingsFilename),
		filepath.Join(projectRoot, ProjectDataDir, ConfigDir, ZqkTestSettingsFilename),
		filepath.Join(projectRoot, TestSettingsFilename),
		filepath.Join(projectRoot, ZqkTestSettingsFilename),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return candidates[0]
}
