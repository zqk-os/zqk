package metrics

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type testSettingsYAMLShapeMetrics struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLMetrics(testRoot string) error {
	configDir := filepath.Join(testRoot, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return err
	}
	p := filepath.Join(configDir, paths.ZqkTestConfigFileName)
	body := testSettingsYAMLShapeMetrics{
		Version: paths.DefaultBrandSettingsVersion,
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
		t.Fatalf("config/zqk-test.yaml: %v", err)
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

func copyObjectSpecYAMLFilesToTestRootMetrics(testRoot, projectRoot string) error {
	return testenvroot.CopyObjectSpecsFromProject(testRoot, projectRoot)
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
