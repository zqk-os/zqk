package paths

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/stampmemo"
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
	CLI struct {
		BinaryPath string `yaml:"binary_path"`
	} `yaml:"cli"`
	KernelState struct {
		ProjectRoot string `yaml:"project_root"`
	} `yaml:"kernel_state"`
}

var parsedBrandFiles stampmemo.Table[[]brandSettingsAliasesFile]

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

func testRootMatches(projectRoot string) bool {
	testRoot := strings.TrimSpace(zqkenv.TestRoot().Get())
	if testRoot == "" || projectRoot == "" {
		return false
	}
	absTest, err1 := filepath.Abs(testRoot)
	absProject, err2 := filepath.Abs(projectRoot)
	return err1 == nil && err2 == nil && absTest == absProject
}

func settingsCandidates(projectRoot string) []string {
	if testRootMatches(projectRoot) {
		return testCandidates(projectRoot)
	}
	return configCandidates(projectRoot)
}

func parsedBrandSettings(projectRoot string) []brandSettingsAliasesFile {
	cands := settingsCandidates(projectRoot)
	list, _ := parsedBrandFiles.Load(projectRoot, stampmemo.OfAll(cands...), func() ([]brandSettingsAliasesFile, error) {
		var out []brandSettingsAliasesFile
		for _, path := range cands {
			data, err := fileutil.ReadFile(path)
			if err != nil {
				continue
			}
			var s brandSettingsAliasesFile
			if yaml.Unmarshal(data, &s) != nil {
				continue
			}
			out = append(out, s)
		}
		return out, nil
	})
	return list
}

// BrandSettingsPath returns the primary configuration file path for projectRoot.
func BrandSettingsPath(projectRoot string) string {
	if hit := stampmemo.FirstExisting(configCandidates(projectRoot)); hit != "" {
		return hit
	}
	return filepath.Join(projectRoot, ConfigDir, ZqkConfigFileName)
}

// SettingsPathForRoot is the settings file LoadBrandSettings should open
// (test-settings when TEST_ROOT matches projectRoot).
func SettingsPathForRoot(projectRoot string) string {
	if testRootMatches(projectRoot) {
		if hit := stampmemo.FirstExisting(testCandidates(projectRoot)); hit != "" {
			return hit
		}
		return testCandidates(projectRoot)[0]
	}
	return BrandSettingsPath(projectRoot)
}

// LoadBrandSettingsProjectRoot reads paths.project_root or kernel_state.project_root from configuration.
func LoadBrandSettingsProjectRoot(projectRoot string) string {
	if projectRoot == emptyValue {
		return ""
	}
	for _, s := range parsedBrandSettings(projectRoot) {
		p := strings.TrimSpace(s.KernelState.ProjectRoot)
		if p == "" {
			p = strings.TrimSpace(s.Paths.ProjectRoot)
		}
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) {
			return filepath.Clean(p)
		}
		return filepath.Clean(filepath.Join(projectRoot, p))
	}
	return ""
}

// LoadBrandCLIBinaryPath reads cli.binary_path from configuration.
func LoadBrandCLIBinaryPath(projectRoot string) string {
	if projectRoot == emptyValue {
		return ""
	}
	for _, s := range parsedBrandSettings(projectRoot) {
		if p := strings.TrimSpace(s.CLI.BinaryPath); p != "" {
			return p
		}
	}
	return ""
}

// LoadBrandPathAliases reads paths.aliases from configuration.
func LoadBrandPathAliases(projectRoot string) map[string]string {
	if projectRoot == emptyValue {
		return nil
	}
	for _, s := range parsedBrandSettings(projectRoot) {
		if len(s.Paths.Aliases) > 0 {
			return s.Paths.Aliases
		}
	}
	return nil
}
