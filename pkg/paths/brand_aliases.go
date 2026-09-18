package paths

import (
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

// DefaultBrandSettingsVersion is the default version used by generated/test brand settings.
const DefaultBrandSettingsVersion = "1.0.0"

type brandSettingsAliasesFile struct {
	Paths struct {
		ProjectRoot string            `yaml:"project_root"`
		Aliases     map[string]string `yaml:"aliases"`
	} `yaml:"paths"`
	KernelState struct {
		ProjectRoot string `yaml:"project_root"`
	} `yaml:"kernel_state"`
}

func configCandidates(projectRoot string) []string {
	return []string{
		filepath.Join(projectRoot, ConfigDir, ZqkLocalConfigFileName),
		filepath.Join(projectRoot, ConfigDir, ZqkConfigFileName),
		filepath.Join(projectRoot, ProjectDataDir, ConfigDir, BrandSettingsFilename),
		filepath.Join(projectRoot, BrandSettingsFilename),
	}
}

func testCandidates(projectRoot string) []string {
	return []string{
		filepath.Join(projectRoot, ConfigDir, ZqkTestConfigFileName),
		filepath.Join(projectRoot, ConfigDir, ZqkLocalConfigFileName),
		filepath.Join(projectRoot, ConfigDir, ZqkConfigFileName),
		filepath.Join(projectRoot, ProjectDataDir, ConfigDir, TestSettingsFilename),
		filepath.Join(projectRoot, ProjectDataDir, ConfigDir, ZqkTestSettingsFilename),
		filepath.Join(projectRoot, TestSettingsFilename),
		filepath.Join(projectRoot, ZqkTestSettingsFilename),
	}
}

// BrandSettingsPath returns the primary configuration file path for projectRoot.
// All configuration originates from config/** with environment overrides.
func BrandSettingsPath(projectRoot string) string {
	for _, p := range configCandidates(projectRoot) {
		if _, err := fileutil.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(projectRoot, ConfigDir, ZqkConfigFileName)
}

// LoadBrandSettingsProjectRoot reads paths.project_root or kernel_state.project_root from configuration.
// Returns empty string if missing, invalid, or empty.
func LoadBrandSettingsProjectRoot(projectRoot string) string {
	if projectRoot == emptyValue {
		return ""
	}
	candidates := configCandidates(projectRoot)
	testRoot := zqkenv.TestRoot().Get()
	if testRoot != emptyValue {
		absTest, err1 := filepath.Abs(testRoot)
		absProject, err2 := filepath.Abs(projectRoot)
		if err1 == nil && err2 == nil && absTest == absProject {
			candidates = testCandidates(projectRoot)
		}
	}
	for _, path := range candidates {
		data, err := fileutil.ReadFile(path)
		if err != nil {
			continue
		}
		var s brandSettingsAliasesFile
		if err := yaml.Unmarshal(data, &s); err != nil {
			continue
		}
		p := strings.TrimSpace(s.KernelState.ProjectRoot)
		if p == "" {
			p = strings.TrimSpace(s.Paths.ProjectRoot)
		}
		if p != "" {
			if filepath.IsAbs(p) {
				return filepath.Clean(p)
			}
			return filepath.Clean(filepath.Join(projectRoot, p))
		}
	}
	return ""
}

// LoadBrandPathAliases reads paths.aliases from configuration.
// Missing or invalid files return nil (callers merge onto defaults).
func LoadBrandPathAliases(projectRoot string) map[string]string {
	if projectRoot == emptyValue {
		return nil
	}
	candidates := configCandidates(projectRoot)
	testRoot := zqkenv.TestRoot().Get()
	if testRoot != emptyValue {
		absTest, err1 := filepath.Abs(testRoot)
		absProject, err2 := filepath.Abs(projectRoot)
		if err1 == nil && err2 == nil && absTest == absProject {
			candidates = testCandidates(projectRoot)
		}
	}
	for _, path := range candidates {
		data, err := fileutil.ReadFile(path)
		if err != nil {
			continue
		}
		var s brandSettingsAliasesFile
		if err := yaml.Unmarshal(data, &s); err != nil {
			continue
		}
		if len(s.Paths.Aliases) > 0 {
			return s.Paths.Aliases
		}
	}
	return nil
}

func settingsPathForRoot(projectRoot string) string {
	testRoot := zqkenv.TestRoot().Get()
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
	for _, p := range testCandidates(projectRoot) {
		if _, err := fileutil.Stat(p); err == nil {
			return p
		}
	}
	return testCandidates(projectRoot)[0]
}
