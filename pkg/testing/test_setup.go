package testing

import (
	"os"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2" // Register spec builders for tests
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storagetesting"
	"github.com/zqk-os/zqk/pkg/testenvroot"
)

// TestEnvironment provides a complete, isolated test environment
// This is the recommended way to set up tests - it handles all common setup requirements
type TestEnvironment struct {
	// TestRoot is the root directory for test data
	TestRoot string

	// Storage is the configured storage provider
	// Type: any so callers are not forced to depend on concrete storage types in signatures
	// Tests should type assert to storage.ObjectStorageProvider when needed
	Storage any

	// SpecLoader is the configured spec loader
	SpecLoader *objects.SpecLoader

	// LifecycleLoader is the configured lifecycle loader
	LifecycleLoader *objects.LifecycleLoader

	// SecurityContext is the default security context for test operations
	SecurityContext *pkgctx.SecurityContext

	// Cleanup function to tear down the test environment
	Cleanup func()
}

// TestEnvironmentOptions configures test environment setup.
type TestEnvironmentOptions struct {
	CopySpecs     bool
	GenerateSpecs bool

	DisableAuditBuffer bool

	NoopStorageMetrics bool

	ProjectRoot string

	CreateMetricsPipeline bool

	SkipStorageCreation bool
}

// StorageFactory is the factory type expected by [SetupCompleteTestEnvironment] (see [storagetesting.IsolationFactory]).
type StorageFactory = storagetesting.IsolationFactory

// TestStorageGlobalsConfigurator is re-exported for callers that only import pkg/testing.
type TestStorageGlobalsConfigurator = storagetesting.GlobalHookConfigurator

// DefaultTestEnvironmentOptions returns recommended default options.
func DefaultTestEnvironmentOptions() *TestEnvironmentOptions {
	return &TestEnvironmentOptions{
		CopySpecs:             true,
		GenerateSpecs:         true,
		DisableAuditBuffer:    true,
		NoopStorageMetrics:    true,
		ProjectRoot:           "",
		CreateMetricsPipeline: false,
	}
}

