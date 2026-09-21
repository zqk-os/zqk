package testkit

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

var isolatedTempProjectOnce sync.Once

const isolatedTempProjectPipelinePrefix = "test.temp_project"

// IsolatedTempProject is the result of [PrepareIsolatedTempProject] or
// [PrepareIsolatedTempProjectForBenchmark].
type IsolatedTempProject struct {
	Root        string
	FileStorage *storage.FileObjectStorage
}

// IsolatedTempProjectOptions configures [PrepareIsolatedTempProject] and
// [PrepareIsolatedTempProjectForBenchmark].
// A nil options value is treated the same as a pointer to the zero struct.
type IsolatedTempProjectOptions struct {
	// SkipSetupTestEnvironment skips [testenvroot.Setup] when tests lay out dirs manually.
	SkipSetupTestEnvironment bool
	// ForceRemoveRootOnCleanup registers os.RemoveAll(Root) in t.Cleanup before storage teardown
	// (LIFO: storage teardown runs first) so t.TempDir() removal succeeds after heavy teardown.
	ForceRemoveRootOnCleanup bool
	// Kind is a short suffix for pipeline observability (e.g. "reports", "scheduler"). Empty uses "default".
	Kind string
	// SeedSchemaPlane copies the whole schema plane — object specs, lifecycles, traits, and
	// configs — from the real project into the temp root before storage is built.
	//
	// Prefer this over copying those directories from AppendStagesBeforeStorage. Hand-rolled
	// seeding requires each caller to know which parts of .zqk/specs the code
	// under test will reach, and the plane is coupled: FileObjectStorage binds a
	// LifecycleLoader to the temp root, so specs without lifecycles fail every Create with
	// "failed to read lifecycle file" — a failure that names a missing file rather than the
	// missing setup step. Requesting the plane as a unit removes that judgment call.
	//
	// Runs before AppendStagesBeforeStorage so callers can still write fixture-specific specs
	// on top of the seeded copies.
	SeedSchemaPlane bool
	// AppendStagesBeforeStorage, if non-nil, returns extra [NamedTestStep] values to run through
	// [RunNamedTestSteps] after optional SetupTestEnvironment and before [storage.NewFileObjectStorageForTest].
	AppendStagesBeforeStorage func(root string) []NamedTestStep
	// UsePlainFileObjectStorage uses [storage.NewFileObjectStorage] instead of [storage.NewFileObjectStorageForTest].
	// Prefer for benchmarks or cases that must match production wiring (default is ForTest + teardown).
	UsePlainFileObjectStorage bool
	// SkipFileStorage leaves FileStorage nil. Required for greenfield tests that must start from a
	// bare root, since constructing storage needs .zqk/process to already exist.
	SkipFileStorage bool
}

// PrepareIsolatedTempProject allocates an isolated project under [testing.T.TempDir], binds
// ZQK_TEST_ROOT via [testing.T.Setenv], runs bootstrap stages through [RunNamedTestSteps]
// (sync test config, optional SetupTestEnvironment, optional extra stages), creates synchronous
// file storage, and registers [RegisterTempProjectTeardown].
//
// For child zqk processes, use WireCLISubprocessForIsolatedProject (or pkg/zqkenv WireExecForIsolatedProject)
// so subprocess env does not inherit ZQK_PROJECT_ROOT / ZQK_TEST_DATA_DIR from the parent.
//
// Callers must not use [testing.T.Parallel] on the same *testing.T ([testing.T.Setenv] restriction).
func PrepareIsolatedTempProject(t *testing.T, opts *IsolatedTempProjectOptions) IsolatedTempProject {
	t.Helper()
	root := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), root)
	// Clear inherited ZQK_PROJECT_ROOT (e.g. Local CI worktree) so TestRoot wins for all
	// paths that prefer PROJECT_ROOT. TRACK: .
	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	t.Setenv(zqkenv.InTest().Name(), "true")
	zqkenv.ApplyIsolatedStorageEnv(t.Setenv)
	bindRepositoryCLIBinary(t.Setenv)

	return runIsolatedTempProjectPipeline(t, root, opts)
}

