package testing

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// ScenarioTestEnvironment represents an isolated copy of a checked-in
// scenario directory from test-scenarios.
//
// The pattern is:
//   - Scenario data is tracked in source control under test-scenarios/
//   - Tests call SetupScenarioTestEnvironment to copy that data into a
//     per-test temporary directory
//   - Tests run against the isolated copy (no mutations to fixtures)
//   - ZQK_TEST_ROOT is pointed at the isolated scenario root so that
//     libraries use the same path they would in production
type ScenarioTestEnvironment struct {
	// ScenarioName is the logical scenario identifier under test-scenarios/
	ScenarioName string

	// SourceDir is the checked-in scenario directory under the project root.
	SourceDir string

	// TestRoot is the per-test temporary root created by t.TempDir().
	TestRoot string

	// ScenarioRoot is the root directory of the isolated scenario copy that
	// tests should use as their working directory (e.g. cmd.Dir).
	ScenarioRoot string
}

// SetupScenarioTestEnvironment prepares an isolated scenario directory for tests.
//
// It:
//   - Locates the project root (using findProjectRoot)
//   - Resolves <projectRoot>/test-scenarios/<scenarioName>
//   - Copies that directory into a per-test temporary directory
//   - Sets ZQK_TEST_ROOT to the isolated ScenarioRoot for the duration of the test
//
// If the scenario directory does not exist, the test is skipped. If the project
// root cannot be determined or copying fails, the test fails.
func SetupScenarioTestEnvironment(t *testing.T, scenarioName string) *ScenarioTestEnvironment {
	t.Helper()

	projectRoot := findProjectRoot()
	if projectRoot == emptyValue {
		t.Fatalf("testing: cannot determine project root for scenario %q", scenarioName)
	}

	sourceDir := filepath.Join(projectRoot, "test-scenarios", scenarioName)
	if fi, err := fileutil.Stat(sourceDir); err != nil {
		if fileutil.IsNotExist(err) {
			t.Skipf("testing: scenario directory does not exist: %s", sourceDir)
		}
		t.Fatalf("testing: failed to stat scenario directory %s: %v", sourceDir, err)
	} else if !fi.IsDir() {
		t.Fatalf("testing: scenario path is not a directory: %s", sourceDir)
	}

	testRoot := t.TempDir()
	scenarioRoot := filepath.Join(testRoot, scenarioName)

	if err := CopyDir(sourceDir, scenarioRoot); err != nil {
		t.Fatalf("testing: failed to copy scenario %s to temp dir: %v", scenarioName, err)
	}

	// Point ZQK_TEST_ROOT at the isolated scenario so that code under test
	// that relies on it sees the scenario as the project root.
	t.Setenv(zqkenv.TestRoot().Name(), scenarioRoot)

	return &ScenarioTestEnvironment{
		ScenarioName: scenarioName,
		SourceDir:    sourceDir,
		TestRoot:     testRoot,
		ScenarioRoot: scenarioRoot,
	}
}

// CopyDir copies a directory tree from src to dst. It is safe to use from tests
// to create isolated working copies of checked-in fixtures.
func CopyDir(src, dst string) error {
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

		// For files, copy contents and preserve executable bit so scenario
		// binaries (e.g. zqk-admin-test-init) remain runnable in the isolated copy.
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

		// Preserve file mode from source so executables remain executable.
		if fi, statErr := fileutil.Stat(path); statErr == nil {
			if chmodErr := fileutil.Chmod(targetPath, fi.Mode()); chmodErr != nil {
				return chmodErr
			}
		}

		return nil
	})
}
