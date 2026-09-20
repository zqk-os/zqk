package coordination

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storagetesting"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1" // Register instance builders for GenerateAllSpecs
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"          // Register spec builders for GenerateAllSpecs
)

type coordinationTestEnvOptions struct {
	CopySpecs          bool
	GenerateSpecs      bool
	DisableAuditBuffer bool
	NoopStorageMetrics bool
}

func defaultCoordinationTestEnvOptions() coordinationTestEnvOptions {
	return coordinationTestEnvOptions{
		CopySpecs:          true,
		GenerateSpecs:      true,
		DisableAuditBuffer: true,
		NoopStorageMetrics: true,
	}
}

type testSettingsYAMLShapeCoord struct {
	Version string         `yaml:"version"`
	Paths   map[string]any `yaml:"paths"`
}

func writeMinimalTestSettingsYAMLCoord(testRoot string) error {
	p := filepath.Join(testRoot, paths.TestSettingsFilename)
	body := testSettingsYAMLShapeCoord{
		Version: paths.DefaultBrandSettingsVersion,
		Paths:   map[string]any{},
	}
	data, err := yaml.Marshal(body)
	if err != nil {
		return err
	}
	return fileutil.WriteSecureFile(p, data) //nolint:gosec // test file
}

// layoutCoordinationTestRootFull mirrors pkg/testing.SetupTestEnvironment: project layout + test-settings.yaml.
func layoutCoordinationTestRootFull(t *testing.T, testRoot string) {
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
	if err := writeMinimalTestSettingsYAMLCoord(absRoot); err != nil {
		t.Fatalf("test-settings.yaml: %v", err)
	}
}

// moduleRootFromGoEnvCoordOrEmpty resolves the module directory via go env GOMOD (same as checkout root for normal runs).
// On failure or missing module, returns "" and logs once (mirrors pkg/testing findProjectRoot when unset).
func moduleRootFromGoEnvCoordOrEmpty(t *testing.T) string {
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

func moduleRootFromGoEnvCoord(t *testing.T) string {
	t.Helper()
	mod := moduleRootFromGoEnvCoordOrEmpty(t)
	if mod == "" {
		t.Skip("no module root (GOMOD empty or not in module context)")
	}
	return mod
}

func copyObjectSpecYAMLFilesToTestRootCoord(testRoot, projectRoot string) error {
	return testenvroot.CopyObjectSpecsFromProject(testRoot, projectRoot)
}

// ensureObjectSpecsForCoordTest lays out a full test project root and copies object_specs from the module (go env GOMOD).
// Skips when copy fails (e.g. not running from a checkout). Coordination cannot import storage *_test.go helpers.
func ensureObjectSpecsForCoordTest(t *testing.T, testRoot string) {
	t.Helper()
	layoutCoordinationTestRootFull(t, testRoot)
	mod := moduleRootFromGoEnvCoord(t)
	if err := copyObjectSpecYAMLFilesToTestRootCoord(testRoot, mod); err != nil {
		t.Skipf("copy object specs (run from module checkout): %v", err)
	}
}

// setupCoordinationCompleteTestEnvironment mirrors pkg/testing.SetupCompleteTestEnvironment for storage.NewTestingFactory().
// When opts is nil, uses defaults equivalent to DefaultTestEnvironmentOptions (NoopStorageMetrics true, copy+generate on).
// Uses os.Setenv for ZQK_TEST_ROOT (safe with t.Parallel(); do not use t.Setenv here).
func setupCoordinationCompleteTestEnvironment(t *testing.T, opts *coordinationTestEnvOptions) (testRoot string, fileStorage storage.ObjectStorageProvider, cleanup func()) {
	t.Helper()
	o := defaultCoordinationTestEnvOptions()
	if opts != nil {
		o = *opts
	}

	testRoot = t.TempDir()

	origTestRoot := zqkenv.TestRoot().Get()
	if err := zqkenv.TestRoot().Set(testRoot); err != nil {
		t.Fatalf("setenv %s: %v", zqkenv.TestRoot(), err)
	}
	t.Cleanup(func() {
		if origTestRoot != "" {
			_ = zqkenv.TestRoot().Set(origTestRoot) //nolint:errcheck // test restore
		} else {
			_ = zqkenv.TestRoot().Unset() //nolint:errcheck // test restore
		}
	})

	layoutCoordinationTestRootFull(t, testRoot)

	if o.CopySpecs {
		mod := moduleRootFromGoEnvCoordOrEmpty(t)
		if mod == "" {
			t.Logf("Warning: could not resolve module root for spec copy")
		} else if err := copyObjectSpecYAMLFilesToTestRootCoord(testRoot, mod); err != nil {
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

	sAny, err := factory.CreateFileStorage(testRoot)
	if err != nil {
		t.Fatalf("CreateFileStorage: %v", err)
	}
	fileStorage = sAny.(storage.ObjectStorageProvider)

	cleanup = func() {
		if sAny == nil {
			return
		}
		if s, ok := sAny.(interface{ GetTestCleanup() func() }); ok {
			if fn := s.GetTestCleanup(); fn != nil {
				fn()
			}
		}
	}
	t.Cleanup(cleanup)
	return testRoot, fileStorage, cleanup
}