// PrepareIsolatedTempProjectForBenchmark is like [PrepareIsolatedTempProject] for benchmarks: it uses
// [testing.B.TempDir], sets ZQK_TEST_ROOT with [os.Setenv], and restores the previous value in [testing.B.Cleanup].
// It registers the same storage teardown as the test helper.
func PrepareIsolatedTempProjectForBenchmark(b *testing.B, opts *IsolatedTempProjectOptions) IsolatedTempProject {
	b.Helper()
	root := b.TempDir()
	b.Setenv(zqkenv.TestRoot().Name(), root)
	b.Setenv(zqkenv.ProjectRoot().Name(), "")
	b.Setenv(zqkenv.InTest().Name(), "true")
	zqkenv.ApplyIsolatedStorageEnv(b.Setenv)
	bindRepositoryCLIBinary(b.Setenv)

	b.Cleanup(func() {
		objects.ResetGlobalKindMapperForTesting()
	})

	objects.ResetGlobalKindMapperForTesting()
	return runIsolatedTempProjectPipeline(b, root, opts)
}

func runIsolatedTempProjectPipeline(tb testing.TB, root string, opts *IsolatedTempProjectOptions) IsolatedTempProject {
	tb.Helper()
	if opts == nil {
		opts = &IsolatedTempProjectOptions{}
	}

	isolatedTempProjectOnce.Do(func() {
		caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	})

	if opts.ForceRemoveRootOnCleanup {
		tb.Cleanup(func() { _ = fileutil.RemoveAll(root) })
	}

	kind := opts.Kind
	if kind == "" {
		kind = "default"
	}

	var steps []NamedTestStep
	if !opts.SkipSetupTestEnvironment {
		steps = append(steps, NamedTestStep{
			Name: "setup_test_environment",
			Fn: func() error {
				_, err := testenvroot.Setup(root)
				return err
			},
		})
	}
	if opts.SeedSchemaPlane {
		steps = append(steps, seedSchemaPlaneStep(root))
	}
	if opts.AppendStagesBeforeStorage != nil {
		steps = append(steps, opts.AppendStagesBeforeStorage(root)...)
	}

	pipelineKind := isolatedTempProjectPipelinePrefix + "." + kind
	if err := RunNamedTestSteps(context.Background(), pipelineKind, steps...); err != nil {
		tb.Fatalf("testkit: isolated temp project pipeline %q: %v", pipelineKind, err)
	}

	if opts.SkipFileStorage {
		RegisterTempProjectTeardown(tb, root, nil)
		return IsolatedTempProject{Root: root}
	}

	var fs *storage.FileObjectStorage
	var err error
	if opts.UsePlainFileObjectStorage {
		fs, err = storage.NewFileObjectStorage(root)
	} else {
		fs, err = storage.NewFileObjectStorageForTest(root)
	}
	if err != nil {
		tb.Fatalf("testkit: isolated temp project file storage: %v", err)
	}
	RegisterTempProjectTeardown(tb, root, fs)
	return IsolatedTempProject{Root: root, FileStorage: fs}
}

// seedSchemaPlaneStep copies the schema plane from the real project into the temp root.
//
// The four directories are copied together on purpose. Their contents reference each other —
// a spec names the lifecycle that governs it, and specs inherit through traits — so seeding a
// subset produces a root that loads until the moment a test creates the wrong kind. Letting
// callers choose is what put specs-without-lifecycles in ten test files.
func seedSchemaPlaneStep(testRoot string) NamedTestStep {
	return NamedTestStep{
		Name: "seed_schema_plane",
		Fn: func() error {
			projectRoot, err := moduleRootFromWorkingDir()
			if err != nil {
				return err
			}
			// BootstrapRoot already composes Setup with all four copies, so this delegates
			// rather than repeating the list — a sixth partial copy of the layout is the
			// problem this option exists to end.
			//
			// Argument order is (destination, source). Reversing it copies the empty temp
			// root over the repository's real schema plane; see
			// for the patch that did exactly that.
			return testenvroot.BootstrapRoot(testRoot, projectRoot)
		},
	}
}

// bindRepositoryCLIBinary points ZQK_BIN at the repository's built CLI when one is present, so
// child zqk processes run this tree's binary instead of whatever PATH offers. Absence is not an
// error: tests that never spawn a subprocess run fine without a build.
func bindRepositoryCLIBinary(setenv func(key, value string)) {
	modRoot, err := moduleRootFromWorkingDir()
	if err != nil {
		return
	}
	candidate := filepath.Join(modRoot, "bin", "zqk")
	if info, err := fileutil.Stat(candidate); err == nil && !info.IsDir() {
		setenv(zqkenv.Bin().Name(), candidate)
	}
}

// moduleRootFromWorkingDir resolves the repository root that seeding and ZQK_BIN read from.
// Tests run with the working directory inside their own package, so the module root is the
// only project reference available once ZQK_PROJECT_ROOT has been cleared for isolation.
func moduleRootFromWorkingDir() (string, error) {
	wd, err := fileutil.Getwd()
	if err != nil {
		return "", err
	}
	return paths.ModuleRootFromPath(wd)
}
