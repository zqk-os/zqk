// Package context: brand settings ({brand}-settings.yaml) as the definitive resource for CLI orientation.
// See .zqk/cli/specs/schemas/brand_settings.schema.json and PATH_ALIAS_RESOLUTION.md §6.
package context

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
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

// BrandSettingsKernelState holds Knowledge Kernel snapshot options (state-commit tip/priors).
type BrandSettingsKernelState struct {
	// ProjectRoot is the seated kernel project root when operating in a worktree.
	ProjectRoot string `yaml:"project_root"`
	// SnapshotBackupDir is where prior tip .csnap files are kept (absolute, ~/, or relative to project root).
	// Empty → CLI default (<parent>/<repo>-csnap-backups).
	SnapshotBackupDir string `yaml:"snapshot_backup_dir"`
	// SnapshotBackupKeep is how many prior tip copies to retain (0 = unset → CLI default).
	SnapshotBackupKeep int `yaml:"snapshot_backup_keep"`
}

// BrandSettings is the in-memory shape of {brand}-settings.yaml (e.g. zqk-settings.yaml).
type BrandSettings struct {
	Schema      string                   `yaml:"$schema"`
	Description string                   `yaml:"description"`
	Version     string                   `yaml:"version"`
	Paths       BrandSettingsPaths       `yaml:"paths"`
	CLI         BrandSettingsCLI         `yaml:"cli"`
	KernelState BrandSettingsKernelState `yaml:"kernel_state"`
}

// ResolveSnapshotBackupDir returns an absolute backup directory from settings, or "" if unset.
// Relative paths are resolved against projectRoot; "~" expands to the user home directory.
func (s *BrandSettings) ResolveSnapshotBackupDir(projectRoot string) string {
	if s == nil {
		return ""
	}
	raw := strings.TrimSpace(s.KernelState.SnapshotBackupDir)
	if raw == emptyValue {
		return ""
	}
	if strings.HasPrefix(raw, "~") {
		home, err := fileutil.UserHomeDir()
		if err == nil && home != emptyValue {
			raw = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(raw, "~"), string(filepath.Separator)))
		}
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	if projectRoot == emptyValue {
		return filepath.Clean(raw)
	}
	return filepath.Clean(filepath.Join(projectRoot, raw))
}

// ProjectRoot returns the effective project root: settings paths.project_root if set and valid,
// otherwise the directory containing the settings file (dir). Relative paths (e.g. "docs", "cache")
// are only accepted if the resolved path contains .zqk; otherwise we return dir to avoid using
// a subdirectory as project root.
func (s *BrandSettings) ProjectRoot(dir string) string {
	pRoot := s.Paths.ProjectRoot
	if pRoot == emptyValue {
		pRoot = s.KernelState.ProjectRoot
	}
	if pRoot == emptyValue {
		// If dir is product config/, project root is parent. If dir is still
		// named config under ProjectDataDir, project root is two levels up.
		if filepath.Base(dir) == "config" {
			if filepath.Base(filepath.Dir(dir)) == paths.ProjectDataDir {
				return filepath.Dir(filepath.Dir(dir))
			}
			return filepath.Dir(dir)
		}
		// Legacy fallback if the settings file is directly in the project root
		return dir
	}
	if filepath.IsAbs(pRoot) {
		return filepath.Clean(pRoot)
	}
	resolved := filepath.Clean(filepath.Join(dir, pRoot))
	// Reject relative paths that point at a subdir without .zqk (e.g. project_root: "docs" or "cache")
	if _, err := fileutil.Stat(filepath.Join(resolved, paths.ProjectDataDir)); err != nil {
		// Try to fallback to the auto-inferred root
		if filepath.Base(dir) == "config" {
			if filepath.Base(filepath.Dir(dir)) == paths.ProjectDataDir {
				return filepath.Dir(filepath.Dir(dir))
			}
			return filepath.Dir(dir)
		}
		return dir
	}
	return resolved
}

// BrandSettingsPath returns the absolute path to the brand settings file for the given project root.
func BrandSettingsPath(projectRoot string) string {
	return paths.BrandSettingsPath(projectRoot)
}

// LoadBrandSettings loads and parses the brand settings file from projectRoot.
// When the branded TEST_ROOT env matches projectRoot, loads test-settings.yaml or zqk-test-settings.yaml; otherwise zqk-settings.yaml.
// Returns an error if the file is missing or invalid.
func LoadBrandSettings(projectRoot string) (*BrandSettings, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root is required to load brand settings")
	}
	s, _, err := LoadBrandSettingsFromFile(paths.SettingsPathForRoot(projectRoot))
	if err != nil {
		return nil, err
	}
	return s, nil
}

// minimalTestBrandSettingsYAML is the default content for test root settings (same schema as zqk-settings.yaml).
type minimalTestBrandSettingsYAML struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func testRootMatchesProject(projectRoot string) bool {
	testRoot := zqkenv.TestRoot().Get()
	if testRoot == emptyValue {
		return false
	}
	absTest, err1 := filepath.Abs(testRoot)
	absProject, err2 := filepath.Abs(projectRoot)
	return err1 == nil && err2 == nil && absTest == absProject
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
	configDir := filepath.Join(projectRoot, "config")
	if err := fileutil.EnsureDir(configDir); err == nil {
		testYaml := filepath.Join(configDir, "zqk-test.yaml")
		if _, err := fileutil.Stat(testYaml); err != nil {
			_ = fileutil.WriteFile(testYaml, data, paths.FilePerm644)
		}
	}
	for _, name := range []string{paths.TestSettingsFilename, paths.ZqkTestSettingsFilename} {
		p := filepath.Join(projectRoot, name)
		if _, err := fileutil.Stat(p); err == nil {
			continue
		}
		if err := fileutil.WriteFile(p, data, paths.FilePerm644); err != nil { //nolint:gosec // project-local settings template
			return errfmt.Newf("write %s", name).Wrap(err)
		}
	}
	return nil
}

