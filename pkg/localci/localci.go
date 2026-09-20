// Package localci implements studio-local CI checkout and demote in pure Go.
// Production must not shell out to scripts/local-ci-*.sh.
package localci

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const defaultArchiveKeep = 5

// GitRun executes git with args in dir (stdout+stderr). Tests inject fakes.
type GitRun func(ctx context.Context, dir string, args ...string) ([]byte, error)

// Options configures Checkout / Demote.
type Options struct {
	RepoRoot    string
	BaseDir     string // empty → env LOCAL_CI_DIR or <root>/<project-data>/local-ci
	SHA         string // empty → HEAD
	SHAPinned   bool   // true when the operator passed --sha (skips dirty check)
	AllowDirty  bool
	Archive     bool
	ArchiveKeep int
	Git         GitRun
}

func (o Options) git() GitRun {
	if o.Git != nil {
		return o.Git
	}
	return runGit
}

func (o Options) keep() int {
	if o.ArchiveKeep > 0 {
		return o.ArchiveKeep
	}
	if n := zqkenv.LocalCIArchiveKeep().IntOrDefault(0); n > 0 {
		return n
	}
	return defaultArchiveKeep
}

// BaseDir resolves the Local CI directory for repoRoot.
func BaseDir(repoRoot, override string) string {
	if s := strings.TrimSpace(override); s != "" {
		return s
	}
	if env := strings.TrimSpace(zqkenv.LocalCIDir().Get()); env != "" {
		return env
	}
	root := strings.TrimSpace(repoRoot)
	return filepath.Join(root, paths.ProjectDataDir, paths.LocalCIDir)
}

func workdirPath(base string) string {
	return filepath.Join(base, paths.LocalCIWorkdirName)
}

func treesDir(base string) string {
	return filepath.Join(base, paths.LocalCITreesDir)
}

func sourceSHAPath(base string) string {
	return filepath.Join(base, paths.LocalCISourceSHAFile)
}
