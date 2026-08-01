package storage

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	clctx "github.com/lanceman/zqk/internal/cli/context"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	"github.com/lanceman/zqk/pkg/storagetesting"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"
)

type testSettingsYAMLShape struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLForTestRoot(testRoot string) error {
	p := filepath.Join(testRoot, paths.TestSettingsFilename)
	body := testSettingsYAMLShape{
		Version: clctx.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

func yamlSpecExtensionOK(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}

// copyObjectSpecYAMLFilesToTestRoot copies top-level *.yaml/*.yml from the module's object_specs dir
// into testRoot (same contract as pkg/testing.copySpecFilesToTest).
func copyObjectSpecYAMLFilesToTestRoot(testRoot, projectRoot string) error {
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
		if entry.IsDir() || !yamlSpecExtensionOK(entry.Name()) {
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

// SetupTestRootLikeSetupTestEnvironmentForExportTest mirrors pkg/testing.SetupTestEnvironment (layout + test-settings only)
// for external test packages (e.g. storage_test) that cannot call unexported helpers.
func SetupTestRootLikeSetupTestEnvironmentForExportTest(t *testing.T, root string) {
	t.Helper()
	setupTestRootLikeSetupTestEnvironmentNoCopy(t, root)
}

// setupTestRootLikeSetupTestEnvironmentNoCopy mirrors pkg/testing.SetupTestEnvironment (layout + test-settings only).
func setupTestRootLikeSetupTestEnvironmentNoCopy(t *testing.T, tmpDir string) {
	t.Helper()
	absRoot, err := filepath.Abs(tmpDir)
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
	if err := writeMinimalTestSettingsYAMLForTestRoot(absRoot); err != nil {
		t.Fatalf("test-settings.yaml: %v", err)
	}

	// Reset global singletons to ensure clean state for this test layout
	objects.ResetGlobalFieldRegistryForTesting()
	objects.ResetGlobalKindMapperForTesting()
}

// bootstrapTestRootFromProjectRoot mirrors pkg/testing.BootstrapTestRoot: layout + test-settings under testRoot,
// then copies top-level object_specs YAML from projectRoot when non-empty.
func bootstrapTestRootFromProjectRoot(t *testing.T, testRoot, projectRoot string) {
	t.Helper()
	setupTestRootLikeSetupTestEnvironmentNoCopy(t, testRoot)
	if projectRoot == "" {
		return
	}
	if err := copyObjectSpecYAMLFilesToTestRoot(testRoot, projectRoot); err != nil {
		t.Fatalf("copy object specs: %v", err)
	}
}

// copyObjectSpecsFromModuleOrSkip ensures process/object_specs layout and copies YAML specs from the module root.
// Skips when copy fails (e.g. not running from a checkout). For tests that use without ZQK_TEST_ROOT.
func copyObjectSpecsFromModuleOrSkip(t *testing.T, tmpDir string) {
	t.Helper()
	mustEnsureProcessSpecsLayout(t, tmpDir)
	modRoot := moduleRootFromGoEnv(t)
	if err := copyObjectSpecYAMLFilesToTestRoot(tmpDir, modRoot); err != nil {
		t.Skipf("copy object specs (run from module checkout): %v", err)
	}
}

// setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip mirrors pkg/testing.SetupTestEnvironment plus
// CopySpecsToTestRoot. Skips when specs cannot be copied (e.g. not running from a module checkout).
func setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t *testing.T, tmpDir string) {
	t.Helper()
	setupTestRootLikeSetupTestEnvironmentNoCopy(t, tmpDir)
	absRoot, err := filepath.Abs(tmpDir)
	if err != nil {
		t.Fatalf("abs test root: %v", err)
	}
	modRoot := moduleRootFromGoEnv(t)
	if err := copyObjectSpecYAMLFilesToTestRoot(absRoot, modRoot); err != nil {
		t.Skipf("copy object specs (run from module checkout): %v", err)
	}
}

// waitForCondition polls until cond returns true or ctx is done (same contract as pkg/testing.WaitForCondition).
func waitForCondition(ctx context.Context, cond func() bool, pollInterval time.Duration) bool {
	if cond() {
		return true
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if cond() {
				return true
			}
		}
	}
}

// waitForConditionWithTimeout polls until cond returns true or timeout elapses (same contract as pkg/testing.WaitForConditionWithTimeout).
func waitForConditionWithTimeout(ctx context.Context, cond func() bool, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if cond() {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// setupTestingFactoryCompleteTestEnvironment mirrors pkg/testing.SetupCompleteTestEnvironment(t, nil, NewTestingFactory()).
func setupTestingFactoryCompleteTestEnvironment(t *testing.T) (testRoot string, fos *FileObjectStorage, secCtx *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir := t.TempDir()
	mustEnsureProcessSpecsLayout(t, tmpDir)
	modRoot := moduleRootFromGoEnv(t)
	if err := copyObjectSpecYAMLFilesToTestRoot(tmpDir, modRoot); err != nil {
		t.Fatalf("copy object specs: %v", err)
	}
	factory := NewTestingFactory()
	if buf := factory.GetAuditBuffer(); buf != nil {
		if b, ok := buf.(interface {
			SetEnabled(enabled bool)
			Flush() error
		}); ok {
			b.SetEnabled(false)
			if err := b.Flush(); err != nil {
				t.Logf("flush audit buffer before test: %v", err)
			}
		}
	}
	sAny, err := factory.CreateFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("CreateFileStorage: %v", err)
	}
	fos, ok := sAny.(*FileObjectStorage)
	if !ok {
		t.Fatalf("expected *FileObjectStorage, got %T", sAny)
	}
	t.Cleanup(func() {
		if fn := fos.GetTestCleanup(); fn != nil {
			fn()
		}
	})
	return tmpDir, fos, pkgctx.NewSystemSecurityContext()
}

// setupStorageInitBudgetTestEnvironmentForTest mirrors pkg/testing.SetupCompleteTestEnvironment with
// CopySpecs, GenerateSpecs, SkipStorageCreation, DisableAuditBuffer for [TestNewFileObjectStorage_InitCompletesWithinBudget].
func setupStorageInitBudgetTestEnvironmentForTest(t *testing.T) string {
	t.Helper()
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), testRoot)
	setupTestRootLikeSetupTestEnvironmentNoCopy(t, testRoot)
	modRoot := moduleRootFromGoEnv(t)
	if err := copyObjectSpecYAMLFilesToTestRoot(testRoot, modRoot); err != nil {
		t.Fatalf("copy object specs: %v", err)
	}
	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.EnsureDir(specsDir); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	generator := builders.NewSpecGenerator(specsDir)
	if err := generator.GenerateAllSpecs(); err != nil {
		t.Logf("Warning: GenerateAllSpecs: %v", err)
	}
	factory := NewTestingFactory()
	if buf := factory.GetAuditBuffer(); buf != nil {
		if b, ok := buf.(interface {
			SetEnabled(enabled bool)
			Flush() error
		}); ok {
			b.SetEnabled(false)
			if err := b.Flush(); err != nil {
				t.Logf("flush audit buffer: %v", err)
			}
		}
	}
	// Match pkg/testing.TestEnvironmentOptions with only Copy/Generate/Skip/Disable set: NoopStorageMetrics defaults to false.
	factory.ConfigureTestGlobals(t, &storagetesting.GlobalHookOptions{NoopStorageMetrics: false})
	return testRoot
}
