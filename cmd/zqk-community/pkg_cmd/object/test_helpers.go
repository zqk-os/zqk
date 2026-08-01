package object

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1" // Register instance builders (was via pkg/testing)
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"          // Register spec builders (was via pkg/testing)
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
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
		tmpBinDir, err := os.MkdirTemp("", "zqk-shared-bin-*")
		if err != nil {
			sharedCLIBuildErr = fmt.Errorf("failed to create global temp dir: %w", err)
			return
		}
		sharedCLIBinary = filepath.Join(tmpBinDir, "zqk-admin")
		// Community candidate builds from cmd/zqk-community (no cmd/zqk tree).
		buildCmd := exec.Command("go", "build", "-o", sharedCLIBinary, "./cmd/zqk-community")
		zqkenv.WireExecForIsolatedProject(buildCmd, projectRoot)
		// buildCmd.Env = os.Environ() removed to preserve WireExecForIsolatedProject env
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

	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Set ZQK_TEST_ROOT environment variable for test isolation
	// This ensures the CLI uses test data instead of project data
	os.Setenv(zqkenv.TestRoot(), tmpDir)
	originalMockGraph := os.Getenv(zqkenv.MockGraph())
	originalMockGraphAdmin := os.Getenv(zqkenv.AdminMockGraph())
	originalMockGraphLegacy := os.Getenv("MOCK_GRAPH")
	originalGraphEnabled := os.Getenv(zqkenv.GraphEnabled())
	originalGraphEnabledAdmin := os.Getenv(zqkenv.AdminGraphEnabled())
	os.Setenv(zqkenv.MockGraph(), "false")
	os.Setenv(zqkenv.AdminMockGraph(), "false")
	os.Setenv("MOCK_GRAPH", "false")
	os.Setenv(zqkenv.GraphEnabled(), "false")
	os.Setenv(zqkenv.AdminGraphEnabled(), "false")
	t.Cleanup(func() {
		if originalMockGraph != "" {
			os.Setenv(zqkenv.MockGraph(), originalMockGraph)
		} else {
			os.Unsetenv(zqkenv.MockGraph())
		}
		if originalMockGraphAdmin != "" {
			os.Setenv(zqkenv.AdminMockGraph(), originalMockGraphAdmin)
		} else {
			os.Unsetenv(zqkenv.AdminMockGraph())
		}
		if originalMockGraphLegacy != "" {
			os.Setenv("MOCK_GRAPH", originalMockGraphLegacy)
		} else {
			os.Unsetenv("MOCK_GRAPH")
		}
		if originalGraphEnabled != "" {
			os.Setenv(zqkenv.GraphEnabled(), originalGraphEnabled)
		} else {
			os.Unsetenv(zqkenv.GraphEnabled())
		}
		if originalGraphEnabledAdmin != "" {
			os.Setenv(zqkenv.AdminGraphEnabled(), originalGraphEnabledAdmin)
		} else {
			os.Unsetenv(zqkenv.AdminGraphEnabled())
		}
		if cliBinary := os.Getenv(zqkenv.TestCLIBinary()); cliBinary != "" {
			stopCmd := exec.Command(cliBinary, "scheduler", "stop", "--force")
			zqkenv.WireExecForIsolatedProject(stopCmd, tmpDir)
			_ = stopCmd.Run()
		} else if sharedCLIBinary != "" {
			stopCmd := exec.Command(sharedCLIBinary, "scheduler", "stop", "--force")
			zqkenv.WireExecForIsolatedProject(stopCmd, tmpDir)
			_ = stopCmd.Run()
		}
		if err := testkit.RunStandardTeardown(testkit.TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("testkit storage teardown: %v", err)
		}
		os.RemoveAll(tmpDir)
	})

	if _, err := setupObjectTestEnvironmentRoot(tmpDir); err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}

	// Create object_specs under process internal layout and copy spec files (see pkg/paths constants).
	specsDir := filepath.Join(tmpDir, paths.ProcessInternalObjectSpecsDir)
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
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
	if err := os.MkdirAll(configsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}
	sourceConfigsDir := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir)
	if err := copySpecFiles(sourceConfigsDir, configsDir); err != nil {
		t.Fatalf("failed to copy config files: %v", err)
	}

	sourceCLISpecs := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CLISpecsDir)
	targetCLISpecs := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CLISpecsDir)
	if err := copyCliSpecsTree(sourceCLISpecs, targetCLISpecs); err != nil {
		t.Fatalf("failed to copy CLI specs tree: %v", err)
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
				if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
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
			if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
				t.Fatalf("failed to create legacy kind dir %s: %v", kindDir, err)
			}
		}
	}

	// Prefer a pre-built binary when set and present (scheduler/CI) to avoid per-test go build.
	// If ZQK_TEST_CLI_BINARY points at a missing path (stale env, binary not installed yet), fall back to building.
	cliBinary := os.Getenv(zqkenv.TestCLIBinary())
	if cliBinary != emptyValue {
		if _, err := os.Stat(cliBinary); err != nil {
			cliBinary = ""
		}
	}
	if cliBinary == emptyValue {
		sharedBin := getSharedCLIBinary(t, projectRoot)
		cliBinary = filepath.Join(tmpDir, "zqk-admin")
		data, err := os.ReadFile(sharedBin)
		if err != nil {
			t.Fatalf("failed to read shared CLI binary: %v", err)
		}
		if err := os.WriteFile(cliBinary, data, 0o755); err != nil { //nolint:gosec // test binary needs execution permissions
			t.Fatalf("failed to write CLI binary to temp dir: %v", err)
		}
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
	cmd := exec.Command(te.CLIBinary, args...)
	zqkenv.WireExecForIsolatedProject(cmd, te.TestRoot)
	return cmd
}

