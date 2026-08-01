package projecttemp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestIsProbableGitWorktreeRoot(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	if IsProbableGitWorktreeRoot(tmp) {
		t.Fatal("empty temp dir should not look like a git worktree root")
	}
	gitPath := filepath.Join(tmp, paths.GitWorktreeMetadataEntry)
	if err := os.WriteFile(gitPath, []byte("gitdir: ../.git/modules/foo\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write git metadata: %v", err)
	}
	if !IsProbableGitWorktreeRoot(tmp) {
		t.Fatal("dir with git worktree marker should be detected")
	}
}

func TestRunIsolatedRootStrip_removesLayout(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	docs := datacell.CellCASPrimaryDir(tmp, "x")
	if err := os.MkdirAll(docs, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	zqkWal := filepath.Join(tmp, paths.ProjectDataDir, paths.WalDir)
	if err := os.MkdirAll(zqkWal, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir .zqk: %v", err)
	}
	if err := RunIsolatedRootStrip(tmp); err != nil {
		t.Fatalf("RunIsolatedRootStrip: %v", err)
	}
	if _, err := os.Stat(docs); err == nil {
		t.Fatalf("expected %s tree removed", paths.ProcessDir)
	}
	if _, err := os.Stat(zqkWal); err == nil {
		t.Fatal("expected .zqk tree removed")
	}
}

func TestRunIsolatedRootStrip_skipsGitWorktree(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, paths.GitWorktreeMetadataEntry), []byte("gitdir: ../.git\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write git metadata: %v", err)
	}
	docs := datacell.CellCASPrimaryDir(tmp, "x")
	if err := os.MkdirAll(docs, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := RunIsolatedRootStrip(tmp); err != nil {
		t.Fatalf("RunIsolatedRootStrip: %v", err)
	}
	if _, err := os.Stat(docs); err != nil {
		t.Fatal("strip must not run on git worktree root")
	}
}
