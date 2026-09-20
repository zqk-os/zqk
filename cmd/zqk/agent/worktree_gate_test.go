package agent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestIsAgentWorktree(t *testing.T) {
	t.Parallel()
	cases := []struct {
		root string
		want bool
	}{
		{filepath.Join("/repo", paths.ProjectDataDir, paths.WorktreesSubdir, "ATK-1"), true},
		{"/tmp/zqk-worktrees/repo-abcd/ATK-1", true},
		{filepath.Join("/repo", paths.ProjectDataDir, paths.WorktreesSubdir, "ATK-1", "subdir"), true},
		{"/tmp/ATK-1", true},
		{"/private/tmp/ATK-99", true},
		{"/repo", false},
		{filepath.Join("/repo", paths.ProjectDataDir, "local-ci", "workdir"), false},
	}
	for _, tc := range cases {
		if got := isAgentWorktree(tc.root); got != tc.want {
			t.Errorf("isAgentWorktree(%q)=%v want %v", tc.root, got, tc.want)
		}
	}
}

func TestAgentWorktreeMainRepo_doesNotHopParents(t *testing.T) {
	t.Parallel()
	wt := filepath.Join("/tmp/zqk-worktrees/repo-abcd/ATK-1")
	got, err := agentWorktreeMainRepo(wt)
	if err == nil {
		t.Fatalf("isolated path without seated kernel must not resolve; got %q", got)
	}
	hop := filepath.Dir(filepath.Dir(filepath.Dir(wt)))
	if got == hop {
		t.Fatalf("three Dir hops leaked: %q", got)
	}
}

func TestWorktreeBuildCheck_Hook(t *testing.T) {
	orig := worktreeBuildCheck
	t.Cleanup(func() { worktreeBuildCheck = orig })
	called := false
	worktreeBuildCheck = func(ctx context.Context, root string) error {
		called = true
		if root != "/tmp/wt" {
			t.Fatalf("root %q", root)
		}
		return errors.New("boom")
	}
	err := worktreeBuildCheck(context.Background(), "/tmp/wt")
	if !called || err == nil {
		t.Fatalf("hook not applied: called=%v err=%v", called, err)
	}
}

func TestDefaultWorktreeBuildCheck_delegatesToAdapter(t *testing.T) {
	t.Parallel()
	if err := defaultWorktreeBuildCheck(context.Background(), t.TempDir()); err != nil {
		t.Fatalf("non-Go worktree must skip via adapter, not fail: %v", err)
	}
}
