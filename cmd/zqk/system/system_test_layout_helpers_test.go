package system

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"

	"gopkg.in/yaml.v3"

	clctx "github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

type testSettingsYAMLShapeSystem struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLSystem(testRoot string) error {
	configDir := filepath.Join(testRoot, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return err
	}
	p := filepath.Join(configDir, paths.ZqkTestConfigFileName)
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

// setupSystemTestEnvironmentRoot mirrors pkg/testing.SetupTestEnvironment: project layout + config/zqk-test.yaml only.
func setupSystemTestEnvironmentRoot(t testing.TB, testRoot string) (string, error) {
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
	// Bind TEST_ROOT before ApplyIsolatedStorageEnv so fallthrough cannot target the live tree.
	t.Setenv(zqkenv.TestRoot().Name(), absRoot)
	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	zqkenv.ApplyIsolatedStorageEnv(t.Setenv)
	return absRoot, nil
}

// systemFindProjectRoot walks up from cwd looking for object_specs or go.mod+.zqk/process (same as pkg/testing.findProjectRoot).
func systemFindProjectRoot() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)); err == nil {
			return dir
		}
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := fileutil.Stat(datacell.ProcessPrimaryDir(dir)); err == nil {
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
		if entry.IsDir() || !systemHasYAMLExtension(entry.Name()) {
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
	return nil
}

// bootstrapSystemTestRoot mirrors pkg/testing.BootstrapTestRoot.
func bootstrapSystemTestRoot(t testing.TB, testRoot, projectRoot string) error {
	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
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
	out, err := testkit.ManagedCommand(t, t.Context(), "go", "env", "GOMOD").Output()
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

// registryFilenameForObject returns the hash-registry key for an object after CAS create (hash basename, not id.yaml).
func registryFilenameForObject(t *testing.T, fs storage.ObjectStorageProvider, kind, id, fallback string) string {
	t.Helper()
	if sp, ok := fs.(*storage.FileObjectStorage); ok {
		if cas, err := sp.GetContentAddressableStorage(kind); err == nil {
			if p, err := cas.GetFilePathForID(id); err == nil && p != "" {
				return filepath.Base(p)
			}
		}
	}
	return fallback
}

// ensureHashRegistryEntryForObject registers the on-disk CAS blob in the kind hash registry (promote may lag registry keys).
func ensureHashRegistryEntryForObject(t *testing.T, fs storage.ObjectStorageProvider, kind, id, kindDir string) {
	t.Helper()
	ctx := pkgctx.NewSystemContext()
	objectFile := ""
	if sp, ok := fs.(*storage.FileObjectStorage); ok {
		if cas, err := sp.GetContentAddressableStorage(kind); err == nil {
			if p, err := cas.GetFilePathForID(id); err == nil {
				objectFile = p
			}
		}
	}
	if objectFile == "" {
		t.Fatalf("ensureHashRegistryEntryForObject: no CAS path for %s", id)
	}
	data, err := fileutil.ReadFile(objectFile)
	if err != nil {
		t.Fatalf("ensureHashRegistryEntryForObject read %s: %v", objectFile, err)
	}
	reg := storage.NewHashRegistry(ctx, kind, kindDir)
	_ = reg.Load() //nolint:errcheck // best-effort load
	reg.SetHash(filepath.Base(objectFile), storage.CalculateSHA256Hash(data))
	if err := reg.Save(); err != nil {
		t.Fatalf("ensureHashRegistryEntryForObject save: %v", err)
	}
}