// SetupCompleteTestEnvironment creates a complete, isolated test environment
// This is the recommended way to set up tests - it handles:
// - Directory structure creation
// - Environment variable setup
// - Spec file copying/generation
// - Storage initialization
// - Spec/lifecycle loader initialization
// - Audit buffer configuration
// - Optional global storage metrics no-op (default on when using storage.TestingFactory; see TestEnvironmentOptions.NoopStorageMetrics)
//
// Example:
//
//	import (
//	    testconfig "github.com/zqk-os/zqk/pkg/testing"
//	    storagepkg "github.com/zqk-os/zqk/pkg/storage"
//	)
//
//	func TestMyFeature(t *testing.T) {
//	    factory := storagepkg.NewTestingFactory()
//	    env := testconfig.SetupCompleteTestEnvironment(t, nil, factory)
//	    defer env.Cleanup()
//
//	    // Type assert storage to use it
//	    storage := env.Storage.(storagepkg.ObjectStorageProvider)
//	    ctx := context.Background()
//	    err := storage.Create(ctx, env.SecurityContext, myObject)
//	    // ...
//	}
//
// Factory must be non-nil (e.g. [github.com/zqk-os/zqk/pkg/storage.NewTestingFactory]); it implements [storagetesting.IsolationFactory].
func SetupCompleteTestEnvironment(t *testing.T, options *TestEnvironmentOptions, factory StorageFactory) *TestEnvironment {
	if options == nil {
		options = DefaultTestEnvironmentOptions()
	}

	// Create temporary directory
	testRoot := t.TempDir()

	// Set test environment variable (POL-CODE-006: Test Data Isolation)
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	zqkenv.ApplyIsolatedStorageEnv(t.Setenv)
	t.Setenv(zqkenv.MockGraph().Name(), "false")
	t.Setenv(zqkenv.AdminMockGraph().Name(), "false")
	t.Setenv(zqkenv.LegacyMockGraph().Name(), "false")
	t.Setenv(zqkenv.GraphEnabled().Name(), "false")
	t.Setenv(zqkenv.AdminGraphEnabled().Name(), "false")

	// Configure test config
	testCfg := GetTestConfig()
	testCfg.TestProjectRoot = testRoot

	// Setup basic directory structure (calls existing SetupTestEnvironment from config.go)
	if _, err := SetupTestEnvironment(testRoot); err != nil {
		t.Fatalf("Failed to setup test environment directories: %v", err)
	}

	// Copy spec files if requested
	if options.CopySpecs {
		projectRoot := options.ProjectRoot
		if projectRoot == emptyValue {
			// Try to auto-detect project root
			projectRoot = findProjectRoot()
		}
		if projectRoot != emptyValue {
			if err := copySpecFilesToTest(testRoot, projectRoot); err != nil {
				t.Logf("Warning: Failed to copy spec files (tests may still work): %v", err)
			}
		}
	}

	// Generate spec files if requested
	if options.GenerateSpecs {
		specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
		if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
			t.Fatalf("Failed to create specs directory: %v", err)
		}
		generator := builders.NewSpecGenerator(specsDir)
		if err := generator.GenerateAllSpecs(); err != nil {
			t.Logf("Warning: Failed to generate specs (tests may still work): %v", err)
		}
	}

	// Optional: global storage metrics recording (storage.TestingFactory implements hook)
	if factory != nil {
		if cfg, ok := factory.(TestStorageGlobalsConfigurator); ok {
			cfg.ConfigureTestGlobals(t, &storagetesting.GlobalHookOptions{
				NoopStorageMetrics: options.NoopStorageMetrics,
			})
		}
	}

	// Disable audit buffer if requested (tests need synchronous behavior)
	if options.DisableAuditBuffer && factory != nil {
		buffer := factory.GetAuditBuffer()
		if buffer != nil {
			// Type assert to use buffer methods
			// We can't import the type, so we use a type switch or interface
			if buf, ok := buffer.(interface {
				SetEnabled(enabled bool)
				Flush() error
			}); ok {
				buf.SetEnabled(false)
				// Flush any existing buffered events
				if err := buf.Flush(); err != nil {
					t.Logf("Warning: Failed to flush buffer before test: %v", err)
				}
			}
		}
	}

	// Create storage provider using factory (unless skipped for e.g. init-time tests)
	var storageProvider any
	if !options.SkipStorageCreation {
		if factory == nil {
			t.Fatalf("StorageFactory is required. Use storage.NewTestingFactory() to create one.")
		}
		var err error
		storageProvider, err = factory.CreateFileStorage(testRoot)
		if err != nil {
			t.Fatalf("Failed to create storage provider: %v", err)
		}
	}

	// Create spec and lifecycle loaders
	// Use testRoot so they can find specs in test environment
	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)

	// Create security context
	secCtx := pkgctx.NewSystemSecurityContext()

	// Cleanup: prefer full test cleanup (storage + local queue) when available, else storage Shutdown only.
	cleanup := func() {
		if storageProvider == nil {
			return
		}
		switch v := storageProvider.(type) {
		case interface{ GetTestCleanup() func() }:
			if fn := v.GetTestCleanup(); fn != nil {
				fn()
				return
			}
		}
	}

	t.Cleanup(cleanup)

	return &TestEnvironment{
		TestRoot:        testRoot,
		Storage:         storageProvider,
		SpecLoader:      specLoader,
		LifecycleLoader: lifecycleLoader,
		SecurityContext: secCtx,
		Cleanup:         cleanup,
	}
}

