package object

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1" // Register instance builders (was via pkg/testing)
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2"          // Register spec builders (was via pkg/testing)
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// testEnvMu serializes tests that mutate process-wide env vars (notably ZQK_TEST_ROOT)
// so parallel tests don't race and load specs/lifecycles from the wrong temp root.
var testEnvMu sync.Mutex

var (
	sharedCLIBinary   string
	sharedCLIBuildErr error
	sharedCLIOnce     sync.Once
	prewarmOnce       sync.Once
)

func prewarmGlobalsOnce(projectRoot string) {
	prewarmOnce.Do(func() {
		objects.PrewarmGlobalsForProjectRoot(projectRoot)
	})
}

func getSharedCLIBinary(t *testing.T, projectRoot string) string {
	sharedCLIOnce.Do(func() {
		if envBin := zqkenv.SharedTestBin().Get(); envBin != "" {
			if _, err := os.Stat(envBin); err == nil {
				sharedCLIBinary = envBin
				return
			}
		}
		tmpBinDir, err := fileutil.MkdirTemp("", "zqk-shared-bin-*")
		if err != nil {
			sharedCLIBuildErr = fmt.Errorf("failed to create global temp dir: %w", err)
			return
		}
		sharedCLIBinary = filepath.Join(tmpBinDir, "zqk-admin")
		buildCmd := execwrap.Command("go", "build", "-o", sharedCLIBinary, "./cmd/zqk")
		buildCmd.Dir = projectRoot
		buildCmd.Env = os.Environ()
		output, err := buildCmd.CombinedOutput()
		if err != nil {
			sharedCLIBuildErr = fmt.Errorf("failed to build CLI: %v\n%s", err, string(output))
			return
		}
	})
	if sharedCLIBuildErr != nil {
		t.Fatalf("global build failure: %v", sharedCLIBuildErr)
	}
	return sharedCLIBinary
}

// TestEnvironment holds the test environment configuration
type TestEnvironment struct {
	TestRoot    string // The test root directory (ZQK_TEST_ROOT)
	CLIBinary   string // Path to the compiled CLI binary
	ProjectRoot string // The actual project root (for copying specs, etc.)
}

