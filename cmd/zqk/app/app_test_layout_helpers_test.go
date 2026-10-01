package app_test

import (
	"os"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/cmd/zqk/app"
	clctx "github.com/zqk-os/zqk/pkg/cliapp/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

type testSettingsYAMLShapeApp struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLApp(testRoot string) error {
	configDir := filepath.Join(testRoot, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return err
	}
	p := filepath.Join(configDir, paths.ZqkTestConfigFileName)
	body := testSettingsYAMLShapeApp{
		Version: clctx.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

// setupAppTestEnvironmentRoot mirrors [github.com/zqk-os/zqk/pkg/testing.SetupTestEnvironment]:
// project layout + config/zqk-test.yaml only.
func setupAppTestEnvironmentRoot(testRoot string) (string, error) {
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
	if err := writeMinimalTestSettingsYAMLApp(absRoot); err != nil {
		return "", fmt.Errorf("failed to write test settings: %w", err)
	}
	return absRoot, nil
}

// appFindProjectRoot walks up from cwd looking for object_specs or go.mod+.zqk/process.
func appFindProjectRootForTest() string {
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

// getProjectRootForIntegrationApp mirrors pkg/testing.GetProjectRootForIntegration.
func getProjectRootForIntegrationApp(t *testing.T) string {
	t.Helper()
	root := appFindProjectRootForTest()
	if root == app.EmptyValue {
		t.Skip("project root not found (run from repo)")
		return ""
	}
	return root
}

func hasYAMLExtensionApp(filename string) bool {
	ext := filepath.Ext(filename)
	return ext == ".yaml" || ext == ".yml"
}

// copySpecFilesToTestApp copies YAML object specs from projectRoot into testRoot.
func copySpecFilesToTestApp(testRoot, projectRoot string) error {
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
		if entry.IsDir() || !hasYAMLExtensionApp(entry.Name()) {
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

// copySpecsToTestRootApp mirrors pkg/testing.CopySpecsToTestRoot.
func copySpecsToTestRootApp(testRoot string) error {
	projectRoot := appFindProjectRootForTest()
	if projectRoot == app.EmptyValue {
		return fmt.Errorf("cannot copy specs: project root not found")
	}
	srcInternal := filepath.Join(projectRoot, paths.ProcessInternalDir)
	dstInternal := filepath.Join(testRoot, paths.ProcessInternalDir)
	return copyDirApp(srcInternal, dstInternal)
}

// copyDirApp copies a directory tree from src to dst (same behavior as pkg/testing.CopyDir).
func copyDirApp(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fileutil.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)
		if d.IsDir() {
			if mkErr := fileutil.EnsureDir(targetPath); mkErr != nil {
				return mkErr
			}
			return nil
		}
		srcFile, err := fileutil.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		if err := fileutil.EnsureDir(filepath.Dir(targetPath)); err != nil {
			return err
		}
		dstFile, err := fileutil.Create(targetPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}
		if fi, statErr := fileutil.Stat(path); statErr == nil {
			if chmodErr := fileutil.Chmod(targetPath, fi.Mode()); chmodErr != nil {
				return chmodErr
			}
		}
		return nil
	})
}

// setupIntegrationTestWithSpecsApp mirrors pkg/testing.SetupIntegrationTestWithSpecs.
func setupIntegrationTestWithSpecsApp(t *testing.T, scenarioName string) (testRoot, projectRoot string) {
	t.Helper()
	projectRoot = getProjectRootForIntegrationApp(t)
	tmpDir := t.TempDir()
	scenarioRoot := filepath.Join(tmpDir, "test-scenarios", scenarioName)
	root, err := setupAppTestEnvironmentRoot(scenarioRoot)
	if err != nil {
		t.Fatalf("setupAppTestEnvironmentRoot: %v", err)
	}
	testRoot = root
	srcInternal := filepath.Join(projectRoot, paths.ProcessInternalDir)
	dstInternal := filepath.Join(testRoot, paths.ProcessInternalDir)
	if err := copyDirApp(srcInternal, dstInternal); err != nil {
		t.Fatalf("copyDir _internal: %v", err)
	}
	orig := zqkenv.TestRoot().Get()
	if err := zqkenv.TestRoot().Set(testRoot); err != nil {
		t.Fatalf("setenv %s: %v", zqkenv.TestRoot(), err)
	}
	// Avoid ApplyIsolatedStorageEnv(t.Setenv): callers may use t.Parallel().
	// TestRoot enables membrane local write (privilegedWriterLocalWriteAllowed).
	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
		_ = fileutil.RemoveAll(filepath.Join(testRoot, paths.ProjectDataDir))
		if orig != app.EmptyValue {
			_ = zqkenv.TestRoot().Set(orig)
		} else {
			_ = zqkenv.TestRoot().Unset()
		}
	})
	return testRoot, projectRoot
}

// captureStdoutApp mirrors pkg/testing.CaptureStdout.
func captureStdoutApp(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })
	var buf bytes.Buffer
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("app", "read stdout").StartSimple(func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	})
	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

// fixtureObjectTitleApp mirrors pkg/testing.FixtureObjectTitle.
func fixtureObjectTitleApp(kind string, seq int) string {
	return fmt.Sprintf("Fixture: %s #%d", kind, seq)
}