// SetupSchedulerTestEnvironment creates a test environment specifically for scheduler tests
// This includes all standard setup plus scheduler-specific configuration
// Note: This is a helper that can be used in scheduler package tests
// Example usage in scheduler package:
//
//	import testconfig "github.com/zqk-os/zqk/pkg/testing"
//
//	func TestSchedulerFeature(t *testing.T) {
//	    env := testconfig.SetupCompleteTestEnvironment(t, nil)
//	    defer env.Cleanup()
//
//	    sched := NewSchedulerWithProjectRoot(
//	        env.Storage,
//	        env.SpecLoader,
//	        env.LifecycleLoader,
//	        env.TestRoot,
//	    )
//	    // ...
//	}

// copySpecFilesToTest copies spec files from project root to test environment
func copySpecFilesToTest(testRoot, projectRoot string) error {
	sourceSpecsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	targetSpecsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)

	// Check if source exists
	if _, err := fileutil.Stat(sourceSpecsDir); fileutil.IsNotExist(err) {
		return errfmt.Errorf("source specs directory does not exist: %s", sourceSpecsDir)
	}

	// Create target directory
	if err := fileutil.MkdirAll(targetSpecsDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create target specs directory").Wrap(err)
	}

	// Copy all .yaml files from source to target
	entries, err := fileutil.ReadDir(sourceSpecsDir)
	if err != nil {
		return errfmt.Newf("failed to read source specs directory").Wrap(err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !hasYAMLExtension(entry.Name()) {
			continue
		}

		sourcePath := filepath.Join(sourceSpecsDir, entry.Name())
		targetPath := filepath.Join(targetSpecsDir, entry.Name())

		// Read source file
		data, err := fileutil.ReadFile(sourcePath)
		if err != nil {
			return errfmt.Errorf("failed to read source file %s: %w", sourcePath, err)
		}

		// Write target file
		//nolint:gosec // G306: 0600 is acceptable for spec files (readable by all)
		if err := fileutil.WriteFile(targetPath, data, paths.FilePerm644); err != nil {
			return errfmt.Errorf("failed to write target file %s: %w", targetPath, err)
		}
	}

	return nil
}

// hasYAMLExtension checks if a filename has a .yaml or .yml extension
func hasYAMLExtension(filename string) bool {
	return fileutil.IsYAMLPath(filename)
}

// CopySpecsToTestRoot copies object spec files from the current project root into
// testRoot so that discovery and validation see a consistent set of kinds (e.g. backlog_item).
// Use this when a test runs CLI commands (e.g. system check) that rely on the field
// registry and need specs to be present under the test root for determinism.
func CopySpecsToTestRoot(testRoot string) error {
	projectRoot := findProjectRoot()
	if projectRoot == emptyValue {
		return errfmt.Errorf("cannot copy specs: project root not found")
	}
	return copySpecFilesToTest(testRoot, projectRoot)
}

// CopyKindSynonymsToTestRoot copies .zqk/process/kind_synonyms from the project root
// (see findProjectRoot) into testRoot. Storage should use testRoot, not the checkout,
// so synonym integration tests do not read or write the developer tree.
func CopyKindSynonymsToTestRoot(testRoot string) error {
	projectRoot := findProjectRoot()
	if projectRoot == emptyValue {
		return errfmt.Errorf("cannot copy kind_synonyms: project root not found")
	}
	src := filepath.Join(datacell.ProcessPrimaryDir(projectRoot), "kind_synonyms")
	dst := filepath.Join(datacell.ProcessPrimaryDir(testRoot), "kind_synonyms")
	return CopyDir(src, dst)
}

// BootstrapTestRoot creates the minimal directory layout under testRoot (.zqk/process,
// _internal/object_specs) and, when projectRoot is non-empty, copies object spec files
// from projectRoot so that ID validation and storage can bootstrap successfully.
// Use this when setting ZQK_TEST_ROOT (e.g. run_wrapper test jobs) so that code requiring
// process directory or object_specs can run without "process directory not found" or
// "failed to load ID patterns".
func BootstrapTestRoot(testRoot, projectRoot string) error {
	return testenvroot.BootstrapRoot(testRoot, projectRoot)
}

