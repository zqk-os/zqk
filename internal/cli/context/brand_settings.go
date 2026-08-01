// Package context: brand settings ({brand}-settings.yaml) as the definitive resource for CLI orientation.
// See .zqk/cli/specs/schemas/brand_settings.schema.json and PATH_ALIAS_RESOLUTION.md §6.
package context

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

// DefaultBrandSettingsVersion is the default version used by generated/test brand settings.
const DefaultBrandSettingsVersion = "1.0.0"

// BrandSettingsPaths holds path-related options from brand settings.
type BrandSettingsPaths struct {
	ProjectRoot        string            `yaml:"project_root"`
	StalenessCheckDirs []string          `yaml:"staleness_check_dirs"`
	Aliases            map[string]string `yaml:"aliases"`
}

// BrandSettingsCLI holds CLI behavior options.
type BrandSettingsCLI struct {
	DefaultContext string `yaml:"default_context"`
	BinaryPath     string `yaml:"binary_path"`
}

// BrandSettings is the in-memory shape of {brand}-settings.yaml (e.g. zqk-settings.yaml).
type BrandSettings struct {
	Schema      string             `yaml:"$schema"`
	Description string             `yaml:"description"`
	Version     string             `yaml:"version"`
	Paths       BrandSettingsPaths `yaml:"paths"`
	CLI         BrandSettingsCLI   `yaml:"cli"`
}

// ProjectRoot returns the effective project root: settings paths.project_root if set and valid,
// otherwise the directory containing the settings file (dir). Relative paths (e.g. "docs", "cache")
// are only accepted if the resolved path contains .zqk; otherwise we return dir to avoid using
// a subdirectory as project root.
func (s *BrandSettings) ProjectRoot(dir string) string {
	if s.Paths.ProjectRoot == emptyValue {
		// If dir is .zqk/config, the project root is dir/../..
		if filepath.Base(dir) == paths.ConfigDir && filepath.Base(filepath.Dir(dir)) == paths.ProjectDataDir {
			return filepath.Dir(filepath.Dir(dir))
		}
		// Legacy fallback if the settings file is directly in the project root
		return dir
	}
	if filepath.IsAbs(s.Paths.ProjectRoot) {
		return filepath.Clean(s.Paths.ProjectRoot)
	}
	resolved := filepath.Clean(filepath.Join(dir, s.Paths.ProjectRoot))
	// Reject relative paths that point at a subdir without .zqk (e.g. project_root: "docs" or "cache")
	if _, err := os.Stat(filepath.Join(resolved, paths.ProjectDataDir)); err != nil {
		// Try to fallback to the auto-inferred root
		if filepath.Base(dir) == paths.ConfigDir && filepath.Base(filepath.Dir(dir)) == paths.ProjectDataDir {
			return filepath.Dir(filepath.Dir(dir))
		}
		return dir
	}
	return resolved
}

// BrandSettingsPath returns the absolute path to the brand settings file for the given project root.
func BrandSettingsPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.BrandSettingsFilename)
}

// TestSettingsPath returns the path to the test settings file at projectRoot.
// Used when ZQK_TEST_ROOT is set so tests never load or depend on zqk-settings.yaml.
func TestSettingsPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.TestSettingsFilename)
}

// settingsPathForRoot returns the path to the settings file to load for the given root.
// When projectRoot is the same as ZQK_TEST_ROOT, returns test-settings.yaml path; otherwise zqk-settings.yaml.
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

// resolveTestBrandSettingsPath picks the first existing file among test-settings.yaml and zqk-test-settings.yaml.
// If neither exists, returns test-settings.yaml path (for a clear missing-file error).
func resolveTestBrandSettingsPath(projectRoot string) string {
	candidates := []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.TestSettingsFilename),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.ZqkTestSettingsFilename),
		filepath.Join(projectRoot, paths.TestSettingsFilename),    // Legacy fallback
		filepath.Join(projectRoot, paths.ZqkTestSettingsFilename), // Legacy fallback
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return candidates[0]
}