// EnvWithTestRoot returns os.Environ() with ZQK_TEST_ROOT set to testRoot (replacing any existing).
// It also strips ZQK_PROJECT_ROOT so subprocess resolution matches [zqkenv.SubprocessEnvironWithTestRoot].
// Use this for any exec.Cmd that invokes the CLI so parallel tests do not share the wrong root.
func EnvWithTestRoot(testRoot string) []string {
	env := zqkenv.SubprocessEnvironWithTestRoot(testRoot)
	env = append(env, "ZQK_TEST_BYPASS_AUTH=1")
	env = append(env, "ZQK_API_KEY=account:system")
	env = append(env, "ZQK_ADMIN_TEST_BYPASS_AUTH=1")
	env = append(env, "ZQK_ADMIN_API_KEY=account:system")
	return env
}

// findProjectRoot finds the project root by looking for go.mod
func findProjectRoot(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root")
		}
		dir = parent
	}
}

// copySpecFiles copies spec files from source to target directory
func copySpecFiles(sourceDir, targetDir string) error {
	// Check if source directory exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		// If source doesn't exist, that's okay - tests might work without it
		return nil
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		sourcePath := filepath.Join(sourceDir, entry.Name())
		targetPath := filepath.Join(targetDir, entry.Name())

		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}

		if err := os.WriteFile(targetPath, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			return err
		}
	}

	return nil
}

// copyCliSpecsTree copies the entire `docs/process/command_specs` tree so subprocess CLIs using CommandSpecBuilder
// resolve spec_ref files under isolated test roots (same layout as repository).
func copyCliSpecsTree(srcRoot, dstRoot string) error {
	if _, err := os.Stat(srcRoot); os.IsNotExist(err) {
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
			return os.MkdirAll(dstRoot, paths.DirPerm755)
		}
		out := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(out, paths.DirPerm755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), paths.DirPerm755); err != nil {
			return err
		}
		return os.WriteFile(out, data, paths.FilePerm644)
	})
}