// MustBootstrapScenarioRootForCLI prepares an isolated scenario directory copied from
// test-scenarios/<name> when the fixture only contains a binary (no .zqk/process).
// It creates .zqk/process, copies object_specs from the module root, writes test-settings,
// and copies id_prefixes_config.yaml so CLI subprocesses do not log config errors to stdout.
// Call after SetupScenarioTestEnvironment before exec'ing zqk-admin-test-init against scenarioRoot.
func MustBootstrapScenarioRootForCLI(t *testing.T, scenarioRoot string) {
	t.Helper()
	mod := ModuleRootFromGoEnv(t)
	if err := BootstrapTestRoot(scenarioRoot, mod); err != nil {
		t.Fatalf("BootstrapTestRoot: %v", err)
	}
	src := filepath.Join(mod, paths.ProcessInternalConfigsDir, paths.IdPrefixesConfigFile)
	data, err := fileutil.ReadFile(src)
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

// ModuleRootFromGoEnv returns the directory containing go.mod for the current module (`go env GOMOD`).
// Use for read-only fixture copy or `go build` without walking from os.Getwd.
func ModuleRootFromGoEnv(t *testing.T) string {
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

// findProjectRoot attempts to find the project root by walking up from current directory
func findProjectRoot() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}

	dir := wd
	for {
		// Check for project root markers
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProcessInternalObjectSpecsDir)); err == nil {
			return dir
		}
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// Check if this looks like our project
			if _, err := fileutil.Stat(datacell.ProcessPrimaryDir(dir)); err == nil {
				return dir
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return ""
}

// CaptureStdout runs fn with os.Stdout redirected to a pipe, then returns the captured output.
// Restores os.Stdout in t.Cleanup. Use from CLI integration tests that assert on command output.
func CaptureStdout(t *testing.T, fn func()) string {
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
	goroutinelabels.NewGoroutine("testing", "capture stdout").StartSimple(func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	})
	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

// GetProjectRootForIntegration finds the repo root (.zqk/specs or go.mod + .zqk/process).
// Skips the test if not found (e.g. when not run from repo). Use from CLI integration tests.
func GetProjectRootForIntegration(t *testing.T) string {
	t.Helper()
	root := findProjectRoot()
	if root == emptyValue {
		t.Skip("project root not found (run from repo)")
		return ""
	}
	return root
}

// SetupIntegrationTestWithSpecs creates a test root with .zqk/specs copied from the
// project, sets ZQK_TEST_ROOT and TestProjectRoot, and returns (testRoot, projectRoot).
//
// This package cannot register [storage.RunProjectTestTeardown] here: pkg/storage tests import
// pkg/testing, so pkg/testing must not import pkg/storage (test-time import cycle). Callers should
// register teardown with storage.TempProjectTeardown (nil *FileObjectStorage when no test-owned
// storage, or the live *FileObjectStorage after NewFileObjectStorage*)—typically via
// pkg/testkit.RegisterStandardTeardown / RunStandardTeardown—or an explicit t.Cleanup. Register
// storage-aware teardown after this helper returns so it runs before env restore (LIFO).
//
// Use from CLI integration tests that need storage and kind specs (e.g. domain, organizational).
func SetupIntegrationTestWithSpecs(t *testing.T, scenarioName string) (testRoot, projectRoot string) {
	t.Helper()
	projectRoot = GetProjectRootForIntegration(t)
	tmpDir := t.TempDir()
	scenarioRoot := filepath.Join(tmpDir, "test-scenarios", scenarioName)
	root, err := SetupTestEnvironment(scenarioRoot)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}
	testRoot = root
	srcInternal := filepath.Join(projectRoot, paths.ProcessInternalDir)
	dstInternal := filepath.Join(testRoot, paths.ProcessInternalDir)
	if err := CopyDir(srcInternal, dstInternal); err != nil {
		t.Fatalf("CopyDir _internal: %v", err)
	}
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	GetTestConfig().TestProjectRoot = testRoot
	return testRoot, projectRoot
}
