package fileutil

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	defaultProcessDir     = ".zqk/process"
	defaultProjectDataDir = ".zqk"
)

var (
	cachedRepoRoot string
	cachedIsInTest bool
	envOnce        sync.Once
)

func findWorkspaceRoot(startPath string) string {
	dir, err := filepath.Abs(startPath)
	if err != nil {
		dir = startPath
	}
	for {
		if _, err := Stat(filepath.Join(dir, defaultProjectDataDir)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func fileutilEnv() (repoRoot string, inTest bool) {
	envOnce.Do(func() {
		cachedRepoRoot = findWorkspaceRoot(".")
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
	testRoot := zqkenv.Get(zqkenv.TestRoot().Name()).Val
	if err := ValidateNoRepoStateMutation(path, repo, testRoot); err != nil {
		// TRACK: [Fatal safety violation]
		panic(err)
	}
}

// ValidateNoRepoStateMutation ensures that targetPath does not write into protected
// repo process state (.zqk/process, .zqk, .zqk-state) when running in a test root context.
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
		filepath.Join(absRepo, defaultProcessDir),
		filepath.Join(absRepo, defaultProjectDataDir),
		filepath.Join(absRepo, ".zqk-state"),
		filepath.Join(absRepo, "docs", "process"),
	}

	for _, protected := range protectedDirs {
		if absTarget == protected || strings.HasPrefix(absTarget, protected+string(PathSeparator)) {
			return errfmt.Errorf("test isolation violation: write target %s falls inside protected repo state %s", absTarget, protected)
		}
	}

	return nil
}
