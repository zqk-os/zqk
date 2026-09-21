package localci

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Demote removes Local CI worktrees and pointer files.
func Demote(ctx context.Context, opt Options) error {
	root := strings.TrimSpace(opt.RepoRoot)
	if root == "" {
		return errfmt.Errorf("project root not found")
	}
	base := BaseDir(root, opt.BaseDir)
	git := opt.git()
	workdir := workdirPath(base)

	st, err := fileutil.Lstat(workdir)
	switch {
	case err == nil && st.Mode()&fileutil.ModeSymlink != 0:
		target, rerr := fileutil.Readlink(workdir)
		if rerr == nil && strings.TrimSpace(target) != "" {
			abs := target
			if !filepath.IsAbs(target) {
				abs = filepath.Join(base, target)
			}
			if dirExists(abs) {
				_, _ = git(ctx, root, "worktree", "remove", "--force", abs)
				_ = fileutil.RemoveAll(abs)
			}
		}
		_ = fileutil.Remove(workdir)
	case err == nil && st.IsDir():
		_, _ = git(ctx, root, "worktree", "remove", "--force", workdir)
		_ = fileutil.RemoveAll(workdir)
	}

	trees := treesDir(base)
	if entries, rerr := fileutil.ReadDir(trees); rerr == nil {
		for _, e := range entries {
			p := filepath.Join(trees, e.Name())
			_, _ = git(ctx, root, "worktree", "remove", "--force", p)
			_ = fileutil.RemoveAll(p)
		}
	}

	for _, name := range []string{paths.LocalCISourceSHAFile, paths.LocalCIPromotedAtFile, paths.LocalCICurrentEnvFile} {
		_ = fileutil.Remove(filepath.Join(base, name))
	}
	return nil
}
