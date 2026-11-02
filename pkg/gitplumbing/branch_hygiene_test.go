package gitplumbing_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/gitplumbing"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git command %v failed: %s", args, string(out))
	}

	run("init", "-b", "main")
	run("config", "user.name", "Test Agent")
	run("config", "user.email", "agent@zqk.local")

	initFile := filepath.Join(dir, "README.md")
	require.NoError(t, fileutil.WriteStandardFile(initFile, []byte("# Test Repo\n")))

	run("add", "README.md")
	run("commit", "-m", "initial commit")

	return dir
}

// Static Floor Invariants (CRIT-1793571090042704000-9f87f241)
func TestBranchHygiene_StaticFloorInvariants(t *testing.T) {
	t.Run("Default configuration initialization", func(t *testing.T) {
		repo := initTestRepo(t)
		engine, err := gitplumbing.NewBranchHygieneEngine(repo, "", "")
		require.NoError(t, err)
		assert.NotNil(t, engine)
	})

	t.Run("Missing repo root fails closed", func(t *testing.T) {
		engine, err := gitplumbing.NewBranchHygieneEngine("", "origin", "main")
		assert.Error(t, err)
		assert.Nil(t, engine)
	})
}

// Operational Dynamic Proof (CRIT-1793571090042705000-03bc24a0)
func TestBranchHygiene_OperationalDynamicProof(t *testing.T) {
	repo := initTestRepo(t)

	run := func(args ...string) {
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git command %v failed: %s", args, string(out))
	}

	// Create integration branch 1 and commit a file
	run("checkout", "-b", "integration/pri-cycle-1")
	file1 := filepath.Join(repo, "feature1.txt")
	require.NoError(t, fileutil.WriteStandardFile(file1, []byte("feature 1\n")))
	run("add", "feature1.txt")
	run("commit", "-m", "feat: feature 1")

	// Merge integration/pri-cycle-1 back to main
	run("checkout", "main")
	run("merge", "--no-ff", "integration/pri-cycle-1", "-m", "merge integration/pri-cycle-1")

	// Create integration branch 2 (unmerged)
	run("checkout", "-b", "integration/pri-cycle-2")
	file2 := filepath.Join(repo, "feature2.txt")
	require.NoError(t, fileutil.WriteStandardFile(file2, []byte("feature 2\n")))
	run("add", "feature2.txt")
	run("commit", "-m", "feat: feature 2")

	// Return to main
	run("checkout", "main")

	ctx := context.Background()
	engine, err := gitplumbing.NewBranchHygieneEngine(repo, "origin", "main")
	require.NoError(t, err)

	// 1. List branches
	branches, err := engine.ListLocalIntegrationBranches(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"integration/pri-cycle-1", "integration/pri-cycle-2"}, branches)

	// 2. Evaluate merged branch
	res1, err := engine.EvaluateConvergence(ctx, "integration/pri-cycle-1")
	require.NoError(t, err)
	assert.True(t, res1.IsMerged)
	assert.True(t, res1.CanPrune)
	assert.Equal(t, 0, res1.AheadCount)

	// 3. Evaluate unmerged branch
	res2, err := engine.EvaluateConvergence(ctx, "integration/pri-cycle-2")
	require.NoError(t, err)
	assert.False(t, res2.IsMerged)
	assert.False(t, res2.CanPrune)
	assert.Greater(t, res2.AheadCount, 0)

	// 4. Prune dry run
	prunedDry, err := engine.PruneMergedBranches(ctx, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"integration/pri-cycle-1"}, prunedDry)

	// Verify branch still exists after dry-run
	remaining, err := engine.ListLocalIntegrationBranches(ctx)
	require.NoError(t, err)
	assert.Contains(t, remaining, "integration/pri-cycle-1")

	// 5. Live prune
	prunedLive, err := engine.PruneMergedBranches(ctx, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"integration/pri-cycle-1"}, prunedLive)

	// Verify branch is deleted
	remainingAfter, err := engine.ListLocalIntegrationBranches(ctx)
	require.NoError(t, err)
	assert.NotContains(t, remainingAfter, "integration/pri-cycle-1")
	assert.Contains(t, remainingAfter, "integration/pri-cycle-2")
}

// Negative Adversarial Boundary (CRIT-1793571090042706000-48d5c0ea)
func TestBranchHygiene_NegativeAdversarialBoundaries(t *testing.T) {
	repo := initTestRepo(t)

	run := func(args ...string) {
		cmd := testkit.ManagedCommand(t, t.Context(), "git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git command %v failed: %s", args, string(out))
	}

	run("checkout", "-b", "integration/pri-active-branch")
	ctx := context.Background()
	engine, err := gitplumbing.NewBranchHygieneEngine(repo, "origin", "main")
	require.NoError(t, err)

	t.Run("Refuse to prune currently checked out branch even if merged", func(t *testing.T) {
		res, evalErr := engine.EvaluateConvergence(ctx, "integration/pri-active-branch")
		require.NoError(t, evalErr)
		// Branch is identical to main (merged), but active checkout prohibits prune
		assert.True(t, res.IsMerged)
		assert.False(t, res.CanPrune)

		pruned, pruneErr := engine.PruneMergedBranches(ctx, false)
		require.NoError(t, pruneErr)
		assert.Empty(t, pruned)
	})

	t.Run("Empty or invalid branch name returns error", func(t *testing.T) {
		res, evalErr := engine.EvaluateConvergence(ctx, "")
		assert.Error(t, evalErr)
		assert.Nil(t, res)

		resNonExistent, evalErr2 := engine.EvaluateConvergence(ctx, "nonexistent-branch-xyz")
		assert.Error(t, evalErr2)
		assert.Nil(t, resNonExistent)
	})
}