// testRootMatchesProject reports whether ZQK_*_TEST_ROOT (brand-prefixed) points at projectRoot.
func testRootMatchesProject(projectRoot string) bool {
	testRoot := os.Getenv(zqkenv.TestRoot())
	if testRoot == emptyValue {
		return false
	}
	absTest, err1 := filepath.Abs(testRoot)
	absProject, err2 := filepath.Abs(projectRoot)
	return err1 == nil && err2 == nil && absTest == absProject
}

// minimalTestBrandSettingsYAML is the default content for test root settings (same schema as zqk-settings.yaml).
type minimalTestBrandSettingsYAML struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

// EnsureTestRootBrandSettingsFiles writes test-settings.yaml and zqk-test-settings.yaml when
// ZQK_*_TEST_ROOT matches projectRoot and either file is missing. Required for scheduler start
// and other commands that load brand settings in isolated test projects (e.g. zqk-ts + ZQK_TS_TEST_ROOT).
func EnsureTestRootBrandSettingsFiles(projectRoot string) error {
	if !testRootMatchesProject(projectRoot) {
		return nil
	}
	body := minimalTestBrandSettingsYAML{
		Version: DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(&body)
	if err != nil {
		return errfmt.Newf("marshal minimal test brand settings").Wrap(err)
	}
	for _, name := range []string{paths.TestSettingsFilename, paths.ZqkTestSettingsFilename} {
		p := filepath.Join(projectRoot, name)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if err := os.WriteFile(p, data, paths.FilePerm644); err != nil { //nolint:gosec // project-local settings template
			return errfmt.Newf("write %s", name).Wrap(err)
		}
	}
	return nil
}

// LoadBrandSettings loads and parses the brand settings file from projectRoot.
// When the branded TEST_ROOT env matches projectRoot, loads test-settings.yaml or zqk-test-settings.yaml; otherwise zqk-settings.yaml.
// Returns an error if the file is missing or invalid.
func LoadBrandSettings(projectRoot string) (*BrandSettings, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root is required to load brand settings")
	}
	path := settingsPathForRoot(projectRoot)
	s, _, err := LoadBrandSettingsFromFile(path)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// LoadBrandSettingsFromFile loads brand settings from an explicit settings file path and returns
// the settings and the effective project root derived from settings.Paths.ProjectRoot or the file's
// directory. Callers are responsible for enforcing workspace constraints (e.g. UnderProjectRoot).
func LoadBrandSettingsFromFile(settingsPath string) (*BrandSettings, string, error) {
	if settingsPath == emptyValue {
		return nil, "", errfmt.Errorf("settings path is required")
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", errfmt.Errorf("brand settings file missing: %s", settingsPath)
		}
		return nil, "", errfmt.Newf("read brand settings").Wrap(err)
	}
	var s BrandSettings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, "", errfmt.Newf("parse brand settings").Wrap(err)
	}
	if s.Version == emptyValue {
		return nil, "", errfmt.Errorf("brand settings must specify version (e.g. %s)", DefaultBrandSettingsVersion)
	}
	dir := filepath.Dir(settingsPath)
	projectRoot := s.ProjectRoot(dir)
	if _, err := os.Stat(filepath.Join(projectRoot, paths.ProjectDataDir)); err != nil {
		return nil, "", errfmt.Errorf("derived project root from settings does not contain %s: %s", paths.ProjectDataDir, projectRoot)
	}
	return &s, projectRoot, nil
}

// ResolveProjectRootFromSettings resolves project root by loading the brand settings file at the
// path given by ResolveProjectRoot(startPath) (e.g. .zqk/current_root or ZQK_PROJECT_ROOT) and
// returning the effective project root from settings (paths.project_root or the settings file dir).
// Use this so project root is always defined by zqk-settings.yaml. Returns error if hint root is
// missing or brand settings file is missing/invalid.
func ResolveProjectRootFromSettings(startPath string) (projectRoot string, settings *BrandSettings, err error) {
	hint := ResolveProjectRoot(startPath)
	if hint == emptyValue {
		return "", nil, errfmt.Errorf("project root not found")
	}
	s, err := LoadBrandSettings(hint)
	if err != nil {
		return "", nil, err
	}
	projectRoot = s.ProjectRoot(hint)
	return projectRoot, s, nil
}
