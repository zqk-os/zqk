package context

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestResolveProjectRootFromSettings_AgentWorktreeRefusedWithoutSettings(t *testing.T) {
	worktreeDir := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "test-repo", "ATK-failclosed-settings")
	if err := fileutil.EnsureDir(filepath.Join(worktreeDir, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileutil.RemoveAll(worktreeDir) }()

	t.Setenv(zqkenv.ProjectRoot().Name(), worktreeDir)
	_, _, err := ResolveProjectRootFromSettings(worktreeDir)
	if err == nil {
		t.Fatal("expected error resolving unbonded worktree from settings, got nil")
	}
}

func TestResolveProjectRootFromSettings_AgentWorktreeBindsToStudio(t *testing.T) {
	studioRoot := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(studioRoot, paths.ConfigDir)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureDir(filepath.Join(studioRoot, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	// Write configuration in studio root config/zqk.yaml
	studioConfig := filepath.Join(studioRoot, paths.ConfigDir, paths.ZqkConfigFileName)
	if err := fileutil.WriteStandardFile(studioConfig, []byte("paths:\n  aliases: {}\n")); err != nil {
		t.Fatal(err)
	}

	worktreeDir := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "test-repo", "ATK-bound-settings")
	if err := fileutil.EnsureDir(filepath.Join(worktreeDir, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileutil.RemoveAll(worktreeDir) }()

	if err := paths.BootstrapWorktreeConfig(worktreeDir, studioRoot); err != nil {
		t.Fatal(err)
	}

	t.Setenv(zqkenv.ProjectRoot().Name(), worktreeDir)
	resolved, settings, err := ResolveProjectRootFromSettings(worktreeDir)
	if err != nil {
		t.Fatalf("unexpected error resolving bound worktree: %v", err)
	}
	want, _ := filepath.Abs(studioRoot)
	if resolved != want {
		t.Fatalf("resolved = %q, want %q", resolved, want)
	}
	if settings == nil {
		t.Fatal("expected non-nil settings")
	}
}
