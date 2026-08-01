package testkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testenvroot"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

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
	// AppendStagesBeforeStorage, if non-nil, returns extra [NamedTestStep] values to run through
	// [RunNamedTestSteps] after optional SetupTestEnvironment and before [storage.NewFileObjectStorageForTest].
	AppendStagesBeforeStorage func(root string) []NamedTestStep
	// UsePlainFileObjectStorage uses [storage.NewFileObjectStorage] instead of [storage.NewFileObjectStorageForTest].
	// Prefer for benchmarks or cases that must match production wiring (default is ForTest + teardown).
	UsePlainFileObjectStorage bool
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
	t.Setenv(zqkenv.TestRoot(), root)
	t.Setenv(zqkenv.InTest(), "true")

	// Resolve the built CLI binary under repository module root and set it to ZQK_BIN
	if wd, err := os.Getwd(); err == nil {
		if modRoot, err := paths.ModuleRootFromPath(wd); err == nil {
			candidate := filepath.Join(modRoot, "bin", "zqk")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				t.Setenv(zqkenv.Bin(), candidate)
			}
		}
	}

	return runIsolatedTempProjectPipeline(t, root, opts)
}

// PrepareIsolatedTempProjectForBenchmark is like [PrepareIsolatedTempProject] for benchmarks: it uses
// [testing.B.TempDir], sets ZQK_TEST_ROOT with [os.Setenv], and restores the previous value in [testing.B.Cleanup].
// It registers the same storage teardown as the test helper.
func PrepareIsolatedTempProjectForBenchmark(b *testing.B, opts *IsolatedTempProjectOptions) IsolatedTempProject {
	b.Helper()
	root := b.TempDir()
	origRoot := os.Getenv(zqkenv.TestRoot())
	origInTest := os.Getenv(zqkenv.InTest())
	origBin := os.Getenv(zqkenv.Bin())

	if err := os.Setenv(zqkenv.TestRoot(), root); err != nil {
		b.Fatalf("testkit.PrepareIsolatedTempProjectForBenchmark: Setenv: %v", err)
	}
	_ = os.Setenv(zqkenv.InTest(), "true")

	if wd, err := os.Getwd(); err == nil {
		if modRoot, err := paths.ModuleRootFromPath(wd); err == nil {
			candidate := filepath.Join(modRoot, "bin", "zqk")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				_ = os.Setenv(zqkenv.Bin(), candidate)
			}
		}
	}

	b.Cleanup(func() {
		if origRoot != "" {
			_ = os.Setenv(zqkenv.TestRoot(), origRoot)
		} else {
			_ = os.Unsetenv(zqkenv.TestRoot())
		}
		if origInTest != "" {
			_ = os.Setenv(zqkenv.InTest(), origInTest)
		} else {
			_ = os.Unsetenv(zqkenv.InTest())
		}
		if origBin != "" {
			_ = os.Setenv(zqkenv.Bin(), origBin)
		} else {
			_ = os.Unsetenv(zqkenv.Bin())
		}
	})
	return runIsolatedTempProjectPipeline(b, root, opts)
}

func runIsolatedTempProjectPipeline(tb testing.TB, root string, opts *IsolatedTempProjectOptions) IsolatedTempProject {
	tb.Helper()
	if opts == nil {
		opts = &IsolatedTempProjectOptions{}
	}

	if opts.ForceRemoveRootOnCleanup {
		tb.Cleanup(func() { _ = os.RemoveAll(root) })
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
	if opts.AppendStagesBeforeStorage != nil {
		steps = append(steps, opts.AppendStagesBeforeStorage(root)...)
	}

	pipelineKind := isolatedTempProjectPipelinePrefix + "." + kind
	if err := RunNamedTestSteps(context.Background(), pipelineKind, steps...); err != nil {
		tb.Fatalf("testkit: isolated temp project pipeline %q: %v", pipelineKind, err)
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
