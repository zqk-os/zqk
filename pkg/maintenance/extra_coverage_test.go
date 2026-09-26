package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"
)

func setupTestRepo(t *testing.T) (string, *GitMaintenanceService) {
	t.Helper()
	tmpDir := t.TempDir()

	run := func(args ...string) {
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = tmpDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\nOutput:\n%s", args, err, string(out))
		}
	}

	run("init")
	run("config", "user.name", "test")
	run("config", "user.email", "test@example.com")
	run("checkout", "-b", "main")
	run("commit", "--allow-empty", "-m", "initial commit")

	return tmpDir, NewGitMaintenanceService(tmpDir)
}

func TestGitMaintenanceService_isProtected(t *testing.T) {
	svc := NewGitMaintenanceService(".")
	cases := []struct {
		branch string
		want   bool
	}{
		{"main", true},
		{"master", true},
		{"integration/PRI-001", true},
		{"release/v1.0.0", true},
		{"feature/my-feat", false},
		{"bugfix/fix-1", false},
	}
	for _, tc := range cases {
		if got := svc.isProtected(tc.branch); got != tc.want {
			t.Errorf("isProtected(%q) = %v, want %v", tc.branch, got, tc.want)
		}
	}
}

func TestGitMaintenanceService_BranchOperations(t *testing.T) {
	dir, svc := setupTestRepo(t)
	ctx := context.Background()

	run := func(args ...string) {
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\nOutput:\n%s", args, err, string(out))
		}
	}

	run("branch", "feature/old-merged")
	run("branch", "feature/active")

	t.Run("getAllBranches", func(t *testing.T) {
		branches := svc.getAllBranches(ctx)
		if len(branches) < 3 {
			t.Fatalf("expected at least 3 branches, got %d (%v)", len(branches), branches)
		}
	})

	t.Run("getBranchCommitTime", func(t *testing.T) {
		ts, err := svc.getBranchCommitTime(ctx, "main")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ts <= 0 {
			t.Fatalf("expected positive unix timestamp, got %d", ts)
		}

		_, err = svc.getBranchCommitTime(ctx, "nonexistent-branch-xyz")
		if err == nil {
			t.Fatal("expected error for nonexistent branch")
		}
	})

	t.Run("hasUniqueDiffs", func(t *testing.T) {
		// feature/old-merged points to same commit as main -> no unique diffs
		if svc.hasUniqueDiffs(ctx, "feature/old-merged") {
			t.Error("expected false for branch pointing to main")
		}

		// Create unique commit on feature/active
		run("checkout", "feature/active")
		run("commit", "--allow-empty", "-m", "feature commit")
		run("checkout", "main")

		if !svc.hasUniqueDiffs(ctx, "feature/active") {
			t.Error("expected true for branch with unique commit")
		}
	})

	t.Run("getMergedBranches", func(t *testing.T) {
		merged := svc.getMergedBranches(ctx, []string{"main"}, nil, time.Now())
		if !merged["feature/old-merged"] {
			t.Errorf("expected feature/old-merged to be in merged set: %v", merged)
		}
	})
}

func TestGitMaintenanceService_CleanupWorktreeAndBranchForID(t *testing.T) {
	dir, svc := setupTestRepo(t)
	ctx := context.Background()

	run := func(args ...string) {
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\nOutput:\n%s", args, err, string(out))
		}
	}

	// Create branches matching target patterns
	bliBranch := "bli/bli-test-123"
	run("branch", bliBranch)

	tskBranch := "task/tsk-abc-456"
	run("branch", tskBranch)

	// Create a worktree for bliBranch
	wtDir := filepath.Join(dir, "wt_test_123")
	run("worktree", "add", wtDir, bliBranch)

	// Verify worktree exists
	if _, err := os.Stat(wtDir); err != nil {
		t.Fatalf("expected worktree directory to exist: %v", err)
	}

	// Cleanup for bli-test-123
	err := svc.CleanupWorktreeAndBranchForID(ctx, "bli-test-123")
	if err != nil {
		t.Fatalf("CleanupWorktreeAndBranchForID failed: %v", err)
	}

	// Worktree directory should be cleaned up
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("expected worktree directory to be removed, but stat returned: %v", err)
	}

	// Cleanup for plain ID which generates task/tsk-abc-456
	err = svc.CleanupWorktreeAndBranchForID(ctx, "abc-456")
	if err != nil {
		t.Fatalf("CleanupWorktreeAndBranchForID failed: %v", err)
	}
}

func TestGitMaintenanceService_PruneStaleBranches(t *testing.T) {
	dir, svc := setupTestRepo(t)
	ctx := context.Background()

	run := func(args ...string) {
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\nOutput:\n%s", args, err, string(out))
		}
	}

	// Create branch pointing to main (no unique diffs)
	run("branch", "stale-merged-feat")

	// Prune branches inactive for 0 seconds
	deleted, err := svc.PruneStaleBranches(ctx, 0*time.Second, []string{"main"})
	if err != nil {
		t.Fatalf("PruneStaleBranches failed: %v", err)
	}
	if deleted < 1 {
		t.Errorf("expected at least 1 branch pruned, got %d", deleted)
	}

	// Check refs/archive/stale-merged-feat exists
	cmd := testkit.ManagedCommand(t, t.Context(), "git", "show-ref", "--verify", "--quiet", "refs/archive/stale-merged-feat")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Errorf("expected archived ref refs/archive/stale-merged-feat to exist")
	}
}
