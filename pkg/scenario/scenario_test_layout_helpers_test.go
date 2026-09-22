package scenario

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storagetesting"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"
)

type scenarioTestEnvOptions struct {
	CopySpecs          bool
	GenerateSpecs      bool
	DisableAuditBuffer bool
	NoopStorageMetrics bool
}

func defaultScenarioTestEnvOptions() scenarioTestEnvOptions {
	return scenarioTestEnvOptions{
		CopySpecs:          true,
		GenerateSpecs:      true,
		DisableAuditBuffer: true,
		NoopStorageMetrics: true,
	}
}

type testSettingsYAMLShapeScenario struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLScenario(testRoot string) error {
	configDir := filepath.Join(testRoot, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		return err
	}
	p := filepath.Join(configDir, paths.ZqkTestConfigFileName)
	body := testSettingsYAMLShapeScenario{
		Version: paths.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

func layoutScenarioTestRootFull(t *testing.T, testRoot string) {
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
	if err := writeMinimalTestSettingsYAMLScenario(absRoot); err != nil {
		t.Fatalf("config/zqk-test.yaml: %v", err)
	}
}

func moduleRootFromGoEnvScenarioOrEmpty(t *testing.T) string {
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

func yamlSpecExtensionOKScenario(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}

func copyObjectSpecYAMLFilesToTestRootScenario(testRoot, projectRoot string) error {
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
		if entry.IsDir() || !yamlSpecExtensionOKScenario(entry.Name()) {
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

// scenarioCompleteTestEnvironment mirrors pkg/testing.TestEnvironment fields used by scenario tests.
type scenarioCompleteTestEnvironment struct {
	TestRoot        string
	Storage         any
	SpecLoader      *objects.SpecLoader
	LifecycleLoader *objects.LifecycleLoader
	SecurityContext *pkgctx.SecurityContext
	Cleanup         func()
}

// setupScenarioCompleteTestEnvironment mirrors pkg/testing.SetupCompleteTestEnvironment(t, nil, storage.NewTestingFactory()).
// When opts is nil, uses defaults equivalent to DefaultTestEnvironmentOptions.
func setupScenarioCompleteTestEnvironment(t *testing.T) *scenarioCompleteTestEnvironment {
	t.Helper()
	o := defaultScenarioTestEnvOptions()

	testRoot := t.TempDir()

	origTestRoot := zqkenv.TestRoot().Get()
	if err := zqkenv.TestRoot().Set(testRoot); err != nil {
		t.Fatalf("setenv %s: %v", zqkenv.TestRoot(), err)
	}
	// Avoid ApplyIsolatedStorageEnv(t.Setenv): callers may use t.Parallel().
	// TestRoot enables membrane local write (privilegedWriterLocalWriteAllowed).
	t.Cleanup(func() {
		if origTestRoot != "" {
			_ = zqkenv.TestRoot().Set(origTestRoot) //nolint:errcheck // test restore
		} else {
			_ = zqkenv.TestRoot().Unset() //nolint:errcheck // test restore
		}
	})

	mod := moduleRootFromGoEnvScenarioOrEmpty(t)
	if mod != "" {
		if err := testenvroot.BootstrapRoot(testRoot, mod); err != nil {
			t.Fatalf("testenvroot.BootstrapRoot: %v", err)
		}
	} else {
		layoutScenarioTestRootFull(t, testRoot)
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

	return &scenarioCompleteTestEnvironment{
		TestRoot:        testRoot,
		Storage:         storageProvider,
		SpecLoader:      specLoader,
		LifecycleLoader: lifecycleLoader,
		SecurityContext: secCtx,
		Cleanup:         cleanup,
	}
}