// SetupTestEnvironment creates a fully isolated test environment with:
// - Temporary directory for test data
// - ZQK_TEST_ROOT environment variable set
// - Test config initialized
// - Spec files copied
// - CLI binary built
// - Proper cleanup on test completion
func SetupTestEnvironment(t *testing.T) *TestEnvironment {
	testEnvMu.Lock()
	defer testEnvMu.Unlock()

	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Set ZQK_TEST_ROOT environment variable for test isolation
	// This ensures the CLI uses test data instead of project data
	_ = zqkenv.TestRoot().Set(tmpDir)
	originalMockGraph := zqkenv.RawMockGraph().Get()
	originalMockGraphAdmin := zqkenv.AdminMockGraph().Get()
	originalMockGraphLegacy := zqkenv.RawMockGraph().Get()
	originalGraphEnabled := zqkenv.RawGraphEnabled().Get()
	originalGraphEnabledAdmin := zqkenv.AdminGraphEnabled().Get()
	_ = zqkenv.RawMockGraph().Set("false")
	_ = zqkenv.AdminMockGraph().Set("false")
	_ = zqkenv.RawMockGraph().Set("false")
	_ = zqkenv.RawGraphEnabled().Set("false")
	_ = zqkenv.AdminGraphEnabled().Set("false")
	t.Cleanup(func() {
		if originalMockGraph != "" {
			_ = zqkenv.RawMockGraph().Set(originalMockGraph)
		} else {
			_ = zqkenv.RawMockGraph().Unset()
		}
		if originalMockGraphAdmin != "" {
			_ = zqkenv.AdminMockGraph().Set(originalMockGraphAdmin)
		} else {
			_ = zqkenv.AdminMockGraph().Unset()
		}
		if originalMockGraphLegacy != "" {
			_ = zqkenv.RawMockGraph().Set(originalMockGraphLegacy)
		} else {
			os.Unsetenv("MOCK_GRAPH")
		}
		if originalGraphEnabled != "" {
			_ = zqkenv.RawGraphEnabled().Set(originalGraphEnabled)
		} else {
			_ = zqkenv.RawGraphEnabled().Unset()
		}
		if originalGraphEnabledAdmin != "" {
			_ = zqkenv.AdminGraphEnabled().Set(originalGraphEnabledAdmin)
		} else {
			_ = zqkenv.AdminGraphEnabled().Unset()
		}
		if cliBinary := zqkenv.TestCLIBinary().Get(); cliBinary != "" {
			stopCmd := execwrap.Command(cliBinary, "scheduler", "stop", "--force")
			zqkenv.WireExecForIsolatedProject(stopCmd, tmpDir)
			_ = stopCmd.Run()
		} else if sharedCLIBinary != "" {
			stopCmd := execwrap.Command(sharedCLIBinary, "scheduler", "stop", "--force")
			zqkenv.WireExecForIsolatedProject(stopCmd, tmpDir)
			_ = stopCmd.Run()
		}
		if err := testkit.RunStandardTeardown(testkit.TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("testkit storage teardown: %v", err)
		}
		_ = fileutil.RemoveAll(tmpDir)
	})

	if _, err := setupObjectTestEnvironmentRoot(tmpDir); err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}

	// Create object_specs under process internal layout and copy spec files (see pkg/paths constants).
	specsDir := filepath.Join(tmpDir, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.EnsureDir(specsDir); err != nil {
		t.Fatalf("failed to create specs dir: %v", err)
	}

	// Copy spec files from project root
	projectRoot := findProjectRoot(t)
	sourceSpecsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := copySpecFiles(sourceSpecsDir, specsDir); err != nil {
		t.Fatalf("failed to copy spec files: %v", err)
	}

	// Copy required internal configs (e.g. id_prefixes_config.yaml) into the temp root.
	// These configs affect validation and kind/storage directory mappings.
	configsDir := filepath.Join(tmpDir, paths.ProcessInternalConfigsDir)
	if err := fileutil.EnsureDir(configsDir); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}
	sourceConfigsDir := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir)
	if err := copySpecFiles(sourceConfigsDir, configsDir); err != nil {
		t.Fatalf("failed to copy config files: %v", err)
	}

	// Copy lifecycles directory
	lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.EnsureDir(lifecyclesDir); err != nil {
		t.Fatalf("failed to create lifecycles dir: %v", err)
	}
	sourceLifecyclesDir := filepath.Join(projectRoot, paths.ProcessInternalLifecyclesDir)
	if err := copySpecFiles(sourceLifecyclesDir, lifecyclesDir); err != nil {
		t.Fatalf("failed to copy lifecycle files: %v", err)
	}

	sourceCLISpecs := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CLISpecsDir)
	targetCLISpecs := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CLISpecsDir)
	if err := copyCliSpecsTree(sourceCLISpecs, targetCLISpecs); err != nil {
		t.Fatalf("failed to copy CLI specs tree: %v", err)
	}

	// Copy spec_index.json and internal YAML configurations from .zqk/specs
	sourceProcessInternalDir := filepath.Join(projectRoot, paths.ProcessInternalDir)
	targetProcessInternalDir := filepath.Join(tmpDir, paths.ProcessInternalDir)
	_ = copySpecFiles(sourceProcessInternalDir, targetProcessInternalDir)
	sourceSpecIndex := filepath.Join(sourceProcessInternalDir, "spec_index.json")
	targetSpecIndex := filepath.Join(targetProcessInternalDir, "spec_index.json")
	if data, err := fileutil.ReadFile(sourceSpecIndex); err == nil {
		_ = fileutil.EnsureDir(targetProcessInternalDir)
		_ = fileutil.WriteStandardFile(targetSpecIndex, data)
	}

	// Dynamically pre-create directories for all known kinds in the registry
	// to ensure the kind mapper successfully registers them at startup.
	fieldRegistry := objects.GetGlobalFieldRegistry()
	_ = fieldRegistry.LoadFields() // Best effort loading
	if kinds, err := fieldRegistry.GetAllKinds(); err == nil {
		for _, kind := range kinds {
			dirName := objects.GetDirectoryFromKind(kind)
			if dirName != "" {
				kindDir := datacell.CellCASPrimaryDir(tmpDir, dirName)
				if err := fileutil.EnsureDir(kindDir); err != nil {
					t.Fatalf("failed to create kind dir %s for kind %s: %v", kindDir, kind, err)
				}
			}
		}
	} else {
		// Fallback to legacyDirs if field registry load fails
		legacyDirs := []string{
			"backlog", "goals", "milestones", "workstreams", "priority_plans",
			objects.KindCriteria, "requirements", "components", "accounts", "decisions",
			"audit", "change_journal", "metrics", "kind_synonyms", "code_references",
			"command_metrics", "context_refresh_policies", "corporate_initiatives",
			"displays", "doc_entries", "extensible_objects", "integrity_manifests",
			"metadata_packages", "personas", "releases", "resolvers", "risk_blockers",
			"roles", "rollback_reports", "rules", "scenarios", "scheduler_jobs",
			"templates", "questions", "technical_debt", "field_registry",
		}
		for _, dirName := range legacyDirs {
			kindDir := datacell.CellCASPrimaryDir(tmpDir, dirName)
			if err := fileutil.EnsureDir(kindDir); err != nil {
				t.Fatalf("failed to create legacy kind dir %s: %v", kindDir, err)
			}
		}
	}

	// Always build/copy CLI from projectRoot. Scheduler-injected ZQK_TEST_CLI_BINARY can point at a
	// daemon binary that predates the Local CI workdir checkout (e.g. template --output - semantics).
	// draft-plane / Local CI binary-SHA parity.
	sharedBin := getSharedCLIBinary(t, projectRoot)
	cliBinary := filepath.Join(tmpDir, "zqk-admin")
	data, err := fileutil.ReadFile(sharedBin)
	if err != nil {
		t.Fatalf("failed to read shared CLI binary: %v", err)
	}
	if err := fileutil.WriteExecutableFile(cliBinary, data); err != nil { //nolint:gosec // test binary needs execution permissions
		t.Fatalf("failed to write CLI binary to temp dir: %v", err)
	}

	// Stream-backed kinds and segment resolution use path alias cache (prefix:streams/<kind>).
	storage.BuildPathAliasCacheForProject(tmpDir)

	// Prewarm globals once using the project root, rather than resetting/re-prewarming on every temp root.
	prewarmGlobalsOnce(projectRoot)

	return &TestEnvironment{
		TestRoot:    tmpDir,
		CLIBinary:   cliBinary,
		ProjectRoot: projectRoot,
	}
}

