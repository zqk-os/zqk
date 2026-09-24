package interactionpolicy

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
)

var (
	reGitWorktreeAdd = regexp.MustCompile(`(?i)git\s+worktree\s+add`)
	reGoTest         = regexp.MustCompile(`(?i)\b(go\s+test|test run|test discover|test bind|test-runner\.sh)\b`)
	reGitCommit      = regexp.MustCompile(`(?i)\bgit\s+commit\b`)
	reAgentExecute   = regexp.MustCompile(`(?i)agent\s+execute\b`)
)

// ClassifyShell maps a shell command to an event class, or empty if unclassified.
func ClassifyShell(cmd string) string {
	c := strings.TrimSpace(cmd)
	if c == "" {
		return ""
	}
	// Tripwire only: nested studio worktrees are forbidden
	// (POL-AGENT-WORKTREE-ISOLATION-001). Legal adds live under
	// $TMPDIR/zqk-worktrees and must not classify (and must not error).
	if reGitWorktreeAdd.MatchString(c) && strings.Contains(c, filepath.Join(paths.ProjectDataDir, paths.WorktreesSubdir)) {
		return EventGitWorktreeAdd
	}
	if reGoTest.MatchString(c) {
		return EventGoTest
	}
	if reGitCommit.MatchString(c) {
		return EventGitCommit
	}
	low := strings.ToLower(c)
	if strings.Contains(low, "agent orchestrate") && !strings.Contains(low, "prepare-context") {
		return EventAgentOrchestrate
	}
	if reAgentExecute.MatchString(c) {
		return EventAgentExecute
	}
	return ""
}
