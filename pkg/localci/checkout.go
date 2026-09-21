package localci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// Result is the outcome of a checkout promote.
type Result struct {
	SHA     string `json:"sha"`
	Short   string `json:"short"`
	Workdir string `json:"workdir"`
	Tree    string `json:"tree"`
	Skipped bool   `json:"skipped,omitempty"`
}

// Checkout promotes a committed SHA into a detached git worktree under BaseDir.
func Checkout(ctx context.Context, opt Options) (Result, error) {
	root := strings.TrimSpace(opt.RepoRoot)
	if root == "" {
		return Result{}, errfmt.Errorf("project root not found")
	}
	git := opt.git()
	shaIn := strings.TrimSpace(opt.SHA)
	if shaIn == "" {
		shaIn = "HEAD"
	}
	sha, err := gitTrim(ctx, git, root, "rev-parse", "--verify", shaIn+"^{commit}")
	if err != nil {
		return Result{}, err
	}
	short, err := gitTrim(ctx, git, root, "rev-parse", "--short", sha)
	if err != nil {
		return Result{}, err
	}

	if !opt.AllowDirty && !opt.SHAPinned {
		porc, perr := gitTrim(ctx, git, root, "status", "--porcelain", "--untracked-files=no")
		if perr != nil {
			return Result{}, perr
		}
		if hasTrackedDirty(porc) {
			return Result{}, errfmt.Errorf("studio has tracked changes; commit first or pass --allow-dirty / --sha <committed>")
		}
	}

	base := BaseDir(root, opt.BaseDir)
	workdir := workdirPath(base)
	trees := treesDir(base)
	tree := filepath.Join(trees, short)
	if err := fileutil.MkdirAll(trees, paths.DirPerm755); err != nil {
		return Result{}, errfmt.Newf("local-ci trees").Wrap(err)
	}
	if err := fileutil.MkdirAll(filepath.Join(base, paths.LocalCIArchivesDir), paths.DirPerm755); err != nil {
		return Result{}, errfmt.Newf("local-ci archives").Wrap(err)
	}

	_, _ = git(ctx, root, "worktree", "remove", "--force", tree)
	_ = fileutil.RemoveAll(tree)
	if _, err := git(ctx, root, "worktree", "add", "--detach", tree, sha); err != nil {
		return Result{}, err
	}

	if err := swingWorkdir(base, workdir, short, git, ctx, root); err != nil {
		return Result{}, err
	}

	if err := pruneOtherTrees(ctx, git, root, trees, short); err != nil {
		return Result{}, err
	}

	if err := fileutil.WriteFile(sourceSHAPath(base), []byte(sha+"\n"), paths.FilePerm644); err != nil {
		return Result{}, errfmt.Newf("write SOURCE_SHA").Wrap(err)
	}
	promoted := zqktime.NowRFC3339UTC()
	if err := fileutil.WriteFile(filepath.Join(base, paths.LocalCIPromotedAtFile), []byte(promoted+"\n"), paths.FilePerm644); err != nil {
		return Result{}, errfmt.Newf("write PROMOTED_AT").Wrap(err)
	}
	env := fmt.Sprintf("workdir=%s\nsha=%s\nshort=%s\ntree=%s\n", workdir, sha, short, tree)
	if err := fileutil.WriteFile(filepath.Join(base, paths.LocalCICurrentEnvFile), []byte(env), paths.FilePerm644); err != nil {
		return Result{}, errfmt.Newf("write CURRENT.env").Wrap(err)
	}

	if opt.Archive {
		if err := writeArchive(ctx, git, root, base, sha, short, promoted, opt.keep()); err != nil {
			return Result{}, err
		}
	}

	return Result{SHA: sha, Short: short, Workdir: workdir, Tree: tree}, nil
}

func dirExists(p string) bool {
	st, err := fileutil.Stat(p)
	return err == nil && st.IsDir()
}

