package paths

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestResolveProjectRoot_EnvPrecedence(t *testing.T) {
	t.Setenv(zqkenv.ProjectRoot().Name(), "/tmp/zqk-explicit-root")
	t.Setenv(zqkenv.TestRoot().Name(), "/tmp/zqk-test-root-should-lose")
	got := ResolveProjectRoot(".")
	want, err := filepath.Abs("/tmp/zqk-explicit-root")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ResolveProjectRoot = %q, want %q", got, want)
	}
}

func TestIsValidProjectRoot(t *testing.T) {
	dir := t.TempDir()
	if IsValidProjectRoot(dir) {
		t.Fatal("empty dir should not be a project root")
	}
	if err := fileutil.EnsureDir(filepath.Join(dir, ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	if !IsValidProjectRoot(dir) {
		t.Fatal("dir with .zqk should be a project root")
	}
}

func TestResolveProjectRoot_AgentWorktreeRefusedWithoutSettings(t *testing.T) {
	worktreeDir := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "test-repo", "ATK-failclosed-"+t.Name())
	if err := fileutil.EnsureDir(filepath.Join(worktreeDir, ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileutil.RemoveAll(worktreeDir) }()

	t.Setenv(zqkenv.ProjectRoot().Name(), worktreeDir)
	got := ResolveProjectRoot(worktreeDir)
	if got == worktreeDir {
		t.Fatalf("ResolveProjectRoot should not return raw agent worktree without seated settings; got %q", got)
	}
}

func TestResolveProjectRoot_AgentWorktreeBindsToSettingsProjectRoot(t *testing.T) {
	studioRoot := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(studioRoot, ProjectDataDir)); err != nil {
		t.Fatal(err)
	}

	worktreeDir := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "test-repo", "ATK-bound-"+t.Name())
	if err := fileutil.EnsureDir(filepath.Join(worktreeDir, ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileutil.RemoveAll(worktreeDir) }()

	if err := BootstrapWorktreeConfig(worktreeDir, studioRoot); err != nil {
		t.Fatalf("BootstrapWorktreeConfig: %v", err)
	}

	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv(zqkenv.ProjectRoot().Name(), worktreeDir)
	got := ResolveProjectRoot(worktreeDir)
	want, _ := filepath.Abs(studioRoot)
	if got != want {
		t.Fatalf("ResolveProjectRoot = %q, want seated studio root %q", got, want)
	}
}
