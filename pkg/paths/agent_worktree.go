package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

const agentWorktreeTempBucket = "zqk-worktrees"

// AgentWorktreeDir returns an isolated git-worktree path for taskID that is
// **not** under projectRoot. Default: $TMPDIR/zqk-worktrees/<repo-key>/<taskID>.
// Override base with env AgentWorktreeRoot (brand-prefixed); in-project
// overrides are ignored so Studio load cannot fork-bomb the kernel tree.
// Kernel: POL-AGENT-WORKTREE-ISOLATION-001. TRACK: REDACTED
// AgentWorktreeContainer is the parent directory that holds per-task worktrees.
func AgentWorktreeContainer(projectRoot string) string {
	absRoot, err := filepath.Abs(strings.TrimSpace(projectRoot))
	if err != nil || absRoot == "" {
		absRoot = projectRoot
	}
	if override := strings.TrimSpace(os.Getenv(zqkenv.AgentWorktreeRoot())); override != "" {
		if !pathUnderRoot(absRoot, override) {
			return override
		}
	}
	return filepath.Join(os.TempDir(), agentWorktreeTempBucket, worktreeRepoKey(absRoot))
}

// AgentWorktreeDir returns an isolated git-worktree path for taskID that is
// **not** under projectRoot. Default: $TMPDIR/zqk-worktrees/<repo-key>/<taskID>.
func AgentWorktreeDir(projectRoot, taskID string) string {
	id := strings.TrimSpace(taskID)
	if id == "" {
		id = "unnamed"
	}
	return filepath.Join(AgentWorktreeContainer(projectRoot), id)
}

// AgentWorktreeLookupDirs is the isolated path plus the legacy in-tree location
// so teardown/fail-closed still sees old .zqk/worktrees trees.
func AgentWorktreeLookupDirs(projectRoot, taskID string) []string {
	id := strings.TrimSpace(taskID)
	if id == "" {
		id = "unnamed"
	}
	isolated := AgentWorktreeDir(projectRoot, id)
	legacy := filepath.Join(projectRoot, ProjectDataDir, "worktrees", id)
	if filepath.Clean(legacy) == filepath.Clean(isolated) {
		return []string{isolated}
	}
	return []string{isolated, legacy}
}

func worktreeRepoKey(absRoot string) string {
	sum := sha256.Sum256([]byte(absRoot))
	base := filepath.Base(absRoot)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "repo"
	}
	return base + "-" + hex.EncodeToString(sum[:4])
}

func pathUnderRoot(root, p string) bool {
	absP, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, absP)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
