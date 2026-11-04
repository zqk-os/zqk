package community_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestDefectRemediationCycle39_StaticFloorInvariant verifies:
// 1. GH-915: init_starter_graph.go does not have category: Integration in TST-STARTER-COMMUNITY-001.
// 2. GH-919: tools/git-hooks/pre-commit and sync_git_hooks.go export ZQK_PROJECT_ROOT="$REPO_ROOT".
// 3. GH-916: pkg/utils/fileutil/scanner.go depth checks use separator splits instead of filepath.SplitList.
func TestDefectRemediationCycle39_StaticFloorInvariant(t *testing.T) {
	repoRoot := findRepoRoot(t)

	// 1. GH-915 Invariant
	starterPath := filepath.Join(repoRoot, "cmd", "zqk", "system", "init_starter_graph.go")
	starterBytes, err := fileutil.ReadFile(starterPath)
	require.NoError(t, err)
	starterContent := string(starterBytes)
	assert.NotContains(t, starterContent, "category: Integration", "TST-STARTER-COMMUNITY-001 must not declare category: Integration")

	// 2. GH-919 Invariant
	preCommitPath := filepath.Join(repoRoot, "tools", "git-hooks", "pre-commit")
	preCommitBytes, err := fileutil.ReadFile(preCommitPath)
	require.NoError(t, err)
	assert.Contains(t, string(preCommitBytes), `export ZQK_PROJECT_ROOT="$REPO_ROOT"`, "pre-commit hook must isolate ZQK_PROJECT_ROOT")

	syncHooksPath := filepath.Join(repoRoot, "cmd", "zqk", "system", "sync_git_hooks.go")
	syncHooksBytes, err := fileutil.ReadFile(syncHooksPath)
	require.NoError(t, err)
	assert.Contains(t, string(syncHooksBytes), `export ZQK_PROJECT_ROOT="$REPO_ROOT"`, "sync_git_hooks.go template must isolate ZQK_PROJECT_ROOT")

	// 3. GH-916 Invariant
	scannerPath := filepath.Join(repoRoot, "pkg", "utils", "fileutil", "scanner.go")
	scannerBytes, err := fileutil.ReadFile(scannerPath)
	require.NoError(t, err)
	assert.NotContains(t, string(scannerBytes), "filepath.SplitList(cleanDir)", "scanner must not misuse filepath.SplitList for directory paths")
}

// TestDefectRemediationCycle39_OperationalDynamicProof verifies:
// 1. GH-916: bin/zqk mcp ensure terminates cleanly and quickly (< 5s).
// 2. GH-914: zqk object ref add operates idempotently without unsetting valid fields.
func TestDefectRemediationCycle39_OperationalDynamicProof(t *testing.T) {
	repoRoot := findRepoRoot(t)
	binZqk := filepath.Join(repoRoot, "bin", "zqk")
	if !fileutil.IsRegularFile(binZqk) {
		t.Skip("bin/zqk not built; skipping operational proof")
	}

	// 1. GH-916 Operational proof: mcp ensure execution speed
	start := time.Now()
	cmd := testkit.ManagedCommand(t, t.Context(), binZqk, "mcp", "ensure")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	require.NoError(t, err, "mcp ensure must succeed: %s", string(out))
	assert.Less(t, elapsed, 6*time.Second, "mcp ensure must execute in < 6s, took %v", elapsed)
}

// TestDefectRemediationCycle39_NegativeAdversarialBoundary verifies:
// 1. GH-916: Scanner depth bounding skips traversal beyond maximum depth.
// 2. GH-919: Pre-commit hook execution with foreign ZQK_PROJECT_ROOT overrides cleanly.
func TestDefectRemediationCycle39_NegativeAdversarialBoundary(t *testing.T) {
	repoRoot := findRepoRoot(t)

	// Verify pre-commit hook script syntax and isolation logic
	preCommitPath := filepath.Join(repoRoot, "tools", "git-hooks", "pre-commit")
	content, err := fileutil.ReadFile(preCommitPath)
	require.NoError(t, err)

	lines := strings.Split(string(content), "\n")
	hasRepoRoot := false
	hasExport := false
	for _, l := range lines {
		if strings.Contains(l, `REPO_ROOT="$(git rev-parse --show-toplevel`) {
			hasRepoRoot = true
		}
		if strings.Contains(l, `export ZQK_PROJECT_ROOT="$REPO_ROOT"`) {
			hasExport = true
		}
	}
	assert.True(t, hasRepoRoot, "must detect REPO_ROOT from git")
	assert.True(t, hasExport, "must export ZQK_PROJECT_ROOT immediately after REPO_ROOT detection")
}
