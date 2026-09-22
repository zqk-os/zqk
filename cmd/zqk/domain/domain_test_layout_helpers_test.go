package domain

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

	clctx "github.com/zqk-os/zqk/internal/cli/context"
	"github.com/zqk-os/zqk/pkg/paths"
)

type testSettingsYAMLShapeDomain struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLDomain(testRoot string) error {
	configDir := filepath.Join(testRoot, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return err
	}
	p := filepath.Join(configDir, paths.ZqkTestConfigFileName)
	body := testSettingsYAMLShapeDomain{
		Version: clctx.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

func setupDomainTestEnvironmentRoot(testRoot string) (string, error) {
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
	if err := writeMinimalTestSettingsYAMLDomain(absRoot); err != nil {
		return "", fmt.Errorf("failed to write test settings: %w", err)
	}
	return absRoot, nil
}

func domainFindProjectRootForTest() string {
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

func getProjectRootForIntegrationDomain(t *testing.T) string {
	t.Helper()
	root := domainFindProjectRootForTest()
	if root == emptyValue {
		t.Skip("project root not found (run from repo)")
		return ""
	}
	return root
}

func copyDirDomain(src, dst string) error {
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

func setupIntegrationTestWithSpecsDomain(t *testing.T, scenarioName string) (testRoot, projectRoot string) {
	t.Helper()
	projectRoot = getProjectRootForIntegrationDomain(t)
	tmpDir := t.TempDir()
	scenarioRoot := filepath.Join(tmpDir, "test-scenarios", scenarioName)
	root, err := setupDomainTestEnvironmentRoot(scenarioRoot)
	if err != nil {
		t.Fatalf("setupDomainTestEnvironmentRoot: %v", err)
	}
	testRoot = root
	srcInternal := filepath.Join(projectRoot, paths.ProcessInternalDir)
	dstInternal := filepath.Join(testRoot, paths.ProcessInternalDir)
	if err := copyDirDomain(srcInternal, dstInternal); err != nil {
		t.Fatalf("copyDir _internal: %v", err)
	}
	orig := zqkenv.TestRoot().Get()
	if err := zqkenv.TestRoot().Set(testRoot); err != nil {
		t.Fatalf("Setenv TestRoot: %v", err)
	}
	t.Cleanup(func() {
		if orig != emptyValue {
			_ = zqkenv.TestRoot().Set(orig)
		} else {
			_ = zqkenv.TestRoot().Unset()
		}
	})
	return testRoot, projectRoot
}

func captureStdoutDomain(t *testing.T, fn func()) string {
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
	goroutinelabels.NewGoroutine("domain", "read stdout").StartSimple(func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	})
	fn()
	_ = w.Close()
	<-done
	return buf.String()
}
