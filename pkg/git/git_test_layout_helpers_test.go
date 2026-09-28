package git

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/testkit"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/packs/work/bldr_v2"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type testSettingsYAMLShapeGit struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLGit(testRoot string) error {
	configDir := filepath.Join(testRoot, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return err
	}
	p := filepath.Join(configDir, paths.ZqkTestConfigFileName)
	body := testSettingsYAMLShapeGit{
		Version: paths.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

func layoutGitIntegrationTestRoot(t *testing.T, testRoot string) {
	t.Helper()
	absRoot, err := filepath.Abs(testRoot)
	if err != nil {
		t.Fatalf("abs test root: %v", err)
	}
	if err := paths.LayoutUnder(absRoot).
		Dir(paths.ProjectDataDir, paths.DirPerm755).
		Dir(paths.ProcessDir, paths.DirPerm755).
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Err(); err != nil {
		t.Fatalf("test project layout: %v", err)
	}
	if err := writeMinimalTestSettingsYAMLGit(absRoot); err != nil {
		t.Fatalf("config/zqk-test.yaml: %v", err)
	}
}

func moduleRootFromGoEnvGitOrEmpty(t *testing.T) string {
	t.Helper()
	out, err := testkit.ManagedCommand(t, t.Context(), "go", "env", "GOMOD").Output()
	if err != nil {
		t.Logf("go env GOMOD: %v", err)
		return ""
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" || modPath == "/dev/null" {
		return ""
	}
	return filepath.Dir(modPath)
}

func moduleRootFromGoEnvGit(t *testing.T) string {
	t.Helper()
	mod := moduleRootFromGoEnvGitOrEmpty(t)
	if mod == "" {
		t.Skip("no module root (GOMOD empty or not in module context)")
	}
	return mod
}

func yamlSpecExtensionOKGit(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}

func copyObjectSpecYAMLFilesToTestRootGit(testRoot, projectRoot string) error {
	sourceSpecsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	targetSpecsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := fileutil.Stat(sourceSpecsDir); fileutil.IsNotExist(err) {
		return fmt.Errorf("source specs directory does not exist: %s", sourceSpecsDir)
	}
	if err := fileutil.EnsureDir(targetSpecsDir); err != nil {
		return fmt.Errorf("failed to create target specs directory: %w", err)
	}
	entries, err := fileutil.ReadDir(sourceSpecsDir)
	if err != nil {
		return fmt.Errorf("failed to read source specs directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !yamlSpecExtensionOKGit(entry.Name()) {
			continue
		}
		sourcePath := filepath.Join(sourceSpecsDir, entry.Name())
		targetPath := filepath.Join(targetSpecsDir, entry.Name())
		data, err := fileutil.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read source file %s: %w", sourcePath, err)
		}
		if err := fileutil.WriteSecureFile(targetPath, data); err != nil { //nolint:gosec // spec files
			return fmt.Errorf("failed to write target file %s: %w", targetPath, err)
		}
	}

	// Also copy pack specs and lifecycles so pack-decoupled kinds (e.g. backlog_item, goal) resolve
	packsDir := filepath.Join(projectRoot, "packs")
	if _, err := fileutil.Stat(packsDir); err == nil {
		_ = filepath.Walk(packsDir, func(path string, info fileutil.FileInfo, wErr error) error {
			if wErr != nil || info == nil || info.IsDir() {
				return nil
			}
			if !yamlSpecExtensionOKGit(info.Name()) {
				return nil
			}
			rel, _ := filepath.Rel(projectRoot, path)
			targetPath := filepath.Join(testRoot, rel)
			_ = fileutil.EnsureDir(filepath.Dir(targetPath))
			if data, rErr := fileutil.ReadFile(path); rErr == nil {
				_ = fileutil.WriteSecureFile(targetPath, data)
				if strings.Contains(rel, "/specs/") {
					_ = fileutil.WriteSecureFile(filepath.Join(targetSpecsDir, info.Name()), data)
				}
				if strings.Contains(rel, "/lifecycles/") {
					targetLifecyclesDir := filepath.Join(testRoot, paths.ProcessInternalLifecyclesDir)
					_ = fileutil.EnsureDir(targetLifecyclesDir)
					_ = fileutil.WriteSecureFile(filepath.Join(targetLifecyclesDir, info.Name()), data)
				}
			}
			return nil
		})
	}
	return nil
}

// ensureObjectSpecsForGitIntegrationTest mirrors pkg/testing.CopySpecsToTestRoot with a full test layout
// (project data + test-settings + object_specs) and copies YAML from the module root.
func ensureObjectSpecsForGitIntegrationTest(t *testing.T, testRoot string) {
	t.Helper()
	layoutGitIntegrationTestRoot(t, testRoot)
	mod := moduleRootFromGoEnvGit(t)
	if err := copyObjectSpecYAMLFilesToTestRootGit(testRoot, mod); err != nil {
		t.Skipf("copy object specs (run from module checkout): %v", err)
	}
}