// GetTestRoot returns the test root directory, ensuring it's set
func (te *TestEnvironment) GetTestRoot() string {
	if te.TestRoot == emptyValue {
		panic("TestEnvironment not properly initialized")
	}
	return te.TestRoot
}

// CreateCLICommand creates an exec.Cmd with the correct environment variables set
// and working directory so the CLI resolves the isolated test root (not the host cwd).
func (te *TestEnvironment) CreateCLICommand(args ...string) *exec.Cmd {
	hasAllowDegraded := false
	for _, arg := range args {
		if arg == "--allow-degraded" {
			hasAllowDegraded = true
			break
		}
	}
	if !hasAllowDegraded {
		isSchedulerOrHelp := false
		if len(args) > 0 {
			first := args[0]
			if first == "scheduler" || first == "version" || first == "help" || first == "new" || first == "workflow" {
				isSchedulerOrHelp = true
			}
		}
		if !isSchedulerOrHelp {
			args = append(args, "--allow-degraded")
		}
	}
	//nolint:gosec // G204: Test helper - CLI binary path is controlled, args are test inputs
	cmd := execwrap.Command(te.CLIBinary, args...)
	// Isolating paths is not enough to isolate identity: WireExecForIsolatedProject passes HOME
	// through, so a child with no explicit key falls back to $HOME/.zqk/credentials and authenticates
	// as whichever developer is logged in, then fails resolving that ZQK- session id against the
	// empty temp store. Supplying the key alone is not sufficient -- the credentials fallback still
	// wins -- so the bypass flags EnvWithTestRoot sets are load-bearing.
	//
	// Tests spell this as the single wireExecForTest call; this file is a non-test file and cannot
	// reference it, so the two-line form stays here.
	// identity isolation belongs in zqkenv, but that
	// function is also on production spawn paths, so widening it needs its own pass.
	zqkenv.WireExecForIsolatedProject(cmd, te.TestRoot)
	cmd.Env = EnvWithTestRoot(te.TestRoot)
	return cmd
}

