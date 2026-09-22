package scheduler

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storagetesting"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"
)

func init() {
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	// Package-scoped init has no *testing.T, so process env is the only scope available here;
	// the key list itself is shared with the t.Setenv call sites.
	zqkenv.ApplyIsolatedStorageEnv(zqkenv.OSEnvSetter)
}

type schedulerTestEnvOptions struct {
	CopySpecs          bool
	GenerateSpecs      bool
	DisableAuditBuffer bool
	NoopStorageMetrics bool
}

func defaultSchedulerTestEnvOptions() schedulerTestEnvOptions {
	return schedulerTestEnvOptions{
		CopySpecs:          true,
		GenerateSpecs:      true,
		DisableAuditBuffer: true,
		NoopStorageMetrics: true,
	}
}

type testSettingsYAMLShapeScheduler struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLScheduler(testRoot string) error {
	configDir := filepath.Join(testRoot, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return err
	}
	p := filepath.Join(configDir, paths.ZqkTestConfigFileName)
	body := testSettingsYAMLShapeScheduler{
		Version: paths.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

// newSchedulerTestStorage is the standard scheduler handler-test preamble: an isolated project root
// under [testing.T.TempDir], file storage bound to it, and teardown registered. Handler tests differ
// in what they do with the storage, not in how they obtain it, so they share this instead of each
// repeating the layout / storage / teardown triple.
func newSchedulerTestStorage(t *testing.T) (string, *storage.FileObjectStorage) {
	t.Helper()
	testRoot := t.TempDir()
	mod := moduleRootFromGoEnvSchedulerOrEmpty(t)
	if err := testenvroot.BootstrapRoot(testRoot, mod); err != nil {
		t.Fatalf("failed to bootstrap root: %v", err)
	}
	store, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, store)
	return testRoot, store
}

// setupSchedulerTestEnvironmentRoot mirrors pkg/testing.SetupTestEnvironment: project layout + config/zqk-test.yaml only.
func setupSchedulerTestEnvironmentRoot(testRoot string) (string, error) {
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
	if err := writeMinimalTestSettingsYAMLScheduler(absRoot); err != nil {
		return "", fmt.Errorf("failed to write test settings: %w", err)
	}
	return absRoot, nil
}

func layoutSchedulerTestRootFull(t *testing.T, testRoot string) {
	t.Helper()
	absRoot, err := filepath.Abs(testRoot)
	if err != nil {
		t.Fatalf("abs test root: %v", err)
	}
	if err := paths.LayoutUnder(absRoot).
		Dir(paths.ProjectDataDir, paths.DirPerm755).
		Dir(paths.ProcessDir, paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "scheduler_jobs"), paths.DirPerm755).
		Dir(filepath.Join(paths.ProcessDir, "watchdog_registrations"), paths.DirPerm755).
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Err(); err != nil {
		t.Fatalf("test project layout: %v", err)
	}
	if err := writeMinimalTestSettingsYAMLScheduler(absRoot); err != nil {
		t.Fatalf("config/zqk-test.yaml: %v", err)
	}
}

var (
	moduleRootCache string
	moduleRootOnce  sync.Once
)

func moduleRootFromGoEnvSchedulerOrEmpty(t *testing.T) string {
	t.Helper()
	moduleRootOnce.Do(func() {
		out, err := exec.Command("go", "env", "GOMOD").Output()
		if err != nil {
			t.Logf("go env GOMOD: %v", err)
			return
		}
		modPath := strings.TrimSpace(string(out))
		if modPath == "" || modPath == "/dev/null" {
			return
		}
		moduleRootCache = filepath.Dir(modPath)
	})
	return moduleRootCache
}

func moduleRootFromGoEnvScheduler(t *testing.T) string {
	t.Helper()
	mod := moduleRootFromGoEnvSchedulerOrEmpty(t)
	if mod == "" {
		t.Skip("no module root (GOMOD empty or not in module context)")
	}
	return mod
}

func yamlSpecExtensionOKScheduler(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}

