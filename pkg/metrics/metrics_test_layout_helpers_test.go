package metrics

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	clctx "github.com/lanceman/zqk/internal/cli/context"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

type testSettingsYAMLShapeMetrics struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLMetrics(testRoot string) error {
	p := filepath.Join(testRoot, paths.TestSettingsFilename)
	body := testSettingsYAMLShapeMetrics{
		Version: clctx.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

func layoutMetricsTestProjectRoot(t *testing.T, testRoot string) {
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
	if err := writeMinimalTestSettingsYAMLMetrics(absRoot); err != nil {
		t.Fatalf("test-settings.yaml: %v", err)
	}
}

func moduleRootFromGoEnvMetricsOrEmpty(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
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

func moduleRootFromGoEnvMetrics(t *testing.T) string {
	t.Helper()
	mod := moduleRootFromGoEnvMetricsOrEmpty(t)
	if mod == "" {
		t.Skip("no module root (GOMOD empty or not in module context)")
	}
	return mod
}

func yamlSpecExtensionOKMetrics(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}

func copyObjectSpecYAMLFilesToTestRootMetrics(testRoot, projectRoot string) error {
	sourceSpecsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	targetSpecsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := os.Stat(sourceSpecsDir); os.IsNotExist(err) {
		return fmt.Errorf("source specs directory does not exist: %s", sourceSpecsDir)
	}
	if err := fileutil.EnsureDir(targetSpecsDir); err != nil {
		return fmt.Errorf("failed to create target specs directory: %w", err)
	}
	entries, err := os.ReadDir(sourceSpecsDir)
	if err != nil {
		return fmt.Errorf("failed to read source specs directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !yamlSpecExtensionOKMetrics(entry.Name()) {
			continue
		}
		sourcePath := filepath.Join(sourceSpecsDir, entry.Name())
		targetPath := filepath.Join(targetSpecsDir, entry.Name())
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read source file %s: %w", sourcePath, err)
		}
		if err := fileutil.WriteSecureFile(targetPath, data); err != nil { //nolint:gosec // spec files
			return fmt.Errorf("failed to write target file %s: %w", targetPath, err)
		}
	}
	return nil
}

// ensureObjectSpecsForAIMetricsTest lays out a test project root and copies object_specs from the module (go env GOMOD).
func ensureObjectSpecsForAIMetricsTest(t *testing.T, testRoot string) {
	t.Helper()
	layoutMetricsTestProjectRoot(t, testRoot)
	mod := moduleRootFromGoEnvMetrics(t)
	if err := copyObjectSpecYAMLFilesToTestRootMetrics(testRoot, mod); err != nil {
		t.Skipf("copy object specs (run from module checkout): %v", err)
	}
}
