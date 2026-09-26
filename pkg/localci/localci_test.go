package localci

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestHasTrackedDirty(t *testing.T) {
	t.Parallel()
	if hasTrackedDirty("") {
		t.Fatal("empty porcelain is clean")
	}
	if hasTrackedDirty("?? untracked.txt\n") {
		t.Fatal("untracked-only is clean for CI honesty")
	}
	if !hasTrackedDirty(" M pkg/foo.go\n") {
		t.Fatal("modified tracked must be dirty")
	}
}

func TestWorktreeListContains(t *testing.T) {
	t.Parallel()
	list := "worktree /tmp/a\nHEAD abc\n\nworktree /tmp/b\n"
	if !worktreeListContains(list, "/tmp/b") {
		t.Fatal("expected /tmp/b")
	}
	if worktreeListContains(list, "/tmp/c") {
		t.Fatal("did not expect /tmp/c")
	}
}

func TestCheckoutDemote_gitWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	ctx := context.Background()
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %s (%v)", strings.Join(args, " "), out, err)
		}
	}
	run("init", "-q")
	run("config", "user.email", "ci@localhost")
	run("config", "user.name", "ci")
	if err := fileutil.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/ci\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	run("add", "go.mod")
	run("commit", "-q", "-m", "init")
	head, err := gitTrim(ctx, runGit, repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	res, err := Checkout(ctx, Options{RepoRoot: repo, Archive: false})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if res.SHA != head {
		t.Fatalf("SHA=%s want %s", res.SHA, head)
	}
	got, err := fileutil.ReadFile(filepath.Join(BaseDir(repo, ""), paths.LocalCISourceSHAFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != head {
		t.Fatalf("SOURCE_SHA=%s", got)
	}
	mod := filepath.Join(res.Workdir, "go.mod")
	if _, err := fileutil.Stat(mod); err != nil {
		t.Fatalf("workdir go.mod: %v", err)
	}

	if err := fileutil.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("x\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	run("add", "dirty.txt")
	if _, err := Checkout(ctx, Options{RepoRoot: repo, Archive: false}); err == nil {
		t.Fatal("expected dirty fail")
	}
	if _, err := Checkout(ctx, Options{RepoRoot: repo, Archive: false, AllowDirty: true}); err != nil {
		t.Fatalf("allow-dirty: %v", err)
	}

	if err := Demote(ctx, Options{RepoRoot: repo}); err != nil {
		t.Fatal(err)
	}
	if _, err := fileutil.Stat(filepath.Join(BaseDir(repo, ""), paths.LocalCISourceSHAFile)); err == nil {
		t.Fatal("SOURCE_SHA should be gone")
	}
}
