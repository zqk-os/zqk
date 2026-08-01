package system

import (
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clctx "github.com/lanceman/zqk/internal/cli/context"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

type testSettingsYAMLShapeSystem struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLSystem(testRoot string) error {
	p := filepath.Join(testRoot, paths.TestSettingsFilename)
	body := testSettingsYAMLShapeSystem{
		Version: clctx.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

// setupSystemTestEnvironmentRoot mirrors pkg/testing.SetupTestEnvironment: project layout + test-settings.yaml only.
func setupSystemTestEnvironmentRoot(testRoot string) (string, error) {
	absRoot, err := filepath.Abs(testRoot)
	if err != nil {
		return "", fmt.Errorf("failed to resolve test root path: %w", err)
	}
	if err := paths.LayoutUnder(absRoot).
		Dir(paths.ProjectDataDir, paths.DirPerm755).
		Dir(paths.ProcessDir, paths.DirPerm755).
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Err(); err != nil {
		return "", fmt.Errorf("failed to create test project layout: %w", err)
	}
	if err := writeMinimalTestSettingsYAMLSystem(absRoot); err != nil {
		return "", fmt.Errorf("failed to write test settings: %w", err)
	}
	return absRoot, nil
}

// systemFindProjectRoot walks up from cwd looking for object_specs or go.mod+docs/process (same as pkg/testing.findProjectRoot).
func systemFindProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(datacell.ProcessPrimaryDir(dir)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// getProjectRootForIntegrationSystem mirrors pkg/testing.GetProjectRootForIntegration.
func getProjectRootForIntegrationSystem(t *testing.T) string {
	t.Helper()
	root := systemFindProjectRoot()
	if root == emptyValue {
		t.Skip("project root not found (run from repo)")
		return ""
	}
	return root
}

func systemHasYAMLExtension(filename string) bool {
	ext := filepath.Ext(filename)
	return ext == ".yaml" || ext == ".yml"
}

// systemCopySpecFilesToTest mirrors pkg/testing.copySpecFilesToTest.
func systemCopySpecFilesToTest(testRoot, projectRoot string) error {
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
		if entry.IsDir() || !systemHasYAMLExtension(entry.Name()) {
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

// bootstrapSystemTestRoot mirrors pkg/testing.BootstrapTestRoot.
func bootstrapSystemTestRoot(testRoot, projectRoot string) error {
	if _, err := setupSystemTestEnvironmentRoot(testRoot); err != nil {
		return err
	}
	if projectRoot == emptyValue {
		return nil
	}
	return systemCopySpecFilesToTest(testRoot, projectRoot)
}

// moduleRootFromGoEnvSystem mirrors pkg/testing.ModuleRootFromGoEnv.
func moduleRootFromGoEnvSystem(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" || modPath == "/dev/null" {
		t.Skip("no module root (GOMOD empty or not in module context)")
	}
	return filepath.Dir(modPath)
}

// waitForConditionWithTimeoutSystem mirrors pkg/testing.WaitForConditionWithTimeout.
func waitForConditionWithTimeoutSystem(ctx context.Context, condition func() bool, timeout, pollInterval time.Duration) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return waitForConditionSystem(timeoutCtx, condition, pollInterval)
}

func waitForConditionSystem(ctx context.Context, condition func() bool, pollInterval time.Duration) bool {
	if condition() {
		return true
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if condition() {
				return true
			}
		}
	}
}