func copyObjectSpecYAMLFilesToTestRootScheduler(testRoot, projectRoot string) error {
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
		if entry.IsDir() || !yamlSpecExtensionOKScheduler(entry.Name()) {
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

// ensureSchedulerObjectSpecsForJobLockTest lays out a test project root and copies object_specs from the module (go env GOMOD).
func ensureSchedulerObjectSpecsForJobLockTest(t *testing.T, testRoot string) {
	t.Helper()
	layoutSchedulerTestRootFull(t, testRoot)
	mod := moduleRootFromGoEnvScheduler(t)
	if err := copyObjectSpecYAMLFilesToTestRootScheduler(testRoot, mod); err != nil {
		t.Skipf("copy object specs (run from module checkout): %v", err)
	}
}

// schedulerCompleteTestEnvironment mirrors pkg/testing.TestEnvironment fields used by scheduler tests.
type schedulerCompleteTestEnvironment struct {
	TestRoot        string
	Storage         any
	SpecLoader      *objects.SpecLoader
	LifecycleLoader *objects.LifecycleLoader
	SecurityContext *pkgctx.SecurityContext
	Cleanup         func()
}

// setupSchedulerCompleteTestEnvironment mirrors pkg/testing.SetupCompleteTestEnvironment(t, nil, storage.NewTestingFactory()).
func setupSchedulerCompleteTestEnvironment(t *testing.T, opts *schedulerTestEnvOptions) *schedulerCompleteTestEnvironment {
	t.Helper()
	o := defaultSchedulerTestEnvOptions()
	if opts != nil {
		o = *opts
	}

	testRoot := t.TempDir()

	origTestRoot := zqkenv.TestRoot().Get()
	if err := zqkenv.TestRoot().Set(testRoot); err != nil {
		t.Fatalf("setenv %s: %v", zqkenv.TestRoot(), err)
	}
	// Do not ApplyIsolatedStorageEnv(t.Setenv): many callers use t.Parallel().
	// TestRoot alone enables membrane local write; init already sets unreachable PW socket.
	t.Cleanup(func() {
		if origTestRoot != "" {
			_ = zqkenv.TestRoot().Set(origTestRoot) //nolint:errcheck // test restore
		} else {
			_ = zqkenv.TestRoot().Unset() //nolint:errcheck // test restore
		}
		objects.ResetGlobalKindMapperForTesting()
	})

	objects.ResetGlobalKindMapperForTesting()

	layoutSchedulerTestRootFull(t, testRoot)

	if o.CopySpecs {
		mod := moduleRootFromGoEnvSchedulerOrEmpty(t)
		if mod == "" {
			t.Logf("Warning: could not resolve module root for spec copy")
		} else if err := testenvroot.BootstrapRoot(testRoot, mod); err != nil {
			t.Logf("Warning: bootstrap root (tests may still work): %v", err)
		}
	}

	if o.GenerateSpecs {
		specsDir := filepath.Join(testRoot, paths.ProcessInternalObjectSpecsDir)
		if err := fileutil.EnsureDir(specsDir); err != nil {
			t.Fatalf("mkdir specs: %v", err)
		}
		generator := builders.NewSpecGenerator(specsDir)
		if err := generator.GenerateAllSpecs(); err != nil {
			t.Logf("Warning: GenerateAllSpecs (tests may still work): %v", err)
		}
	}

	factory := storage.NewTestingFactory()
	factory.ConfigureTestGlobals(t, &storagetesting.GlobalHookOptions{NoopStorageMetrics: o.NoopStorageMetrics})

	if o.DisableAuditBuffer {
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
	}

	storageProvider, err := factory.CreateFileStorage(testRoot)
	if err != nil {
		t.Fatalf("CreateFileStorage: %v", err)
	}

	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)
	secCtx := pkgctx.NewSystemSecurityContext()

	// Two-phase teardown: Shutdown first (closing write-behind and index writers), then RunProjectTestTeardown.
	cleanup := func() {
		if storageProvider == nil {
			return
		}
		if s, ok := storageProvider.(interface{ Shutdown(context.Context) error }); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.Shutdown(ctx)
		}
		if s, ok := storageProvider.(interface{ GetTestCleanup() func() }); ok {
			if fn := s.GetTestCleanup(); fn != nil {
				fn()
			}
		}
	}
	t.Cleanup(cleanup)

	return &schedulerCompleteTestEnvironment{
		TestRoot:        testRoot,
		Storage:         storageProvider,
		SpecLoader:      specLoader,
		LifecycleLoader: lifecycleLoader,
		SecurityContext: secCtx,
		Cleanup:         cleanup,
	}
}
