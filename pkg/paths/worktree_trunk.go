package paths

import (
	"path/filepath"
	"strings"
)

const (
	GitTrunkMain   = "main"
	GitTrunkMaster = "master"
)

// IsGitTrunkBranch reports whether branch is a local trunk ref that git will
// lock to a single worktree. origin/main is not a lock.
func IsGitTrunkBranch(branch string) bool {
	switch strings.TrimSpace(branch) {
	case GitTrunkMain, GitTrunkMaster:
		return true
	default:
		return false
	}
}

func canonPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}

// ShouldEvictLinkedTrunkLock is true when a non-primary worktree has local
// main/master checked out. That lock is why other seats see "already checked
// out" and fall back to detached HEAD.
//
// The primary checkout may sit on trunk only momentarily while merging from
// origin. Linked worktrees never should. TRACK: POL-AGENT-WORKTREE-ISOLATION-001
func ShouldEvictLinkedTrunkLock(worktreePath, branch, primaryRoot string) bool {
	if !IsGitTrunkBranch(branch) {
		return false
	}
	wt := canonPath(worktreePath)
	primary := canonPath(primaryRoot)
	if wt == "" || primary == "" {
		return false
	}
	return wt != primary
}
