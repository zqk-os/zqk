package scheduler

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clctx "github.com/lanceman/zqk/internal/cli/context"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/storagetesting"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"gopkg.in/yaml.v3"

	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"
)

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
	p := filepath.Join(testRoot, paths.TestSettingsFilename)
	body := testSettingsYAMLShapeScheduler{
		Version: clctx.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

// setupSchedulerTestEnvironmentRoot mirrors pkg/testing.SetupTestEnvironment: project layout + test-settings.yaml only.
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
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Err(); err != nil {
		t.Fatalf("test project layout: %v", err)
	}
	if err := writeMinimalTestSettingsYAMLScheduler(absRoot); err != nil {
		t.Fatalf("test-settings.yaml: %v", err)
	}
}

func moduleRootFromGoEnvSchedulerOrEmpty(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Logf("go env GOMOD: %v", err)
		return ""
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" || modPath == "/dev/null" {
		return ""
	}
	return filepath.Dir(modPath)
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
		if entry.IsDir() || !yamlSpecExtensionOKScheduler(entry.Name()) {
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

	origTestRoot := os.Getenv(zqkenv.TestRoot())
	if err := os.Setenv(zqkenv.TestRoot(), testRoot); err != nil {
		t.Fatalf("setenv %s: %v", zqkenv.TestRoot(), err)
	}
	t.Cleanup(func() {
		if origTestRoot != "" {
			_ = os.Setenv(zqkenv.TestRoot(), origTestRoot) //nolint:errcheck // test restore
		} else {
			_ = os.Unsetenv(zqkenv.TestRoot()) //nolint:errcheck // test restore
		}
	})

	layoutSchedulerTestRootFull(t, testRoot)

	if o.CopySpecs {
		mod := moduleRootFromGoEnvSchedulerOrEmpty(t)
		if mod == "" {
			t.Logf("Warning: could not resolve module root for spec copy")
		} else if err := copyObjectSpecYAMLFilesToTestRootScheduler(testRoot, mod); err != nil {
			t.Logf("Warning: copy spec files (tests may still work): %v", err)
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

	// Prefer GetTestCleanup first: *storage.FileObjectStorage runs RunProjectTestTeardown (ITEM-177483 pipeline).
	cleanup := func() {
		if storageProvider == nil {
			return
		}
		if s, ok := storageProvider.(interface{ GetTestCleanup() func() }); ok {
			if fn := s.GetTestCleanup(); fn != nil {
				fn()
				return
			}
		}
		if s, ok := storageProvider.(interface{ Shutdown(context.Context) error }); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.Shutdown(ctx)
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
