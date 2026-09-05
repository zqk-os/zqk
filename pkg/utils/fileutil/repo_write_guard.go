package fileutil

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

var (
	cachedRepoRoot string
	cachedIsInTest bool
	envOnce        sync.Once
)

func fileutilEnv() (repoRoot string, inTest bool) {
	envOnce.Do(func() {
		cachedRepoRoot = paths.FindWorkspaceRoot(".")
		cachedIsInTest = zqkenv.IsInTest()
	})
	return cachedRepoRoot, cachedIsInTest
}

// guardRepoMutation panics in tests when path would mutate live repo process state.
// Production (non-test) writes are not gated here.
func guardRepoMutation(path string) {
	if path == "" {
		return
	}
	repo, inTest := fileutilEnv()
	if !inTest {
		return
	}
	testRoot := zqkenv.Get(zqkenv.TestRoot()).Val
	if err := ValidateNoRepoStateMutation(path, repo, testRoot); err != nil {
		// TRACK: [Fatal safety violation]
		panic(err)
	}
}

// ValidateNoRepoStateMutation ensures that targetPath does not write into protected
// repo process state (docs/process, .zqk, .zqk-state) when running in a test root context.
func ValidateNoRepoStateMutation(targetPath, repoRoot, testRoot string) error {
	if targetPath == "" || repoRoot == "" {
		return nil
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return errfmt.Errorf("failed to resolve target path: %w", err)
	}
	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return errfmt.Errorf("failed to resolve repo root: %w", err)
	}
	absTest := ""
	if testRoot != "" {
		if t, err := filepath.Abs(testRoot); err == nil {
			absTest = t
		}
	}

	if absTest != "" && strings.HasPrefix(absTarget, absTest+string(PathSeparator)) {
		return nil
	}

	protectedDirs := []string{
		filepath.Join(absRepo, paths.ProcessDir),
		filepath.Join(absRepo, paths.ProjectDataDir),
		filepath.Join(absRepo, ".zqk-state"),
	}

	for _, protected := range protectedDirs {
		if absTarget == protected || strings.HasPrefix(absTarget, protected+string(PathSeparator)) {
			return errfmt.Errorf("test isolation violation: write target %s falls inside protected repo state %s", absTarget, protected)
		}
	}

	return nil
}