// EnvWithTestRoot returns os.Environ() with ZQK_TEST_ROOT set to testRoot (replacing any existing).
// It also strips ZQK_PROJECT_ROOT so subprocess resolution matches [zqkenv.SubprocessEnvironWithTestRoot].
// Use this for any exec.Cmd that invokes the CLI so parallel tests do not share the wrong root.
func EnvWithTestRoot(testRoot string) []string {
	env := zqkenv.SubprocessEnvironWithTestRoot(testRoot)
	env = append(env, "ZQK_TEST_BYPASS_AUTH=1")
	env = append(env, "ZQK_API_KEY="+pkgctx.TestHarnessAccountID)
	env = append(env, "ZQK_ADMIN_TEST_BYPASS_AUTH=1")
	env = append(env, "ZQK_ADMIN_API_KEY="+pkgctx.TestHarnessAccountID)
	return env
}

// findProjectRoot finds the project root by looking for go.mod
func findProjectRoot(t *testing.T) string {
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root")
		}
		dir = parent
	}
}

// copySpecFiles copies spec files from source to target directory recursively
func copySpecFiles(sourceDir, targetDir string) error {
	// Check if source directory exists
	if _, err := fileutil.Stat(sourceDir); fileutil.IsNotExist(err) {
		// If source doesn't exist, that's okay - tests might work without it
		return nil
	}

	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return fileutil.EnsureDir(targetDir)
		}
		out := filepath.Join(targetDir, rel)
		if d.IsDir() {
			return fileutil.EnsureDir(out)
		}
		ext := filepath.Ext(d.Name())
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return err
		}
		if err := fileutil.EnsureDir(filepath.Dir(out)); err != nil {
			return err
		}
		return fileutil.WriteStandardFile(out, data)
	})
}

// copyCliSpecsTree copies the entire `.zqk/cli/specs` tree so subprocess CLIs using CommandSpecBuilder
// resolve spec_ref files under isolated test roots (same layout as repository).
func copyCliSpecsTree(srcRoot, dstRoot string) error {
	if _, err := fileutil.Stat(srcRoot); fileutil.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return fileutil.EnsureDir(dstRoot)
		}
		out := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return fileutil.EnsureDir(out)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			target, readlinkErr := os.Readlink(path)
			if readlinkErr == nil {
				_ = os.Remove(out)
				if err := fileutil.EnsureDir(filepath.Dir(out)); err != nil {
					return err
				}
				return os.Symlink(target, out)
			}
			info, statErr := fileutil.Stat(path)
			if statErr == nil && info.IsDir() {
				return fileutil.EnsureDir(out)
			}
		}
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return err
		}
		if err := fileutil.EnsureDir(filepath.Dir(out)); err != nil {
			return err
		}
		return fileutil.WriteStandardFile(out, data)
	})
}