func swingWorkdir(base, workdir, short string, git GitRun, ctx context.Context, root string) error {
	rel := filepath.Join(paths.LocalCITreesDir, short)
	st, err := fileutil.Lstat(workdir)
	if err != nil {
		if !fileutil.IsNotExist(err) {
			return err
		}
		return fileutil.Symlink(rel, workdir)
	}
	if st.Mode()&fileutil.ModeSymlink != 0 {
		_ = fileutil.Remove(workdir)
		return fileutil.Symlink(rel, workdir)
	}
	if st.IsDir() {
		legacy := filepath.Join(base, fmt.Sprintf("workdir.legacy.%d", os.Getpid()))
		if err := fileutil.Rename(workdir, legacy); err != nil {
			return errfmt.Newf("migrate legacy workdir").Wrap(err)
		}
		if err := fileutil.Symlink(rel, workdir); err != nil {
			return err
		}
		_, _ = git(ctx, root, "worktree", "remove", "--force", legacy)
		_ = fileutil.RemoveAll(legacy)
		return nil
	}
	return errfmt.Errorf("local-ci: refusing to clobber unexpected workdir at %s", workdir)
}

func pruneOtherTrees(ctx context.Context, git GitRun, root, trees, keepShort string) error {
	entries, err := fileutil.ReadDir(trees)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.Name() == keepShort {
			continue
		}
		p := filepath.Join(trees, e.Name())
		_, _ = git(ctx, root, "worktree", "remove", "--force", p)
		_ = fileutil.RemoveAll(p)
	}
	return nil
}

func writeArchive(ctx context.Context, git GitRun, root, base, sha, short, promoted string, keep int) error {
	archives := filepath.Join(base, paths.LocalCIArchivesDir)
	tarPath := filepath.Join(archives, "drop-"+short+".tar")
	if _, err := git(ctx, root, "archive", "--format=tar", "-o", tarPath, sha); err != nil {
		return err
	}
	sum, err := sha256File(tarPath)
	if err != nil {
		return err
	}
	archiveGit := filepath.Join(base, paths.LocalCIArchiveGitDir)
	if _, err := fileutil.Stat(filepath.Join(archiveGit, ".git")); err != nil {
		if err := fileutil.MkdirAll(archiveGit, paths.DirPerm755); err != nil {
			return errfmt.Newf("archive-git dir").Wrap(err)
		}
		if _, err := git(ctx, archiveGit, "init", "-q"); err != nil {
			return err
		}
	}
	drop := fmt.Sprintf("sha=%s\nshort=%s\ntar=%s\nsha256=%s\npromoted_at=%s\n",
		sha, short, filepath.Base(tarPath), sum, promoted)
	if err := fileutil.WriteFile(filepath.Join(archiveGit, "drop-"+short+".txt"), []byte(drop), paths.FilePerm644); err != nil {
		return err
	}
	manifest, err := listDropManifest(archiveGit)
	if err != nil {
		return err
	}
	if err := fileutil.WriteFile(filepath.Join(archiveGit, "MANIFEST"), []byte(manifest), paths.FilePerm644); err != nil {
		return err
	}
	if _, err := git(ctx, archiveGit, "add", "-A"); err != nil {
		return err
	}
	_, _ = git(ctx, archiveGit,
		"-c", "user.email=local-ci@localhost",
		"-c", "user.name=local-ci",
		"commit", "-q", "-m", "local-ci drop "+short+" ("+sha+")")
	return pruneOldTars(archives, keep)
}

func listDropManifest(archiveGit string) (string, error) {
	entries, err := fileutil.ReadDir(archiveGit)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, "drop-") && strings.HasSuffix(n, ".txt") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "", nil
	}
	return strings.Join(names, "\n") + "\n", nil
}

func sha256File(path string) (string, error) {
	f, err := fileutil.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func pruneOldTars(archives string, keep int) error {
	if keep <= 0 {
		keep = defaultArchiveKeep
	}
	entries, err := fileutil.ReadDir(archives)
	if err != nil {
		return nil
	}
	type item struct {
		name string
		mod  int64
	}
	var tars []item
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, "drop-") || !strings.HasSuffix(n, ".tar") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		tars = append(tars, item{name: n, mod: info.ModTime().UnixNano()})
	}
	sort.Slice(tars, func(i, j int) bool { return tars[i].mod > tars[j].mod })
	for i := keep; i < len(tars); i++ {
		_ = fileutil.Remove(filepath.Join(archives, tars[i].name))
	}
	return nil
}