type brandFileHit struct {
	settings *BrandSettings
	root     string
}

// brandFiles is keyed by the settings file path. Stamp is that file.
var brandFiles stampmemo.Table[brandFileHit]

// LoadBrandSettingsFromFile loads brand settings from an explicit settings file path and returns
// the settings and the effective project root derived from settings.Paths.ProjectRoot or the file's
// directory. Callers are responsible for enforcing workspace constraints (e.g. UnderProjectRoot).
func LoadBrandSettingsFromFile(settingsPath string) (*BrandSettings, string, error) {
	if settingsPath == emptyValue {
		return nil, "", errfmt.Errorf("settings path is required")
	}
	hit, err := brandFiles.Load(settingsPath, stampmemo.Of(settingsPath), func() (brandFileHit, error) {
		data, err := fileutil.ReadFile(settingsPath)
		if err != nil {
			if fileutil.IsNotExist(err) {
				return brandFileHit{}, errfmt.Errorf("brand settings file missing: %s", settingsPath)
			}
			return brandFileHit{}, errfmt.Newf("read brand settings").Wrap(err)
		}
		var s BrandSettings
		if err := yaml.Unmarshal(data, &s); err != nil {
			return brandFileHit{}, errfmt.Newf("parse brand settings").Wrap(err)
		}
		if s.Version == emptyValue {
			s.Version = DefaultBrandSettingsVersion
		}
		if s.Paths.ProjectRoot == emptyValue && s.KernelState.ProjectRoot != emptyValue {
			s.Paths.ProjectRoot = s.KernelState.ProjectRoot
		}
		dir := filepath.Dir(settingsPath)
		projectRoot := s.ProjectRoot(dir)
		if _, err := fileutil.Stat(filepath.Join(projectRoot, paths.ProjectDataDir)); err != nil {
			return brandFileHit{}, errfmt.Errorf("derived project root from settings does not contain %s: %s", paths.ProjectDataDir, projectRoot)
		}
		return brandFileHit{settings: &s, root: projectRoot}, nil
	})
	if err != nil {
		return nil, "", err
	}
	return hit.settings, hit.root, nil
}

// ResolveProjectRootFromSettings resolves project root by loading the brand settings file at the
// path given by ResolveProjectRoot(startPath) (e.g. .zqk/current_root or ZQK_PROJECT_ROOT) and
// returning the effective project root from settings (paths.project_root or the settings file dir).
// Use this so project root is always defined by zqk-settings.yaml. Returns error if hint root is
// missing or brand settings file is missing/invalid.
func ResolveProjectRootFromSettings(startPath string) (projectRoot string, settings *BrandSettings, err error) {
	hint := ResolveProjectRoot(startPath)
	if hint == emptyValue {
		if envRoot := zqkenv.ProjectRoot().Get(); envRoot != "" && paths.IsAgentWorktreePath(envRoot) {
			return "", nil, errfmt.Errorf("agent worktree %s cannot serve as project root without seated kernel in brand settings (paths.project_root; see POL-AGENT-KERNEL-ROOT-BINDING-001)", envRoot)
		}
		if paths.IsAgentWorktreePath(startPath) {
			return "", nil, errfmt.Errorf("agent worktree %s cannot serve as project root without seated kernel in brand settings (paths.project_root; see POL-AGENT-KERNEL-ROOT-BINDING-001)", startPath)
		}
		return "", nil, errfmt.Errorf("project root not found")
	}
	s, err := LoadBrandSettings(hint)
	if err != nil {
		if paths.IsAgentWorktreePath(hint) {
			return "", nil, errfmt.Errorf("agent worktree %s cannot serve as project root without seated kernel in brand settings: %w (POL-AGENT-KERNEL-ROOT-BINDING-001)", hint, err)
		}
		return "", nil, err
	}
	projectRoot = s.ProjectRoot(hint)
	if paths.IsAgentWorktreePath(hint) {
		if strings.TrimSpace(s.Paths.ProjectRoot) == "" || projectRoot == hint {
			return "", nil, errfmt.Errorf("agent worktree %s cannot serve as project root: brand settings does not define paths.project_root (POL-AGENT-KERNEL-ROOT-BINDING-001)", hint)
		}
		if paths.IsAgentWorktreePath(projectRoot) {
			return "", nil, errfmt.Errorf("agent worktree %s paths.project_root %s cannot point to an agent worktree (POL-AGENT-KERNEL-ROOT-BINDING-001)", hint, projectRoot)
		}
		if !paths.IsValidProjectRoot(projectRoot) {
			return "", nil, errfmt.Errorf("agent worktree %s paths.project_root %s is not a valid project root (must contain .zqk) (POL-AGENT-KERNEL-ROOT-BINDING-001)", hint, projectRoot)
		}
	}
	return projectRoot, s, nil
}
