//go:build !production
// +build !production

package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storagetesting"
	"github.com/zqk-os/zqk/pkg/testenvroot"
)

// SetupTestRootLikeSetupTestEnvironmentForExportTest mirrors pkg/testing.SetupTestEnvironment (layout + test-settings only)
// for external test packages (e.g. storage_test) that cannot call unexported helpers.
func SetupTestRootLikeSetupTestEnvironmentForExportTest(t *testing.T, root string) {
	t.Helper()
	if _, err := testenvroot.Setup(root); err != nil {
		t.Fatalf("test project layout: %v", err)
	}
	objects.ResetGlobalFieldRegistryForTesting()
	objects.ResetGlobalKindMapperForTesting()
}

// bootstrapTestRootFromProjectRoot delegates to testenvroot.BootstrapRoot: layout + test-settings under testRoot,
// copying specs, lifecycles, traits, and configs from projectRoot without repo-tree fallback.
func bootstrapTestRootFromProjectRoot(t *testing.T, testRoot, projectRoot string) {
	t.Helper()
	if err := testenvroot.BootstrapRoot(testRoot, projectRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	// Reset global singletons to ensure clean state for this test layout
	objects.ResetGlobalFieldRegistryForTesting()
	objects.ResetGlobalKindMapperForTesting()
}

// CopyObjectSpecsFromModuleOrSkip ensures process/object_specs layout and copies YAML specs from the module root.
// Skips when copy fails (e.g. not running from a checkout). For tests that use without ZQK_TEST_ROOT.
func CopyObjectSpecsFromModuleOrSkip(t *testing.T, tmpDir string) {
	t.Helper()
	modRoot := moduleRootFromGoEnv(t)
	if err := testenvroot.BootstrapRoot(tmpDir, modRoot); err != nil {
		t.Skipf("bootstrap root (run from module checkout): %v", err)
	}
}

// setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip mirrors pkg/testing.SetupTestEnvironment plus
// copying specs/lifecycles. Skips when specs cannot be copied (e.g. not running from a module checkout).
func setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t *testing.T, tmpDir string) {
	t.Helper()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)
	objects.ResetGlobalFieldRegistryForTesting()
	objects.ResetGlobalKindMapperForTesting()
	ensureTestIdentityCacheHandler()
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
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return waitForCondition(ctx, cond, interval)
}

// setupTestingFactoryCompleteTestEnvironment mirrors pkg/testing.SetupCompleteTestEnvironment(t, nil, NewTestingFactory()).
func setupTestingFactoryCompleteTestEnvironment(t *testing.T) (testRoot string, fos *FileObjectStorage, secCtx *pkgctx.SecurityContext) {
	t.Helper()
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	tmpDir := t.TempDir()
	modRoot := moduleRootFromGoEnv(t)
	bootstrapTestRootFromProjectRoot(t, tmpDir, modRoot)

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
	ensureTestIdentityCacheHandler()
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
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)

	modRoot := moduleRootFromGoEnv(t)
	bootstrapTestRootFromProjectRoot(t, testRoot, modRoot)

	specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
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

// CopyObjectSpecsFromModuleOrSkipForTest calls the unexported CopyObjectSpecsFromModuleOrSkip
func CopyObjectSpecsFromModuleOrSkipForTest(t *testing.T, dest string) {
	CopyObjectSpecsFromModuleOrSkip(t, dest)
}
