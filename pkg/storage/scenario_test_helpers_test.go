package storage

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/projecttemp"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// runIsolatedCLICommand creates an *exec.Cmd configured with the scenario root's subprocess environment.
func runIsolatedCLICommand(binaryPath string, args []string, scenarioDir string) *exec.Cmd {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = scenarioDir
	cmd.Env = zqkenv.SubprocessEnvironWithTestRoot(scenarioDir)
	return cmd
}

// copyScenarioTreeForTest copies a directory tree from src to dst (same behavior as pkg/testing.CopyDir).
func copyScenarioTreeForTest(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)
		if d.IsDir() {
			return fileutil.EnsureDir(targetPath)
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		if err := fileutil.EnsureDir(filepath.Dir(targetPath)); err != nil {
			return err
		}
		dstFile, err := os.Create(targetPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}
		if fi, statErr := os.Stat(path); statErr == nil {
			if chmodErr := os.Chmod(targetPath, fi.Mode()); chmodErr != nil {
				return chmodErr
			}
		}
		return nil
	})
}

// scenarioTestEnv mirrors pkg/testing.ScenarioTestEnvironment for isolated test-scenarios copies.
type scenarioTestEnv struct {
	ScenarioName string
	SourceDir    string
	TestRoot     string
	ScenarioRoot string
}

// setupScenarioTestEnvironmentForTest copies test-scenarios/<name> from the module root into t.TempDir()
// and sets ZQK_TEST_ROOT to the isolated scenario root via [testing.T.Setenv].
func setupScenarioTestEnvironmentForTest(t *testing.T, scenarioName string) *scenarioTestEnv {
	t.Helper()
	modRoot := moduleRootFromGoEnv(t)
	sourceDir := filepath.Join(modRoot, "test-scenarios", scenarioName)
	fi, err := os.Stat(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("scenario directory does not exist: %s", sourceDir)
		}
		t.Fatalf("stat scenario: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("scenario path is not a directory: %s", sourceDir)
	}
	testRoot := t.TempDir()
	scenarioRoot := filepath.Join(testRoot, scenarioName)
	if err := copyScenarioTreeForTest(sourceDir, scenarioRoot); err != nil {
		t.Fatalf("copy scenario: %v", err)
	}
	t.Setenv(zqkenv.TestRoot(), scenarioRoot)
	// Strip .zqk / process layout under the scenario root before t.TempDir cleanup (ITEM-177483); same
	// pipeline stage as storage TempProjectTeardown when no FileObjectStorage is opened (e.g. CLI-only tests).
	t.Cleanup(func() {
		if err := projecttemp.RunIsolatedRootStrip(scenarioRoot); err != nil {
			t.Logf("scenario isolated root strip: %v", err)
		}
	})
	return &scenarioTestEnv{
		ScenarioName: scenarioName,
		SourceDir:    sourceDir,
		TestRoot:     testRoot,
		ScenarioRoot: scenarioRoot,
	}
}

// mustBootstrapScenarioRootForCLI mirrors pkg/testing.MustBootstrapScenarioRootForCLI.
func mustBootstrapScenarioRootForCLI(t *testing.T, scenarioRoot string) {
	t.Helper()
	mod := moduleRootFromGoEnv(t)
	bootstrapTestRootFromProjectRoot(t, scenarioRoot, mod)
	src := filepath.Join(mod, paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read id_prefixes config: %v", err)
	}
	dst := filepath.Join(scenarioRoot, paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile)
	if err := fileutil.EnsureDir(filepath.Dir(dst)); err != nil {
		t.Fatalf("mkdir for id_prefixes: %v", err)
	}
	if err := fileutil.WriteSecureFile(dst, data); err != nil {
		t.Fatalf("write id_prefixes config: %v", err)
	}
}
